package ocppserver

import (
	"fmt"
	"log"
	"net/http"
)

// OCPPServer represents an OCPP central server
type OCPPServer struct {
	config  *Config
	handler OCPPHandler // Changed from http.Handler to OCPPHandler
	server  *http.Server
}

// NewOCPPServer creates a new OCPP server with the given configuration and handler
func NewOCPPServer(config *Config, handler OCPPHandler) *OCPPServer {
	mux := http.NewServeMux()

	// Register WebSocket handler
	mux.Handle("/", handler)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.WebSocketPort), // Binds to all interfaces
		Handler: mux,
	}

	return &OCPPServer{
		config:  config,
		handler: handler,
		server:  server,
	}
}

// Start initiates and starts the OCPP server
func (s *OCPPServer) Start() error {
	// Start the server in a goroutine
	go func() {
		wsURL := fmt.Sprintf("ws://%s:%d", s.config.Host, s.config.WebSocketPort)
		log.Printf("OCPP Central System listening on %s", wsURL)

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	return nil
}

// GetHandler returns the handler used by the server
func (s *OCPPServer) GetHandler() OCPPHandler {
	return s.handler
}

// RunForever keeps the server running until the program exits
func (s *OCPPServer) RunForever() {
	select {} // Runs until program exits
}
