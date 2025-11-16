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

	log.Println("Starting Shadspace Master Node")
    log.Printf("Configuration loaded: %+v", cfg)

	if err := masterNode.Start(); err != nil {
		log.Fatalf("Failed to start master node: %v", err)
	}

	// Start admin API with error handling
	log.Printf("Starting API server on %s", cfg.API.ListenAddr)

	// Start API server in a goroutine but capture errors
	apiErr := make(chan error, 1)
	go func() {
		log.Printf("🌍 API server starting on: %s", cfg.API.ListenAddr)
		if err := masterNode.ServeAPI(cfg.API.ListenAddr); err != nil {
			log.Printf("❌ API server failed: %v", err)
			apiErr <- err
		}
	}()

	// Give the API server a moment to start and check for immediate errors
	select {
	case err := <-apiErr:
		log.Fatalf("API server failed to start: %v", err)
	case <-time.After(2 * time.Second):
		log.Printf("✅ API server started successfully on %s", cfg.API.ListenAddr)
		log.Printf("🔗 Test with: curl http://localhost%s/health", cfg.API.ListenAddr)
	}

	// Test the API server immediately
	go func() {
		time.Sleep(3 * time.Second)
		log.Printf("🏁 Master node fully operational")
		log.Printf("📊 Dashboard: http://localhost%s/dashboard", cfg.API.ListenAddr)
		log.Printf("❤️  Health: http://localhost%s/health", cfg.API.ListenAddr)
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	
	select {
	case <-sigCh:
		log.Println("Shutting down master node...")
	case err := <-apiErr:
		log.Printf("API server error: %v", err)
	}

	masterNode.Stop()
	time.Sleep(1 * time.Second)
	log.Println("Master node shutdown complete")
}