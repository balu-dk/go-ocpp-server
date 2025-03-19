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

	//  Endpoint for administrative closing
	mux.HandleFunc("/api/admin/close-transaction", s.handleAdminCloseTransaction)

	//  Endpoint for energy reports
	mux.HandleFunc("/api/reports/energy", s.handleEnergyReport)
}

// Start initiates the API server
func (s *APIServerWithDB) Start() error {
	// Start OCPP server
	if err := s.ocppServer.Start(); err != nil {
		return err
	}

	// Start HTTP API server
	go func() {
		protocol := "http"
		if s.config.UseTLS {
			protocol = "https"
		}

		apiURL := fmt.Sprintf("%s://%s:%d", protocol, s.config.Host, s.config.APIPort)
		log.Printf("HTTP API server listening on %s\n", apiURL)

		var err error
		if s.config.UseTLS {
			err = s.httpServer.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
		} else {
			err = s.httpServer.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
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

func (s *APIServerWithDB) handleAdminCloseTransaction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		TransactionID int     `json:"transactionId"`
		FinalEnergy   float64 `json:"finalEnergy"` // in kWh
		Reason        string  `json:"reason"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.TransactionID <= 0 {
		http.Error(w, "transactionId is required", http.StatusBadRequest)
		return
	}

	// Get the transaction
	tx, err := s.dbService.GetTransaction(request.TransactionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Transaction not found: %v", err), http.StatusNotFound)
		return
	}

	if tx.IsComplete {
		http.Error(w, "Transaction is already complete", http.StatusBadRequest)
		return
	}

	// Update transaction
	tx.StopTimestamp = time.Now()
	tx.IsComplete = true

	// If final energy is provided, use it
	if request.FinalEnergy > 0 {
		// Convert kWh to Wh for MeterStop
		tx.MeterStop = tx.MeterStart + int(request.FinalEnergy*1000)
		tx.EnergyDelivered = request.FinalEnergy
	} else {
		// Use the latest meter values if available
		meterValues, _ := s.dbService.GetLatestMeterValueForTransaction(tx.TransactionID)
		if len(meterValues) > 0 {
			// Find the latest energy value
			var latestEnergyValue float64
			for _, mv := range meterValues {
				if mv.Measurand == "Energy.Active.Import.Register" {
					latestEnergyValue = mv.Value
					// Convert to Wh if necessary
					if mv.Unit == "kWh" {
						latestEnergyValue *= 1000
					}
				}
			}

			if latestEnergyValue > 0 {
				tx.MeterStop = int(latestEnergyValue)
				tx.EnergyDelivered = float64(tx.MeterStop-tx.MeterStart) / 1000.0
			}
		}
	}

	if request.Reason != "" {
		tx.StopReason = request.Reason
	} else {
		tx.StopReason = "AdminClosed"
	}

	if err := s.dbService.UpdateTransaction(tx); err != nil {
		http.Error(w, fmt.Sprintf("Failed to update transaction: %v", err), http.StatusInternalServerError)
		return
	}

	// Also update connector status if needed
	connector, err := s.dbService.GetConnector(tx.ChargePointID, tx.ConnectorID)
	if err == nil {
		connector.Status = "Available"
		connector.UpdatedAt = time.Now()
		s.dbService.SaveConnector(connector)
	}

	// Log this administrative action
	log.Printf("Transaction %d administratively closed. Final energy: %.2f kWh, Reason: %s",
		tx.TransactionID, tx.EnergyDelivered, tx.StopReason)

	// Create a log entry
	logEntry := &database.Log{
		ChargePointID: tx.ChargePointID,
		Timestamp:     time.Now(),
		Level:         "WARNING",
		Source:        "Admin",
		Message: fmt.Sprintf("Transaction %d administratively closed. Final energy: %.2f kWh, Reason: %s",
			tx.TransactionID, tx.EnergyDelivered, tx.StopReason),
	}
	s.dbService.AddLog(logEntry)

	// Return response
	response := struct {
		Success         bool    `json:"success"`
		Message         string  `json:"message"`
		TransactionID   int     `json:"transactionId"`
		EnergyDelivered float64 `json:"energyDelivered"`
	}{
		Success:         true,
		Message:         "Transaction closed successfully",
		TransactionID:   tx.TransactionID,
		EnergyDelivered: tx.EnergyDelivered,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// Handle Energy Report
func (s *APIServerWithDB) handleEnergyReport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	startDateStr := r.URL.Query().Get("startDate")
	endDateStr := r.URL.Query().Get("endDate")

	if startDateStr == "" || endDateStr == "" {
		http.Error(w, "startDate and endDate parameters are required", http.StatusBadRequest)
		return
	}

	startDate, err := time.Parse("2006-01-02", startDateStr)
	if err != nil {
		http.Error(w, "Invalid startDate format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	endDate, err := time.Parse("2006-01-02", endDateStr)
	if err != nil {
		http.Error(w, "Invalid endDate format. Use YYYY-MM-DD", http.StatusBadRequest)
		return
	}

	// Add one day to endDate to include the full day
	endDate = endDate.Add(24 * time.Hour)

	// Get transactions for the period
	transactions, err := s.dbService.GetTransactionsForPeriod(startDate, endDate)
	if err != nil {
		http.Error(w, fmt.Sprintf("Error retrieving transactions: %v", err), http.StatusInternalServerError)
		return
	}

	// Create energy consumption record structure
	type EnergyRecord struct {
		ChargePointID   string    `json:"chargePointId"`
		TransactionID   int       `json:"transactionId"`
		StartTimestamp  time.Time `json:"startTimestamp"`
		StopTimestamp   time.Time `json:"stopTimestamp"`
		EnergyDelivered float64   `json:"energyDelivered"`
		IdTag           string    `json:"idTag"`
		Complete        bool      `json:"complete"`
	}

	records := make([]EnergyRecord, 0, len(transactions))

	for _, tx := range transactions {
		records = append(records, EnergyRecord{
			ChargePointID:   tx.ChargePointID,
			TransactionID:   tx.TransactionID,
			StartTimestamp:  tx.StartTimestamp,
			StopTimestamp:   tx.StopTimestamp,
			EnergyDelivered: tx.EnergyDelivered,
			IdTag:           tx.IdTag,
			Complete:        tx.IsComplete,
		})
	}

	// Calculate totals
	var totalEnergy float64
	var completeTransactions int
	var incompleteTransactions int

	for _, record := range records {
		totalEnergy += record.EnergyDelivered
		if record.Complete {
			completeTransactions++
		} else {
			incompleteTransactions++
		}
	}

	response := struct {
		StartDate              string         `json:"startDate"`
		EndDate                string         `json:"endDate"`
		TotalEnergyDelivered   float64        `json:"totalEnergyDelivered"`
		CompleteTransactions   int            `json:"completeTransactions"`
		IncompleteTransactions int            `json:"incompleteTransactions"`
		Transactions           []EnergyRecord `json:"transactions"`
	}{
		StartDate:              startDateStr,
		EndDate:                endDateStr,
		TotalEnergyDelivered:   totalEnergy,
		CompleteTransactions:   completeTransactions,
		IncompleteTransactions: incompleteTransactions,
		Transactions:           records,
	}

	// Set content type and filename for download
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=energy-report-%s-to-%s.json", startDateStr, endDateStr))

	json.NewEncoder(w).Encode(response)
}
