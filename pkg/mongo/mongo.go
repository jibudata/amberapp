package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/appconfig"
)

type MG struct {
	config     appconfig.Config
	usePrimary bool
}

func (mg *MG) Init(appConfig appconfig.Config) error {
	mg.config = appConfig
	return nil
}

func (mg *MG) Connect() error {
	var err error
	var result bson.M
	var opts *options.RunCmdOptions

	log.Log.Info("mongodb connecting...")

	client, err := getMongodbClient(mg.config)
	if err != nil {
		return err
	}
	// list all databases to check the connection is ok
	filter := bson.D{{}}
	_, err = client.ListDatabaseNames(context.TODO(), filter)
	if err != nil {
		log.Log.Error(err, "failed to list databases")
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
		log.Log.Error(err, "failed to run hello")
		return err
	}

	secondary := result["secondary"]

	if secondary == false {
		log.Log.Info("Warning, not connected to secondary for quiesce")
	} else {
		log.Log.Info("connected to secondary")
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

	log.Log.Info("mongodb quiesce in progress")
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
		log.Log.Error(err, "failed to run hello")
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

	log.Log.Info("quiesce mongo", "endpoint", mongoResult.MongoEndpoint, "primary", primary)
	quiResult := &v1alpha1.QuiesceResult{Mongo: mongoResult}

	cmdResult := db.RunCommand(context.TODO(), bson.D{{Key: "fsync", Value: 1}, {Key: "lock", Value: true}}, opts)
	if cmdResult.Err() != nil {
		log.Log.Error(cmdResult.Err(), fmt.Sprintf("failed to quiesce %s", mg.config.Name))
		return quiResult, cmdResult.Err()
	}

	return quiResult, nil
}

func (mg *MG) Unquiesce(prev *v1alpha1.PreservedConfig) error {
	log.Log.Info("mongodb unquiesce in progress")
	client, err := getMongodbClient(mg.config)
	if err != nil {
		return err
	}
	db := client.Database("admin")
	var opts *options.RunCmdOptions
	if !mg.config.QuiesceFromPrimary {
		opts = options.RunCmd().SetReadPreference(readpref.Secondary())
	}
	result := db.RunCommand(context.TODO(), bson.D{{Key: "fsyncUnlock", Value: 1}}, opts)
	if result.Err() != nil {
		log.Log.Error(result.Err(), fmt.Sprintf("failed to unquiesce %s", mg.config.Name))
		return result.Err()
	}

	return nil
}

func (mg *MG) runHello(db *mongo.Database, opts *options.RunCmdOptions, result *bson.M) error {
	cmd := bson.D{{Key: "hello", Value: 1}}

	err := db.RunCommand(context.TODO(), cmd, opts).Decode(result)
	if err != nil {
		log.Log.Error(err, "failed to run hello command")
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
		log.Log.Error(err, fmt.Sprintf("failed to connect mongodb %s", appConfig.Name))
		return client, err
	}
	return client, nil
}
