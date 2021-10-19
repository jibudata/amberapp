package driver

import (
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	storagev1alpha1 "github.com/jibudata/app-hook-operator/api/v1alpha1"
	"github.com/jibudata/app-hook-operator/pkg/appconfig"
	"github.com/jibudata/app-hook-operator/pkg/mongo"
	"github.com/jibudata/app-hook-operator/pkg/mysql"
	"github.com/jibudata/app-hook-operator/pkg/postgres"
	corev1 "k8s.io/api/core/v1"
)

type SupportedDB string

const (
	MySQL    SupportedDB = "MySQL"
	Postgres SupportedDB = "Postgres"
	MongoDB  SupportedDB = "MongoDB"
)

type Database interface {
	Init(appconfig.Config) error
	Connect() error
	Quiesce() error
	Unquiesce() error
}

type DriverManager struct {
	client.Client
	namespace string
	appName   string
	ready     bool
	db        Database
	appConfig appconfig.Config
}

func NewManager(k8sclient client.Client, instance *storagev1alpha1.AppHook, secret *corev1.Secret) (*DriverManager, error) {
	var CacheManager DriverManager
	var err error
	CacheManager.Client = k8sclient
	CacheManager.appName = instance.Name
	CacheManager.namespace = instance.Namespace

	// init database
	if strings.EqualFold(instance.Spec.AppProvider, string(Postgres)) { // postgres
		CacheManager.db = new(postgres.PG)
	} else if strings.EqualFold(instance.Spec.AppProvider, string(MySQL)) { // mysql
		CacheManager.db = new(mysql.MYSQL)
	} else if strings.EqualFold(instance.Spec.AppProvider, string(MongoDB)) { // mongo
		CacheManager.db = new(mongo.MG)
	} else {
		CacheManager.NotReady()
		err = fmt.Errorf("provider %s is not supported", instance.Spec.AppProvider)
		log.Log.Error(err, "err")
		return &CacheManager, err
	}

	CacheManager.appConfig = appconfig.Config{
		Name:      instance.Name,
		Host:      instance.Spec.EndPoint,
		Databases: instance.Spec.Databases,
		Username:  string(secret.Data["username"]),
		Password:  string(secret.Data["password"]),
		Provider:  instance.Spec.AppProvider,
		Operation: instance.Spec.OperationType,
	}
	CacheManager.db.Init(CacheManager.appConfig)

	return &CacheManager, nil
}

func (d *DriverManager) Ready() {
	d.ready = true
}

func (d *DriverManager) NotReady() {
	d.ready = false
}

func (d *DriverManager) DBConnect() error {
	return d.db.Connect()
}

func (d *DriverManager) DBQuiesce() error {
	return d.db.Quiesce()
}

func (d *DriverManager) DBUnquiesce() error {
	return d.db.Unquiesce()
}
