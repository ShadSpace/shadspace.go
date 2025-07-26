package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/gob"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
)

type FileInfo struct {
	Name string
	Size int64
	Hash string
}

type MasterNode struct {
	host      host.Host
	peers     map[peer.ID]peer.AddrInfo
	files     map[string]FileInfo
	filesLock sync.RWMutex
	peersLock sync.RWMutex
	ctx       context.Context
}

func loadOrCreateKey(keyPath string) (crypto.PrivKey, error) {
	if _, err := os.Stat(keyPath); err == nil {
		keyBytes, err := os.ReadFile(keyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read key file: %w", err)
		}
		
		decoded, err := hex.DecodeString(string(keyBytes))
		if err != nil {
			return nil, fmt.Errorf("failed to decode key: %w", err)
		}
		
		priv, err := crypto.UnmarshalPrivateKey(decoded)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal key: %w", err)
		}
		return priv, nil
	}

	priv, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		return nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	keyBytes, err := crypto.MarshalPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	err = os.WriteFile(keyPath, []byte(hex.EncodeToString(keyBytes)), 0600)
	if err != nil {
		return nil, fmt.Errorf("failed to write key file: %w", err)
	}

	return priv, nil
}

func NewMasterNode(ctx context.Context, keyPath string) (*MasterNode, error) {
	priv, err := loadOrCreateKey(keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to get private key: %w", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/53798"),
	)
	if err != nil {
		return nil, err
	}

	return &MasterNode{
		host:  h,
		peers: make(map[peer.ID]peer.AddrInfo),
		files: make(map[string]FileInfo),
		ctx:   ctx,
	}, nil
}

func (mn *MasterNode) BroadcastFile(filePath string) error {
    log.Printf("Starting broadcast of file %s", filepath.Base(filePath))
    
    data, err := os.ReadFile(filePath)
    if err != nil {
        return fmt.Errorf("failed to read file: %w", err)
    }

    fileInfo := FileInfo{
        Name: filepath.Base(filePath),
        Size: int64(len(data)),
        Hash: fmt.Sprintf("%x", sha256.Sum256(data)),
    }
    log.Printf("File info - Name: %s, Size: %d, Hash: %s", 
        fileInfo.Name, fileInfo.Size, fileInfo.Hash)


    mn.filesLock.Lock()
    mn.files[fileInfo.Hash] = fileInfo
    mn.filesLock.Unlock()

    mn.peersLock.RLock()
    defer mn.peersLock.RUnlock()

    var wg sync.WaitGroup
    for peerID := range mn.peers {
        wg.Add(1)
        go func(p peer.ID) {
            defer wg.Done()
            
            ctx, cancel := context.WithTimeout(mn.ctx, 30*time.Second)
            defer cancel()
            
            log.Printf("Opening stream to %s", p)
            s, err := mn.host.NewStream(ctx, p, "/shadspace/file/1.0.0")
            if err != nil {
                log.Printf("Failed to create stream to %s: %v", p, err)
                return
            }
            defer s.Close()

            // Set deadline for the entire transfer
            s.SetDeadline(time.Now().Add(30 * time.Second))

            log.Printf("Sending file info to %s", p)
            if err := gob.NewEncoder(s).Encode(fileInfo); err != nil {
                log.Printf("Failed to send file info to %s: %v", p, err)
                return
            }

            log.Printf("Sending file data to %s", p)
            file, err := os.Open(filePath)
            if err != nil {
                log.Printf("Failed to open file: %v", err)
                return
            }
            defer file.Close()

            if _, err := io.Copy(s, file); err != nil {
                log.Printf("Failed to send data to %s: %v", p, err)
                return
            }

            log.Printf("Successfully sent %s to %s", fileInfo.Name, p)
        }(peerID)
    }
    wg.Wait()
	log.Printf("Broadcast completed for %s to %d peers", 
        fileInfo.Name, len(mn.peers))

    return nil
}

func (mn *MasterNode) handleClientStream(s network.Stream) {
    defer s.Close()
    peerID := s.Conn().RemotePeer()
    log.Printf("New file upload from client %s", peerID)

    scanner := bufio.NewScanner(s)
    if !scanner.Scan() {
        log.Printf("Failed to read file name from %s", peerID)
        return
    }
    fileName := scanner.Text()
    log.Printf("Receiving file %s from client %s", fileName, peerID)

    tempFile, err := os.CreateTemp("", "shadspace-")
    if err != nil {
        log.Printf("Failed to create temp file: %v", err)
        return
    }
    defer os.Remove(tempFile.Name())
    defer tempFile.Close()

    bytesReceived, err := io.Copy(tempFile, s)
    if err != nil {
        log.Printf("Failed to receive file data: %v", err)
        return
    }
    log.Printf("Received %d bytes for file %s", bytesReceived, fileName)

    if err := mn.BroadcastFile(tempFile.Name()); err != nil {
        log.Printf("Failed to broadcast file: %v", err)
        return
    }

    log.Printf("Successfully processed and broadcasted file %s", fileName)
}

func (mn *MasterNode) Start() {
	mn.host.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(n network.Network, c network.Conn) {
			peerID := c.RemotePeer()
			remoteAddr := c.RemoteMultiaddr()
			log.Printf("Connected to peer %s at %s\n", peerID, remoteAddr)
			
			mn.peersLock.Lock()
			defer mn.peersLock.Unlock()
			mn.peers[peerID] = peer.AddrInfo{
				ID:    peerID,
				Addrs: []multiaddr.Multiaddr{remoteAddr},
			}
		},
		DisconnectedF: func(n network.Network, c network.Conn) {
			peerID := c.RemotePeer()
			log.Printf("Disconnected from peer %s\n", peerID)
			
			mn.peersLock.Lock()
			defer mn.peersLock.Unlock()
			delete(mn.peers, peerID)
		},
	})

	mn.host.SetStreamHandler("/shadspace/1.0.0", func(s network.Stream) {
		log.Println("New control stream from:", s.Conn().RemotePeer())
		s.Close()
	})
	mn.host.SetStreamHandler("/shadspace/client/1.0.0", mn.handleClientStream)

	fmt.Println("Master node started with ID:", mn.host.ID())
	fmt.Println("Listening on:")
	for _, addr := range mn.host.Addrs() {
		fmt.Println("  ", addr, "/p2p/", mn.host.ID())
	}

	go mn.logConnectedPeers()

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Shutting down...")
	mn.host.Close()
}

func (mn *MasterNode) logConnectedPeers() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-ticker.C:
            mn.peersLock.RLock()
            mn.filesLock.RLock()
            
            log.Println("\n=== Current Node Status ===")
            log.Printf("Connected peers (%d):", len(mn.peers))
            for peerID := range mn.peers {
                log.Printf("- %s", peerID)
            }
            
            log.Printf("Stored files (%d):", len(mn.files))
            for _, file := range mn.files {
                log.Printf("- %s (%d bytes)", file.Name, file.Size)
            }
            
            mn.filesLock.RUnlock()
            mn.peersLock.RUnlock()
            log.Println("=========================\n")
            
        case <-mn.ctx.Done():
            return
        }
    }
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	const keyPath = "master_node.key"
	mn, err := NewMasterNode(ctx, keyPath)
	if err != nil {
		log.Fatalf("Failed to create master node: %v", err)
	}

	mn.Start()
}