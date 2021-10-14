package postgres

import (
	"github.com/jibudata/app-hook-operator/pkg/appconfig"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type PG struct {
	config appconfig.Config
}

func (pg *PG) Connect(appConfig appconfig.Config) error {
	pg.config = appConfig
	log.Log.Info("postgres init")
	return nil
}

func (pg *PG) Quiesce() error {
	log.Log.Info("postgres Quiesce")
	return nil
}

func (pg *PG) Unquiesce() error {
	log.Log.Info("postgres Unquiesce")
	return nil
}
