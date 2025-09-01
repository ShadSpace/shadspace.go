// farmer/metrics.go
package farmer

import (
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"time"
)

type NodeMetrics struct {
	Timestamp   time.Time
	CPU         CPUStats
	Memory      MemoryStats
	Disk        DiskStats
	Network     NetworkStats
	Load        LoadStats
	Storage     StorageStats
	Uptime      string `json:"uptime"`
}

type CPUStats struct {
	Busy float64 `json:"busy"`
}

type MemoryStats struct {
	Total     uint64  `json:"total"`
	Used      uint64  `json:"used"`
	UsedPerc  float64 `json:"used_perc"`
	SwapUsed  uint64  `json:"swap_used"`
	SwapTotal uint64  `json:"swap_total"`
}

type DiskStats struct {
	Total uint64  `json:"total"`
	Used  uint64  `json:"used"`
	Free  uint64  `json:"free"`
	UsedPerc float64 `json:"used_perc"`
}

type NetworkStats struct {
	BytesSent uint64 `json:"bytes_sent"`
	BytesRecv uint64 `json:"bytes_recv"`
}

type LoadStats struct {
	Load1  float64 `json:"load1"`
	Load5  float64 `json:"load5"`
	Load15 float64 `json:"load15"`
}

type StorageStats struct {
	Chunks     int     `json:"chunks"`
	UsedGB     float64 `json:"used_gb"`
	TotalGB    float64 `json:"total_gb"`
	UsedPerc   float64 `json:"used_perc"`
}

func (f *FarmerNode) GatherMetrics() (*NodeMetrics, error) {
	// CPU
	cpuPercent, _ := cpu.Percent(time.Second, false)
	loadAvg, _ := load.Avg()

	// Memory
	vmem, _ := mem.VirtualMemory()
	swap, _ := mem.SwapMemory()

	// Disk
	diskUsage, _ := disk.Usage("/")

	// Network
	netIO, _ := net.IOCounters(false)

	// Storage
	storageStats := f.storage.GetStats()

	return &NodeMetrics{
		Timestamp: time.Now(),
		CPU:       CPUStats{Busy: cpuPercent[0]},
		Load:      LoadStats{
			Load1:  loadAvg.Load1,
			Load5:  loadAvg.Load5,
			Load15: loadAvg.Load15,
		},
		Memory: MemoryStats{
			Total:    vmem.Total,
			Used:     vmem.Used,
			UsedPerc: vmem.UsedPercent,
			SwapUsed: swap.Used,
			SwapTotal: swap.Total,
		},
		Disk: DiskStats{
			Total:   diskUsage.Total,
			Used:    diskUsage.Used,
			Free:    diskUsage.Free,
			UsedPerc: diskUsage.UsedPercent,
		},
		Network: NetworkStats{
			BytesSent: netIO[0].BytesSent,
			BytesRecv: netIO[0].BytesRecv,
		},
		Storage: StorageStats{
			Chunks:   storageStats["chunks"].(int),
			UsedGB:   storageStats["usedGB"].(float64),
			TotalGB:  storageStats["allocatedGB"].(float64),
			UsedPerc: storageStats["utilization"].(float64),
		},
		Uptime: time.Since(f.startTime).String(), 
	}, nil
}