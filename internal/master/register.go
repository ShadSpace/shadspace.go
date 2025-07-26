package master

import (
	"sync"
	
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/lestonEth/shadspace/internal/core"
)

type FileRegistry struct {
	mu     sync.RWMutex
	files  map[string]*core.FileMetadata
	peers  map[string][]peer.ID
}

func NewFileRegistry() *FileRegistry {
	return &FileRegistry{
		files: make(map[string]*core.FileMetadata),
		peers: make(map[string][]peer.ID),
	}
}

func (r *FileRegistry) RegisterFile(meta core.FileMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.files[meta.Hash] = &meta
}

func (r *FileRegistry) AddFileLocation(hash string, peerID peer.ID) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.peers[hash] = append(r.peers[hash], peerID)
}

func (r *FileRegistry) GetFile(hash string) (*core.FileMetadata, []peer.ID, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	meta, ok1 := r.files[hash]
	peers, ok2 := r.peers[hash]
	return meta, peers, ok1 && ok2
}

func (r *FileRegistry) FileCount() int {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.files)
}