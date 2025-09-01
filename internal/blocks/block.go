package blocks

import (
    "crypto/sha256"
    "time"
    "encoding/json"
)

type MetricRecord struct {
    NodeID      string                 `json:"node_id"`
    Timestamp   int64                  `json:"timestamp"`
    UptimeSecs  uint64                 `json:"uptime_secs"`
    CPUBusy     float64                `json:"cpu_busy"`
    MemUsedPerc float64                `json:"mem_used_perc"`
    DiskUsedPerc float64               `json:"disk_used_perc"`
    StorageUsedGB float64              `json:"storage_used_gb"`
    Extra       map[string]interface{} `json:"extra,omitempty"`
}

type BlockHeader struct {
    Version     uint32 `json:"version"`
    PrevHash    []byte `json:"prev_hash"`
    BlockNumber uint64 `json:"block_number"`
    Timestamp   int64  `json:"timestamp"`
    MerkleRoot  []byte `json:"merkle_root"`
    Producer    []byte `json:"producer"`  // producer public key or peer.ID bytes
}

type Block struct {
    Header    BlockHeader    `json:"header"`
    Records   []MetricRecord `json:"records"`
    Signature []byte         `json:"signature"` // signature over Header
}

// Canonical hash of header
func (h *BlockHeader) Hash() []byte {
    b, _ := json.Marshal(h) // canonical JSON is ok; for production use deterministic encoding
    sum := sha256.Sum256(b)
    return sum[:]
}

// BlockID
func (b *Block) ID() []byte {
    return b.Header.Hash()
}
