package p2pquic

import (
	"time"
)

// Candidate represents a NAT traversal candidate (IP:Port pair)
type Candidate struct {
	IP   string `json:"ip"`
	Port int    `json:"port"`
}

// PeerInfo stores information about a peer
type PeerInfo struct {
	ID         string      `json:"id"`
	Candidates []Candidate `json:"candidates"`
	Timestamp  time.Time   `json:"timestamp"`
}

// Config holds configuration for a Peer
type Config struct {
	// PeerID is the unique identifier for this peer
	PeerID string

	// LocalPort is the UDP port to bind to
	LocalPort int

	// SignalingURL is the URL of the signaling server
	SignalingURL string

	// EnableSTUN enables STUN-based public IP discovery
	EnableSTUN bool
}

// connectConfig holds internal configuration for Connect calls
type connectConfig struct {
	candidates []Candidate
}

// ConnectOption is a functional option for configuring Connect calls
type ConnectOption func(*connectConfig)

// WithCandidates adds candidates to the connection attempt
func WithCandidates(candidates ...Candidate) ConnectOption {
	return func(c *connectConfig) {
		c.candidates = append(c.candidates, candidates...)
	}
}
