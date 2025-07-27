package master

import (
	"net/http"
	"time"
	"log"
	"io"
	"crypto/sha256"
	"encoding/hex"
	"bytes"
	"encoding/gob"
	"fmt"
	"sync"
	"context"
	
	"github.com/gin-gonic/gin"
	"github.com/klauspost/reedsolomon"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lestonEth/shadspace/internal/core"
	"github.com/gin-contrib/cors"

)

func (c *Coordinator) ServeAPI(addr string) error {
	router := gin.Default()

	// Configure CORS
	router.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization"},
		ExposeHeaders:    []string{"Content-Length"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	router.GET("/status", c.handleStatus)
	router.GET("/files/:hash", c.handleGetFile)
	router.POST("/files", c.handleUploadFile)
	router.GET("/reconstruct/:hash", c.handleReconstructFile)

	return router.Run(addr)
}

func corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Credentials", "true")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type, Content-Length, Accept-Encoding, X-CSRF-Token, Authorization, accept, origin, Cache-Control, X-Requested-With")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS, GET, PUT, DELETE")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}


func (c *Coordinator) handleStatus(ctx *gin.Context) {
	peers := c.network.GetPeers()
	peerCount := len(peers)

	status := gin.H{
		"peers":    peerCount,
		"files":    c.registry.FileCount(),
		"uptime":   time.Since(c.startTime).String(),
		"replicas": c.replicator.GetStats(),
	}
	log.Printf("Current peers: %d (%+v)", peerCount, peers)
	ctx.JSON(http.StatusOK, status)
}

func (c *Coordinator) handleGetFile(ctx *gin.Context) {
	hash := ctx.Param("hash")
	meta, peers, exists := c.registry.GetFile(hash)
	if !exists {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}
	
	ctx.JSON(http.StatusOK, gin.H{
		"metadata": meta,
		"peers":    peers,
	})
}

func (c *Coordinator) handleUploadFile(ctx *gin.Context) {
	file, header, err := ctx.Request.FormFile("file")
	if err != nil {
		ctx.JSON(http.StatusBadRequest, gin.H{"error": "file upload error"})
		return
	}
	defer file.Close()

	fileData, err := io.ReadAll(file)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": "file read error"})
		return
	}

	hash := sha256.Sum256(fileData)
	hashStr := hex.EncodeToString(hash[:])
	meta := core.FileMetadata{
		Name:      header.Filename,
		Size:      int64(len(fileData)),
		Hash:      hashStr,
		CreatedAt: time.Now(),
	}

	log.Printf("Uploading file: %+v", meta)

	storageNodes := c.selectStorageNodes()
	log.Printf("Selected storage nodes: %v", storageNodes)
	if len(storageNodes) < c.cfg.Replication.MinReplicas {
		ctx.JSON(http.StatusServiceUnavailable, gin.H{"error": "not enough storage nodes available"})
		return
	}

	err = c.distributeWithErasureCoding(meta, fileData, storageNodes)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.JSON(http.StatusOK, gin.H{
		"hash":    hashStr,
		"size":    meta.Size,
		"nodes":   len(storageNodes),
		"message": "file successfully distributed",
	})
}

func (c *Coordinator) handleReconstructFile(ctx *gin.Context) {
	hash := ctx.Param("hash")
	meta, peerIDs, exists := c.registry.GetFile(hash)
	if !exists {
		ctx.JSON(http.StatusNotFound, gin.H{"error": "file not found"})
		return
	}

	shards := make([][]byte, len(peerIDs))
	var wg sync.WaitGroup
	var mu sync.Mutex
	var collectErrors []error

	for i, peerID := range peerIDs {
		wg.Add(1)
		go func(idx int, pid peer.ID) {
			defer wg.Done()
			
			shard, err := c.retrieveShardFromNode(hash, pid)
			if err != nil {
				mu.Lock()
				collectErrors = append(collectErrors, err)
				mu.Unlock()
				return
			}
			
			mu.Lock()
			shards[idx] = shard
			mu.Unlock()
		}(i, peerID)
	}

	wg.Wait()

	if len(collectErrors) > (len(peerIDs) - (len(peerIDs) / 3)) {
		ctx.JSON(http.StatusInternalServerError, gin.H{
			"error": "not enough shards available for reconstruction",
			"missing": len(collectErrors),
			"required": len(peerIDs) / 3,
		})
		return
	}

	dataShards := len(peerIDs) - (len(peerIDs) / 3)
	enc, err := reedsolomon.New(dataShards, len(peerIDs)-dataShards)
	if err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if err := enc.Reconstruct(shards); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	var buf bytes.Buffer
	if err := enc.Join(&buf, shards, int(meta.Size)); err != nil {
		ctx.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	ctx.Data(http.StatusOK, "application/octet-stream", buf.Bytes())
}

func (c *Coordinator) retrieveShardFromNode(hash string, nodeID peer.ID) ([]byte, error) {
	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	stream, err := c.network.Host().NewStream(ctx, nodeID, "/shadspace/retrieve/1.0.0")
	if err != nil {
		return nil, fmt.Errorf("failed to open stream: %w", err)
	}
	defer stream.Close()

	enc := gob.NewEncoder(stream)
	if err := enc.Encode(hash); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	shard, err := io.ReadAll(stream)
	if err != nil {
		return nil, fmt.Errorf("failed to read shard: %w", err)
	}

	return shard, nil
}