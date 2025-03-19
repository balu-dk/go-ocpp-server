package server

import (
	"fmt"
	"log"
	"net/http"

	ocppserver "ocpp-server/ocpp"
)

// APIServer integrates REST API functionality for the OCPP server
type APIServer struct {
	ocppServer *ocppserver.OCPPServer
	httpServer *http.Server
	config     *ocppserver.Config
}

// NewAPIServer starts a new API server that wraps the OCPP server
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

// Start() starts both the OCPP server and the HTTP API Server
func (s *APIServer) Start() error {
	// Start the OCPP
	if err := s.ocppServer.Start(); err != nil {
		return err
	}

	// Start the HTTP API server
	go func() {
		apiURL := fmt.Sprintf("http://%s:%d", s.config.Host, s.config.APIPort)
		log.Printf("HTTP API server listening on %s\n", apiURL)

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	return nil
}

// RunForever keeps APIServer running until terminated
func (s *APIServer) RunForever() {
	s.ocppServer.RunForever()
}
