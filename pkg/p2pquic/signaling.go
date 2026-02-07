package p2pquic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// SignalingClient handles communication with the signaling server
type SignalingClient struct {
	serverURL string
}

// NewSignalingClient creates a new signaling client
func NewSignalingClient(serverURL string) *SignalingClient {
	return &SignalingClient{
		serverURL: serverURL,
	}
}

// Register registers this peer with the signaling server
func (s *SignalingClient) Register(peerID string, candidates []Candidate) error {
	peer := PeerInfo{
		ID:         peerID,
		Candidates: candidates,
	}

	data, err := json.Marshal(peer)
	if err != nil {
		return err
	}

	resp, err := http.Post(s.serverURL+"/register", "application/json", bytes.NewReader(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("registration failed: %s", body)
	}

	return nil
}

// GetPeer retrieves peer information from signaling server
func (s *SignalingClient) GetPeer(peerID string) (*PeerInfo, error) {
	resp, err := http.Get(fmt.Sprintf("%s/peer?id=%s", s.serverURL, peerID))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peer not found")
	}

	var peer PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peer); err != nil {
		return nil, err
	}

	return &peer, nil
}

// GetAllPeers retrieves all registered peers from signaling server
func (s *SignalingClient) GetAllPeers() ([]PeerInfo, error) {
	resp, err := http.Get(fmt.Sprintf("%s/peers", s.serverURL))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get peers")
	}

	var peers []PeerInfo
	if err := json.NewDecoder(resp.Body).Decode(&peers); err != nil {
		return nil, err
	}

	return peers, nil
}
