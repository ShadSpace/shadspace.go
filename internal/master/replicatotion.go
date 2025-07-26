package master

import (
	"context"
	"sync"
	"time"
	
	"github.com/lestonEth/shadspace/internal/core"
)

type ReplicationManager struct {
	network    *NetworkManager
	registry   *FileRegistry
	strategies map[string]ReplicationStrategy
	activeJobs map[string]context.CancelFunc
	mu         sync.Mutex
	cfg        ReplicationConfig
}

type ReplicationStrategy interface {
	Replicate(file *core.FileMetadata, peers []peer.AddrInfo) error
}

func NewReplicationManager(network *NetworkManager, registry *FileRegistry, cfg ReplicationConfig) *ReplicationManager {
	return &ReplicationManager{
		network:    network,
		registry:   registry,
		strategies: make(map[string]ReplicationStrategy),
		activeJobs: make(map[string]context.CancelFunc),
		cfg:        cfg,
	}
}

func (r *ReplicationManager) ReplicateFile(hash string) error {
	meta, peers, exists := r.registry.GetFile(hash)
	if !exists {
		return core.ErrFileNotFound
	}

	strategy, ok := r.strategies[r.cfg.Strategy]
	if !ok {
		return core.ErrInvalidStrategy
	}

	ctx, cancel := context.WithTimeout(context.Background(), r.cfg.HealthCheckInterval)
	r.mu.Lock()
	r.activeJobs[hash] = cancel
	r.mu.Unlock()

	go func() {
		defer cancel()
		if err := strategy.Replicate(meta, peers); err != nil {
			log.Printf("Replication failed for %s: %v", hash, err)
		}
	}()

	return nil
}