package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"ocpp-server/pkg/ocppserver"
)

// registerCommandEndpoints adds API endpoints for sending commands to charge points
func (s *APIServerWithDB) registerCommandEndpoints(mux *http.ServeMux) {
	// Endpoint for remote start transaction
	mux.HandleFunc("/api/commands/remoteStart", s.handleRemoteStart)

	// Endpoint for remote stop transaction
	mux.HandleFunc("/api/commands/remoteStop", s.handleRemoteStop)

	// Endpoint for reset
	mux.HandleFunc("/api/commands/reset", s.handleReset)

	// Endpoint for unlock connector
	mux.HandleFunc("/api/commands/unlockConnector", s.handleUnlockConnector)

	// Endpoint for get configuration
	mux.HandleFunc("/api/commands/getConfiguration", s.handleGetConfiguration)

	// Endpoint for change configuration
	mux.HandleFunc("/api/commands/changeConfiguration", s.handleChangeConfiguration)

	// Endpoint for clear cache
	mux.HandleFunc("/api/commands/clearCache", s.handleClearCache)

	// Endpoint for trigger message
	mux.HandleFunc("/api/commands/triggerMessage", s.handleTriggerMessage)

	// Endpoint for generic commands (allows any OCPP command to be sent)
	mux.HandleFunc("/api/commands/generic", s.handleGenericCommand)
}

// getCommandManager returns the command manager from the OCPP server
// getCommandManager returns the command manager from the OCPP server
func (s *APIServerWithDB) getCommandManager() *ocppserver.CommandManager {
	// Now we can directly access the CommandManager through our OCPPHandler interface
	return s.ocppServer.GetHandler().GetCommandManager()
}

// handleRemoteStart handles requests to start a transaction remotely
func (s *APIServerWithDB) handleRemoteStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ChargePointID string `json:"chargePointId"`
		IdTag         string `json:"idTag"`
		ConnectorID   *int   `json:"connectorId,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" || request.IdTag == "" {
		http.Error(w, "chargePointId and idTag are required", http.StatusBadRequest)
		return
	}

	// Send command
	success, err := s.getCommandManager().RemoteStartTransaction(request.ChargePointID, request.IdTag, request.ConnectorID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: success,
		Message: fmt.Sprintf("Remote start %s", successStr(success)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleRemoteStop handles requests to stop a transaction remotely
func (s *APIServerWithDB) handleRemoteStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body - we support two formats:
	// 1. The standard format with transactionId
	// 2. A simplified format with just chargePointId and connectorId
	var request struct {
		ChargePointID string `json:"chargePointId"`
		TransactionID *int   `json:"transactionId,omitempty"`
		ConnectorID   *int   `json:"connectorId,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" {
		http.Error(w, "chargePointId is required", http.StatusBadRequest)
		return
	}

	var transactionID int

	// If transaction ID is provided, use it directly
	if request.TransactionID != nil && *request.TransactionID > 0 {
		transactionID = *request.TransactionID
	} else if request.ConnectorID != nil && *request.ConnectorID > 0 {
		// If only connector ID is provided, try to find the active transaction
		tx, err := s.dbService.GetActiveTransactionForConnector(request.ChargePointID, *request.ConnectorID)
		if err != nil {
			http.Error(w, fmt.Sprintf("No active transaction found: %v", err), http.StatusNotFound)
			return
		}
		transactionID = tx.TransactionID
	} else {
		http.Error(w, "either transactionId or connectorId must be provided", http.StatusBadRequest)
		return
	}

	// Send command
	success, err := s.getCommandManager().RemoteStopTransaction(request.ChargePointID, transactionID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success       bool   `json:"success"`
		Message       string `json:"message"`
		TransactionID int    `json:"transactionId"`
	}{
		Success:       success,
		Message:       fmt.Sprintf("Remote stop %s", successStr(success)),
		TransactionID: transactionID,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleReset handles requests to reset a charge point
func (s *APIServerWithDB) handleReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ChargePointID string `json:"chargePointId"`
		Type          string `json:"type"` // "Hard" or "Soft"
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" {
		http.Error(w, "chargePointId is required", http.StatusBadRequest)
		return
	}

	if request.Type != "Hard" && request.Type != "Soft" {
		http.Error(w, "type must be 'Hard' or 'Soft'", http.StatusBadRequest)
		return
	}

	// Send command
	success, err := s.getCommandManager().Reset(request.ChargePointID, request.Type)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: success,
		Message: fmt.Sprintf("%s reset %s", request.Type, successStr(success)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleUnlockConnector handles requests to unlock a connector
func (s *APIServerWithDB) handleUnlockConnector(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ChargePointID string `json:"chargePointId"`
		ConnectorID   int    `json:"connectorId"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" || request.ConnectorID <= 0 {
		http.Error(w, "chargePointId and valid connectorId are required", http.StatusBadRequest)
		return
	}

	// Send command
	success, err := s.getCommandManager().UnlockConnector(request.ChargePointID, request.ConnectorID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: success,
		Message: fmt.Sprintf("Unlock connector %d %s", request.ConnectorID, successStr(success)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGetConfiguration handles requests to get configuration
func (s *APIServerWithDB) handleGetConfiguration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var chargePointID string
	var keys []string

	if r.Method == http.MethodPost {
		// Parse request body for POST
		var request struct {
			ChargePointID string   `json:"chargePointId"`
			Keys          []string `json:"keys,omitempty"`
		}

		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
			return
		}

		chargePointID = request.ChargePointID
		keys = request.Keys
	} else {
		// Parse query parameters for GET
		chargePointID = r.URL.Query().Get("chargePointId")
		keysParam := r.URL.Query().Get("keys")
		if keysParam != "" {
			keys = strings.Split(keysParam, ",")
		}
	}

	if chargePointID == "" {
		http.Error(w, "chargePointId is required", http.StatusBadRequest)
		return
	}

	// Send command
	config, err := s.getCommandManager().GetConfiguration(chargePointID, keys)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(config)
}

// handleChangeConfiguration handles requests to change configuration
func (s *APIServerWithDB) handleChangeConfiguration(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ChargePointID string `json:"chargePointId"`
		Key           string `json:"key"`
		Value         string `json:"value"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" || request.Key == "" {
		http.Error(w, "chargePointId and key are required", http.StatusBadRequest)
		return
	}

	// Send command
	success, err := s.getCommandManager().ChangeConfiguration(request.ChargePointID, request.Key, request.Value)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: success,
		Message: fmt.Sprintf("Configuration change %s", successStr(success)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleClearCache handles requests to clear the cache
func (s *APIServerWithDB) handleClearCache(w http.ResponseWriter, r *http.Request) {
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

	// Send command
	success, err := s.getCommandManager().ClearCache(request.ChargePointID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: success,
		Message: fmt.Sprintf("Clear cache %s", successStr(success)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleTriggerMessage handles requests to trigger a message
func (s *APIServerWithDB) handleTriggerMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ChargePointID    string `json:"chargePointId"`
		RequestedMessage string `json:"requestedMessage"`
		ConnectorID      *int   `json:"connectorId,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" || request.RequestedMessage == "" {
		http.Error(w, "chargePointId and requestedMessage are required", http.StatusBadRequest)
		return
	}

	// Valid message types for OCPP 1.6
	validMessages := map[string]bool{
		"BootNotification":              true,
		"DiagnosticsStatusNotification": true,
		"FirmwareStatusNotification":    true,
		"Heartbeat":                     true,
		"MeterValues":                   true,
		"StatusNotification":            true,
	}

	if !validMessages[request.RequestedMessage] {
		http.Error(w, "Invalid requestedMessage", http.StatusBadRequest)
		return
	}

	// Send command
	success, err := s.getCommandManager().TriggerMessage(request.ChargePointID, request.RequestedMessage, request.ConnectorID)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}{
		Success: success,
		Message: fmt.Sprintf("Trigger message %s %s", request.RequestedMessage, successStr(success)),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleGenericCommand handles any OCPP command in a generic way
func (s *APIServerWithDB) handleGenericCommand(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		ChargePointID string                 `json:"chargePointId"`
		Action        string                 `json:"action"`
		Payload       map[string]interface{} `json:"payload"`
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		http.Error(w, fmt.Sprintf("Invalid request body: %v", err), http.StatusBadRequest)
		return
	}

	if request.ChargePointID == "" || request.Action == "" {
		http.Error(w, "chargePointId and action are required", http.StatusBadRequest)
		return
	}

	// Initialize payload if nil
	if request.Payload == nil {
		request.Payload = make(map[string]interface{})
	}

	// Send command
	result, err := s.getCommandManager().SendGenericCommand(request.ChargePointID, request.Action, request.Payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to send command: %v", err), http.StatusInternalServerError)
		return
	}

	// Return response
	response := struct {
		Success bool        `json:"success"`
		Message string      `json:"message"`
		Result  interface{} `json:"result"`
	}{
		Success: true,
		Message: fmt.Sprintf("Command %s sent successfully", request.Action),
		Result:  result,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// successStr returns a string representation of a success status
func successStr(success bool) string {
	if success {
		return "successful"
	}
	return "failed"
}
