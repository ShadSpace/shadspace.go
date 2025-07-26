package master

import (
	"context"
	"sync"
	"time"
	"bufio"

	"github.com/lestonEth/shadspace/internal/p2p"
	"github.com/klauspost/reedsolomon"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lestonEth/shadspace/internal/core"
	"fmt"
	"log"
	"encoding/gob"
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

func (c *Coordinator) selectStorageNodes() []peer.AddrInfo {
    allPeers := c.network.GetPeers()
    
    // Filter peers that are storage nodes (you might want to add node type metadata)
    var storagePeers []peer.AddrInfo
    for _, p := range allPeers {
        storagePeers = append(storagePeers, p)
    }
    
    // Simple selection - in reality you'd want to consider:
    // - Available space
    // - Geographic location
    // - Network latency
    // - Current load
    if len(storagePeers) > c.cfg.Storage.ReplicationFactor {
        return storagePeers[:c.cfg.Storage.ReplicationFactor]
    }
    return storagePeers
}

func (c *Coordinator) distributeWithErasureCoding(meta core.FileMetadata, data []byte, nodes []peer.AddrInfo) error {
    // Parameters for erasure coding
    dataShards := len(nodes) - (len(nodes) / 3) // Allow 1/3 nodes to fail
    if dataShards < 1 {
        dataShards = 1
    }
    parityShards := len(nodes) - dataShards

    // Create encoder
    enc, err := reedsolomon.New(dataShards, parityShards)
    if err != nil {
        return fmt.Errorf("failed to create encoder: %w", err)
    }

    // Split data into shards
    shards, err := enc.Split(data)
    if err != nil {
        return fmt.Errorf("failed to split data: %w", err)
    }

    // Encode parity shards
    err = enc.Encode(shards)
    if err != nil {
        return fmt.Errorf("failed to encode data: %w", err)
    }

    // Distribute shards to nodes
    var wg sync.WaitGroup
    errorCh := make(chan error, len(nodes))

    for i, node := range nodes {
        wg.Add(1)
        go func(idx int, node peer.AddrInfo) {
            defer wg.Done()
            
            // Create a shard metadata
            shardMeta := meta
            shardMeta.IsShard = true
            shardMeta.ShardIndex = idx
            shardMeta.TotalShards = len(nodes)
            
            err := c.sendShardToNode(shards[idx], shardMeta, node)
            if err != nil {
                errorCh <- fmt.Errorf("node %s failed: %w", node.ID, err)
                return
            }
        }(i, node)
    }

    wg.Wait()
    close(errorCh)

    // Check for errors
    var errors []error
    for err := range errorCh {
        errors = append(errors, err)
    }

	log.Printf("Errors: %v", errors)

    if len(errors) > parityShards {
        return fmt.Errorf("too many storage errors (%d), cannot guarantee recovery", len(errors))
    }

    // If we had some failures but still enough shards, register with available nodes
    if len(errors) > 0 {
        log.Printf("Warning: stored with %d errors (within tolerance)", len(errors))
    }

    // Register file in registry with all intended nodes
    var peerIDs []peer.ID
    for _, node := range nodes {
        peerIDs = append(peerIDs, node.ID)
    }
    c.registry.RegisterFile(meta, peerIDs)

    return nil
}

func (c *Coordinator) sendShardToNode(shard []byte, meta core.FileMetadata, node peer.AddrInfo) error {
    ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
    defer cancel()

    log.Printf("Sending shard %d to %s", meta.ShardIndex, node.ID)

    stream, err := c.network.Host().NewStream(ctx, node.ID, "/shadspace/storage/1.0.0")
    if err != nil {
        return fmt.Errorf("stream failed: %w", err)
    }
    defer stream.Close()

    // Set deadline for the entire operation
    if err := stream.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
        return fmt.Errorf("deadline failed: %w", err)
    }

    bufStream := bufio.NewReadWriter(
        bufio.NewReader(stream),
        bufio.NewWriter(stream),
    )
    enc := gob.NewEncoder(bufStream)
    dec := gob.NewDecoder(bufStream)

    // 1. Send metadata
    if err := enc.Encode(meta); err != nil {
        return fmt.Errorf("metadata send failed: %w", err)
    }

    // 2. Flush metadata
    if err := bufStream.Flush(); err != nil {
        return fmt.Errorf("metadata flush failed: %w", err)
    }

    // 3. Send shard
    if _, err := bufStream.Write(shard); err != nil {
        return fmt.Errorf("shard send failed: %w", err)
    }

    // 4. Flush shard
    if err := bufStream.Flush(); err != nil {
        return fmt.Errorf("shard flush failed: %w", err)
    }

    // 5. Wait for ACK
    var ack string
    if err := dec.Decode(&ack); err != nil {
        return fmt.Errorf("ACK receive failed: %w", err)
    }

    if ack != "OK" {
        return fmt.Errorf("invalid ACK: %s", ack)
    }

    log.Printf("Shard %d stored on %s", meta.ShardIndex, node.ID)
    return nil
}