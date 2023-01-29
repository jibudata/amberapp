package redis

import (
	"context"
	"fmt"
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

	AppendOnly       RedisConfigCmdType = "appendonly"
	Save             RedisConfigCmdType = "save"
	AOFReWriteOption RedisConfigCmdType = "auto-aof-rewrite-percentage"

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

	log.Log.Info("Redis connected")
	return nil
}

func (r *Redis) Quiesce() (*v1alpha1.QuiesceResult, error) {
	if !r.IsSupported() {
		return nil, nil
	}

	return nil, nil
}

func (r *Redis) Unquiesce() error {
	if !r.IsSupported() {
		return nil
	}

	return nil
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
