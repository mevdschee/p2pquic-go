package main

import (
	"context"
	"flag"
	"log"
	"time"

	"github.com/mevdschee/p2pquic-go/pkg/p2pquic"
	"github.com/quic-go/quic-go"
)

func main() {
	mode := flag.String("mode", "server", "Mode: server or client")
	peerID := flag.String("id", "", "This peer's ID (defaults to 'server' or 'client' based on mode)")
	remotePeerID := flag.String("remote", "", "Remote peer ID (for client mode)")
	signalingURL := flag.String("signaling", "http://localhost:8080", "Signaling server URL")
	// IMPORTANT: Both client and server need a specific port for UDP hole-punching to work.
	// The port is used for:
	// 1. STUN discovery to find the public IP:port mapping
	// 2. Sending UDP punch packets to create NAT mappings
	// 3. Receiving the actual QUIC connection
	// All three must use the SAME port, otherwise NAT mappings won't match.
	// Different ports in examples (9000 vs 9001) are only for local testing on the same machine.
	port := flag.Int("port", 9000, "Local UDP port (required for hole-punching)")
	enableSTUN := flag.Bool("stun", true, "Enable STUN for public IP discovery")
	flag.Parse()

	// Set default peer ID based on mode if not provided
	if *peerID == "" {
		*peerID = *mode
	}

	log.Printf("Starting in %s mode as %s on port %d", *mode, *peerID, *port)

	// Create peer
	config := p2pquic.Config{
		PeerID:       *peerID,
		LocalPort:    *port,
		SignalingURL: *signalingURL,
		EnableSTUN:   *enableSTUN,
	}

	peer, err := p2pquic.NewPeer(config)
	if err != nil {
		log.Fatalf("Failed to create peer: %v", err)
	}
	defer peer.Close()

	// Discover candidates
	log.Println("Discovering NAT candidates...")
	candidates, err := peer.DiscoverCandidates()
	if err != nil {
		log.Fatalf("Failed to discover candidates: %v", err)
	}

	log.Printf("Total candidates: %d", len(candidates))
	for _, c := range candidates {
		log.Printf("  - %s:%d", c.IP, c.Port)
	}

	// Register with signaling server
	log.Println("Registering with signaling server...")
	if err := peer.Register(); err != nil {
		log.Fatalf("Failed to register: %v", err)
	}
	log.Println("Registration successful")

	if *mode == "server" {
		runServer(peer)
	} else {
		if *remotePeerID == "" {
			log.Fatal("Remote peer ID required in client mode (use -remote flag)")
		}
		runClient(peer, *remotePeerID)
	}
}

func runServer(peer *p2pquic.Peer) {
	// Start listening
	if err := peer.Listen(); err != nil {
		log.Fatalf("Failed to start listening: %v", err)
	}

	// Start continuous hole-punching in background
	ctx := context.Background()
	go peer.ContinuousHolePunch(ctx)

	log.Println("Waiting for incoming connections...")

	for {
		conn, err := peer.Accept(context.Background())
		if err != nil {
			log.Printf("Failed to accept connection: %v", err)
			continue
		}

		log.Printf("Accepted connection from %s", conn.RemoteAddr())

		go handleConnection(conn)
	}
}

func handleConnection(conn quic.Connection) {
	defer conn.CloseWithError(0, "done")

	stream, err := conn.AcceptStream(context.Background())
	if err != nil {
		log.Printf("Failed to accept stream: %v", err)
		return
	}
	defer stream.Close()

	buf := make([]byte, 1024)
	n, err := stream.Read(buf)
	if err != nil {
		log.Printf("Failed to read: %v", err)
		return
	}

	message := string(buf[:n])
	log.Printf("Received from %s: %s", conn.RemoteAddr().String(), message)

	// Continuously exchange messages
	for {
		// Send response
		response := "Hello from server!"
		_, err := stream.Write([]byte(response))
		if err != nil {
			log.Printf("Failed to write: %v", err)
			return
		}
		log.Printf("Sent to %s: %s", conn.RemoteAddr().String(), response)

		// Wait for next message
		time.Sleep(5 * time.Second)

		n, err := stream.Read(buf)
		if err != nil {
			log.Printf("Connection closed: %v", err)
			return
		}
		log.Printf("Received from %s: %s", conn.RemoteAddr().String(), string(buf[:n]))
	}
}

func runClient(peer *p2pquic.Peer, remotePeerID string) {
	log.Printf("Waiting for remote peer %s to register...", remotePeerID)

	// Wait for remote peer to be available
	time.Sleep(2 * time.Second)

	// Connect to remote peer
	conn, err := peer.Connect(remotePeerID)
	if err != nil {
		log.Fatalf("Failed to connect to remote peer: %v", err)
	}
	defer conn.CloseWithError(0, "done")

	log.Println("QUIC connection established!")

	// Open a stream
	stream, err := conn.OpenStreamSync(context.Background())
	if err != nil {
		log.Fatalf("Failed to open stream: %v", err)
	}
	defer stream.Close()

	log.Println("Starting continuous message exchange...")

	buf := make([]byte, 1024)
	for {
		// Send message
		message := "Hello from client!"
		_, err := stream.Write([]byte(message))
		if err != nil {
			log.Printf("Failed to send: %v", err)
			break
		}
		log.Printf("Sent: %s", message)

		// Receive response
		n, err := stream.Read(buf)
		if err != nil {
			log.Printf("Connection closed: %v", err)
			break
		}
		log.Printf("Received: %s", string(buf[:n]))

		// Wait before next message
		time.Sleep(5 * time.Second)
	}
}
