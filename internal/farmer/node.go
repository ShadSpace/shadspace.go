package farmer

import (
	"context"
	"sync"
	"time"
	"log"
	"fmt"

	"github.com/lestonEth/shadspace/internal/p2p"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
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

	return &FarmerNode{
		ctx:      ctx,
		cancel:   cancel,
		network:  network,
		storage:  storage,
		verifier: verifier,
		cfg:      cfg,
		bootstrapPeers: bootstrapPeers,
		lastReconnect: time.Now(),
	}, nil
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

