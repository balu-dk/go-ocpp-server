package server

import (
	"fmt"
	"log"
	"net/http"

	ocppserver "ocpp-server/ocpp"
)

// APIServer tilføjer REST API-funktionalitet til OCPP-serveren
type APIServer struct {
	ocppServer *ocppserver.OCPPServer
	httpServer *http.Server
	config     *ocppserver.Config
}

// NewAPIServer opretter en ny API-server der wrapperer OCPP-serveren
func NewAPIServer(ocppServer *ocppserver.OCPPServer, config *ocppserver.Config) *APIServer {
	mux := http.NewServeMux()

	// Tilføj API-endpoints her
	mux.HandleFunc("/api/status", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"status":"running"}`))
	})

	// Tilføj flere endpoints her for styring af ladestationer

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.APIPort), // Binder til alle interfaces
		Handler: mux,
	}

	return &APIServer{
		ocppServer: ocppServer,
		httpServer: httpServer,
		config:     config,
	}
}

// Start starter både OCPP-serveren og HTTP API
func (s *APIServer) Start() error {
	// Start OCPP-serveren
	if err := s.ocppServer.Start(); err != nil {
		return err
	}

	// Start HTTP API-serveren
	go func() {
		apiURL := fmt.Sprintf("http://%s:%d", s.config.Host, s.config.APIPort)
		log.Printf("HTTP API server listening on %s\n", apiURL)

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	return nil
}

// RunForever holder serveren kørende indtil programmet afsluttes
func (s *APIServer) RunForever() {
	s.ocppServer.RunForever()
}
