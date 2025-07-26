package master

import (
	"context"
	"log"
	"sync"
	
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lestonEth/shadspace/internal/core"
	"github.com/lestonEth/shadspace/internal/p2p"
)

type ReplicationManager struct {
	network    *p2p.NetworkManager
	registry   *FileRegistry
	strategies map[string]ReplicationStrategy
	activeJobs map[string]context.CancelFunc
	mu         sync.Mutex
	cfg        ReplicationConfig
}

type ReplicationStrategy interface {
	Replicate(file *core.FileMetadata, peers []peer.AddrInfo) error
}

func NewReplicationManager(network *p2p.NetworkManager, registry *FileRegistry, cfg ReplicationConfig) *ReplicationManager {
	return &ReplicationManager{
		network:    network,
		registry:   registry,
		strategies: make(map[string]ReplicationStrategy),
		activeJobs: make(map[string]context.CancelFunc),
		cfg:        cfg,
	}
}

func (r *ReplicationManager) UpdatePeerList(peers []peer.AddrInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	// TODO: Implement peer list update logic here
}

// Fix the ReplicateFile method
func (r *ReplicationManager) ReplicateFile(hash string) error {
	meta, peerIDs, exists := r.registry.GetFile(hash)
	if !exists {
		return core.ErrFileNotFound
	}

	strategy, ok := r.strategies[r.cfg.Strategy]
	if !ok {
		return core.ErrInvalidStrategy
	}

	// Convert peer.IDs to peer.AddrInfo
	var peers []peer.AddrInfo
	for _, pid := range peerIDs {
		if addrs := r.network.Host().Peerstore().Addrs(pid); len(addrs) > 0 {
			peers = append(peers, peer.AddrInfo{
				ID:    pid,
				Addrs: addrs,
			})
		}
	}

	_, cancel := context.WithTimeout(context.Background(), r.cfg.HealthCheckInterval)
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