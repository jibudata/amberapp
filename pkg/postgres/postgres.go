package postgres

import (
	"database/sql"
	"fmt"
	"strconv"
	"strings"

	_ "github.com/lib/pq"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/jibudata/amberapp/api/v1alpha1"
	"github.com/jibudata/amberapp/pkg/appconfig"
)

const (
	// < v15.0
	PG_START_BACKUP = "pg_start_backup"
	PG_STOP_BACKUP  = "pg_stop_backup"
	// >= v15.0
	PG_BACKUP_START = "pg_backup_start"
	PG_BACKUP_STOP  = "pg_backup_stop"
)

type PG struct {
	config  appconfig.Config
	db      *sql.DB
	version string
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

		err = pg.getVersion()
		if err != nil {
			return err
		}

		pg.db.Close()
	}
	log.Log.Info("connected to postgres")
	return nil
}

func (pg *PG) Prepare() (*v1alpha1.PreservedConfig, error) {
	return nil, nil
}

func (pg *PG) Quiesce() (*v1alpha1.QuiesceResult, error) {
	var err error
	log.Log.Info("postgres quiesce in progress...")

	backupName := "test"
	fastStartString := "true"

	connectionConfigStrings := pg.getConnectionString()
	if len(connectionConfigStrings) == 0 {
		return nil, fmt.Errorf("no database found in %s", pg.config.Name)
	}

	for i := 0; i < len(connectionConfigStrings); i++ {
		pg.db, err = sql.Open("postgres", connectionConfigStrings[i])
		if err != nil {
			log.Log.Error(err, "cannot connect to postgres")
			return nil, err
		}

		queryStr := fmt.Sprintf("select %s('%s', %s);", pg.getQuiesceCmd(), backupName, fastStartString)

		result, queryErr := pg.db.Query(queryStr)

		if queryErr != nil {
			if strings.Contains(queryErr.Error(), "backup is already in progress") {
				pg.db.Close()
				continue
			}
			log.Log.Error(queryErr, "could not start postgres backup")
			return nil, queryErr
		}

		var snapshotLocation string
		result.Next()

		scanErr := result.Scan(&snapshotLocation)
		if scanErr != nil {
			log.Log.Error(scanErr, "Postgres backup apparently started but could not understand server response")
			return nil, scanErr
		}
		log.Log.Info(fmt.Sprintf("Successfully reach consistent recovery state at %s", snapshotLocation))
		pg.db.Close()
	}
	return nil, nil
}

func (pg *PG) Unquiesce(prev *v1alpha1.PreservedConfig) error {
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

		result, queryErr := pg.db.Query(fmt.Sprintf("select %s();", pg.getUnQuiesceCmd()))
		if queryErr != nil {
			if strings.Contains(queryErr.Error(), "not in progress") {
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

func (pg *PG) getVersion() error {
	var version string
	result, queryErr := pg.db.Query("show server_version;")
	if queryErr != nil {
		log.Log.Error(queryErr, "could get postgres version")
		return queryErr
	}

	result.Next()
	scanErr := result.Scan(&version)
	if scanErr != nil {
		log.Log.Error(scanErr, "scan postgres version with error")
		return scanErr
	}

	pg.version = strings.Split(version, " ")[0]
	log.Log.Info("get postgres version", "version", version, "instance", pg.config.Name)

	return nil
}

func (pg *PG) isVersionAboveV15() bool {
	aboveV15 := false
	if pg.version != "" {
		version, err := strconv.ParseFloat(pg.version, 64)
		if err != nil {
			log.Log.Error(err, "failed to convert version to number", "version", pg.version)
		} else {
			if version >= 15.0 {
				aboveV15 = true
			}
		}
	}

	return aboveV15
}

func (pg *PG) getQuiesceCmd() string {
	if pg.isVersionAboveV15() {
		return PG_BACKUP_START
	}
	return PG_START_BACKUP
}

func (pg *PG) getUnQuiesceCmd() string {
	if pg.isVersionAboveV15() {
		return PG_BACKUP_STOP
	}
	return PG_STOP_BACKUP
}
