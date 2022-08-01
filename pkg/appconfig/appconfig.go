package appconfig

import (
	"time"
)

const (
	ConnectionTimeout = 10 * time.Second
)

type Config struct {
	Name               string
	Host               string
	Databases          []string
	Username           string
	Password           string
	Provider           string
	Operation          string
	QuiesceFromPrimary bool
	Params             map[string]string
}
