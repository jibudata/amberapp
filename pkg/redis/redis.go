package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/mod/semver"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/go-redis/redis/v8"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/appconfig"
)

const (
	Standalone ArchitectureType = "standalone"
	Sentinel   ArchitectureType = "sentinel"
	Cluster    ArchitectureType = "cluster"

	Snapshot BackupMethod = "snapshot"
	AOFOnly  BackupMethod = "aofonly"
	None     BackupMethod = "none"

	AppendOnly               RedisConfigCmdType = "appendonly"
	Save                     RedisConfigCmdType = "save"
	AutoAOFReWritePercentage RedisConfigCmdType = "auto-aof-rewrite-percentage"

	ServerInfo      RedisInfoCmdType = "server"
	PersistenceInfo RedisInfoCmdType = "persistence"

	DefaultTimeout = 3 * time.Minute
)

type ArchitectureType string
type BackupMethod string
type RedisConfigCmdType string
type RedisInfoCmdType string

type Redis struct {
	config       appconfig.Config
	architecture ArchitectureType
	mode         BackupMethod
	opTimeout    time.Duration
	version      int
	rdb          *redis.Client
}

func (r *Redis) Init(appConfig appconfig.Config) error {
	r.config = appConfig
	r.architecture = Standalone
	r.mode = getBackupMethod(appConfig)
	if appConfig.QuiesceTimeout != 0 {
		r.opTimeout = appConfig.QuiesceTimeout
	} else {
		r.opTimeout = DefaultTimeout
	}

	r.version = 0 // unknown version

	log.Log.Info("Redis init...", appConfig.Name, r.String())

	return nil
}

func (r *Redis) Connect() error {
	var err error
	log.Log.Info("Redis connecting...")

	if r.rdb != nil {
		err = r.rdb.Ping(context.TODO()).Err()
		if err != nil {
			r.rdb = nil
		}
	}

	if r.rdb == nil {
		r.rdb, _ = newRedisClient(r.config)
	}

	err = r.getRedisVersion()
	if err != nil {
		return err
	}

	err = r.getRedisMode()
	if err != nil {
		return err
	}

	if !r.IsSupported() {
		return fmt.Errorf("%s isn't supported yet", r.String())
	}

	log.Log.Info("Redis connected")
	return nil
}

func (r *Redis) Prepare() (*v1alpha1.PreservedConfig, error) {
	log.Log.Info("Redis preparing...")
	err := r.Connect()
	if err != nil {
		return nil, err
	}

	isAOFEnabled, err := r.isAOFEnabled()
	if err != nil {
		return nil, err
	}

	snapshot, err := r.getSnapshotConfig()
	if err != nil {
		return nil, err
	}

	saved := false
	preserved := make(map[string]string)

	if isAOFEnabled {
		preserved[string(AppendOnly)] = "yes"

		percentage, err := r.getAutoAOFRewritePercentage()
		if err != nil {
			return nil, err
		}

		preserved[string(AutoAOFReWritePercentage)] = fmt.Sprintf("%d", percentage)
		saved = true
	}

	if snapshot != "" {
		preserved[string(Save)] = snapshot
		saved = true
	}

	if saved {
		log.Log.Info("Redis prepared", "params", preserved)
		return &v1alpha1.PreservedConfig{
			Params: preserved,
		}, nil
	}

	log.Log.Info("Redis prepared")
	return nil, nil
}

func (r *Redis) Quiesce() (*v1alpha1.QuiesceResult, error) {
	var err error
	isAOFEnabled, err := r.isAOFEnabled()
	if err != nil {
		return nil, err
	}

	if r.mode == AOFOnly && isAOFEnabled {
		log.Log.Info("disable redis auto aof rewrite")
		// disable AOF rewrite
		err = r.disableAutoAOFRewrite()
		if err != nil {
			return nil, err
		}

		log.Log.Info("wait for previous rewrite done")
		done := false
		err = wait.PollImmediate(3*time.Second, DefaultTimeout, func() (bool, error) {
			ongoing, err := r.isAOFRewriteInProgress()
			if err != nil {
				return false, err
			}

			if ongoing {
				return false, nil
			}
			done = true
			return true, nil
		})

		if err != nil {
			return nil, err
		}

		if !done {
			return nil, fmt.Errorf("timeout to wait auto aof rewrite done")
		}
	}

	if r.mode == Snapshot {
		log.Log.Info("wait for previous rbd snapshot done")
		done := false
		err = wait.PollImmediate(3*time.Second, DefaultTimeout, func() (bool, error) {
			ongoing, err := r.isRDBBgSaveInProgress()
			if err != nil {
				return false, err
			}

			if ongoing {
				return false, nil
			}
			done = true
			return true, nil
		})

		if err != nil {
			return nil, err
		}

		if !done {
			return nil, fmt.Errorf("timeout to wait last bgsave done")
		}

		log.Log.Info("take rbd snapshot")
		// bgsave for rbd snapshot
		lastSaveHandler, err := r.rdb.LastSave(context.TODO()).Result()
		if err != nil {
			return nil, err
		}

		// issue bgsave
		err = r.rdb.BgSave(context.TODO()).Err()
		if err != nil {
			return nil, err
		}

		log.Log.Info("wait for rbd snapshot done")
		var newSaveHandler int64
		err = wait.PollImmediate(3*time.Second, DefaultTimeout, func() (bool, error) {
			newSaveHandler, err = r.rdb.LastSave(context.TODO()).Result()
			if err != nil {
				return false, err
			}

			if newSaveHandler == lastSaveHandler {
				return false, nil
			}

			return true, nil
		})

		if err != nil {
			return nil, err
		}

		if newSaveHandler == lastSaveHandler {
			return nil, fmt.Errorf("timeout to wait bgsave done")
		}
		log.Log.Info("take rbd snapshot done")
	}

	return nil, nil
}

func (r *Redis) Unquiesce(prev *v1alpha1.PreservedConfig) error {
	if prev == nil {
		return nil
	}

	// restore original redis settings
	for k, v := range prev.Params {
		log.Log.Info("restore redis persistence setting", k, v)
		_, err := r.rdb.ConfigSet(context.TODO(), k, v).Result()
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *Redis) String() string {
	return fmt.Sprintf("redis instance %s with version: %d, topology: %s, backup method: %s, timeout: %d",
		r.config.Host,
		r.version,
		r.architecture,
		r.mode,
		r.opTimeout,
	)
}

func (r *Redis) getRedisVersion() error {
	val, err := r.rdb.Info(context.TODO(), string(ServerInfo)).Result()
	if err != nil {
		return err
	}

	version := extractRedisInfoResult(val, "redis_version:")
	if version == "" {
		return fmt.Errorf("can't find redis version from result %s", val)
	}

	log.Log.Info("getRedisVersion", "version", version)

	if semver.Compare("v"+version, "v7.0.0") >= 0 && semver.Compare("v"+version, "v8.0.0") == -1 {
		r.version = 7
	} else {
		r.version = 0
	}

	return nil
}

func (r *Redis) getRedisMode() error {
	val, err := r.rdb.Info(context.TODO(), string(ServerInfo)).Result()
	if err != nil {
		return err
	}

	mode := extractRedisInfoResult(val, "redis_mode:")
	if mode == "" {
		return fmt.Errorf("can't find redis mode from result %s", val)
	}

	log.Log.Info("getRedisMode", "mode", mode)

	switch mode {
	case string(Standalone):
		r.architecture = Standalone
	case string(Sentinel):
		r.architecture = Sentinel
	case string(Cluster):
		r.architecture = Cluster
	default:
		return fmt.Errorf("invalid cluster mode %s", mode)
	}

	return nil
}

func (r *Redis) isAOFEnabled() (bool, error) {
	v, err := r.rdb.ConfigGet(context.TODO(), string(AppendOnly)).Result()
	if err != nil {
		return false, err
	}

	if len(v) != 2 {
		return false, fmt.Errorf("invalid result length: %#v from config get %s", v, AppendOnly)
	}

	result := v[1].(string)
	if result != "yes" && result != "no" {
		return false, fmt.Errorf("invalid result %s from %#v, must be yes or no", result, v)
	}

	return result == "yes", nil
}

func (r *Redis) getSnapshotConfig() (string, error) {

	v, err := r.rdb.ConfigGet(context.TODO(), string(Save)).Result()
	if err != nil {
		return "", err
	}

	if len(v) != 2 {
		return "", fmt.Errorf("invalid result length: %#v from config get %s", v, Save)
	}

	result, ok := v[1].(string)
	if !ok {
		return "", fmt.Errorf("failed to convert result %#v from config get %#v to string", result, v)
	}

	return result, nil
}

func (r *Redis) getAutoAOFRewritePercentage() (int, error) {

	v, err := r.rdb.ConfigGet(context.TODO(), string(AutoAOFReWritePercentage)).Result()
	if err != nil {
		return -1, err
	}

	if len(v) != 2 {
		return -1, fmt.Errorf("invalid result length: %#v from config get %s", v, AutoAOFReWritePercentage)
	}

	result, err := strconv.Atoi(v[1].(string))
	if err != nil {
		return -1, fmt.Errorf("failed to convert result %#v from config get %#v to string, err: %v", result, v[1], err)
	}

	return result, nil
}

func (r *Redis) disableAutoAOFRewrite() error {
	_, err := r.rdb.ConfigSet(context.TODO(), string(AutoAOFReWritePercentage), "0").Result()
	return err
}

func (r *Redis) isAOFRewriteInProgress() (bool, error) {
	val, err := r.rdb.Info(context.TODO(), string(PersistenceInfo)).Result()
	if err != nil {
		return false, err
	}

	result := extractRedisInfoResult(val, "aof_rewrite_in_progress:")
	if result == "" {
		return false, fmt.Errorf("can't get aof_rewrite_in_progress from result %s", val)
	}

	inprogresFlag, err := strconv.Atoi(result)
	if err != nil {
		return false, fmt.Errorf("failed to convert %s to int flag from result %s", result, val)
	}

	return inprogresFlag == 1, nil
}

func (r *Redis) isRDBBgSaveInProgress() (bool, error) {
	val, err := r.rdb.Info(context.TODO(), string(PersistenceInfo)).Result()
	if err != nil {
		return false, err
	}

	result := extractRedisInfoResult(val, "rdb_bgsave_in_progress:")
	if result == "" {
		return false, fmt.Errorf("can't get rdb_bgsave_in_progress from result %s", val)
	}

	inprogresFlag, err := strconv.Atoi(result)
	if err != nil {
		return false, fmt.Errorf("failed to convert %s to int flag from result %s", result, val)
	}

	return inprogresFlag == 1, nil
}

func extractRedisInfoResult(result string, prefix string) string {
	rows := strings.Split(result, "\n")
	for _, row := range rows {
		if strings.HasPrefix(row, prefix) {
			arr := strings.Split(row, ":")
			if len(arr) != 2 {
				return ""
			}
			return strings.TrimSpace(arr[1])
		}
	}

	return ""
}

func (r *Redis) IsSupported() bool {
	if r.version != 7 || r.mode == None || r.architecture != Standalone {
		return false
	}

	return true
}

func newRedisClient(appConfig appconfig.Config) (*redis.Client, error) {

	// TODO: add redis cluster mode support
	// TODO: add TLS support
	rdb := redis.NewClient(&redis.Options{
		Addr:     appConfig.Host,
		Password: appConfig.Password,
		DB:       0, // use default DB
	})

	return rdb, nil
}

func getBackupMethod(appConfig appconfig.Config) BackupMethod {
	if appConfig.Params == nil {
		// no special action
		return None
	}

	method, ok := appConfig.Params[v1alpha1.RedisBackupMethod]
	if ok {
		switch method {
		case v1alpha1.RedisBackupByRDB:
			return Snapshot
		case v1alpha1.RedisBackupByAOF:
			return AOFOnly
		default:
			return None
		}
	}

	return None
}
