package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lestonEth/shadspace/internal/master"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := master.LoadConfig("configs/master.yaml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	masterNode, err := master.NewCoordinator(ctx, cfg)
	if err != nil {
		log.Fatalf("Failed to create master node: %v", err)
	}

	if err := masterNode.Start(); err != nil {
		log.Fatalf("Failed to start master node: %v", err)
	}

	// Start admin API
	go masterNode.ServeAPI(cfg.API.ListenAddr)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down master node...")
	masterNode.Stop()
	time.Sleep(1 * time.Second)
}