package master

import (
	"time"
	
	"github.com/lestonEth/shadspace/internal/p2p"
	"github.com/spf13/viper"
)

type Config struct {
	Network      p2p.NetworkConfig
	Storage      StorageConfig
	Replication  ReplicationConfig
	Verification VerificationConfig
	API          APIConfig
}

type NetworkConfig struct {
	ListenAddr     string
	PrivateKey     string
	BootstrapPeers []string
	ProtocolTimeout time.Duration
}

type StorageConfig struct {
	ReplicationFactor int
	ShardingEnabled   bool
	ProofTTL          time.Duration
}

type ReplicationConfig struct {
	Strategy         string
	MinReplicas      int
	HealthCheckInterval time.Duration
}

type VerificationConfig struct {
	BatchSize    int
	Timeout      time.Duration
	MaxVerifiers int
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