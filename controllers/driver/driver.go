package driver

import (
	"fmt"
	"strings"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	v1alpha1 "github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/controllers/util"
	"github.com/jibudata/amberapp/pkg/appconfig"
	"github.com/jibudata/amberapp/pkg/mongo"
	"github.com/jibudata/amberapp/pkg/mysql"
	"github.com/jibudata/amberapp/pkg/postgres"
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

func NewManager(k8sclient client.Client, instance *v1alpha1.AppHook, secret *corev1.Secret) (*DriverManager, error) {
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

// danger, do NOT update the config when database is quiesced
func (d *DriverManager) Update(instance *v1alpha1.AppHook, secret *corev1.Secret) error {
	if d.appConfig.Name != instance.Name {
		return fmt.Errorf("apphook name %s cannot be changed", d.appConfig.Name)
	}
	if d.appConfig.Provider != instance.Spec.AppProvider {
		return fmt.Errorf("apphook %s provider %s cannot be changed", d.appConfig.Name, d.appConfig.Provider)
	}

	isChanged := false
	if d.appConfig.Host != instance.Spec.EndPoint {
		d.appConfig.Host = instance.Spec.EndPoint
		isChanged = true
	}
	if !equalStr(d.appConfig.Databases, instance.Spec.Databases) {
		d.appConfig.Databases = instance.Spec.Databases
		isChanged = true
	}
	if d.appConfig.Username != string(secret.Data["username"]) {
		d.appConfig.Username = string(secret.Data["username"])
		isChanged = true
	}
	if d.appConfig.Password != string(secret.Data["password"]) {
		d.appConfig.Password = string(secret.Data["password"])
		isChanged = true
	}

	if isChanged {
		log.Log.Info(fmt.Sprintf("detected %s configuration was changed, updating", d.appConfig.Name))
		if instance.Status.Phase == v1alpha1.HookQUIESCED {
			log.Log.Info(fmt.Sprintf("warning: %s hook status is quiesced when updating configuration", d.appConfig.Name))
		}
		err := d.db.Init(d.appConfig)
		if err != nil {
			return err
		}
		err = d.db.Connect()
		if err != nil {
			return err
		}
	}

	return nil
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

func equalStr(str1, str2 []string) bool {
	if len(str1) != len(str2) {
		return false
	}
	var i int
	for i = 0; i < len(str1); i++ {
		if !util.IsContain(str2, str1[i]) {
			return false
		}
	}
	for i = 0; i < len(str2); i++ {
		if !util.IsContain(str1, str2[i]) {
			return false
		}
	}
	return true
}
