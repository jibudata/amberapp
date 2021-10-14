package appconfig

type Config struct {
	Name      string
	Host      string
	Databases []string
	Username  string
	Password  string
	Provider  string
	Operation string
}
