package main

import (
	"context"
	"crypto/sha256"
	"encoding/gob"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
)

type FileInfo struct {
	Name string
	Size int64
	Hash string
}

type FarmerNode struct {
	host       host.Host
	ctx        context.Context
	masterPID  peer.ID
	masterAddr multiaddr.Multiaddr
	storageDir string
}

func NewFarmerNode(ctx context.Context, storageDir string) (*FarmerNode, error) {
	priv, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		return nil, err
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(storageDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create storage dir: %w", err)
	}

	return &FarmerNode{
		host:       h,
		ctx:        ctx,
		storageDir: storageDir,
	}, nil
}

func (fn *FarmerNode) ConnectToBootstrap() error {
	bootstrapAddr := "/ip4/127.0.0.1/tcp/53798/p2p/QmbikEcRNSydft9MWeK19PzaBQEBWnitquRN4wBprRHNDD"
	
	maddr, err := multiaddr.NewMultiaddr(bootstrapAddr)
	if err != nil {
		return fmt.Errorf("failed to parse bootstrap address: %w", err)
	}

	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		return fmt.Errorf("failed to get peer info: %w", err)
	}

	fn.host.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)

	fmt.Printf("Connecting to bootstrap node %s...\n", info.ID)
	if err := fn.host.Connect(fn.ctx, *info); err != nil {
		return fmt.Errorf("failed to connect to bootstrap: %w", err)
	}
	
	// Verify connection
	if fn.host.Network().Connectedness(info.ID) != network.Connected {
		return fmt.Errorf("not actually connected to %s", info.ID)
	}

	fn.masterPID = info.ID
	fn.masterAddr = maddr

	log.Printf("Successfully connected to bootstrap node: %s", info.ID)
	log.Printf("Connection details: %+v", fn.host.Network().ConnsToPeer(info.ID))
	return nil
}

func (fn *FarmerNode) handleFileStream(s network.Stream) {
    peerID := s.Conn().RemotePeer()
    log.Printf("NEW FILE STREAM FROM: %s", peerID)
    defer func() {
        s.Close()
        log.Printf("Closed stream from %s", peerID)
    }()

    // Set strict timeouts
    s.SetReadDeadline(time.Now().Add(30 * time.Second))

    // First read the file metadata
    var fileInfo FileInfo
    if err := gob.NewDecoder(s).Decode(&fileInfo); err != nil {
        log.Printf("DECODE ERROR from %s: %v", peerID, err)
        return
    }
    log.Printf("Receiving file: %s (%d bytes) from %s", 
        fileInfo.Name, fileInfo.Size, peerID)

    // Prepare storage
    if err := os.MkdirAll(fn.storageDir, 0755); err != nil {
        log.Printf("STORAGE ERROR: %v", err)
        return
    }

    filePath := filepath.Join(fn.storageDir, fileInfo.Name)
    f, err := os.Create(filePath)
    if err != nil {
        log.Printf("FILE CREATE ERROR: %v", err)
        return
    }
    defer f.Close()

    // Track progress
    progressTicker := time.NewTicker(2 * time.Second)
    defer progressTicker.Stop()
    go func() {
        for range progressTicker.C {
            stat, _ := f.Stat()
            log.Printf("Receiving %s: %d/%d bytes (%.1f%%)", 
                fileInfo.Name, stat.Size(), fileInfo.Size, 
                float64(stat.Size())/float64(fileInfo.Size)*100)
        }
    }()

    // Stream the file content
    received, err := io.Copy(f, s)
    if err != nil {
        log.Printf("COPY ERROR: %v", err)
        os.Remove(filePath)
        return
    }

    // Verify
    if received != fileInfo.Size {
        log.Printf("SIZE MISMATCH: expected %d, got %d", fileInfo.Size, received)
        os.Remove(filePath)
        return
    }

    data, err := os.ReadFile(filePath)
    if err != nil {
        log.Printf("VERIFY READ ERROR: %v", err)
        os.Remove(filePath)
        return
    }

    hash := fmt.Sprintf("%x", sha256.Sum256(data))
    if hash != fileInfo.Hash {
        log.Printf("HASH MISMATCH: expected %s, got %s", fileInfo.Hash, hash)
        os.Remove(filePath)
        return
    }

    log.Printf("SUCCESS: Stored %s (%d bytes, hash: %s)", 
        fileInfo.Name, received, hash)
}

func (fn *FarmerNode) Start() {
	fn.host.SetStreamHandler("/shadspace/file/1.0.0", fn.handleFileStream)
	log.Println("Registered file stream handler")

	fmt.Println("Farmer node started with ID:", fn.host.ID())
	fmt.Println("Listening on:")
	for _, addr := range fn.host.Addrs() {
		fmt.Println("  ", addr, "/p2p/", fn.host.ID())
	}

	
    log.Println("Storage directory:", fn.storageDir)
    if err := fn.ConnectToBootstrap(); err != nil {
        log.Printf("Bootstrap connection error: %v", err)
    }

    go fn.monitorConnection()
    go fn.logStorageStatus()  // Add this new function

    ch := make(chan os.Signal, 1)
    signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
    <-ch
    fmt.Println("Shutting down...")
    fn.host.Close()
}


func (fn *FarmerNode) logStorageStatus() {
    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            files, err := os.ReadDir(fn.storageDir)
            if err != nil {
                log.Printf("Failed to read storage dir: %v", err)
                continue
            }
            log.Printf("Storage status: %d files in %s", len(files), fn.storageDir)
            for _, file := range files {
                info, _ := file.Info()
                log.Printf("- %s (%d bytes)", file.Name(), info.Size())
            }
        case <-fn.ctx.Done():
            return
        }
    }
}

func (fn *FarmerNode) monitorConnection() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			if fn.masterPID != "" && fn.host.Network().Connectedness(fn.masterPID) != network.Connected {
				log.Println("Lost connection to bootstrap, reconnecting...")
				if err := fn.ConnectToBootstrap(); err != nil {
					log.Printf("Reconnection failed: %v", err)
				}
			}
		case <-fn.ctx.Done():
			return
		}
	}
}
 
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	storageDir := "./farmer-storage"
	if len(os.Args) > 1 {
		storageDir = os.Args[1]
	}

	farmer, err := NewFarmerNode(ctx, storageDir)
	if err != nil {
		log.Fatalf("Failed to create farmer node: %v", err)
	}

	farmer.Start()
}