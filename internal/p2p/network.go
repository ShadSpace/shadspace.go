package master

import (
	"context"
	"sync"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
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