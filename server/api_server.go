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

	// Endpoint for raw message logs
	mux.HandleFunc("/api/raw-logs", s.handleRawMessageLogs)

	// Endpoint for force stop command
	mux.HandleFunc("/api/commands/force-stop", s.handleForceStop)

	mux.HandleFunc("/api/proxy/destinations", s.handleProxyDestinations)
	mux.HandleFunc("/api/proxy/charge-points", s.handleChargePointProxies)
	mux.HandleFunc("/api/proxy/mappings", s.handleProxyMappings)
	mux.HandleFunc("/api/proxy/logs", s.handleProxyLogs)
	mux.HandleFunc("/api/proxy/connect", s.handleManualProxyConnect)
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
	}{
		Status:                "running",
		ServerTime:            time.Now(),
		ChargePointsTotal:     len(chargePoints),
		ChargePointsConnected: connectedCount,
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

// handleRawMessageLogs handles requests to retrieve raw OCPP message logs
func (s *APIServerWithDB) handleRawMessageLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	chargePointID := r.URL.Query().Get("chargePointId")
	direction := r.URL.Query().Get("direction")
	action := r.URL.Query().Get("action")
	transactionIDStr := r.URL.Query().Get("transactionId")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	// Default limit and offset
	limit := 50
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	var logs []database.RawMessageLog
	var err error

	// If transaction ID is provided, get logs for that transaction
	if transactionIDStr != "" {
		transactionID, err := strconv.Atoi(transactionIDStr)
		if err != nil {
			http.Error(w, "Invalid transactionId parameter", http.StatusBadRequest)
			return
		}

		logs, err = s.dbService.GetRawMessageLogsForTransaction(transactionID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching raw logs for transaction: %v", err), http.StatusInternalServerError)
			return
		}
	} else {
		// Otherwise, get logs with filters
		logs, err = s.dbService.GetRawMessageLogs(chargePointID, direction, action, limit, offset)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching raw logs: %v", err), http.StatusInternalServerError)
			return
		}
	}

	// Add total count for pagination
	response := struct {
		Count int                      `json:"count"`
		Logs  []database.RawMessageLog `json:"logs"`
	}{
		Count: len(logs),
		Logs:  logs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleProxyDestinations handles CRUD operations for proxy destinations
func (s *APIServerWithDB) handleProxyDestinations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get a specific destination if ID is provided
		idStr := r.URL.Query().Get("id")
		if idStr != "" {
			id, err := strconv.ParseUint(idStr, 10, 32)
			if err != nil {
				http.Error(w, "Invalid id parameter", http.StatusBadRequest)
				return
			}

			destination, err := s.dbService.GetProxyDestination(uint(id))
			if err != nil {
				http.Error(w, fmt.Sprintf("Proxy destination not found: %v", err), http.StatusNotFound)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(destination)
			return
		}

		// List all destinations
		destinations, err := s.dbService.ListProxyDestinations()
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching proxy destinations: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(destinations)

	case http.MethodPost:
		// Create or update proxy destination
		var destination database.ProxyDestination
		if err := json.NewDecoder(r.Body).Decode(&destination); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		// Check if this is an update (ID is provided) or a new destination
		isUpdate := destination.ID > 0

		// For updates, verify the destination exists
		if isUpdate {
			existing, err := s.dbService.GetProxyDestination(destination.ID)
			if err != nil {
				http.Error(w, fmt.Sprintf("Proxy destination not found: %v", err), http.StatusNotFound)
				return
			}

			// Preserve creation timestamp for updates
			destination.CreatedAt = existing.CreatedAt
		} else {
			// Set created time for new destinations
			destination.CreatedAt = time.Now()
		}

		// Always update timestamp
		destination.UpdatedAt = time.Now()

		if err := s.dbService.SaveProxyDestination(&destination); err != nil {
			http.Error(w, fmt.Sprintf("Error saving proxy destination: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if !isUpdate {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(destination)

	case http.MethodDelete:
		// Delete proxy destination
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			http.Error(w, "id query parameter is required", http.StatusBadRequest)
			return
		}

		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			http.Error(w, "Invalid id parameter", http.StatusBadRequest)
			return
		}

		if err := s.dbService.DeleteProxyDestination(uint(id)); err != nil {
			http.Error(w, fmt.Sprintf("Error deleting proxy destination: %v", err), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleChargePointProxies handles proxy configuration for charge points
func (s *APIServerWithDB) handleChargePointProxies(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get proxy config for a specific charge point
		chargePointID := r.URL.Query().Get("chargePointId")
		if chargePointID == "" {
			// If no specific charge point ID is provided, list all configs
			var proxyConfigs []database.ChargePointProxy
			result := s.dbService.GetDB().Find(&proxyConfigs)
			if result.Error != nil {
				http.Error(w, fmt.Sprintf("Error fetching proxy configs: %v", result.Error), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(proxyConfigs)
			return
		}

		// Get specific charge point config
		proxyConfig, err := s.dbService.GetChargePointProxy(chargePointID)
		if err != nil {
			// If not found, return an empty config
			proxyConfig = &database.ChargePointProxy{
				ChargePointID: chargePointID,
				ProxyEnabled:  false,
			}
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(proxyConfig)

	case http.MethodPost:
		// Create or update proxy config for a charge point
		var proxyConfig database.ChargePointProxy
		if err := json.NewDecoder(r.Body).Decode(&proxyConfig); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		if proxyConfig.ChargePointID == "" {
			http.Error(w, "chargePointId is required", http.StatusBadRequest)
			return
		}

		// Check if this is an update or a new config
		isUpdate := proxyConfig.ID > 0

		// For updates, check if the config exists
		if isUpdate {
			var existing database.ChargePointProxy
			result := s.dbService.GetDB().First(&existing, proxyConfig.ID)
			if result.Error != nil {
				http.Error(w, fmt.Sprintf("Proxy config not found: %v", result.Error), http.StatusNotFound)
				return
			}

			// Preserve creation timestamp
			proxyConfig.CreatedAt = existing.CreatedAt
		} else {
			// Check if a config already exists for this charge point
			existing, err := s.dbService.GetChargePointProxy(proxyConfig.ChargePointID)
			if err == nil {
				// Config exists, use its ID for an update
				proxyConfig.ID = existing.ID
				proxyConfig.CreatedAt = existing.CreatedAt
				isUpdate = true
			} else {
				// New config
				proxyConfig.CreatedAt = time.Now()
			}
		}

		// Always update timestamp
		proxyConfig.UpdatedAt = time.Now()

		if err := s.dbService.SaveChargePointProxy(&proxyConfig); err != nil {
			http.Error(w, fmt.Sprintf("Error saving proxy config: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if !isUpdate {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(proxyConfig)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProxyMappings handles mappings between charge points and proxy destinations
func (s *APIServerWithDB) handleProxyMappings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		// Get mappings for a specific charge point
		chargePointID := r.URL.Query().Get("chargePointId")
		if chargePointID == "" {
			// List all mappings if no charge point ID is provided
			var mappings []database.ChargePointProxyMapping
			result := s.dbService.GetDB().Find(&mappings)
			if result.Error != nil {
				http.Error(w, fmt.Sprintf("Error fetching proxy mappings: %v", result.Error), http.StatusInternalServerError)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(mappings)
			return
		}

		// Get mappings for a specific charge point
		mappings, err := s.dbService.GetChargePointProxyMappings(chargePointID)
		if err != nil {
			http.Error(w, fmt.Sprintf("Error fetching proxy mappings: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(mappings)

	case http.MethodPost:
		// Create or update a proxy mapping
		var mapping database.ChargePointProxyMapping
		if err := json.NewDecoder(r.Body).Decode(&mapping); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		if mapping.ChargePointID == "" || mapping.ProxyDestinationID == 0 {
			http.Error(w, "chargePointId and proxyDestinationId are required", http.StatusBadRequest)
			return
		}

		// Check if this is an update or a new mapping
		isUpdate := mapping.ID > 0

		// For updates, check if the mapping exists
		if isUpdate {
			var existing database.ChargePointProxyMapping
			result := s.dbService.GetDB().First(&existing, mapping.ID)
			if result.Error != nil {
				http.Error(w, fmt.Sprintf("Proxy mapping not found: %v", result.Error), http.StatusNotFound)
				return
			}

			// Preserve creation timestamp
			mapping.CreatedAt = existing.CreatedAt
		} else {
			// Check if a mapping already exists for this charge point and destination
			var existing database.ChargePointProxyMapping
			result := s.dbService.GetDB().Where(
				"charge_point_id = ? AND proxy_destination_id = ?",
				mapping.ChargePointID,
				mapping.ProxyDestinationID,
			).First(&existing)

			if result.Error == nil {
				// Mapping exists, use its ID for an update
				mapping.ID = existing.ID
				mapping.CreatedAt = existing.CreatedAt
				isUpdate = true
			} else {
				// New mapping
				mapping.CreatedAt = time.Now()
			}
		}

		// Always update timestamp
		mapping.UpdatedAt = time.Now()

		if err := s.dbService.SaveChargePointProxyMapping(&mapping); err != nil {
			http.Error(w, fmt.Sprintf("Error saving proxy mapping: %v", err), http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if !isUpdate {
			w.WriteHeader(http.StatusCreated)
		}
		json.NewEncoder(w).Encode(mapping)

	case http.MethodDelete:
		// Delete a proxy mapping
		idStr := r.URL.Query().Get("id")
		if idStr == "" {
			// Try with chargePointId and proxyDestinationId
			chargePointID := r.URL.Query().Get("chargePointId")
			proxyDestIDStr := r.URL.Query().Get("proxyDestinationId")

			if chargePointID != "" && proxyDestIDStr != "" {
				proxyDestID, err := strconv.ParseUint(proxyDestIDStr, 10, 32)
				if err != nil {
					http.Error(w, "Invalid proxyDestinationId parameter", http.StatusBadRequest)
					return
				}

				// Find mapping by charge point ID and proxy destination ID
				var mapping database.ChargePointProxyMapping
				result := s.dbService.GetDB().Where(
					"charge_point_id = ? AND proxy_destination_id = ?",
					chargePointID,
					uint(proxyDestID),
				).First(&mapping)

				if result.Error != nil {
					http.Error(w, fmt.Sprintf("Mapping not found: %v", result.Error), http.StatusNotFound)
					return
				}

				// Delete the mapping
				if err := s.dbService.GetDB().Delete(&mapping).Error; err != nil {
					http.Error(w, fmt.Sprintf("Error deleting mapping: %v", err), http.StatusInternalServerError)
					return
				}

				w.WriteHeader(http.StatusNoContent)
				return
			}

			http.Error(w, "Either id or both chargePointId and proxyDestinationId must be provided", http.StatusBadRequest)
			return
		}

		id, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			http.Error(w, "Invalid id parameter", http.StatusBadRequest)
			return
		}

		// Delete the mapping
		result := s.dbService.GetDB().Delete(&database.ChargePointProxyMapping{}, id)
		if result.Error != nil {
			http.Error(w, fmt.Sprintf("Error deleting proxy mapping: %v", result.Error), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusNoContent)

	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleProxyLogs handles retrieving proxy message logs
func (s *APIServerWithDB) handleProxyLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse query parameters
	chargePointID := r.URL.Query().Get("chargePointId")
	direction := r.URL.Query().Get("direction")
	limitStr := r.URL.Query().Get("limit")
	offsetStr := r.URL.Query().Get("offset")

	// Default limit and offset
	limit := 50
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err == nil && parsedLimit > 0 {
			limit = parsedLimit
		}
	}

	offset := 0
	if offsetStr != "" {
		parsedOffset, err := strconv.Atoi(offsetStr)
		if err == nil && parsedOffset >= 0 {
			offset = parsedOffset
		}
	}

	// Query the database
	db := s.dbService.GetDB()
	var logs []database.ProxyMessageLog

	query := db.Order("timestamp desc").Limit(limit).Offset(offset)

	if chargePointID != "" {
		query = query.Where("charge_point_id = ?", chargePointID)
	}

	if direction != "" {
		query = query.Where("direction = ?", direction)
	}

	result := query.Find(&logs)
	if result.Error != nil {
		http.Error(w, fmt.Sprintf("Error fetching proxy logs: %v", result.Error), http.StatusInternalServerError)
		return
	}

	// Response with count for pagination
	response := struct {
		Count int                        `json:"count"`
		Logs  []database.ProxyMessageLog `json:"logs"`
	}{
		Count: len(logs),
		Logs:  logs,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleManualProxyConnect handles requests to manually connect a charge point to its proxies
func (s *APIServerWithDB) handleManualProxyConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ChargePointID string `json:"chargePointId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" {
		http.Error(w, "chargePointId is required", http.StatusBadRequest)
		return
	}

	// Get the proxy manager from the OCPP server handler
	handler := s.ocppServer.GetHandler()
	if csHandler, ok := handler.(*ocppserver.CentralSystemHandlerWithDB); ok {
		if csHandler.GetProxyManager() != nil {
			err := csHandler.GetProxyManager().ManuallyConnectToProxies(request.ChargePointID)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to connect to proxies: %v", err), http.StatusInternalServerError)
				return
			}

			// Return success response
			response := struct {
				Success bool   `json:"success"`
				Message string `json:"message"`
			}{
				Success: true,
				Message: fmt.Sprintf("Manually connected charge point %s to proxies", request.ChargePointID),
			}

			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(response)
			return
		}
	}

	http.Error(w, "Proxy manager not available", http.StatusInternalServerError)
}
