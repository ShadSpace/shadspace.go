package farmer

import (
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"log"
	"fmt"

	"github.com/lestonEth/shadspace/internal/core"
)

type StorageManager struct {
	cfg      StorageConfig
	dataDir  string
	mu       sync.RWMutex
	chunks   map[string]*core.FileMetadata
	capacity uint64
	used     uint64
	reserved uint64
}

func NewStorageManager(cfg StorageConfig) (*StorageManager, error) {
	if err := os.MkdirAll(cfg.DataDir, 0755); err != nil {
		return nil, err
	}

	// Get actual disk space
	diskFree, err := getDiskFreeSpace(cfg.DataDir)
	if err != nil {
		return nil, fmt.Errorf("failed to get disk space: %w", err)
	}

	// Calculate effective capacity (minimum of configured and actual available space)
	configuredCapacity := uint64(cfg.MaxCapacityGB) << 30
	reservedSpace := uint64(cfg.ReservedSpaceGB) << 30
	effectiveCapacity := min(configuredCapacity, diskFree - reservedSpace)

	if effectiveCapacity <= 0 {
		return nil, fmt.Errorf("insufficient disk space (available: %.2fGB, reserved: %.2fGB)", 
			float64(diskFree)/(1<<30), float64(reservedSpace)/(1<<30))
	}

	log.Printf("Storage initialized - Capacity: %.2fGB, Reserved: %.2fGB, Available: %.2fGB",
		float64(configuredCapacity)/(1<<30),
		float64(reservedSpace)/(1<<30),
		float64(effectiveCapacity)/(1<<30))


	return &StorageManager{
		cfg:     cfg,
		dataDir: cfg.DataDir,
		chunks:  make(map[string]*core.FileMetadata),
		capacity: uint64(cfg.MaxCapacityGB) * 1 << 30,
		used:    0,
	}, nil
}

// getDiskFreeSpace returns available bytes in the filesystem
func getDiskFreeSpace(path string) (uint64, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, err
	}
	return stat.Bavail * uint64(stat.Bsize), nil
}

func (s *StorageManager) StoreChunk(hash string, data []byte, meta *core.FileMetadata) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	dataSize := uint64(len(data))

	// Check available space
	if s.used+dataSize > s.capacity {
		// Check actual disk space in case something changed
		diskFree, err := getDiskFreeSpace(s.dataDir)
		if err != nil {
			return fmt.Errorf("failed to check disk space: %w", err)
		}

		if diskFree < s.reserved+dataSize {
			log.Printf("CRITICAL: Insufficient disk space (needed: %.2fGB, available: %.2fGB, reserved: %.2fGB)",
				float64(dataSize)/(1<<30),
				float64(diskFree)/(1<<30),
				float64(s.reserved)/(1<<30))
			return core.ErrStorageFull
		}

		// If we got here, we can expand our capacity
		newCapacity := diskFree - s.reserved
		log.Printf("Adjusting storage capacity from %.2fGB to %.2fGB",
			float64(s.capacity)/(1<<30),
			float64(newCapacity)/(1<<30))
		s.capacity = newCapacity
	}

	path := filepath.Join(s.dataDir, hash)
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}

	s.chunks[hash] = meta
	s.used += dataSize
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

	diskFree, _ := getDiskFreeSpace(s.dataDir)
	totalDisk := diskFree + s.used  // Approximate total disk space

	return map[string]interface{}{
		"chunks":          len(s.chunks),
		"usedGB":          float64(s.used) / (1 << 30),
		"allocatedGB":     float64(s.capacity) / (1 << 30),
		"availableGB":     float64(s.capacity-s.used) / (1 << 30),
		"diskFreeGB":      float64(diskFree) / (1 << 30),
		"diskTotalGB":     float64(totalDisk) / (1 << 30),
		"reservedGB":      float64(s.reserved) / (1 << 30),
		"utilization":     float64(s.used) / float64(s.capacity) * 100,
	}
}


func (s *StorageManager) Close() {
	// Cleanup if needed
}

func min(a, b uint64) uint64 {
	if a < b {
		return a
	}
	return b
}
