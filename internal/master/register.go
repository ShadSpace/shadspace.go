package master

import (
	"sync"
	"time"

	"github.com/yourorg/shadspace/internal/core"
)

type FileRegistry struct {
	mu     sync.RWMutex
	files  map[string]*core.FileMetadata  // Key: ContentHash
	peers  map[string][]string           // Key: ContentHash, Value: List of peerIDs
}

type FileMetadata struct {
	Name         string
	Size         int64
	Hash         string
	Timestamp    time.Time
	Replication  int
	ProofType    string  // ZK proof type if applicable
}

func NewFileRegistry() *FileRegistry {
	return &FileRegistry{
		files: make(map[string]*core.FileMetadata),
		peers: make(map[string][]string),
	}
}

func (r *FileRegistry) RegisterFile(meta core.FileMetadata) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	r.files[meta.Hash] = &meta
}

func (r *FileRegistry) AddFileLocation(hash string, peerID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	
	if _, exists := r.files[hash]; exists {
		r.peers[hash] = append(r.peers[hash], peerID)
	}
}

func (r *FileRegistry) GetFile(hash string) (*core.FileMetadata, []string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	
	meta, ok1 := r.files[hash]
	peers, ok2 := r.peers[hash]
	
	return meta, peers, ok1 && ok2
}