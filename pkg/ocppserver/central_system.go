package ocppserver

import (
	"fmt"
	"log"
	"net/http"
)

// OCPPServer repræsenterer en OCPP centralserver
type OCPPServer struct {
	config  *Config
	handler *CentralSystemHandler
	server  *http.Server
}

// NewOCPPServer opretter en ny OCPP server med de angivne konfigurationer
func NewOCPPServer(config *Config, handler *CentralSystemHandler) *OCPPServer {
	mux := http.NewServeMux()

	// Registrer WebSocket handler
	mux.HandleFunc("/", handler.HandleWebSocket)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.WebSocketPort), // Binder til alle interfaces
		Handler: mux,
	}

	return &OCPPServer{
		config:  config,
		handler: handler,
		server:  server,
	}
}

// Start initierer og starter OCPP-serveren
func (s *OCPPServer) Start() error {
	// Start serveren i en goroutine
	go func() {
		wsURL := fmt.Sprintf("ws://%s:%d", s.config.Host, s.config.WebSocketPort)
		log.Printf("OCPP Central System listening on %s", wsURL)

		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	return nil
}

// RunForever holder serveren kørende indtil programmet afsluttes
func (s *OCPPServer) RunForever() {
	select {} // Kører indtil programmet afsluttes
}
