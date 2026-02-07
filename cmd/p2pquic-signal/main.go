package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/mevdschee/p2pquic-go/pkg/signaling"
)

// HTTPServer wraps the signaling server with HTTP handlers
type HTTPServer struct {
	server *signaling.Server
}

// NewHTTPServer creates a new HTTP signaling server
func NewHTTPServer() *HTTPServer {
	return &HTTPServer{
		server: signaling.NewServer(),
	}
}

// handleRegister handles peer registration requests
func (h *HTTPServer) handleRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var peer signaling.PeerInfo
	if err := json.NewDecoder(r.Body).Decode(&peer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if err := h.server.Register(peer.ID, peer.Candidates); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	log.Printf("Registered peer %s with %d candidates", peer.ID, len(peer.Candidates))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

// handleGetPeer handles peer lookup requests
func (h *HTTPServer) handleGetPeer(w http.ResponseWriter, r *http.Request) {
	peerID := r.URL.Query().Get("id")
	if peerID == "" {
		http.Error(w, "Missing peer ID", http.StatusBadRequest)
		return
	}

	peer, exists := h.server.GetPeer(peerID)
	if !exists {
		http.Error(w, "Peer not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peer)
}

// handleListPeers handles requests to list all peers
func (h *HTTPServer) handleListPeers(w http.ResponseWriter, r *http.Request) {
	peerList := h.server.GetAllPeers()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(peerList)
}

func main() {
	port := flag.String("port", "8080", "Port to listen on")
	flag.Parse()

	httpServer := NewHTTPServer()

	http.HandleFunc("/register", httpServer.handleRegister)
	http.HandleFunc("/peer", httpServer.handleGetPeer)
	http.HandleFunc("/peers", httpServer.handleListPeers)

	addr := ":" + *port
	log.Printf("Signaling server listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
