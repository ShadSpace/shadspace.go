package farmer

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Network  NetworkConfig
	Storage  StorageConfig
	Master   MasterConfig
	Logging  LoggingConfig
}

type NetworkConfig struct {
	ListenAddr     string
	BootstrapPeers []string
	ProtocolTimeout time.Duration
	EnableNAT      bool
	EnableRelay    bool
	PublicIP       string
	AnnounceAddrs  []string
}

type StorageConfig struct {
	DataDir         string
	MaxCapacityGB   int
	ReservedSpaceGB int
	CleanupInterval time.Duration
}

type MasterConfig struct {
	APIUrl             string
	ReportInterval     time.Duration
	HealthCheckInterval time.Duration
}

type LoggingConfig struct {
	Level  string
	Format string
}

func LoadConfig(path string) (Config, error) {
	viper.SetConfigFile(path)
	if err := viper.ReadInConfig(); err != nil {
		return Config{}, err
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return Config{}, err
	}

	return cfg, nil
}