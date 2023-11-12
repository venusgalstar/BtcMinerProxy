package system

type osConfigurator interface {
	GetConfig() (*Config, error)
	ApplyConfig(cfg *Config) error
}
