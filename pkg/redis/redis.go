package redis

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/mod/semver"

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
)

type ArchitectureType string
type BackupMethod string
type RedisConfigCmdType string
type RedisInfoCmdType string

type Redis struct {
	config       appconfig.Config
	architecture ArchitectureType
	mode         BackupMethod
	version      int
	rdb          *redis.Client
}

func (r *Redis) Init(appConfig appconfig.Config) error {
	r.config = appConfig
	r.architecture = Standalone
	r.mode = getBackupMethod(appConfig)
	r.version = 0 // unknonw
	return nil
}

func (r *Redis) Connect() error {
	log.Log.Info("Redis connecting...")

	var err error
	rdb, _ := newRedisClient(r.config)
	res := rdb.Ping(context.TODO())
	if res.Err() != nil {
		return res.Err()
	}

	r.rdb = rdb
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

func (r *Redis) Quiesce() (*v1alpha1.QuiesceResult, error) {
	isAOFEnabled, err := r.isAOFEnabled()
	if err != nil {
		return nil, err
	}

	snapshot, err := r.getSnapshotConfig()
	if err != nil {
		return nil, err
	}

	preserved := make(map[string]string)

	if isAOFEnabled {
		preserved[string(AppendOnly)] = "yes"

		percentage, err := r.getAutoAOFRewritePercentage()
		if err != nil {
			return nil, err
		}

		preserved[string(AutoAOFReWritePercentage)] = fmt.Sprintf("%d", percentage)
	}

	if snapshot != "" {
		preserved[string(Save)] = snapshot
	}

	return &v1alpha1.QuiesceResult{
		Redis: &v1alpha1.RedisResult{
			QuiescePreservedConfig: preserved,
		},
	}, nil
}

func (r *Redis) Unquiesce() error {

	return nil
}

func (r *Redis) String() string {
	return fmt.Sprintf("redis instance %s with version: %d, topology: %s, backup method: %s",
		r.config.Host,
		r.version,
		r.architecture,
		r.mode)
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

	result, ok := v[1].(int)
	if !ok {
		return -1, fmt.Errorf("failed to convert result %#v from config get %#v to string", result, v)
	}

	return result, nil
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
		return true
	}

	return false
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
