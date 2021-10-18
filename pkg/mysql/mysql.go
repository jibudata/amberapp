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
	dbs := appConfig.Databases
	if len(dbs) == 0 {
		err = fmt.Errorf("no database specified in %s", appConfig.Name)
		log.Log.Error(err, "")
		return err
	}
	for _, database := range dbs {
		dsn := fmt.Sprintf("%s:%s@%s(%s)/%s", appConfig.Username, appConfig.Password, "tcp", appConfig.Host, database)
		m.db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("failed to init connection to mysql database %s, in %s", database, appConfig.Name))
			return err
		}
		err = m.db.Ping()
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("cannot access mysql databases %s in %s", database, appConfig.Name))
			return err
		}
		m.db.Close()
	}
	/*
		result, _ := m.db.Query("select * from tb_1;")
		data, _ := result.Columns()
		for _, v := range data {
			fmt.Println(v)
		}
		type Tag struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		}
		for result.Next() {
			var tag Tag
			result.Scan(&tag.ID, &tag.Name)
			fmt.Println(tag.Name)
		}
	*/
	return nil
}

func (m *MYSQL) Quiesce(appConfig appconfig.Config) error {
	var err error
	m.config = appConfig
	log.Log.Info("mysql quiesce in progress...")
	dbs := appConfig.Databases
	if len(dbs) == 0 {
		err = fmt.Errorf("no database specified in %s", appConfig.Name)
		log.Log.Error(err, "")
		return err
	}
	opCh = make(chan sqlOp)
	statusCh = make(chan sqlStatus)
	for _, database := range dbs {
		dsn := fmt.Sprintf("%s:%s@%s(%s)/%s", appConfig.Username, appConfig.Password, "tcp", appConfig.Host, database)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("failed to init connection to mysql database %s, in %s", database, appConfig.Name))
			return err
		}
		go mysqlQuiesce(db)
		//if err != nil {
		//	log.Log.Error(err, fmt.Sprintf("failed to quiesce mysql database %s, in %s", database, appConfig.Name))
		//	return err
		//}
	}

	for i := 0; i < len(dbs); i++ {
		c := <-statusCh
		if c != Quiesced {
			return fmt.Errorf("failed to quiesce %s, %s", dbs[i], appConfig.Name)
		}
		log.Log.Info(fmt.Sprintf("quiesced database %s", dbs[i]))
	}

	return nil
}

func (m *MYSQL) Unquiesce(appConfig appconfig.Config) error {
	var err error
	m.config = appConfig
	log.Log.Info("mysql unquiesce in progress...")
	dbs := appConfig.Databases
	if len(dbs) == 0 {
		err = fmt.Errorf("no database specified in %s", appConfig.Name)
		log.Log.Error(err, "")
		return err
	}
	return nil
}

type sqlStatus string
type sqlOp string

const (
	Quiesced   sqlStatus = "Quiesced"
	Unquiesced sqlStatus = "Unquiesced"
)
const (
	Quiesce   sqlOp = "Quiesce"
	Unquiesce sqlOp = "Unquiesce"
)

var opCh chan sqlOp
var statusCh chan sqlStatus

func mysqlQuiesce(db *sql.DB) error {
	_, err := db.Exec("FLUSH TABLES WITH READ LOCK;")
	if err != nil {
		return fmt.Errorf("failed to lock")
	}
	log.Log.Info("lock done")
	c := Quiesced
	statusCh <- c
	for v := range opCh {
		if v == Unquiesce {
			db.Close()
			break
		}
	}

	return nil
}
