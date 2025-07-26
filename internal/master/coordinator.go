package master

import (
	"context"
	"sync"
	"time"

	"github.com/lestonEth/shadspace/internal/p2p"
)

type Coordinator struct {
	ctx        context.Context
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	registry   *FileRegistry
	network    *p2p.NetworkManager
	replicator *ReplicationManager
	cfg        Config
	startTime  time.Time
}

func NewCoordinator(parentCtx context.Context, cfg Config) (*Coordinator, error) {
	ctx, cancel := context.WithCancel(parentCtx)

	// Initialize components
	registry := NewFileRegistry()
	network, err := p2p.NewNetworkManager(ctx, cfg.Network)
	if err != nil {
		cancel()
		return nil, err
	}
	

	replicator := NewReplicationManager(network, registry, cfg.Replication)

	return &Coordinator{
		ctx:        ctx,
		cancel:     cancel,
		registry:   registry,
		network:    network,
		replicator: replicator,
		cfg:        cfg,
		startTime:  time.Now(),
	}, nil
}

func (c *Coordinator) Start() error {
	if err := c.network.Start(); err != nil {
		return err
	}
	
	c.wg.Add(1)
	go c.monitorPeers()
	
	return nil
}

func (c *Coordinator) Stop() {
	c.cancel()
	c.wg.Wait()
	c.network.Stop()
}

func (c *Coordinator) monitorPeers() {
	defer c.wg.Done()
	
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	
	for {
		select {
		case <-ticker.C:
			peers := c.network.GetPeers()
			c.replicator.UpdatePeerList(peers)
			
		case <-c.ctx.Done():
			return
		}
	}
}