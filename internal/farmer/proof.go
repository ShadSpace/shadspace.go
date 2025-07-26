// internal/farmer/proof.go
type ProofHandler struct {
    verifier   zk.Verifier
    cache      ProofCache
    validator  ProofValidator
}
