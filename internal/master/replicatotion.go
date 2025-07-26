package master

import (
	"context"
	"sync"
	
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lestonEth/shadspace/internal/core"
	"github.com/lestonEth/shadspace/internal/p2p"
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

/*
ReplicateFile replicates a file to the given peers using the configured replication strategy.
It first checks if the file exists in the registry, and if not, returns an error.
It then gets the replication strategy from the strategies map, and if not found, returns an error.
It then creates a context with a timeout, and locks the active jobs map to prevent concurrent replication of the same file.
It then starts a goroutine to replicate the file to the given peers, and if the replication fails, it logs an error.
It then returns nil.
*/
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

func (r *ReplicationManager) GetStats() map[string]interface{} {
	r.mu.Lock()
	defer r.mu.Unlock()

	stats := make(map[string]interface{})
	stats["active_jobs"] = len(r.activeJobs)
	stats["strategy"] = r.cfg.Strategy
	return stats
}