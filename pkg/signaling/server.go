package signaling

import (
	"sync"
	"time"

	"github.com/mevdschee/p2pquic-go/pkg/p2pquic"
)

// Server manages peer registration and discovery
type Server struct {
	peers map[string]*p2pquic.PeerInfo
	mu    sync.RWMutex
}

// NewServer creates a new signaling server
func NewServer() *Server {
	return &Server{
		peers: make(map[string]*p2pquic.PeerInfo),
	}
}

// Register registers a peer with its candidates
func (s *Server) Register(peerID string, candidates []p2pquic.Candidate) error {
	peer := &p2pquic.PeerInfo{
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
func (s *Server) GetPeer(peerID string) (*p2pquic.PeerInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peer, exists := s.peers[peerID]
	return peer, exists
}

// GetAllPeers retrieves all registered peers
func (s *Server) GetAllPeers() []*p2pquic.PeerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peerList := make([]*p2pquic.PeerInfo, 0, len(s.peers))
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
