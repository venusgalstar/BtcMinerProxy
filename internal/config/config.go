package config

import (
	"runtime"
	"strings"
	"time"
)

// Validation tags described here: https://pkg.go.dev/github.com/go-playground/validator/v10
type Config struct {
	Blockchain struct {
		EthNodeAddress string `env:"ETH_NODE_ADDRESS"   flag:"eth-node-address"   validate:"required,url"`
		EthLegacyTx    bool   `env:"ETH_NODE_LEGACY_TX" flag:"eth-node-legacy-tx" desc:"use it to disable EIP-1559 transactions"`
	}
	Environment string `env:"ENVIRONMENT" flag:"environment"`
	Hashrate    struct {
		CycleDuration          time.Duration `env:"HASHRATE_CYCLE_DURATION"           flag:"hashrate-cycle-duration"           validate:"omitempty,duration" desc:"duration of the hashrate cycle, after which the hashrate is evaluated, applies to both seller and buyer"`
		ValidationStartTimeout time.Duration `env:"HASHRATE_VALIDATION_START_TIMEOUT" flag:"hashrate-validation-start-timeout" validate:"omitempty,duration" desc:"time when validation kicks in, applies for buyer"`
		ShareTimeout           time.Duration `env:"HASHRATE_SHARE_TIMEOUT"            flag:"hashrate-share-timeout"            validate:"omitempty,duration" desc:"time to wait for the share to arrive, otherwise close contract, applies for buyer"`
		ErrorThreshold         float64       `env:"HASHRATE_ERROR_THRESHOLD"          flag:"hashrate-error-threshold"                                        desc:"hashrate relative error threshold for the contract to be considered fulfilling accurately, applies for buyer"`
		ErrorTimeout           time.Duration `env:"HASHRATE_ERROR_TIMEOUT"            flag:"hashrate-error-timeout"            validate:"omitempty,duration" desc:"time to wait for for the hashrate to fall within acceptable limits, otherwise close contract, applies for buyer"`
	}
	Marketplace struct {
		CloneFactoryAddress string `env:"CLONE_FACTORY_ADDRESS" flag:"contract-address"   validate:"required_if=Disable false,omitempty,eth_addr"`
		Mnemonic            string `env:"CONTRACT_MNEMONIC"     flag:"contract-mnemonic"  validate:"required_without=WalletPrivateKey|required_if=Disable false"`
		WalletPrivateKey    string `env:"WALLET_PRIVATE_KEY"    flag:"wallet-private-key" validate:"required_without=Mnemonic|required_if=Disable false"`
	}
	Miner struct {
		NotPropagateWorkerName bool          `env:"MINER_NOT_PROPAGATE_WORKER_NAME" flag:"miner-not-propagate-worker-name"     validate:""           desc:"not preserve worker name from the source in the destination pool. Preserving works only if the source miner worker name is defined as 'accountName.workerName'. Does not apply for contracts"`
		ShareTimeout           time.Duration `env:"MINER_SHARE_TIMEOUT"             flag:"miner-share-timeout"                 validate:"omitempty,duration"`
		VettingDuration        time.Duration `env:"MINER_VETTING_DURATION"          flag:"miner-vetting-duration"              validate:"omitempty,duration"`
	}
	Log struct {
		Color           bool   `env:"LOG_COLOR"            flag:"log-color"`
		FolderPath      string `env:"LOG_FOLDER_PATH"      flag:"log-folder-path"      validate:"omitempty,dirpath"    desc:"enables file logging and sets the folder path"`
		IsProd          bool   `env:"LOG_IS_PROD"          flag:"log-is-prod"          validate:""                     desc:"affects the format of the log output"`
		JSON            bool   `env:"LOG_JSON"             flag:"log-json"`
		LevelApp        string `env:"LOG_LEVEL_APP"        flag:"log-level-app"        validate:"omitempty,oneof=debug info warn error dpanic panic fatal"`
		LevelConnection string `env:"LOG_LEVEL_CONNECTION" flag:"log-level-connection" validate:"omitempty,oneof=debug info warn error dpanic panic fatal"`
		LevelProxy      string `env:"LOG_LEVEL_PROXY"      flag:"log-level-proxy"      validate:"omitempty,oneof=debug info warn error dpanic panic fatal"`
		LevelScheduler  string `env:"LOG_LEVEL_SCHEDULER"  flag:"log-level-scheduler"  validate:"omitempty,oneof=debug info warn error dpanic panic fatal"`
	}
	Pool struct {
		Address string `env:"POOL_ADDRESS" flag:"pool-address" validate:"required,uri"`
		// ConnTimeout time.Duration `env:"POOL_CONN_TIMEOUT" flag:"pool-conn-timeout" validate:"duration"`
	}
	Proxy struct {
		Address string `env:"PROXY_ADDRESS" flag:"proxy-address" validate:"required,hostname_port"`
	}
	System struct {
		Enable           bool   `env:"SYS_ENABLE"              flag:"sys-enable" desc:"enable system level configuration adjustments"`
		LocalPortRange   string `env:"SYS_LOCAL_PORT_RANGE"    flag:"sys-local-port-range"    desc:""`
		NetdevMaxBacklog string `env:"SYS_NET_DEV_MAX_BACKLOG" flag:"sys-netdev-max-backlog"  desc:""`
		RlimitHard       uint64 `env:"SYS_RLIMIT_HARD"         flag:"sys-rlimit-hard"         desc:""`
		RlimitSoft       uint64 `env:"SYS_RLIMIT_SOFT"         flag:"sys-rlimit-soft"         desc:""`
		Somaxconn        string `env:"SYS_SOMAXCONN"           flag:"sys-somaxconn"           desc:""`
		TcpMaxSynBacklog string `env:"SYS_TCP_MAX_SYN_BACKLOG" flag:"sys-tcp-max-syn-backlog" desc:""`
	}
	Web struct {
		Address   string `env:"WEB_ADDRESS"    flag:"web-address"    validate:"required,hostname_port" desc:"http server address host:port"`
		PublicUrl string `env:"WEB_PUBLIC_URL" flag:"web-public-url" validate:"omitempty,url"          desc:"public url of the proxyrouter, falls back to web-address if empty" `
	}
}

func (cfg *Config) SetDefaults() {
	if cfg.Environment == "" {
		cfg.Environment = "development"
	}

	// Hashrate

	if cfg.Hashrate.CycleDuration == 0 {
		cfg.Hashrate.CycleDuration = 5 * time.Minute
	}
	if cfg.Hashrate.ValidationStartTimeout == 0 {
		cfg.Hashrate.ValidationStartTimeout = 15 * time.Minute
	}
	if cfg.Hashrate.ShareTimeout == 0 {
		cfg.Hashrate.ShareTimeout = 10 * time.Minute
	}
	if cfg.Hashrate.ErrorThreshold == 0 {
		cfg.Hashrate.ErrorThreshold = 0.05
	}
	if cfg.Hashrate.ErrorTimeout == 0 {
		cfg.Hashrate.ErrorTimeout = cfg.Hashrate.CycleDuration*3 + 30*time.Second
	}

	// Marketplace

	// normalizes private key
	// TODO: convert and validate to ecies.PrivateKey
	cfg.Marketplace.WalletPrivateKey = strings.TrimPrefix(cfg.Marketplace.WalletPrivateKey, "0x")

	// Miner

	if cfg.Miner.VettingDuration == 0 {
		cfg.Miner.VettingDuration = 0 * time.Minute
	}

	if cfg.Miner.ShareTimeout == 0 {
		cfg.Miner.ShareTimeout = 3 * time.Minute
	}

	// Log

	if cfg.Log.LevelConnection == "" {
		cfg.Log.LevelConnection = "info"
	}
	if cfg.Log.LevelProxy == "" {
		cfg.Log.LevelProxy = "info"
	}
	if cfg.Log.LevelScheduler == "" {
		cfg.Log.LevelScheduler = "info"
	}
	if cfg.Log.LevelApp == "" {
		cfg.Log.LevelApp = "debug"
	}

	// System

	// cfg.System.Enable = true // TODO: Temporary override, remove this line

	if cfg.System.LocalPortRange == "" {
		cfg.System.LocalPortRange = "1024 65535"
	}
	if cfg.System.TcpMaxSynBacklog == "" {
		cfg.System.TcpMaxSynBacklog = "100000"
	}
	if cfg.System.Somaxconn == "" && runtime.GOOS == "linux" {
		cfg.System.Somaxconn = "100000"
	}
	if cfg.System.Somaxconn == "" && runtime.GOOS == "darwin" {
		// setting high value like 1000000 on darwin
		// for some reason blocks incoming connections
		// TODO: investigate best value for this
		cfg.System.Somaxconn = "2048"
	}
	if cfg.System.NetdevMaxBacklog == "" {
		cfg.System.NetdevMaxBacklog = "100000"
	}
	if cfg.System.RlimitSoft == 0 {
		cfg.System.RlimitSoft = 524288
	}
	if cfg.System.RlimitHard == 0 {
		cfg.System.RlimitHard = 524288
	}

	// Proxy

	if cfg.Proxy.Address == "" {
		cfg.Proxy.Address = "0.0.0.0:3333"
	}
	if cfg.Web.Address == "" {
		cfg.Web.Address = "0.0.0.0:8080"
	}
	if cfg.Web.PublicUrl == "" {
		cfg.Web.PublicUrl = "http://localhost:8080"
	}
}
