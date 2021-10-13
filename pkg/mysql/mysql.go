package mysql

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jibudata/app-hook-operator/pkg/appconfig"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type MYSQL struct {
	config appconfig.Config
	db     *sql.DB
}

func (m *MYSQL) Connect(appConfig appconfig.Config) error {
	var err error
	m.config = appConfig
	log.Log.Info("mysql init")
	var database = "test"
	dsn := fmt.Sprintf("%s:%s@%s(%s)/%s", appConfig.Username, appConfig.Password, "tcp", appConfig.Host, database)
	m.db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Log.Error(err, "Open mysql failed")
		return err
	}
	m.db.Ping()
	return nil
}

func (m *MYSQL) Quiesce() error {
	log.Log.Info("mysql Quiesce")
	return nil
}

func (m *MYSQL) Unquiesce() error {
	log.Log.Info("mysql Unquiesce")
	return nil
}
