package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"time"

	ocppserver "ocpp-server/ocpp"
	"ocpp-server/server/database"
)

// APIServerWithDB enhances the API server with database integration
type APIServerWithDB struct {
	ocppServer *ocppserver.OCPPServer
	httpServer *http.Server
	config     *ocppserver.Config
	dbService  *database.Service
}

// NewAPIServerWithDB creates a new enhanced API server
func NewAPIServerWithDB(ocppServer *ocppserver.OCPPServer, config *ocppserver.Config, dbService *database.Service) *APIServerWithDB {
	mux := http.NewServeMux()

	server := &APIServerWithDB{
		ocppServer: ocppServer,
		config:     config,
		dbService:  dbService,
	}

	// Register API endpoints
	server.registerAPIEndpoints(mux)

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.APIPort),
		Handler: mux,
	}

	server.httpServer = httpServer
	return server
}

// registerAPIEndpoints registers all API routes
func (s *APIServerWithDB) registerAPIEndpoints(mux *http.ServeMux) {
	// Basic server status endpoint
	mux.HandleFunc("/api/status", s.handleServerStatus)

	// Charge Points endpoints
	mux.HandleFunc("/api/charge-points", s.handleChargePoints)

	// Find charge point by ID
	mux.HandleFunc("/api/charge-points/", func(w http.ResponseWriter, r *http.Request) {
		// Extract ID from path: /api/charge-points/{id}
		path := r.URL.Path
		if !strings.HasPrefix(path, "/api/charge-points/") {
			http.NotFound(w, r)
			return
		}

		id := strings.TrimPrefix(path, "/api/charge-points/")
		if id == "" {
			http.NotFound(w, r)
			return
		}

		s.handleChargePointByID(w, r, id)
	})

	// Connectors endpoints
	mux.HandleFunc("/api/connectors", s.handleConnectors)

	// Transactions endpoints
	mux.HandleFunc("/api/transactions", s.handleTransactions)

	// Logs endpoint
	mux.HandleFunc("/api/logs", s.handleLogs)

	// Authorization endpoints
	mux.HandleFunc("/api/authorizations", s.handleAuthorizations)

	// OCPP Command endpoints

	// Endpoint for remote start transaction
	mux.HandleFunc("/api/commands/remote-start", s.handleRemoteStart)

	// Endpoint for remote stop transaction
	mux.HandleFunc("/api/commands/remote-stop", s.handleRemoteStop)

	// Endpoint for reset
	mux.HandleFunc("/api/commands/reset", s.handleReset)

	// Endpoint for unlock connector
	mux.HandleFunc("/api/commands/unlock-connector", s.handleUnlockConnector)

	// Endpoint for get configuration
	mux.HandleFunc("/api/commands/get-configuration", s.handleGetConfiguration)

	// Endpoint for change configuration
	mux.HandleFunc("/api/commands/change-configuration", s.handleChangeConfiguration)

	// Endpoint for clear cache
	mux.HandleFunc("/api/commands/clear-cache", s.handleClearCache)

	// Endpoint for trigger message
	mux.HandleFunc("/api/commands/trigger-message", s.handleTriggerMessage)

	// Endpoint for generic commands (allows any OCPP command to be sent)
	mux.HandleFunc("/api/commands/generic", s.handleGenericCommand)
}

// Start initiates the API server
func (s *APIServerWithDB) Start() error {
	// Start OCPP server
	if err := s.ocppServer.Start(); err != nil {
		return err
	}

	// Start HTTP API server
	go func() {
		apiURL := fmt.Sprintf("http://%s:%d", s.config.Host, s.config.APIPort)
		log.Printf("HTTP API server listening on %s\n", apiURL)

		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to start HTTP server: %v", err)
		}
	}()

	return nil
}

// Shutdown gracefully shuts down the server
func (s *APIServerWithDB) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := s.httpServer.Shutdown(ctx); err != nil {
		log.Printf("Error during HTTP server shutdown: %v", err)
	}

	log.Println("Server shutdown complete")
}

// RunForever keeps the server running until the program is terminated
func (s *APIServerWithDB) RunForever() {
	select {} // Run until program is terminated
}

// handleServerStatus handles the server status endpoint
func (s *APIServerWithDB) handleServerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Count connected charge points
	chargePoints, err := s.dbService.ListChargePoints()
	connectedCount := 0
	if err == nil {
		for _, cp := range chargePoints {
			if cp.IsConnected {
				connectedCount++
			}
		}
	}

	status := struct {
		Status                string    `json:"status"`
		ServerTime            time.Time `json:"serverTime"`
		ChargePointsTotal     int       `json:"chargePointsTotal"`
		ChargePointsConnected int       `json:"chargePointsConnected"`
		DatabaseType          string    `json:"databaseType"`
		Uptime                string    `json:"uptime"`
	}{
		Status:                "running",
		ServerTime:            time.Now(),
		ChargePointsTotal:     len(chargePoints),
		ChargePointsConnected: connectedCount,
		DatabaseType:          string(s.dbService.GetDatabaseType()),
		Uptime:                "N/A", // Would need to track start time
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// handleChargePoints handles the charge points endpoint
func (s *APIServerWithDB) handleChargePoints(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	chargePoints, err := s.dbService.ListChargePoints()
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching charge points: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(chargePoints)
}

// handleChargePointByID handles the charge point detail endpoint
func (s *APIServerWithDB) handleChargePointByID(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get charge point
	chargePoint, err := s.dbService.GetChargePoint(id)
	if err != nil {
		http.Error(w, fmt.Sprintf("Charge point not found: %v", err), http.StatusNotFound)
		return
	}

	// Get connectors for this charge point
	connectors, err := s.dbService.ListConnectors(id)
	if err != nil {
		log.Printf("Error fetching connectors: %v", err)
		// Continue without connectors
	}

	// Create response with charge point and its connectors
	response := struct {
		*database.ChargePoint
		Connectors []database.Connector `json:"connectors"`
	}{
		ChargePoint: chargePoint,
		Connectors:  connectors,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleConnectors handles the connectors endpoint
func (s *APIServerWithDB) handleConnectors(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get charge point ID from query parameter
	chargePointID := r.URL.Query().Get("chargePointId")
	if chargePointID == "" {
		http.Error(w, "chargePointId query parameter is required", http.StatusBadRequest)
		return
	}

	connectors, err := s.dbService.ListConnectors(chargePointID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching connectors: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(connectors)
}

// handleTransactions handles the transactions endpoint
func (s *APIServerWithDB) handleTransactions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	chargePointID := r.URL.Query().Get("chargePointId")
	transactionIDStr := r.URL.Query().Get("transactionId")
	isCompleteStr := r.URL.Query().Get("isComplete")

	var transactions []database.Transaction
	var err error

	if transactionIDStr != "" {
		// Get single transaction
		transactionID, err := strconv.Atoi(transactionIDStr)
		if err != nil {
			http.Error(w, "Invalid transactionId parameter", http.StatusBadRequest)
			return
		}

		tx, err := s.dbService.GetTransaction(transactionID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Transaction not found: %v", err), http.StatusNotFound)
			return
		}
		transactions = []database.Transaction{*tx}
	} else {
		// List transactions with optional filters
		var isComplete *bool
		if isCompleteStr != "" {
			isCompleteVal := isCompleteStr == "true"
			isComplete = &isCompleteVal
		}

		transactions, err = s.dbService.ListTransactions(chargePointID, isComplete)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching transactions: %v", err), http.StatusInternalServerError)
			return
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(transactions)
}

// handleLogs handles the logs endpoint
func (s *APIServerWithDB) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	chargePointID := r.URL.Query().Get("chargePointId")
	level := r.URL.Query().Get("level")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	limit := 100 // Default limit
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0 // Default offset
	if offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	logs, err := s.dbService.GetLogs(chargePointID, level, limit, offset)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error fetching logs: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(logs)
}

// handleAuthorizations handles the authorizations endpoint
func (s *APIServerWithDB) handleAuthorizations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// List or get authorization
		idTag := r.URL.Query().Get("idTag")
		if idTag != "" {
			// Get specific authorization
			auth, err := s.dbService.GetAuthorization(idTag)
			if err != nil {
				http.Error(w, fmt.Sprintf("Authorization not found: %v", err), http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(auth)
		} else {
			// List all authorizations
			auths, err := s.dbService.ListAuthorizations()
			if err != nil {
				http.Error(w, fmt.Sprintf("Error fetching authorizations: %v", err), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(auths)
		}

	case http.MethodPost:
		// Create or update authorization
		var auth database.Authorization
		if err := json.NewDecoder(r.Body).Decode(&auth); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		if auth.IdTag == "" {
			http.Error(w, "IdTag is required", http.StatusBadRequest)
			return
		}

		// Set default values if not provided
		if auth.Status == "" {
			auth.Status = "Accepted"
		}

		auth.UpdatedAt = time.Now()
		if auth.CreatedAt.IsZero() {
			auth.CreatedAt = time.Now()
		}

		if err := s.dbService.SaveAuthorization(&auth); err != nil {
			http.Error(w, fmt.Sprintf("Error saving authorization: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(auth)

	case http.MethodDelete:
		// Delete authorization
		idTag := r.URL.Query().Get("idTag")
		if idTag == "" {
			http.Error(w, "idTag query parameter is required", http.StatusBadRequest)
			return
		}

		if err := s.dbService.DeleteAuthorization(idTag); err != nil {
			http.Error(w, fmt.Sprintf("Error deleting authorization: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}
