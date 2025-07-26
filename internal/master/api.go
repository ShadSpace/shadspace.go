package master

import (
	"net/http"
	"time"
	"log"
	
	"github.com/gin-gonic/gin"
)

func (c *Coordinator) ServeAPI(addr string) error {
	router := gin.Default()
	
	router.GET("/status", c.handleStatus)
	router.GET("/files/:hash", c.handleGetFile)
	router.POST("/files", c.handleUploadFile)
	
	return router.Run(addr)
}

func (c *Coordinator) handleStatus(ctx *gin.Context) {
	peers := c.network.GetPeers()
    peerCount := len(peers)

	status := gin.H{
		"peers":    c.network.GetPeerCount(),
		"files":    c.registry.FileCount(),
		"uptime":   time.Since(c.startTime).String(),
		"replicas": c.replicator.GetStats(),
	}
	log.Printf("Current peers: %d (%+v)", peerCount, peers) // Debug logging
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
	// TODO: Implement file upload logic here
	ctx.JSON(http.StatusNotImplemented, gin.H{"error": "not implemented"})
}