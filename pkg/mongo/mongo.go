package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"go.mongodb.org/mongo-driver/mongo/readpref"
	"sigs.k8s.io/controller-runtime/pkg/log"

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
	cmd := bson.D{{Key: "hello", Value: 1}}
	var result bson.M
	var opts *options.RunCmdOptions
	if !mg.config.QuiesceFromPrimary {
		opts = options.RunCmd().SetReadPreference(readpref.Secondary())
	}
	db := client.Database("admin")
	err = db.RunCommand(context.TODO(), cmd, opts).Decode(&result)
	if err != nil {
		log.Log.Error(err, "failed to run hello command")
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

func (mg *MG) Quiesce() error {
	var err error
	log.Log.Info("mongodb quiesce in progress")
	client, err := getMongodbClient(mg.config)
	if err != nil {
		return err
	}
	db := client.Database("admin")
	var opts *options.RunCmdOptions
	if !mg.config.QuiesceFromPrimary {
		opts = options.RunCmd().SetReadPreference(readpref.Secondary())
	}
	result := db.RunCommand(context.TODO(), bson.D{{Key: "fsync", Value: 1}, {Key: "lock", Value: true}}, opts)
	if result.Err() != nil {
		log.Log.Error(result.Err(), fmt.Sprintf("failed to quiesce %s", mg.config.Name))
		return result.Err()
	}

	return nil
}

func (mg *MG) Unquiesce() error {
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
