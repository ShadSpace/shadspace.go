# Master Node Architecture

## Overview

The Master Node serves as the central coordinator for the ShadSpace decentralized storage network. It manages file distribution, peer coordination, and provides monitoring capabilities for the entire network.

## 🏗️ Key Components

### 1. Coordinator (Master Node)

The coordinator is the core orchestrator that manages the entire distributed storage network.

**Responsibilities:**
- Manages the file registry and peer network
- Handles file replication across peers
- Provides status monitoring via HTTP API
- Uses context for graceful shutdown

**Key Features:**
- **Peer Management**: Tracks all connected farmer nodes
- **File Distribution**: Coordinates file replication across the network
- **Network Monitoring**: Real-time status of connected peers
- **Graceful Shutdown**: Context-based cancellation for clean termination

### 2. File Registry

A thread-safe registry that tracks file metadata and storage locations across the network.

**Implementation Details:**
- **Thread Safety**: Uses `sync.RWMutex` for concurrent access
- **Metadata Storage**: Maps file hashes to metadata and peer locations
- **File Tracking**: Maintains comprehensive file information including:
  - File name and size
  - SHA-256 hash for integrity verification
  - Storage locations across peers
  - Replication status

**Data Structure:**
```go
type FileInfo struct {
    Name string
    Size int64
    Hash string
}

type MasterNode struct {
    files     map[string]FileInfo
    filesLock sync.RWMutex
    // ... other fields
}
```

### 3. Network Manager

Built on libp2p for robust peer-to-peer communication and discovery.

**Core Functionality:**
- **Peer Discovery**: Automatic discovery of farmer nodes
- **Stream Management**: Handles control and file transfer streams
- **Connection Tracking**: Maintains list of connected peers
- **Protocol Support**: Implements custom protocols for file operations

**Protocols:**
- `/shadspace/client/1.0.0`: Client upload protocol
- `/shadspace/file/1.0.0`: File transfer protocol
- `/shadspace/control/1.0.0`: Control and monitoring protocol

### 4. Replication Manager

Manages file replication strategies and ensures data availability across the network.

**Features:**
- **Replication Strategies**: Configurable replication policies
- **Job Management**: Tracks active replication jobs
- **Replication Factor**: Ensures files meet configured redundancy requirements
- **Health Monitoring**: Monitors replication status across peers

## 🔧 Implementation Details

### Concurrency Safety

The master node implements comprehensive concurrency safety:

```go
type MasterNode struct {
    peers     map[peer.ID]peer.AddrInfo
    files     map[string]FileInfo
    peersLock sync.RWMutex
    filesLock sync.RWMutex
    ctx       context.Context
}
```

- **Read-Write Mutexes**: Separate locks for peers and files
- **Atomic Operations**: Safe concurrent access to shared data
- **Context Cancellation**: Graceful shutdown handling

### File Distribution Flow

1. **Client Upload**: Client connects and uploads file to master
2. **Metadata Registration**: File info stored in registry
3. **Peer Selection**: Master selects available farmer nodes
4. **Distribution**: File distributed to selected peers
5. **Verification**: Hash verification on all receiving nodes
6. **Status Update**: Registry updated with storage locations

### Peer Management

```go
func (mn *MasterNode) handlePeerStream(s network.Stream) {
    peerID := s.Conn().RemotePeer()
    
    // Add peer to registry
    mn.peersLock.Lock()
    mn.peers[peerID] = peer.AddrInfo{ID: peerID}
    mn.peersLock.Unlock()
    
    // Monitor peer health
    go mn.monitorPeer(peerID)
}
```

## 📊 API Endpoints

The master node provides RESTful API endpoints for monitoring and management:

### Status Endpoints
- `GET /status`: Overall network status
- `GET /peers`: List of connected peers
- `GET /files`: File registry information
- `GET /health`: Health check endpoint

### Management Endpoints
- `POST /files/upload`: Manual file upload
- `DELETE /files/{hash}`: Remove file from network
- `POST /peers/disconnect`: Disconnect specific peer

## 🔒 Security Features

### Cryptographic Integrity
- **SHA-256 Verification**: All files verified using cryptographic hashes
- **Key Management**: RSA key pairs for node identification
- **Secure Communication**: libp2p provides encrypted P2P communication

### Access Control
- **Peer Authentication**: Verified peer connections
- **File Integrity**: Hash-based file corruption detection
- **Network Security**: Encrypted stream communication

## 📈 Monitoring & Observability

### Real-time Monitoring
- **Connection Status**: Live peer connection monitoring
- **File Distribution**: Real-time file replication tracking
- **Performance Metrics**: Transfer rates and latency monitoring
- **Error Tracking**: Comprehensive error logging and reporting

### Health Checks
```go
func (mn *MasterNode) logConnectedPeers() {
    ticker := time.NewTicker(30 * time.Second)
    defer ticker.Stop()
    
    for {
        select {
        case <-ticker.C:
            mn.peersLock.RLock()
            peerCount := len(mn.peers)
            mn.peersLock.RUnlock()
            
            log.Printf("Connected peers: %d", peerCount)
        case <-mn.ctx.Done():
            return
        }
    }
}
```

## 🚀 Configuration

### Network Configuration
```go
// Default configuration
const (
    DefaultPort = 53798
    DefaultProtocol = "/shadspace/file/1.0.0"
    DefaultReplicationFactor = 3
)
```

### Key Management
- **Automatic Key Generation**: RSA keys generated on first run
- **Key Persistence**: Keys stored securely on disk
- **Key Rotation**: Support for key rotation and updates

## 🔮 Future Enhancements

### Planned Improvements

1. **Enhanced Error Handling**
   - Detailed error contexts and logging
   - Error recovery mechanisms
   - Circuit breaker patterns

2. **Metrics Integration**
   - Prometheus metrics integration
   - Custom metrics for file operations
   - Performance dashboards

3. **Peer Health Monitoring**
   - Detailed peer health checks
   - Automatic peer recovery
   - Load balancing across peers

4. **Advanced File Transfer**
   - Chunked file transfer
   - Resume capability for large files
   - Bandwidth optimization

5. **Proof Verification**
   - Storage proof implementation
   - Zero-knowledge proof verification
   - Cryptographic attestations

## 🛠️ Development Guidelines

### Code Organization
- **Modular Design**: Clear separation of concerns
- **Interface-based**: Dependency injection for testability
- **Error Handling**: Comprehensive error management
- **Documentation**: Inline code documentation

### Testing Strategy
- **Unit Tests**: Component-level testing
- **Integration Tests**: Network interaction testing
- **Performance Tests**: Load and stress testing
- **Security Tests**: Cryptographic verification testing

## 📚 Related Documentation

- [Farmer Node Documentation](./FARMER.md)
- [Client Documentation](./CLIENT.md)
- [Network Protocol Specification](./PROTOCOL.md)
- [API Reference](./API.md)
