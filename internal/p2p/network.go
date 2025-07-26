package p2p

import (
	"context"
	"sync"
	"time"
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
)

type NetworkManager struct {
	ctx    context.Context
	host   host.Host
	peers  map[peer.ID]peer.AddrInfo
	peersMu sync.RWMutex
	cfg    NetworkConfig
}

type NetworkConfig struct {
	ListenAddr   string
	BootstrapPeers []string
}

func NewNetworkManager(ctx context.Context, cfg NetworkConfig) (*NetworkManager, error) {
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(cfg.ListenAddr),
	)
	if err != nil {
		return nil, err
	}

	return &NetworkManager{
		ctx:  ctx,
		host: h,
		peers: make(map[peer.ID]peer.AddrInfo),
		cfg:  cfg,
	}, nil
}

func (n *NetworkManager) Start() error {
	// Set up stream handlers
	n.host.SetStreamHandler("/shadspace/1.0.0", n.handleControlStream)
	n.host.SetStreamHandler("/shadspace/file/1.0.0", n.handleFileStream)
	
	// Bootstrap connection
	if len(n.cfg.BootstrapPeers) > 0 {
		go n.bootstrapConnect()
	}
	
	return nil
}


func (n *NetworkManager) handleControlStream(s network.Stream) {
	// Handle control messages
	defer s.Close()
}

func (n *NetworkManager) handleFileStream(s network.Stream) {
	// Handle file transfers
	defer s.Close()
}

func (n *NetworkManager) GetPeers() []peer.AddrInfo {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	
	peers := make([]peer.AddrInfo, 0, len(n.peers))
	for _, p := range n.peers {
		peers = append(peers, p)
	}
	return peers
}

func (n *NetworkManager) GetPeerCount() int {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	return len(n.peers)
}

func (n *NetworkManager) Host() host.Host {
	return n.host
}

func (n *NetworkManager) Stop() {
	n.host.Close()
}

func (n *NetworkManager) bootstrapConnect() {
	// Parse bootstrap peers and connect to them
	for _, addrStr := range n.cfg.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			fmt.Printf("Error parsing bootstrap address %s: %v\n", addrStr, err)
			continue
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			fmt.Printf("Error creating addr info from %s: %v\n", addrStr, err)
			continue
		}

		// Add the bootstrap peer to the peerstore
		n.host.Peerstore().AddAddrs(addrInfo.ID, addrInfo.Addrs, peerstore.PermanentAddrTTL)

		// Try to connect with retries
		for i := 0; i < 3; i++ {
			ctx, cancel := context.WithTimeout(n.ctx, 5*time.Second)
			err = n.host.Connect(ctx, *addrInfo)
			cancel()

			if err == nil {
				n.peersMu.Lock()
				n.peers[addrInfo.ID] = *addrInfo
				n.peersMu.Unlock()
				fmt.Printf("Connected to bootstrap peer: %s\n", addrInfo.ID)
				break
			}

			fmt.Printf("Failed to connect to bootstrap peer %s (attempt %d): %v\n", 
				addrInfo.ID, i+1, err)
			time.Sleep(time.Second * time.Duration(i+1))
		}
	}
}
