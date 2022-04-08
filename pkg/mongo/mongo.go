package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jibudata/amberapp/pkg/appconfig"
)

type MG struct {
	config appconfig.Config
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
	result := db.RunCommand(context.TODO(), bson.D{{Key: "fsync", Value: 1}, {Key: "lock", Value: true}})
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
	result := db.RunCommand(context.TODO(), bson.D{{Key: "fsyncUnlock", Value: 1}})
	if result.Err() != nil {
		log.Log.Error(result.Err(), fmt.Sprintf("failed to unquiesce %s", mg.config.Name))
		return result.Err()
	}

	return nil
}

func getMongodbClient(appConfig appconfig.Config) (*mongo.Client, error) {
	host := fmt.Sprintf("mongodb://%s:%s@%s",
		appConfig.Username,
		appConfig.Password,
		appConfig.Host)
	clientOptions := options.Client().ApplyURI(host)
	client, err := mongo.Connect(context.TODO(), clientOptions)
	if err != nil {
		log.Log.Error(err, fmt.Sprintf("failed to connect mongodb %s", appConfig.Name))
		return client, err
	}
	return client, nil
}
