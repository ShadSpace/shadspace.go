package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lestonEth/shadspace/internal/farmer"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := farmer.LoadConfig("configs/farmer.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	farmerNode, err := farmer.NewFarmerNode(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to create farmer node: %v", err)
	}

	log.Println("Starting Shadspace Farmer Node")
	log.Printf("Configuration loaded: %+v", cfg)

	if err := farmerNode.Start(); err != nil {
		log.Fatalf("Failed to start farmer node: %v", err)
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down farmer node...")
	farmerNode.Stop()
	time.Sleep(1 * time.Second)
}