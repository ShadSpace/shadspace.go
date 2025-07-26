package farmer

import (
	"time"

	"github.com/spf13/viper"
)

type Config struct {
	Network NetworkConfig
	Storage StorageConfig
	API     APIConfig
}

type NetworkConfig struct {
	ListenAddr     string
	BootstrapPeers []string
	ProtocolTimeout time.Duration
}

type StorageConfig struct {
	DataDir         string
	MaxCapacityGB   int
	ReservedSpaceGB int
}

type APIConfig struct {
	ListenAddr string
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