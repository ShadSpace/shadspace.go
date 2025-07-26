package farmer

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/lestonEth/shadspace/internal/core"
)

type StorageManager struct {
	cfg      StorageConfig
	dataDir  string
	mu       sync.RWMutex
	chunks   map[string]*core.FileMetadata
	capacity uint64
	used     uint64
}

func NewStorageManager(cfg StorageConfig) (*StorageManager, error) {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, err
	}

	return &StorageManager{
		cfg:     cfg,
		dataDir: cfg.DataDir,
		chunks:  make(map[string]*core.FileMetadata),
		capacity: uint64(cfg.MaxCapacityGB) * 1 << 30,
		used:    0,
	}, nil
}

func (s *StorageManager) StoreChunk(hash string, data []byte, meta *core.FileMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check available space
	if s.used+uint64(len(data)) > s.capacity {
		return core.ErrStorageFull
	}

	path := filepath.Join(s.dataDir, hash)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return err
	}

	s.chunks[hash] = meta
	s.used += uint64(len(data))
	return nil
}

func (s *StorageManager) RetrieveChunk(hash string) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	path := filepath.Join(s.dataDir, hash)
	return os.ReadFile(path)
}

func (s *StorageManager) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return map[string]interface{}{
		"chunks":    len(s.chunks),
		"usedGB":    float64(s.used) / (1 << 30),
		"totalGB":   float64(s.capacity) / (1 << 30),
		"available": float64(s.capacity-s.used) / (1 << 30),
	}
}

func (s *StorageManager) Close() {
	// Cleanup if needed
}