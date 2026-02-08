package signaling

import (
	"sync"
	"time"

	"github.com/mevdschee/p2pquic-go/pkg/p2pquic"
)

const (
	// peerTTL is the time-to-live for peer registrations
	peerTTL = 30 * time.Second

	// cleanupInterval is how often we check for expired peers
	cleanupInterval = 5 * time.Second
)

// Server manages peer registration and discovery
type Server struct {
	peers       map[string]*p2pquic.PeerInfo
	mu          sync.RWMutex
	stopCleanup chan struct{}
	cleanupOnce sync.Once
}

// NewServer creates a new signaling server with TTL-based cleanup
func NewServer() *Server {
	s := &Server{
		peers:       make(map[string]*p2pquic.PeerInfo),
		stopCleanup: make(chan struct{}),
	}

	// Start background cleanup goroutine
	go s.cleanupLoop()

	return s
}

// cleanupLoop periodically removes expired peer registrations
func (s *Server) cleanupLoop() {
	ticker := time.NewTicker(cleanupInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			s.cleanupExpired()
		case <-s.stopCleanup:
			return
		}
	}
}

// cleanupExpired removes peers that have exceeded TTL
func (s *Server) cleanupExpired() {
	now := time.Now()
	s.mu.Lock()
	defer s.mu.Unlock()

	for id, peer := range s.peers {
		if now.Sub(peer.Timestamp) > peerTTL {
			delete(s.peers, id)
		}
	}
}

// Close stops the cleanup goroutine
func (s *Server) Close() {
	s.cleanupOnce.Do(func() {
		close(s.stopCleanup)
	})
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

// GetPeer retrieves peer information by ID (returns nil if expired)
func (s *Server) GetPeer(peerID string) (*p2pquic.PeerInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	peer, exists := s.peers[peerID]
	if !exists {
		return nil, false
	}

	// Check if peer has expired (inline check for race conditions)
	if time.Since(peer.Timestamp) > peerTTL {
		return nil, false
	}

	return peer, true
}

// GetAllPeers retrieves all registered peers (excludes expired)
func (s *Server) GetAllPeers() []*p2pquic.PeerInfo {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	peerList := make([]*p2pquic.PeerInfo, 0, len(s.peers))
	for _, peer := range s.peers {
		if now.Sub(peer.Timestamp) <= peerTTL {
			peerList = append(peerList, peer)
		}
	}

	return peerList
}

// RemovePeer removes a peer from the registry
func (s *Server) RemovePeer(peerID string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	delete(s.peers, peerID)
}

// PeerCount returns the number of registered (non-expired) peers
func (s *Server) PeerCount() int {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now := time.Now()
	count := 0
	for _, peer := range s.peers {
		if now.Sub(peer.Timestamp) <= peerTTL {
			count++
		}
	}

	return count
}
