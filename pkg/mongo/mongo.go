package mongo

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/appconfig"
)

type MG struct {
	config appconfig.Config
}

var log = ctrllog.Log.WithName("mongo")

func (mg *MG) Init(appConfig appconfig.Config) error {
	mg.config = appConfig
	return nil
}

func (mg *MG) Connect() error {
	var err error
	var result bson.M
	var opts *options.RunCmdOptions

	log.Info("mongodb connecting...")

	client, err := getMongodbClient(mg.config)
	if err != nil {
		return err
	}
	// list all databases to check the connection is ok
	filter := bson.D{{}}
	_, err = client.ListDatabaseNames(context.TODO(), filter)
	if err != nil {
		log.Error(err, "failed to list databases")
		return err
	}

	// Get hello result, determine if it's secondary
	if !mg.config.QuiesceFromPrimary {
		opts = options.RunCmd().SetReadPreference(readpref.Secondary())
	}
	db := client.Database("admin")

	// Run hello
	err = mg.runHello(db, opts, &result)
	if err != nil {
		log.Error(err, "failed to run hello")
		return err
	}

	secondary := result["secondary"]

	if secondary == false {
		log.Info("Warning, not connected to secondary for quiesce")
	} else {
		log.Info("connected to secondary")
	}

	return nil
}

func (mg *MG) Prepare() (*v1alpha1.PreservedConfig, error) {
	return nil, nil
}

func (mg *MG) Quiesce() (*v1alpha1.QuiesceResult, error) {
	var err error
	var result bson.M
	var opts *options.RunCmdOptions

	log.Info("mongodb quiesce in progress")
	client, err := getMongodbClient(mg.config)
	if err != nil {
		return nil, err
	}
	db := client.Database("admin")
	if !mg.config.QuiesceFromPrimary {
		opts = options.RunCmd().SetReadPreference(readpref.Secondary())
	}

	err = mg.runHello(db, opts, &result)
	if err != nil {
		log.Error(err, "failed to run hello")
		return nil, err
	}

	secondary := result["secondary"]
	primary := false
	if secondary == false {
		primary = true
	}

	mongoResult := &v1alpha1.MongoResult{
		IsPrimary: primary,
	}
	if result["me"] == nil {
		// standalone mongo
		mongoResult.MongoEndpoint = mg.config.Host
	} else {
		mongoResult.MongoEndpoint = result["me"].(string)
	}
	quiResult := &v1alpha1.QuiesceResult{Mongo: mongoResult}

	isLocked, err := isDBLocked(db, opts)
	if err != nil {
		log.Error(err, "failed to check lock status of database to quiesce", "instance name", mg.config.Name)
		return quiResult, err
	}
	if isLocked {
		log.Info("mongodb already locked", "instacne", mg.config.Name)
		return quiResult, nil
	}

	log.Info("quiesce mongo", "endpoint", mongoResult.MongoEndpoint, "primary", primary)

	cmdResult := db.RunCommand(context.TODO(), bson.D{{Key: "fsync", Value: 1}, {Key: "lock", Value: true}}, opts)
	if cmdResult.Err() != nil {
		log.Error(cmdResult.Err(), fmt.Sprintf("failed to quiesce %s", mg.config.Name))
		return quiResult, cmdResult.Err()
	}

	return quiResult, nil
}

func (mg *MG) Unquiesce(prev *v1alpha1.PreservedConfig) error {
	log.Info("mongodb unquiesce in progress")

	client, err := getMongodbClient(mg.config)
	if err != nil {
		return err
	}
	db := client.Database("admin")
	var opts *options.RunCmdOptions
	if !mg.config.QuiesceFromPrimary {
		opts = options.RunCmd().SetReadPreference(readpref.Secondary())
	}

	isLocked := true
	for isLocked {
		isLocked, err = isDBLocked(db, opts)
		if err != nil {
			log.Error(err, "failed to check lock status of database to unquiesce", "instance name", mg.config.Name)
			return err
		}
		if !isLocked {
			return nil
		}

		result := db.RunCommand(context.TODO(), bson.D{{Key: "fsyncUnlock", Value: 1}}, opts)
		if result.Err() != nil {
			// fsyncUnlock called when not locked
			if strings.Contains(result.Err().Error(), "not locked") {
				return nil
			}
			log.Error(result.Err(), fmt.Sprintf("failed to unquiesce %s", mg.config.Name))
			return result.Err()
		}
	}

	return nil
}

func (mg *MG) runHello(db *mongo.Database, opts *options.RunCmdOptions, result *bson.M) error {
	cmd := bson.D{{Key: "hello", Value: 1}}

	err := db.RunCommand(context.TODO(), cmd, opts).Decode(result)
	if err != nil {
		log.Error(err, "failed to run hello command")
		return err
	}

	return nil
}

func getMongodbClient(appConfig appconfig.Config) (*mongo.Client, error) {
	ctx, cancel := context.WithTimeout(context.Background(), appconfig.ConnectionTimeout)
	defer cancel()

	host := fmt.Sprintf("mongodb://%s:%s@%s",
		appConfig.Username,
		appConfig.Password,
		appConfig.Host)
	clientOptions := options.Client().ApplyURI(host)
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		log.Error(err, fmt.Sprintf("failed to connect mongodb %s", appConfig.Name))
		return client, err
	}
	return client, nil
}

type LockResult struct {
	LockInfo []interface{}
}

func isDBLocked(db *mongo.Database, opts *options.RunCmdOptions) (bool, error) {
	result := db.RunCommand(context.TODO(), bson.D{{Key: "lockInfo", Value: 1}}, opts)
	if result.Err() != nil {
		return false, result.Err()
	}

	resultData, err := result.DecodeBytes()
	if err != nil {
		return false, err
	}

	lockResult := &LockResult{}
	err = json.Unmarshal([]byte(resultData.String()), lockResult)
	if err != nil {
		return false, err
	}
	if len(lockResult.LockInfo) > 0 {
		return true, nil
	}
	return false, nil
}
