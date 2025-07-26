package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/multiformats/go-multiaddr"
)

func main() {
	masterAddr := flag.String("master", "", "Master node multiaddress")
	filePath := flag.String("file", "", "File to distribute")
	flag.Parse()

	if *masterAddr == "" || *filePath == "" {
		fmt.Println("Usage: ./client -master <master-addr> -file <file-path>")
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	priv, _, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
	if err != nil {
		log.Fatalf("Failed to generate key pair: %v", err)
	}

	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
	)
	if err != nil {
		log.Fatalf("Failed to create host: %v", err)
	}

	maddr, err := multiaddr.NewMultiaddr(*masterAddr)
	if err != nil {
		log.Fatalf("Failed to parse master address: %v", err)
	}

	info, err := peer.AddrInfoFromP2pAddr(maddr)
	if err != nil {
		log.Fatalf("Failed to get peer info: %v", err)
	}

	h.Peerstore().AddAddrs(info.ID, info.Addrs, peerstore.PermanentAddrTTL)
	if err := h.Connect(ctx, *info); err != nil {
		log.Fatalf("Failed to connect to master: %v", err)
	}

	fmt.Printf("Connected to master node %s\n", info.ID)

	s, err := h.NewStream(ctx, info.ID, "/shadspace/client/1.0.0")
	if err != nil {
		log.Fatalf("Failed to create stream: %v", err)
	}
	defer s.Close()

	// Open the file to send
	file, err := os.Open(*filePath)
	if err != nil {
		log.Fatalf("Failed to open file: %v", err)
	}
	defer file.Close()

	fileName := filepath.Base(*filePath)
	if _, err := fmt.Fprintf(s, "%s\n", fileName); err != nil {
		log.Fatalf("Failed to send file name: %v", err)
	}

	if _, err := io.Copy(s, file); err != nil {
		log.Fatalf("Failed to send file data: %v", err)
	}

	fmt.Printf("File %s successfully sent to master for distribution\n", fileName)

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM)
	<-ch
	fmt.Println("Shutting down...")
	h.Close()
}