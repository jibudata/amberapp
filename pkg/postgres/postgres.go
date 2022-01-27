package postgres

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/jibudata/amberapp/pkg/appconfig"
	_ "github.com/lib/pq"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type PG struct {
	config appconfig.Config
	db     *sql.DB
}

func (pg *PG) Init(appConfig appconfig.Config) error {
	pg.config = appConfig
	return nil
}

func (pg *PG) Connect() error {
	var err error
	log.Log.Info("postgres connecting")

	connectionConfigStrings := pg.getConnectionString()
	if len(connectionConfigStrings) == 0 {
		return fmt.Errorf("no database found in %s", pg.config.Name)
	}

	for i := 0; i < len(connectionConfigStrings); i++ {
		pg.db, err = sql.Open("postgres", connectionConfigStrings[i])
		if err != nil {
			log.Log.Error(err, "cannot connect to postgres")
			return err
		}

		err = pg.db.Ping()
		if err != nil {
			log.Log.Error(err, fmt.Sprintf("cannot connect to postgres database %s", pg.config.Databases[i]))
			return err
		}
		pg.db.Close()
	}
	return nil
}

func (pg *PG) Quiesce() error {
	var err error
	log.Log.Info("postgres quiesce in progress...")

	backupName := "test"
	fastStartString := "true"

	connectionConfigStrings := pg.getConnectionString()
	if len(connectionConfigStrings) == 0 {
		return fmt.Errorf("no database found in %s", pg.config.Name)
	}

	for i := 0; i < len(connectionConfigStrings); i++ {
		pg.db, err = sql.Open("postgres", connectionConfigStrings[i])
		if err != nil {
			log.Log.Error(err, "cannot connect to postgres")
			return err
		}

		queryStr := fmt.Sprintf("select pg_start_backup('%s', %s);", backupName, fastStartString)

		result, queryErr := pg.db.Query(queryStr)

		if queryErr != nil {
			if strings.Contains(queryErr.Error(), "backup is already in progress") {
				pg.db.Close()
				continue
			}
			log.Log.Error(queryErr, "could not start postgres backup")
			return queryErr
		}

		var snapshotLocation string
		result.Next()

		scanErr := result.Scan(&snapshotLocation)
		if scanErr != nil {
			log.Log.Error(scanErr, "Postgres backup apparently started but could not understand server response")
			return scanErr
		}
		log.Log.Info(fmt.Sprintf("Successfully reach consistent recovery state at %s", snapshotLocation))
		pg.db.Close()
	}
	return nil
}

func (pg *PG) Unquiesce() error {
	var err error
	log.Log.Info("postgres unquiesce in progress...")
	connectionConfigStrings := pg.getConnectionString()
	if len(connectionConfigStrings) == 0 {
		return fmt.Errorf("no database found in %s", pg.config.Name)
	}

	for i := 0; i < len(connectionConfigStrings); i++ {
		pg.db, err = sql.Open("postgres", connectionConfigStrings[i])
		if err != nil {
			log.Log.Error(err, "cannot connect to postgres")
			return err
		}
		defer pg.db.Close()

		result, queryErr := pg.db.Query("select pg_stop_backup();")
		if queryErr != nil {
			if strings.Contains(queryErr.Error(), "exclusive backup not in progress") {
				pg.db.Close()
				continue
			}
			log.Log.Error(queryErr, "could not stop backup")
			return queryErr
		}

		var snapshotLocation string
		result.Next()

		scanErr := result.Scan(&snapshotLocation)
		if scanErr != nil {
			log.Log.Error(scanErr, "Postgres backup apparently stopped but could not understand server response")
			return scanErr
		}
	}
	return nil
}

func (pg *PG) getConnectionString() []string {
	var dbname string
	var connstr []string

	if len(pg.config.Databases) == 0 {
		log.Log.Error(fmt.Errorf("no database found in %s", pg.config.Name), "")
		return connstr
	}

	for i := 0; i < len(pg.config.Databases); i++ {
		dbname = pg.config.Databases[i]
		connstr = append(connstr, fmt.Sprintf("host=%s user=%s password=%s dbname=%s sslmode=disable", pg.config.Host, pg.config.Username, pg.config.Password, dbname))
	}
	return connstr
}
