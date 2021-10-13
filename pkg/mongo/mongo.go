package mongo

import (
	"github.com/jibudata/app-hook-operator/pkg/appconfig"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type MG struct {
	config appconfig.Config
}

func (mg *MG) Connect(appConfig appconfig.Config) error {
	mg.config = appConfig
	log.Log.Info("mongodb init")
	return nil
}

func (mg *MG) Quiesce() error {
	log.Log.Info("mongodb Quiesce")
	return nil
}

func (mg *MG) Unquiesce() error {
	log.Log.Info("mongodb Unquiesce")
	return nil
}
