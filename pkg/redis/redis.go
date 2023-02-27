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
	ClusterInfo     RedisInfoCmdType = "cluster"

	DefaultTimeout = 3 * time.Minute
)

type ArchitectureType string
type BackupMethod string
type RedisConfigCmdType string
type RedisInfoCmdType string

type Redis struct {
	config       appconfig.Config
	architecture ArchitectureType
	masters      []string
	slaves       []string
	mode         BackupMethod
	opTimeout    time.Duration
	version      int
	rdb          *redis.Client
}

func (r *Redis) Init(appConfig appconfig.Config) error {
	r.config = appConfig
	r.mode = getBackupMethod(appConfig)
	r.opTimeout = getQuiesceTimeout(appConfig)
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
		r.rdb, _ = r.newRedisClient()
	}

	err = r.getRedisVersion()
	if err != nil {
		return err
	}

	err = r.getRedisMode()
	if err != nil {
		return err
	}

	if r.architecture == Cluster {
		enabled, err := r.isRedisClusterEnabled()
		if err != nil {
			return err
		}

		if enabled {
			ready, clusterSize, clusterNodes, err := r.getClusterInfo()
			if err != nil {
				return err
			}

			if !ready {
				return fmt.Errorf("cluster isn't ready yet, fail this operation")
			}

			masters, slaves, err := r.getClusterNodes()
			if err != nil {
				return err
			}
			r.masters = masters
			r.slaves = slaves

			if len(r.masters)+len(r.slaves) != clusterNodes {
				return fmt.Errorf("inconsistent cluster nodes, master:%d, slaves:%d, known_nodes:%d", len(r.masters), len(r.slaves), clusterNodes)
			}

			if len(r.masters) != clusterSize {
				return fmt.Errorf("inconsistent cluster size, master:%d, cluster_size:%d", len(r.masters), clusterSize)
			}
		}
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

	// redis >= v2.4 has `redis_version`, `redis_mode` and `cluster_enabled` sections
	if semver.Compare("v"+version, "v2.4") >= 0 && semver.Compare("v"+version, "v8.0.0") == -1 {
		items := strings.Split(version, ".")
		if len(items) == 0 {
			return fmt.Errorf("unexpected redis version format: %s", version)
		}
		r.version, err = strconv.Atoi(items[0])
		if err != nil {
			return fmt.Errorf("failed to parse redis version, err=%s", err)
		}
	} else {
		return fmt.Errorf("unsupported redis version: %s", version)
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

func (r *Redis) isRedisClusterEnabled() (bool, error) {
	val, err := r.rdb.Info(context.TODO(), string(ClusterInfo)).Result()
	if err != nil {
		return false, err
	}

	enabled := extractRedisInfoResult(val, "cluster_enabled")
	flag, err := strconv.Atoi(enabled)
	if err != nil {
		return false, fmt.Errorf("failed to parse cluster_enabled, err:%s, raw:%s", err, val)
	}

	return flag != 0, nil
}

// get redis cluster state, cluster size, known nodes
func (r *Redis) getClusterInfo() (bool, int, int, error) {

	res, err := r.rdb.ClusterInfo(context.TODO()).Result()
	if err != nil {
		return false, 0, 0, err
	}

	state := extractRedisInfoResult(res, "cluster_state")
	size, err := strconv.Atoi(extractRedisInfoResult(res, "cluster_size"))
	if err != nil {
		return false, 0, 0, fmt.Errorf("invalid cluster size, err:%s, raw:%s", err, res)
	}

	nodes, err := strconv.Atoi(extractRedisInfoResult(res, "cluster_known_nodes"))
	if err != nil {
		return false, 0, 0, fmt.Errorf("invalid cluster nodes, err:%s, raw:%s", err, res)
	}

	return state == "ok", size, nodes, nil
}

// return master nodes and slave nodes
func (r *Redis) getClusterNodes() ([]string, []string, error) {
	var masters []string
	var slaves []string
	val, err := r.rdb.ClusterNodes(context.TODO()).Result()
	if err != nil {
		return masters, slaves, err
	}

	rows := strings.Split(val, "\n")

	for _, row := range rows {
		items := strings.Fields(row)
		if len(items) >= 3 {
			if strings.Contains(items[2], "master") {
				masters = append(masters, items[1])
			} else {
				slaves = append(slaves, items[1])
			}
		}
	}

	log.Log.Info("extractRedisNodes", "masters", masters, "slaves", slaves)
	return masters, slaves, nil
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
	if r.mode == None || (r.architecture != Standalone && r.architecture != Cluster) {
		return false
	}

	return true
}

func (r *Redis) newRedisClient() (*redis.Client, error) {

	// TODO: add TLS support
	rdb := redis.NewClient(&redis.Options{
		Addr:     r.config.Host,
		Password: r.config.Password,
		DB:       0, // use default DB
	})
	return rdb, nil
}

func getBackupMethod(appConfig appconfig.Config) BackupMethod {
	if appConfig.Params == nil {
		// by default, trigger rdb bgsave
		return Snapshot
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

	return Snapshot
}

func getQuiesceTimeout(appConfig appconfig.Config) time.Duration {
	if appConfig.QuiesceTimeout != 0 {
		return appConfig.QuiesceTimeout
	}

	return DefaultTimeout
}
