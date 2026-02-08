# p2pquic-go

This is a peer-to-peer QUIC library in Go that enables direct connections between peers behind NAT using UDP hole-punching and QUIC transport. It provides both a reusable library and command-line tools for testing.

## Features

- **NAT Traversal**: UDP hole-punching with STUN support
- **QUIC Transport**: Reliable, encrypted peer-to-peer connections
- **Simple API**: Easy-to-use library for building P2P applications
- **Signaling Server**: Coordinate peer discovery and connection setup
- **Testing Tools**: Command-line utilities for testing peer connections

## Installation

```bash
go get github.com/mevdschee/p2pquic-go
```

## Project Structure

```
p2pquic-go/
├── pkg/
│   ├── p2pquic/          # Core P2P QUIC library
│   │   ├── p2pquic.go    # Main peer implementation
│   │   ├── signaling.go  # Signaling client
│   │   └── types.go      # Data structures
│   └── signaling/        # Decoupled signaling server (transport-agnostic)
│       └── server.go     # Peer registry logic
├── cmd/
│   ├── p2pquic-test/     # Peer testing tool
│   └── p2pquic-signal/   # HTTP signaling server
└── examples/
    └── simple/           # Basic usage example
```

**Key Packages:**
- **`pkg/p2pquic`** - Reusable library for P2P QUIC connections with NAT traversal
- **`pkg/signaling`** - Transport-agnostic signaling server (not coupled to HTTP)
- **`cmd/p2pquic-test`** - Command-line tool for testing peer connections
- **`cmd/p2pquic-signal`** - HTTP wrapper around the signaling package

## Library Usage

### Basic Example

```go
package main

import (
    "context"
    "log"
    
    "github.com/mevdschee/p2pquic-go/pkg/p2pquic"
)

func main() {
    // Create a peer
    config := p2pquic.Config{
        PeerID:       "my-peer",
        LocalPort:    9000,
        SignalingURL: "http://signaling-server:8080",
        EnableSTUN:   true,
    }
    
    peer, err := p2pquic.NewPeer(config)
    if err != nil {
        log.Fatal(err)
    }
    defer peer.Close()
    
    // Discover NAT candidates
    candidates, err := peer.DiscoverCandidates()
    if err != nil {
        log.Fatal(err)
    }
    
    // Register with signaling server
    if err := peer.Register(); err != nil {
        log.Fatal(err)
    }
    
    // Server mode: listen for connections
    if err := peer.Listen(); err != nil {
        log.Fatal(err)
    }
    
    conn, err := peer.Accept(context.Background())
    if err != nil {
        log.Fatal(err)
    }
    
    // Use the QUIC connection...
}
```

### Client Mode

```go
// Connect to a remote peer (candidates fetched from signaling server)
conn, err := peer.Connect("remote-peer-id")
if err != nil {
    log.Fatal(err)
}
defer conn.CloseWithError(0, "done")

// Or provide candidates directly:
conn, err := peer.Connect("remote-peer-id",
    p2pquic.WithCandidates(
        p2pquic.Candidate{IP: "192.168.1.100", Port: 9000},
        p2pquic.Candidate{IP: "1.2.3.4", Port: 9000},
    ),
)

// Open a stream and communicate
stream, err := conn.OpenStreamSync(context.Background())
if err != nil {
    log.Fatal(err)
}
defer stream.Close()

// Write and read data
stream.Write([]byte("Hello!"))
buf := make([]byte, 1024)
n, _ := stream.Read(buf)
log.Printf("Received: %s", buf[:n])
```

## Command-Line Tools

### Signaling Server

The signaling server is available both as a standalone HTTP server and as a reusable package.

#### As a Command-Line Tool

Start the HTTP signaling server on a publicly accessible machine:

```bash
# Build
go build ./cmd/p2pquic-signal

# Run
./p2pquic-signal -port 8080
```

#### As a Library

The `pkg/signaling` package is **transport-agnostic** and can be used with any transport layer (HTTP, gRPC, WebSocket, etc.):

```go
import "github.com/mevdschee/p2pquic-go/pkg/signaling"

// Create signaling server
server := signaling.NewServer()

// Register a peer
server.Register("peer-id", []signaling.Candidate{
    {IP: "192.168.1.100", Port: 9000},
})

// Get peer info
peer, exists := server.GetPeer("peer-id")

// List all peers
peers := server.GetAllPeers()
```

The HTTP server in `cmd/p2pquic-signal` is just a thin wrapper around this package.

### Peer Testing Tool

Test peer-to-peer connections:

**Server Mode:**

```bash
# Build
go build ./cmd/p2pquic-test

# Run server peer (defaults to ID "server")
./p2pquic-test -mode server -signaling http://localhost:8080
```

**Client Mode:**

```bash
# Run client peer connecting to "server" (defaults to ID "client")
./p2pquic-test -mode client -signaling http://localhost:8080
```

**Flags:**
- `-mode`: Operation mode (default: `server`)
- `-id`: Unique peer identifier (default: same as mode)
- `-remote`: Remote peer ID to connect to, client mode only (default: `server`)
- `-port`: Local UDP port to bind to (default: `0`, auto-assign)
- `-signaling`: Signaling server URL (default: `http://localhost:8080`)
- `-stun`: Enable STUN for public IP discovery (default: `true`)

## How It Works

1. **Candidate Discovery**: Each peer discovers its network candidates using STUN (public IP) and local network interfaces
2. **Signaling**: Peers register their candidates with a central signaling server
3. **UDP Hole-Punching**: Client sends UDP packets to all server candidates to "punch holes" in NATs
4. **QUIC Connection**: After hole-punching, a QUIC connection is established directly between peers

## Architecture

```
┌─────────────┐         ┌──────────────────┐         ┌─────────────┐
│   Peer A    │         │ Signaling Server │         │   Peer B    │
│ (Behind NAT)│         │  (Public Server) │         │ (Behind NAT)│
└──────┬──────┘         └────────┬─────────┘         └──────┬──────┘
       │                         │                          │
       │  1. Register candidates │                          │
       ├────────────────────────►│                          │
       │                         │  2. Register candidates  │
       │                         │◄─────────────────────────┤
       │                         │                          │
       │  3. Get peer B info     │                          │
       ├────────────────────────►│                          │
       │                         │                          │
       │  4. UDP hole-punch packets                         │
       ├───────────────────────────────────────────────────►│
       │◄───────────────────────────────────────────────────┤
       │                         │                          │
       │  5. QUIC connection established                    │
       ├═══════════════════════════════════════════════════►│
```

## API Reference

### `Config`

Configuration for creating a peer:

```go
type Config struct {
    PeerID       string  // Unique peer identifier
    LocalPort    int     // UDP port to bind to
    SignalingURL string  // Signaling server URL
    EnableSTUN   bool    // Enable STUN discovery
}
```

### `Peer`

Main peer interface:

- `NewPeer(config Config) (*Peer, error)` - Create a new peer
- `DiscoverCandidates() ([]Candidate, error)` - Discover NAT candidates (run after `Listen` or `Bind`)
- `Register() error` - Register with signaling server
- `Listen() error` - Start listening for incoming connections
- `Bind() error` - Bind to a specific port
- `Accept(ctx context.Context) (*quic.Conn, error)` - Accept incoming connection
- `Connect(remotePeerID string, opts ...ConnectOption) (*quic.Conn, error)` - Connect to remote peer
- `ContinuousHolePunch(ctx context.Context)` - Continuously punch holes to discovered peers
- `Close() error` - Close peer and release resources

### `ConnectOption`

Functional options for customizing connection behavior:

- `WithCandidates(candidates ...Candidate)` - Provide candidates directly instead of fetching from signaling server

### `signaling.Server`

Transport-agnostic signaling server (in `pkg/signaling`):

- `NewServer() *Server` - Create a new signaling server
- `Register(peerID string, candidates []Candidate) error` - Register a peer
- `GetPeer(peerID string) (*PeerInfo, bool)` - Get peer information
- `GetAllPeers() []*PeerInfo` - List all registered peers
- `RemovePeer(peerID string)` - Remove a peer from registry
- `PeerCount() int` - Get number of registered peers

## Testing NAT Traversal

To test actual NAT traversal:

1. Deploy signaling server on a **public server** (VPS, cloud instance)
2. Run server peer on **Machine A** (behind NAT)
3. Run client peer on **Machine B** (behind different NAT)
4. Observe direct P2P connection establishment

## Limitations

- **Symmetric NAT**: May fail if both peers have strict symmetric NAT
- **Firewall Rules**: Some firewalls block all unsolicited UDP traffic
- **Port Randomization**: Some NATs use cryptographic port randomization

## Dependencies

- `github.com/quic-go/quic-go` - QUIC implementation in Go

## License

See LICENSE file for details.
