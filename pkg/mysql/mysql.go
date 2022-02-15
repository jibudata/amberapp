package mysql

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/jibudata/amberapp/pkg/appconfig"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type sqlStatus string
type sqlOp string

const (
	Locked   sqlStatus = "Locked"
	Unlocked sqlStatus = "Unlocked"
)
const (
	Lock   sqlOp = "Lock"
	Unlock sqlOp = "Unlock"
)

const sqlOperationTimeout = 30

type MYSQL struct {
	config    appconfig.Config
	db        *sql.DB
	sqlOpCh   *chan sqlOp
	sqlStatCh *chan sqlStatus
}

func (m *MYSQL) Init(appConfig appconfig.Config) error {
	m.config = appConfig
	opCh := make(chan sqlOp)
	m.sqlOpCh = &opCh
	dbs := m.config.Databases
	if len(dbs) == 0 {
		err := fmt.Errorf("no database specified in %s", m.config.Name)
		log.Log.Error(err, "")
		return err
	}
	statusCh := make(chan sqlStatus, len(dbs))
	m.sqlStatCh = &statusCh
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
		m.db, err = sql.Open("mysql", dsn)
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("failed to init connection to mysql database %s, in %s", database, m.config.Name))
			return err
		}
		err = m.db.Ping()
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("cannot access mysql databases %s in %s", database, m.config.Name))
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

func (m *MYSQL) Quiesce() error {
	var err error
	log.Log.Info("mysql quiesce in progress...")
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
		go m.mysqlLock(db)
	}

	for i := 0; i < len(dbs); i++ {
		c := <-*m.sqlStatCh
		if c != Locked {
			return fmt.Errorf("failed to lock database %s, %s", dbs[i], m.config.Name)
		}
		log.Log.Info(fmt.Sprintf("locked database %s", dbs[i]))
	}

	return nil
}

func (m *MYSQL) Unquiesce() error {
	var err error
	log.Log.Info("mysql unquiesce in progress...")
	dbs := m.config.Databases
	if len(dbs) == 0 {
		err = fmt.Errorf("no database specified in %s", m.config.Name)
		log.Log.Error(err, "")
		return err
	}

	go m.mysqlUnlock()
	// check there are quiescing ongoing
	for i := 0; i < sqlOperationTimeout; i++ {
		time.Sleep(1 * time.Second)
		if len(*m.sqlStatCh) == len(dbs) {
			break
		}
	}
	if len(*m.sqlStatCh) == 0 { // no quiescing, that is to say controller restarted
		log.Log.Info(fmt.Sprintf("no locking for %s", m.config.Name))
		return nil
	} else if len(*m.sqlStatCh) != len(dbs) {
		log.Log.Info(fmt.Sprintf("the number of unlocking database: %d is mismatch with: %d in %s", len(*m.sqlStatCh), len(dbs), m.config.Name))
	}
	for i := 0; i < len(dbs); i++ {
		c := <-*m.sqlStatCh
		if c != Unlocked {
			return fmt.Errorf("failed to unlock %s, %s", dbs[i], m.config.Name)
		}
		log.Log.Info(fmt.Sprintf("unlocked database %s", dbs[i]))
	}

	return nil
}

func (m *MYSQL) mysqlLock(db *sql.DB) error {
	_, err := db.Exec("FLUSH TABLES WITH READ LOCK;")
	if err != nil {
		return fmt.Errorf("failed to lock")
	}
	*m.sqlStatCh <- Locked
	// wait for unlock
	v := <-*m.sqlOpCh
	if v == Unlock {
		*m.sqlStatCh <- Unlocked
		db.Close()
	}

	return nil
}

func (m *MYSQL) mysqlUnlock() {
	*m.sqlOpCh <- Unlock
}
