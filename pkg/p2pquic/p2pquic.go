package p2pquic

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"time"

	"github.com/quic-go/quic-go"
)

// Peer represents a P2P QUIC peer
type Peer struct {
	config          Config
	signalingClient *SignalingClient
	udpConn         *net.UDPConn
	quicListener    *quic.Listener
	tlsConfig       *tls.Config
	candidates      []Candidate
}

// NewPeer creates a new P2P QUIC peer
func NewPeer(config Config) (*Peer, error) {
	if config.PeerID == "" {
		return nil, fmt.Errorf("peer ID is required")
	}
	if config.LocalPort == 0 {
		config.LocalPort = 9000
	}
	if config.SignalingURL == "" {
		config.SignalingURL = "http://localhost:8080"
	}

	peer := &Peer{
		config:          config,
		signalingClient: NewSignalingClient(config.SignalingURL),
		tlsConfig:       generateTLSConfig(),
	}

	return peer, nil
}

// DiscoverCandidates discovers NAT traversal candidates
func (p *Peer) DiscoverCandidates() ([]Candidate, error) {
	var candidates []Candidate

	// Get local port
	localPort := p.config.LocalPort
	if p.udpConn != nil {
		// Use actual port from UDP connection
		addr := p.udpConn.LocalAddr().(*net.UDPAddr)
		localPort = addr.Port
	}

	// Try STUN discovery if enabled - use temporary socket to avoid blocking
	if p.config.EnableSTUN {
		log.Printf("Attempting STUN discovery...")
		if stunCand, err := discoverPublicIP(localPort); err == nil {
			log.Printf("STUN discovered: %s:%d", stunCand.IP, stunCand.Port)
			candidates = append(candidates, *stunCand)
		} else {
			log.Printf("STUN discovery failed: %v (continuing with local candidates)", err)
		}
	}

	// Add local candidates
	localCands := getLocalCandidates(localPort)
	candidates = append(candidates, localCands...)

	p.candidates = candidates
	return candidates, nil
}

// Register registers this peer with the signaling server
func (p *Peer) Register() error {
	if len(p.candidates) == 0 {
		return fmt.Errorf("no candidates to register, call DiscoverCandidates first")
	}

	return p.signalingClient.Register(p.config.PeerID, p.candidates)
}

// Listen starts listening for incoming QUIC connections
func (p *Peer) Listen() error {
	udpAddr := &net.UDPAddr{
		IP:   net.IPv4zero,
		Port: p.config.LocalPort,
	}

	var err error
	p.udpConn, err = net.ListenUDP("udp4", udpAddr)
	if err != nil {
		return fmt.Errorf("failed to create UDP socket: %w", err)
	}

	// Configure QUIC with extended idle timeout and keepalive
	quicConfig := &quic.Config{
		MaxIdleTimeout:  5 * time.Minute,  // Extended idle timeout
		KeepAlivePeriod: 30 * time.Second, // Send keepalive pings
	}

	p.quicListener, err = quic.Listen(p.udpConn, p.tlsConfig, quicConfig)
	if err != nil {
		p.udpConn.Close()
		return fmt.Errorf("failed to start QUIC listener: %w", err)
	}

	log.Printf("QUIC listener started on port %d", p.config.LocalPort)
	return nil
}

// Accept accepts an incoming QUIC connection
func (p *Peer) Accept(ctx context.Context) (*quic.Conn, error) {
	if p.quicListener == nil {
		return nil, fmt.Errorf("peer is not listening, call Listen first")
	}

	return p.quicListener.Accept(ctx)
}

// GetActualPort returns the actual port from the UDP listener
func (p *Peer) GetActualPort() int {
	if p.udpConn == nil {
		return p.config.LocalPort
	}
	addr := p.udpConn.LocalAddr().(*net.UDPAddr)
	return addr.Port
}

// UpdateSignalingClient updates the signaling client with a new URL
func (p *Peer) UpdateSignalingClient(url string) {
	p.config.SignalingURL = url
	p.signalingClient = NewSignalingClient(url)
	log.Printf("Updated signaling client to use %s", url)
}

// Connect connects to a remote peer.
// If no candidates are provided via options, the peer's candidates are fetched from the signaling server.
// Use WithCandidates to provide candidates directly and bypass the signaling server lookup.
func (p *Peer) Connect(remotePeerID string, opts ...ConnectOption) (*quic.Conn, error) {
	// Apply options
	cfg := &connectConfig{}
	for _, opt := range opts {
		opt(cfg)
	}

	var candidates []Candidate

	// Use provided candidates or fetch from signaling server
	if len(cfg.candidates) > 0 {
		candidates = cfg.candidates
		log.Printf("Using %d provided candidates", len(candidates))
	} else {
		// Get remote peer info from signaling server
		remotePeer, err := p.signalingClient.GetPeer(remotePeerID)
		if err != nil {
			return nil, fmt.Errorf("failed to get remote peer info: %w", err)
		}
		candidates = remotePeer.Candidates
		log.Printf("Found remote peer with %d candidates", len(candidates))
	}

	// Create UDP connection if not already created
	if p.udpConn == nil {
		udpAddr := &net.UDPAddr{
			IP:   net.IPv4zero,
			Port: p.config.LocalPort,
		}
		var err error
		p.udpConn, err = net.ListenUDP("udp4", udpAddr)
		if err != nil {
			return nil, fmt.Errorf("failed to create UDP socket: %w", err)
		}
	}

	// Perform UDP hole-punching
	log.Println("Performing UDP hole-punch...")
	if err := p.holePunch(candidates); err != nil {
		return nil, fmt.Errorf("hole-punch failed: %w", err)
	}

	// Wait for holes to be established
	time.Sleep(2 * time.Second)

	// Attempt QUIC connection
	log.Println("Attempting QUIC connection...")
	return p.connectQUIC(candidates)
}

// ContinuousHolePunch continuously polls for peers and sends punch packets
func (p *Peer) ContinuousHolePunch(ctx context.Context) {
	if p.udpConn == nil {
		log.Println("Warning: UDP connection not initialized for continuous hole-punching")
		return
	}

	knownPeers := make(map[string]bool)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			peers, err := p.signalingClient.GetAllPeers()
			if err != nil {
				log.Printf("Failed to get peers: %v", err)
				continue
			}

			for _, peer := range peers {
				// Skip ourselves
				if peer.ID == p.config.PeerID {
					continue
				}

				// Check if this is a new peer
				if !knownPeers[peer.ID] {
					log.Printf("Discovered new peer: %s with %d candidates", peer.ID, len(peer.Candidates))
					knownPeers[peer.ID] = true
				}

				// Send punch packets to all candidates
				for _, candidate := range peer.Candidates {
					addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", candidate.IP, candidate.Port))
					if err != nil {
						continue
					}
					p.udpConn.WriteToUDP([]byte("PUNCH"), addr)
				}
			}
		}
	}
}

// Close closes the peer and releases resources
func (p *Peer) Close() error {
	if p.quicListener != nil {
		p.quicListener.Close()
	}
	if p.udpConn != nil {
		return p.udpConn.Close()
	}
	return nil
}

// GetUDPConn returns the underlying UDP connection for manual hole-punching
func (p *Peer) GetUDPConn() *net.UDPConn {
	return p.udpConn
}

// holePunch performs UDP hole-punching to remote candidates
func (p *Peer) holePunch(remoteCandidates []Candidate) error {
	for _, candidate := range remoteCandidates {
		addr, err := net.ResolveUDPAddr("udp4", fmt.Sprintf("%s:%d", candidate.IP, candidate.Port))
		if err != nil {
			log.Printf("Failed to resolve %s:%d: %v", candidate.IP, candidate.Port, err)
			continue
		}

		// Send multiple packets to ensure hole is punched
		for i := 0; i < 5; i++ {
			_, err = p.udpConn.WriteToUDP([]byte("PUNCH"), addr)
			if err != nil {
				log.Printf("Failed to send punch packet to %s: %v", addr, err)
			} else {
				log.Printf("Sent punch packet to %s", addr)
			}
			time.Sleep(100 * time.Millisecond)
		}
	}

	return nil
}

// connectQUIC attempts to connect to remote candidates via QUIC
func (p *Peer) connectQUIC(remoteCandidates []Candidate) (*quic.Conn, error) {
	for _, candidate := range remoteCandidates {
		addr := fmt.Sprintf("%s:%d", candidate.IP, candidate.Port)
		log.Printf("Attempting QUIC connection to %s", addr)

		remoteAddr, err := net.ResolveUDPAddr("udp4", addr)
		if err != nil {
			log.Printf("Failed to resolve %s: %v", addr, err)
			continue
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Configure QUIC with extended idle timeout and keepalive
		quicConfig := &quic.Config{
			MaxIdleTimeout:  5 * time.Minute,  // Extended idle timeout
			KeepAlivePeriod: 30 * time.Second, // Send keepalive pings
		}

		quicConn, err := quic.Dial(ctx, p.udpConn, remoteAddr, p.tlsConfig, quicConfig)
		if err != nil {
			log.Printf("Failed to connect to %s: %v", addr, err)
			continue
		}

		log.Printf("Successfully connected to %s", addr)
		return quicConn, nil
	}

	return nil, fmt.Errorf("failed to connect to any candidate")
}

// discoverPublicIP uses STUN to discover public IP
func discoverPublicIP(localPort int) (*Candidate, error) {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{Port: 0}) // Use random port to avoid conflicts
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	return performSTUNDiscovery(conn, localPort)
}

// discoverPublicIPWithConn uses STUN with an existing UDP connection
func discoverPublicIPWithConn(conn *net.UDPConn) (*Candidate, error) {
	addr := conn.LocalAddr().(*net.UDPAddr)
	return performSTUNDiscovery(conn, addr.Port)
}

// performSTUNDiscovery performs the actual STUN discovery
func performSTUNDiscovery(conn *net.UDPConn, localPort int) (*Candidate, error) {
	stunAddr, _ := net.ResolveUDPAddr("udp4", "stun.l.google.com:19302")

	// Simple STUN binding request
	stunReq := []byte{
		0x00, 0x01, // Binding Request
		0x00, 0x00, // Length
		0x21, 0x12, 0xa4, 0x42, // Magic Cookie
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Transaction ID
	}

	conn.SetDeadline(time.Now().Add(3 * time.Second))
	defer conn.SetDeadline(time.Time{}) // Clear deadline

	_, err := conn.WriteToUDP(stunReq, stunAddr)
	if err != nil {
		return nil, err
	}

	buf := make([]byte, 1024)
	n, _, err := conn.ReadFromUDP(buf)
	if err != nil {
		return nil, err
	}

	// Parse STUN response (simplified)
	if n > 20 {
		// Look for XOR-MAPPED-ADDRESS attribute (0x0020)
		for i := 20; i < n-8; i++ {
			if buf[i] == 0x00 && buf[i+1] == 0x20 {
				// Found XOR-MAPPED-ADDRESS
				// port := int(buf[i+6])<<8 | int(buf[i+7])
				// port ^= 0x2112 // XOR with magic cookie

				ip := net.IPv4(
					buf[i+8]^0x21,
					buf[i+9]^0x12,
					buf[i+10]^0xa4,
					buf[i+11]^0x42,
				)

				// Return discovered public IP with our local port
				return &Candidate{
					IP:   ip.String(),
					Port: localPort,
				}, nil
			}
		}
	}

	return nil, fmt.Errorf("failed to parse STUN response")
}

// getLocalCandidates returns local network candidates
func getLocalCandidates(port int) []Candidate {
	candidates := []Candidate{}

	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return candidates
	}

	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				candidates = append(candidates, Candidate{
					IP:   ipnet.IP.String(),
					Port: port,
				})
			}
		}
	}

	return candidates
}

// generateTLSConfig creates a self-signed certificate for QUIC
func generateTLSConfig() *tls.Config {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		panic(err)
	}

	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		panic(err)
	}

	return &tls.Config{
		Certificates:       []tls.Certificate{tlsCert},
		InsecureSkipVerify: true, // For demo purposes only
		NextProtos:         []string{"p2pquic"},
	}
}
