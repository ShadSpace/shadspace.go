// internal/farmer/storage.go
type StorageEngine struct {
    proofStorage   ProofStorage
    modelStorage   ModelStorage
    challengeSys   ChallengeSystem
    replicationAPI ReplicationAPI
}