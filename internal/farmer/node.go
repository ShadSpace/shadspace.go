package farmer

import (
	"context"
	"sync"
	"time"
	"log"
	"fmt"
	"bufio"
	"encoding/gob"
	"io"

	"github.com/lestonEth/shadspace/internal/p2p"
	"github.com/lestonEth/shadspace/internal/core"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/protocol"
	"github.com/multiformats/go-multiaddr"
	"github.com/libp2p/go-libp2p/core/network"

)

type FarmerNode struct {
	ctx          context.Context
	cancel       context.CancelFunc
	wg           sync.WaitGroup
	network      *p2p.NetworkManager
	storage      *StorageManager
	verifier     *ProofVerifier
	cfg          Config
	bootstrapPeers []peer.AddrInfo
	lastReconnect time.Time
}


func NewFarmerNode(parentCtx context.Context, cfg Config) (*FarmerNode, error) {
	ctx, cancel := context.WithCancel(parentCtx)

	// Parse bootstrap peers
	bootstrapPeers := make([]peer.AddrInfo, 0, len(cfg.Network.BootstrapPeers))

	for _, addrStr := range cfg.Network.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid bootstrap address %s: %w", addrStr, err)
		}
		addrInfo, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			cancel()
			return nil, fmt.Errorf("invalid bootstrap peer info %s: %w", addrStr, err)
		}
		bootstrapPeers = append(bootstrapPeers, *addrInfo)
	}


	network, err := p2p.NewNetworkManager(ctx, p2p.NetworkConfig{
		ListenAddr:     cfg.Network.ListenAddr,
		BootstrapPeers: cfg.Network.BootstrapPeers,
	})

	if err != nil {
		cancel()
		return nil, err
	}

	storage, err := NewStorageManager(cfg.Storage)
	if err != nil {
		cancel()
		return nil, err
	}

	verifier := NewProofVerifier()

	node := &FarmerNode{
		ctx:            ctx,
		cancel:         cancel,
		network:        network,
		storage:        storage,
		verifier:       verifier,
		cfg:            cfg,
		bootstrapPeers: bootstrapPeers,
		lastReconnect:  time.Now(),
	}

	// Register stream handlers
	network.Host().SetStreamHandler(protocol.ID("/shadspace/storage/1.0.0"), node.handleStorageStream)
	network.Host().SetStreamHandler(protocol.ID("/shadspace/retrieve/1.0.0"), node.handleRetrieveStream)

	return node, nil
}

func (f *FarmerNode) handleStorageStream(stream network.Stream) {
    defer func() {
        if err := stream.Close(); err != nil {
            log.Printf("Error closing stream: %v", err)
        }
    }()

    // Set a deadline for the entire operation
    if err := stream.SetDeadline(time.Now().Add(30 * time.Second)); err != nil {
        log.Printf("Failed to set deadline: %v", err)
        return
    }

    bufStream := bufio.NewReadWriter(
        bufio.NewReader(stream),
        bufio.NewWriter(stream),
    )
    dec := gob.NewDecoder(bufStream)
    enc := gob.NewEncoder(bufStream)

    // 1. Read metadata
    var meta core.FileMetadata
    if err := dec.Decode(&meta); err != nil {
        log.Printf("Failed to decode metadata: %v", err)
        return
    }

    // 2. Read shard data (with size limit)
    maxShardSize := 256 << 20 // 256MB
    shard, err := io.ReadAll(io.LimitReader(bufStream, int64(maxShardSize)))
    if err != nil {
        log.Printf("Failed to read shard: %v", err)
        return
    }

    // 3. Store the shard
    if err := f.storage.StoreChunk(meta.Hash, shard, &meta); err != nil {
        log.Printf("Storage failed: %v", err)
        return
    }

    // 4. Send acknowledgment
    if err := enc.Encode("OK"); err != nil {
        log.Printf("Failed to send ACK: %v", err)
        return
    }

    // 5. Flush the ACK
    if err := bufStream.Flush(); err != nil {
        log.Printf("Failed to flush ACK: %v", err)
        return
    }

    log.Printf("Stored shard %d/%d (%s, %d bytes)", 
        meta.ShardIndex+1, meta.TotalShards, meta.Hash[:8], len(shard))
}

func (f *FarmerNode) handleRetrieveStream(stream network.Stream) {
	defer stream.Close()

	// Create buffered reader/writer
	bufStream := bufio.NewReadWriter(
		bufio.NewReader(stream),
		bufio.NewWriter(stream),
	)
	dec := gob.NewDecoder(bufStream)

	// 1. Read the requested hash
	var hash string
	if err := dec.Decode(&hash); err != nil {
		log.Printf("Failed to decode retrieve request: %v", err)
		return
	}

	// 2. Get the shard from storage
	shard, err := f.storage.RetrieveChunk(hash)
	if err != nil {
		log.Printf("Failed to retrieve shard %s: %v", hash, err)
		return
	}

	// 3. Send the shard data
	if _, err := bufStream.Write(shard); err != nil {
		log.Printf("Failed to send shard data: %v", err)
		return
	}

	// Flush the data
	if err := bufStream.Flush(); err != nil {
		log.Printf("Failed to flush shard data: %v", err)
	}
}

func (f *FarmerNode) Start() error {
	if err := f.network.Start(); err != nil {
		return err
	}

	f.wg.Add(2)
	go f.monitorStorage()
	go f.monitorBootstrapConnections()

	return nil
}

func (f *FarmerNode) monitorBootstrapConnections() {
	defer f.wg.Done()

	checkInterval := 30 * time.Second
	reconnectTimeout := 5 * time.Minute

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if !f.hasActiveBootstrapConnection() {
				if time.Since(f.lastReconnect) > reconnectTimeout {
					log.Println("No active bootstrap connections, attempting to reconnect...")
					f.reconnectBootstrapPeers()
					f.lastReconnect = time.Now()
				} else {
					log.Println("No active bootstrap connections, but waiting before reconnecting...")
				}
			}
		case <-f.ctx.Done():
			return
		}
	}
}

func (f *FarmerNode) monitorStorage() {
    defer f.wg.Done()

    // Check storage immediately on startup
    f.checkStorageStatus()

    // Set up periodic checking
    ticker := time.NewTicker(5 * time.Minute)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            f.checkStorageStatus()
        case <-f.ctx.Done():
            log.Println("Stopping storage monitoring")
            return
        }
    }
}

func (f *FarmerNode) checkStorageStatus() {
    stats := f.storage.GetStats()
    
    // Log basic storage info
    log.Printf("[Storage] Chunks: %d, Used: %.2fGB/%.2fGB (%.1f%%)",
        stats["chunks"],
        stats["usedGB"],
        stats["allocatedGB"],
        stats["utilization"])
    
    // Check for critical conditions
    if avail, ok := stats["availableGB"].(float64); ok {
        if avail < 1.0 { // Critical if less than 1GB available
            log.Printf("CRITICAL: Only %.2fGB storage remaining", avail)
        } else if avail < 5.0 { // Warning if less than 5GB
            log.Printf("WARNING: Low storage - %.2fGB remaining", avail)
        }
    }
    
    // Check disk space against reserved threshold
    if diskFree, ok := stats["diskFreeGB"].(float64); ok {
        reserved := float64(f.cfg.Storage.ReservedSpaceGB)
        if diskFree < reserved {
            log.Printf("CRITICAL: Disk space below reserved threshold (%.2fGB < %.2fGB)", 
                diskFree, reserved)
        }
    }
}

func (f *FarmerNode) hasActiveBootstrapConnection() bool {
	currentPeers := f.network.GetPeers()
	for _, peer := range currentPeers {
		for _, bootstrapPeer := range f.bootstrapPeers {
			if peer.ID == bootstrapPeer.ID {
				return true
			}
		}
	}
	return false
}

func (f *FarmerNode) reconnectBootstrapPeers() {
	for _, peerInfo := range f.bootstrapPeers {
		ctx, cancel := context.WithTimeout(f.ctx, 10*time.Second)
		err := f.network.Host().Connect(ctx, peerInfo)
		cancel()

		if err != nil {
			log.Printf("Failed to reconnect to bootstrap peer %s: %v", peerInfo.ID, err)
		} else {
			log.Printf("Successfully reconnected to bootstrap peer %s", peerInfo.ID)
			return // Successfully connected to at least one bootstrap peer
		}
	}
	log.Println("Failed to reconnect to all bootstrap peers")
}

func (f *FarmerNode) Stop() {
	f.cancel()
	f.wg.Wait()
	f.network.Stop()
	f.storage.Close()
}

