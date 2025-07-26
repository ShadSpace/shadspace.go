package core

import (
	"context"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
)

// BaseNode represents common functionality for all node types
type BaseNode struct {
	Host      host.Host
	Ctx       context.Context
	StartTime time.Time
}

// NewBaseNode creates a new BaseNode instance
func NewBaseNode(ctx context.Context, h host.Host) *BaseNode {
	return &BaseNode{
		Host:      h,
		Ctx:       ctx,
		StartTime: time.Now(),
	}
}

// IsAlive checks if the node is still running
func (n *BaseNode) IsAlive() bool {
	select {
	case <-n.Ctx.Done():
		return false
	default:
		return true
	}
}

// Uptime returns how long the node has been running
func (n *BaseNode) Uptime() time.Duration {
	return time.Since(n.StartTime)
}