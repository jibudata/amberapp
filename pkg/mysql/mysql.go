package mysql

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/appconfig"
)

type MYSQL struct {
	config appconfig.Config
	db     *sql.DB
}

func (m *MYSQL) Init(appConfig appconfig.Config) error {
	m.config = appConfig
	if m.db != nil {
		m.db.Close()
	}
	m.db = nil
	dbs := m.config.Databases
	if len(dbs) == 0 {
		err := fmt.Errorf("no database specified in %s", m.config.Name)
		log.Log.Error(err, "")
		return err
	}
	return nil
}

func (m *MYSQL) Connect() error {
	var err error
	log.Log.Info("mysql init")
	dbs := m.config.Databases
	if len(dbs) == 0 {
		err = fmt.Errorf("no database specified in %s", m.config.Name)
		log.Log.Error(err, "")
		return err
	}
	for _, database := range dbs {
		dsn := fmt.Sprintf("%s:%s@%s(%s)/%s", m.config.Username, m.config.Password, "tcp", m.config.Host, database)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("failed to init connection to mysql database %s, in %s", database, m.config.Name))
			return err
		}
		err = db.Ping()
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("cannot access mysql databases %s in %s", database, m.config.Name))
			return err
		}
		db.Close()
	}
	log.Log.Info("mysql connected")
	return nil
}

func (m *MYSQL) Quiesce() (*v1alpha1.QuiesceResult, error) {
	var err error
	log.Log.Info("mysql quiesce in progress...")

	dsn := fmt.Sprintf("%s:%s@%s(%s)/%s", m.config.Username, m.config.Password, "tcp", m.config.Host, m.config.Databases[0])
	m.db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Log.Error(err, fmt.Sprintf("failed to init connection to mysql database %s, in %s", m.config.Databases[0], m.config.Name))
		return nil, err
	}

	return nil, m.mysqlLock()
}

func (m *MYSQL) Unquiesce() error {
	log.Log.Info("mysql unquiesce in progress...")
	return m.mysqlUnlock()
}

func (m *MYSQL) mysqlLock() error {
	_, err := m.db.Exec("FLUSH TABLES WITH READ LOCK;")
	return err
}

func (m *MYSQL) mysqlUnlock() error {
	if m.db == nil {
		return nil
	}
	return m.db.Close()
}
