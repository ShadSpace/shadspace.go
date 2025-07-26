package p2p

import (
	"context"
	"sync"
	"time"
	"fmt"
	"log"
	"crypto/rand"
	"encoding/base64"

	"github.com/libp2p/go-libp2p/core/crypto" 
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
	connNotifee *connNotifee
}

type connNotifee struct {
    nm *NetworkManager
}

type NetworkConfig struct {
	ListenAddr   string
	PrivateKey   string
	BootstrapPeers []string
}

func NewNetworkManager(ctx context.Context, cfg NetworkConfig) (*NetworkManager, error) {
	var priv crypto.PrivKey
	var err error

	// If private key is configured, use it
	if cfg.PrivateKey != "" {
		priv, err = decodePrivateKey(cfg.PrivateKey)
		if err != nil {
			return nil, fmt.Errorf("failed to decode private key: %w", err)
		}
	} else {
		// Generate new key if none configured
		priv, _, err = crypto.GenerateKeyPairWithReader(crypto.ECDSA, 2048, rand.Reader)
		if err != nil {
			return nil, err
		}
	}

	// Create host with private key
	h, err := libp2p.New(
		libp2p.ListenAddrStrings(cfg.ListenAddr),
		libp2p.Identity(priv),
	)

	if err != nil {
		return nil, err
	}


	nm := &NetworkManager{
        ctx:   ctx,
        host:  h,
        peers: make(map[peer.ID]peer.AddrInfo),
        cfg:   cfg,
    }
    
    // Set up connection notifee
    nm.connNotifee = &connNotifee{nm: nm}
    h.Network().Notify(nm.connNotifee)

	log.Printf("Created peer with ID: %s", h.ID())
	return nm, nil
}

// Helper function to decode private key
func decodePrivateKey(keyStr string) (crypto.PrivKey, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(keyStr)
	if err != nil {
		return nil, fmt.Errorf("failed to base64 decode private key: %w", err)
	}

	priv, err := crypto.UnmarshalPrivateKey(keyBytes)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal private key: %w", err)
	}

	return priv, nil
}

func (n *NetworkManager) Start() error {
	// Set up stream handlers
	n.host.SetStreamHandler("/shadspace/1.0.0", n.handleControlStream)
	n.host.SetStreamHandler("/shadspace/file/1.0.0", n.handleFileStream)

	// Add farmer-specific handlers
	n.host.SetStreamHandler("/shadspace/storage/1.0.0", n.handleStorageRequest)
	n.host.SetStreamHandler("/shadspace/proof/1.0.0", n.handleProofVerification)
	
	// Bootstrap connection
	if len(n.cfg.BootstrapPeers) > 0 {
		go n.bootstrapConnect()
	}
	
	return nil
}

func (n *connNotifee) Listen(network.Network, multiaddr.Multiaddr)      {}
func (n *connNotifee) ListenClose(network.Network, multiaddr.Multiaddr) {}
func (n *connNotifee) Connected(_ network.Network, conn network.Conn) {
    peerID := conn.RemotePeer()
    addrs := conn.RemoteMultiaddr()
    
    n.nm.peersMu.Lock()
    defer n.nm.peersMu.Unlock()
    
    n.nm.peers[peerID] = peer.AddrInfo{
        ID:    peerID,
        Addrs: []multiaddr.Multiaddr{addrs},
    }
    log.Printf("Added new peer connection: %s", peerID)
}

func (n *connNotifee) Disconnected(_ network.Network, conn network.Conn) {
    peerID := conn.RemotePeer()
    
    n.nm.peersMu.Lock()
    defer n.nm.peersMu.Unlock()
    
    delete(n.nm.peers, peerID)
    log.Printf("Removed disconnected peer: %s", peerID)
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
	log.Printf("Getting peer count: %d", len(n.peers))
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

func (n *NetworkManager) handleStorageRequest(s network.Stream) {
	defer s.Close()
	// Handle storage requests from master nodes
}

func (n *NetworkManager) handleProofVerification(s network.Stream) {
	defer s.Close()
	// Handle proof verification requests
}

func (n *NetworkManager) IsConnectedTo(peerID peer.ID) bool {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	
	_, exists := n.peers[peerID]
	return exists
}

