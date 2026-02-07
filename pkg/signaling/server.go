package signaling

import (
	"sync"
	"time"
)

// Candidate represents a NAT traversal candidate
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

// Server manages peer registration and discovery
type Server struct {
	peers map[string]*PeerInfo
	mu    sync.RWMutex
}

// NewServer creates a new signaling server
func NewServer() *Server {
	return &Server{
		peers: make(map[string]*PeerInfo),
	}
}

// Register registers a peer with its candidates
func (s *Server) Register(peerID string, candidates []Candidate) error {
	peer := &PeerInfo{
		ID:         peerID,
		Candidates: candidates,
		Timestamp:  time.Now(),
	}

	s.mu.Lock()
	s.peers[peerID] = peer
	s.mu.Unlock()

	return nil
}

// GetPeer retrieves peer information by ID
func (s *Server) GetPeer(peerID string) (*PeerInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peer, exists := s.peers[peerID]
	return peer, exists
}

// GetAllPeers retrieves all registered peers
func (s *Server) GetAllPeers() []*PeerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peerList := make([]*PeerInfo, 0, len(s.peers))
	for _, peer := range s.peers {
		peerList = append(peerList, peer)
	}

	return peerList
}

// RemovePeer removes a peer from the registry
func (s *Server) RemovePeer(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.peers, peerID)
}

// PeerCount returns the number of registered peers
func (s *Server) PeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return len(s.peers)
}
