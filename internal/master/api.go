package master

import (
	"net/http"
	
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
	status := gin.H{
		"peers":    c.network.GetPeerCount(),
		"files":    c.registry.FileCount(),
		"uptime":   time.Since(c.startTime).String(),
		"replicas": c.replicator.GetStats(),
	}
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