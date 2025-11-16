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
	"github.com/libp2p/go-libp2p/core/protocol"
	// "github.com/libp2p/go-libp2p/p2p/net/nat"
    "github.com/libp2p/go-libp2p/p2p/host/autorelay"
    // "github.com/libp2p/go-libp2p/core/transport"
    manet "github.com/multiformats/go-multiaddr/net"
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
	ListenAddr     string
	PrivateKey     string
	BootstrapPeers []string
	Protocols      []ProtocolHandler
	EnableNAT      bool
	EnableRelay    bool
	PublicIP       string
	AnnounceAddrs  []string
}

type ProtocolHandler struct {
	ProtocolID string
	Handler    func(network.Stream)
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

	// Enhanced options for NAT traversal
	opts := []libp2p.Option{
		libp2p.ListenAddrStrings(cfg.ListenAddr),
		libp2p.Identity(priv),
	}

	// Add NAT traversal options if enabled
	if cfg.EnableNAT {
		opts = append(opts,
			libp2p.NATPortMap(),        // Enable UPnP port mapping
			libp2p.EnableNATService(),  // Enable NAT hole punching
			libp2p.EnableHolePunching(), // Enable hole punching
		)
	}

	// Add relay if enabled
	if cfg.EnableRelay {
		opts = append(opts, libp2p.EnableRelay()) // Enable circuit relay
		
		// Enable auto relay with bootstrap peers as potential relays
		if len(cfg.BootstrapPeers) > 0 {
			opts = append(opts, 
				libp2p.EnableAutoRelay(
					autorelay.WithPeerSource(func(ctx context.Context, num int) <-chan peer.AddrInfo {
						ch := make(chan peer.AddrInfo)
						go func() {
							defer close(ch)
							for _, addrStr := range cfg.BootstrapPeers {
								if ma, err := multiaddr.NewMultiaddr(addrStr); err == nil {
									if addrInfo, err := peer.AddrInfoFromP2pAddr(ma); err == nil {
										select {
										case ch <- *addrInfo:
										case <-ctx.Done():
											return
										}
									}
								}
							}
						}()
						return ch
					}),
				),
			)
		}
	}

	// Add public IP announcement if specified
	if cfg.PublicIP != "" || len(cfg.AnnounceAddrs) > 0 {
		opts = append(opts, withPublicAddresses(cfg.PublicIP, cfg.AnnounceAddrs))
	}

	// Create host with enhanced options
	h, err := libp2p.New(opts...)
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

	log.Printf("🚀 Created peer with ID: %s", h.ID())
	
	// Log all addresses for debugging
	nm.logAddresses()
	
	return nm, nil
}

// Helper function to handle public address announcement
func withPublicAddresses(publicIP string, announceAddrs []string) libp2p.Option {
	return func(cfg *libp2p.Config) error {
		var publicAddrs []multiaddr.Multiaddr
		
		// Add manual public IP if provided
		if publicIP != "" {
			publicAddr, err := multiaddr.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/53799", publicIP))
			if err != nil {
				return err
			}
			publicAddrs = append(publicAddrs, publicAddr)
		}
		
		// Add any manually specified announce addresses
		for _, addrStr := range announceAddrs {
			addr, err := multiaddr.NewMultiaddr(addrStr)
			if err != nil {
				return err
			}
			publicAddrs = append(publicAddrs, addr)
		}
		
		if len(publicAddrs) == 0 {
			return nil
		}
		
		// Add address factory that includes public addresses
		return cfg.Apply(libp2p.AddrsFactory(func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			var filtered []multiaddr.Multiaddr
			
			// Keep non-private addresses
			for _, addr := range addrs {
				if !manet.IsPrivateAddr(addr) {
					filtered = append(filtered, addr)
				}
			}
			
			// Add our public addresses
			filtered = append(filtered, publicAddrs...)
			
			log.Printf("Address factory: %d addresses (added %d public)", 
				len(filtered), len(publicAddrs))
				
			return filtered
		}))
	}
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

// Log all addresses for debugging NAT status
func (n *NetworkManager) logAddresses() {
	addrs := n.host.Addrs()
	log.Printf("📡 Network addresses (%d total):", len(addrs))
	
	privateCount := 0
	publicCount := 0
	
	for i, addr := range addrs {
		fullAddr := fmt.Sprintf("%s/p2p/%s", addr, n.host.ID())
		if manet.IsPrivateAddr(addr) {
			log.Printf("  [%d] %s (PRIVATE)", i, fullAddr)
			privateCount++
		} else {
			log.Printf("  [%d] %s (PUBLIC)", i, fullAddr)
			publicCount++
		}
	}
	
	log.Printf("📊 Address summary: %d private, %d public", privateCount, publicCount)
	
	if publicCount == 0 && n.cfg.EnableNAT {
		log.Printf("⚠️  No public addresses found. NAT traversal may be in progress...")
	}
}

func (n *NetworkManager) Start() error {
	// Register protocol handlers
	for _, ph := range n.cfg.Protocols {
		n.host.SetStreamHandler(protocol.ID(ph.ProtocolID), ph.Handler)
		log.Printf("✅ Registered handler for protocol: %s", ph.ProtocolID)
	}

	// Bootstrap connection
	if len(n.cfg.BootstrapPeers) > 0 {
		log.Printf("🔗 Connecting to %d bootstrap peers...", len(n.cfg.BootstrapPeers))
		go n.bootstrapConnect()
	} else {
		log.Printf("ℹ️  No bootstrap peers configured")
	}
	
	// Log NAT status
	n.logNATStatus()
	
	return nil
}

// Log detailed NAT status
func (n *NetworkManager) logNATStatus() {
	log.Printf("🌐 NAT Configuration:")
	log.Printf("   - NAT Enabled: %v", n.cfg.EnableNAT)
	log.Printf("   - Relay Enabled: %v", n.cfg.EnableRelay)
	log.Printf("   - Public IP: %s", n.cfg.PublicIP)
	log.Printf("   - Announce Addresses: %v", n.cfg.AnnounceAddrs)
}

func (n *connNotifee) Listen(network.Network, multiaddr.Multiaddr)      {}
func (n *connNotifee) ListenClose(network.Network, multiaddr.Multiaddr) {}

func (n *connNotifee) Connected(_ network.Network, conn network.Conn) {
    peerID := conn.RemotePeer()
    remoteAddr := conn.RemoteMultiaddr()
    localAddr := conn.LocalMultiaddr()
    
    n.nm.peersMu.Lock()
    defer n.nm.peersMu.Unlock()
    
    n.nm.peers[peerID] = peer.AddrInfo{
        ID:    peerID,
        Addrs: []multiaddr.Multiaddr{remoteAddr},
    }
    
    log.Printf("🔗 CONNECTED to peer: %s", peerID)
    log.Printf("   📍 Local: %s", localAddr)
    log.Printf("   📍 Remote: %s", remoteAddr)
    
    // Log connection direction
    if conn.Stat().Direction == network.DirInbound {
        log.Printf("   🎯 Direction: INBOUND (peer connected to us)")
    } else {
        log.Printf("   🎯 Direction: OUTBOUND (we connected to peer)")
    }
}

func (n *connNotifee) Disconnected(_ network.Network, conn network.Conn) {
    peerID := conn.RemotePeer()
    
    n.nm.peersMu.Lock()
    defer n.nm.peersMu.Unlock()
    
    delete(n.nm.peers, peerID)
    log.Printf("🔌 DISCONNECTED from peer: %s", peerID)
}

func (n *NetworkManager) GetPeers() []peer.AddrInfo {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	
	peers := make([]peer.AddrInfo, 0, len(n.peers))
	for _, p := range n.peers {
		peers = append(peers, p)
	}
	
	log.Printf("👥 Current peers: %d", len(peers))
	for i, peer := range peers {
		log.Printf("   [%d] %s", i, peer.ID)
	}
	
	return peers
}

func (n *NetworkManager) GetPeerCount() int {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	count := len(n.peers)
	log.Printf("📊 Peer count: %d", count)
	return count
}

func (n *NetworkManager) Host() host.Host {
	return n.host
}

func (n *NetworkManager) Stop() {
	log.Printf("🛑 Stopping network manager...")
	n.host.Close()
	log.Printf("✅ Network manager stopped")
}

func (n *NetworkManager) bootstrapConnect() {
	successCount := 0
	failureCount := 0

	for _, addrStr := range n.cfg.BootstrapPeers {
		ma, err := multiaddr.NewMultiaddr(addrStr)
		if err != nil {
			log.Printf("❌ Error parsing bootstrap address %s: %v", addrStr, err)
			failureCount++
			continue
		}

		addrInfo, err := peer.AddrInfoFromP2pAddr(ma)
		if err != nil {
			log.Printf("❌ Error creating addr info from %s: %v", addrStr, err)
			failureCount++
			continue
		}

		// Add the bootstrap peer to the peerstore
		n.host.Peerstore().AddAddrs(addrInfo.ID, addrInfo.Addrs, peerstore.PermanentAddrTTL)

		// Try to connect with retries
		connected := false
		for i := 0; i < 3; i++ {
			ctx, cancel := context.WithTimeout(n.ctx, 10*time.Second) // Increased timeout
			err = n.host.Connect(ctx, *addrInfo)
			cancel()

			if err == nil {
				n.peersMu.Lock()
				n.peers[addrInfo.ID] = *addrInfo
				n.peersMu.Unlock()
				log.Printf("✅ Connected to bootstrap peer: %s", addrInfo.ID)
				successCount++
				connected = true
				break
			}

			log.Printf("⚠️ Failed to connect to bootstrap peer %s (attempt %d): %v", 
				addrInfo.ID, i+1, err)
			time.Sleep(time.Second * time.Duration(i+1))
		}
		
		if !connected {
			failureCount++
		}
	}

	log.Printf("📊 Bootstrap results: %d successful, %d failed", successCount, failureCount)
}

func (n *NetworkManager) IsConnectedTo(peerID peer.ID) bool {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()
	
	_, exists := n.peers[peerID]
	log.Printf("🔍 Check connection to %s: %v", peerID, exists)
	return exists
}

// New method: Get connection quality metrics
func (n *NetworkManager) GetConnectionMetrics() map[string]interface{} {
	n.peersMu.RLock()
	defer n.peersMu.RUnlock()

	metrics := make(map[string]interface{})
	metrics["total_peers"] = len(n.peers)
	
	// Count connection directions
	inbound := 0
	outbound := 0
	for _, conn := range n.host.Network().Conns() {
		if conn.Stat().Direction == network.DirInbound {
			inbound++
		} else {
			outbound++
		}
	}
	
	metrics["inbound_connections"] = inbound
	metrics["outbound_connections"] = outbound
	metrics["nat_enabled"] = n.cfg.EnableNAT
	metrics["relay_enabled"] = n.cfg.EnableRelay
	
	return metrics
}