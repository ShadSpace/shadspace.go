package core

import "time"

type FileMetadata struct {
	Name        string
	Size        int64
	Hash        string
	CreatedAt   time.Time
	ProofType   string  // "groth16", "plonk", etc.
	IsModel     bool    // Whether this is an AI model
}

type StorageProof struct {
	Challenge   string
	Response    string
	Timestamp   time.Time
}

type ReplicationRequest struct {
	FileHash    string
	TargetPeers []string
	Priority    int
}