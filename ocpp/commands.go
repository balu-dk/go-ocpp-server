package ocppserver

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"ocpp-server/server/database"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
)

// CommandManager handles sending OCPP commands to charge points
type CommandManager struct {
	clients          map[string]*websocket.Conn
	pendingRequests  map[string]chan interface{}
	requestTimeouts  map[string]*time.Timer
	mutex            sync.Mutex // To synchronize access to maps
	defaultTimeoutMs int
	dbLogger         *DatabaseMessageLogger
}

// NewCommandManager creates a new command manager
func NewCommandManager(clients map[string]*websocket.Conn, dbService *database.Service) *CommandManager {
	// Create the command manager with database logger
	dbLogger := NewDatabaseMessageLogger(dbService)

	return &CommandManager{
		clients:          clients,
		pendingRequests:  make(map[string]chan interface{}),
		requestTimeouts:  make(map[string]*time.Timer),
		defaultTimeoutMs: 30000, // 30 seconds default timeout
		dbLogger:         dbLogger,
	}
}

// SendCommand sends an OCPP command to a charge point and waits for the response
func (cm *CommandManager) SendCommand(chargePointID string, action string, payload interface{}) (interface{}, error) {
	// Check if charge point is connected
	conn, exists := cm.clients[chargePointID]
	if !exists || conn == nil {
		return nil, fmt.Errorf("charge point %s is not connected", chargePointID)
	}

	// Generate a unique message ID for this request
	messageID := uuid.New().String()

	// Create OCPP message (Call format)
	// [MessageTypeId, UniqueId, Action, Payload]
	// MessageTypeId: 2 = Call (request from Central System to Charge Point)
	ocppMessage := []interface{}{2, messageID, action, payload}

	// Create channel to receive response
	responseChan := make(chan interface{}, 1)

	// Register this request in pending requests map
	cm.mutex.Lock()
	cm.pendingRequests[messageID] = responseChan

	// Set timeout for this request
	timer := time.AfterFunc(time.Duration(cm.defaultTimeoutMs)*time.Millisecond, func() {
		cm.mutex.Lock()
		defer cm.mutex.Unlock()

		// Close channel if it still exists
		if ch, exists := cm.pendingRequests[messageID]; exists {
			close(ch)
			delete(cm.pendingRequests, messageID)
			delete(cm.requestTimeouts, messageID)
		}
	})

	cm.requestTimeouts[messageID] = timer
	cm.mutex.Unlock()

	// Log the outgoing command using the database logger
	if cm.dbLogger != nil {
		messageJSON, err := json.Marshal(ocppMessage)
		if err == nil {
			if err := cm.dbLogger.LogRawMessage("SEND", chargePointID, messageJSON); err != nil {
				log.Printf("Error logging raw message: %v", err)
			}
		}
	}

	// Send the message
	if err := conn.WriteJSON(ocppMessage); err != nil {
		// Clean up if sending fails
		cm.mutex.Lock()
		delete(cm.pendingRequests, messageID)
		if timer, exists := cm.requestTimeouts[messageID]; exists {
			timer.Stop()
			delete(cm.requestTimeouts, messageID)
		}
		cm.mutex.Unlock()

		return nil, fmt.Errorf("failed to send command to charge point: %v", err)
	}

	log.Printf("Sent %s command to %s with message ID %s", action, chargePointID, messageID)

	// Wait for response or timeout
	response, ok := <-responseChan
	if !ok {
		return nil, fmt.Errorf("command timed out after %d ms", cm.defaultTimeoutMs)
	}

	// Log the received response using the database logger
	if cm.dbLogger != nil {
		responseMessage := []interface{}{3, messageID, response} // 3 = CallResult
		responseJSON, err := json.Marshal(responseMessage)
		if err == nil {
			if err := cm.dbLogger.LogRawMessage("RECV", chargePointID, responseJSON); err != nil {
				log.Printf("Error logging raw message: %v", err)
			}
		}
	}

	return response, nil
}

// HandleCommandResponse processes OCPP command responses from charge points
// This should be called when receiving CallResult (3) messages
func (cm *CommandManager) HandleCommandResponse(messageID string, payload interface{}) {
	cm.mutex.Lock()
	defer cm.mutex.Unlock()

	// Find the pending request channel for this message ID
	if responseChan, exists := cm.pendingRequests[messageID]; exists {
		// Send the response
		responseChan <- payload
		close(responseChan)

		// Clean up
		delete(cm.pendingRequests, messageID)
		if timer, exists := cm.requestTimeouts[messageID]; exists {
			timer.Stop()
			delete(cm.requestTimeouts, messageID)
		}

		log.Printf("Received response for message ID %s", messageID)
	} else {
		log.Printf("Received response for unknown message ID: %s", messageID)
	}
}

// OCPP Command Functions

// RemoteStartTransaction sends a RemoteStartTransaction command to a charge point
func (cm *CommandManager) RemoteStartTransaction(chargePointID string, idTag string, connectorID *int) (bool, error) {
	payload := map[string]interface{}{
		"idTag": idTag,
	}

	if connectorID != nil {
		payload["connectorId"] = *connectorID
	}

	response, err := cm.SendCommand(chargePointID, "RemoteStartTransaction", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// RemoteStopTransaction sends a RemoteStopTransaction command to a charge point
func (cm *CommandManager) RemoteStopTransaction(chargePointID string, transactionID int) (bool, error) {
	payload := map[string]interface{}{
		"transactionId": transactionID,
	}

	response, err := cm.SendCommand(chargePointID, "RemoteStopTransaction", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// Reset sends a Reset command to a charge point
func (cm *CommandManager) Reset(chargePointID string, resetType string) (bool, error) {
	// resetType should be "Hard" or "Soft"
	if resetType != "Hard" && resetType != "Soft" {
		return false, errors.New("resetType must be 'Hard' or 'Soft'")
	}

	payload := map[string]interface{}{
		"type": resetType,
	}

	response, err := cm.SendCommand(chargePointID, "Reset", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// UnlockConnector sends an UnlockConnector command to a charge point
func (cm *CommandManager) UnlockConnector(chargePointID string, connectorID int) (bool, error) {
	payload := map[string]interface{}{
		"connectorId": connectorID,
	}

	response, err := cm.SendCommand(chargePointID, "UnlockConnector", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// GetConfiguration sends a GetConfiguration command to a charge point
func (cm *CommandManager) GetConfiguration(chargePointID string, keys []string) (map[string]interface{}, error) {
	payload := map[string]interface{}{}

	if len(keys) > 0 {
		payload["key"] = keys
	}

	response, err := cm.SendCommand(chargePointID, "GetConfiguration", payload)
	if err != nil {
		return nil, err
	}

	// Response should contain configurationKey array and unknownKey array
	if resp, ok := response.(map[string]interface{}); ok {
		return resp, nil
	}

	return nil, errors.New("invalid response format")
}

// ChangeConfiguration sends a ChangeConfiguration command to a charge point
func (cm *CommandManager) ChangeConfiguration(chargePointID string, key string, value string) (bool, error) {
	payload := map[string]interface{}{
		"key":   key,
		"value": value,
	}

	response, err := cm.SendCommand(chargePointID, "ChangeConfiguration", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// ClearCache sends a ClearCache command to a charge point
func (cm *CommandManager) ClearCache(chargePointID string) (bool, error) {
	payload := map[string]interface{}{}

	response, err := cm.SendCommand(chargePointID, "ClearCache", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// DataTransfer sends a DataTransfer command to a charge point
func (cm *CommandManager) DataTransfer(chargePointID string, vendorID string, messageID string, data string) (map[string]interface{}, error) {
	payload := map[string]interface{}{
		"vendorId": vendorID,
	}

	if messageID != "" {
		payload["messageId"] = messageID
	}

	if data != "" {
		payload["data"] = data
	}

	response, err := cm.SendCommand(chargePointID, "DataTransfer", payload)
	if err != nil {
		return nil, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		return respMap, nil
	}

	return nil, errors.New("invalid response format")
}

// GetDiagnostics sends a GetDiagnostics command to a charge point
func (cm *CommandManager) GetDiagnostics(chargePointID string, location string, startTime *time.Time, stopTime *time.Time, retries *int) (string, error) {
	payload := map[string]interface{}{
		"location": location,
	}

	if startTime != nil {
		payload["startTime"] = startTime.Format(time.RFC3339)
	}

	if stopTime != nil {
		payload["stopTime"] = stopTime.Format(time.RFC3339)
	}

	if retries != nil {
		payload["retries"] = *retries
	}

	response, err := cm.SendCommand(chargePointID, "GetDiagnostics", payload)
	if err != nil {
		return "", err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if fileName, ok := respMap["fileName"].(string); ok {
			return fileName, nil
		}
	}

	return "", errors.New("invalid response format")
}

// TriggerMessage sends a TriggerMessage command to a charge point
func (cm *CommandManager) TriggerMessage(chargePointID string, requestedMessage string, connectorID *int) (bool, error) {
	payload := map[string]interface{}{
		"requestedMessage": requestedMessage,
	}

	if connectorID != nil {
		payload["connectorId"] = *connectorID
	}

	response, err := cm.SendCommand(chargePointID, "TriggerMessage", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// UpdateFirmware sends an UpdateFirmware command to a charge point
func (cm *CommandManager) UpdateFirmware(chargePointID string, location string, retrieveDate time.Time, retries *int, retryInterval *int) (bool, error) {
	payload := map[string]interface{}{
		"location":     location,
		"retrieveDate": retrieveDate.Format(time.RFC3339),
	}

	if retries != nil {
		payload["retries"] = *retries
	}

	if retryInterval != nil {
		payload["retryInterval"] = *retryInterval
	}

	// UpdateFirmware response is empty, so we just check if the command was sent successfully
	_, err := cm.SendCommand(chargePointID, "UpdateFirmware", payload)
	if err != nil {
		return false, err
	}

	return true, nil
}

// ReserveNow sends a ReserveNow command to a charge point
func (cm *CommandManager) ReserveNow(chargePointID string, connectorID int, expiryDate time.Time, idTag string, reservationID int, parentIdTag *string) (bool, error) {
	payload := map[string]interface{}{
		"connectorId":   connectorID,
		"expiryDate":    expiryDate.Format(time.RFC3339),
		"idTag":         idTag,
		"reservationId": reservationID,
	}

	if parentIdTag != nil {
		payload["parentIdTag"] = *parentIdTag
	}

	response, err := cm.SendCommand(chargePointID, "ReserveNow", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// CancelReservation sends a CancelReservation command to a charge point
func (cm *CommandManager) CancelReservation(chargePointID string, reservationID int) (bool, error) {
	payload := map[string]interface{}{
		"reservationId": reservationID,
	}

	response, err := cm.SendCommand(chargePointID, "CancelReservation", payload)
	if err != nil {
		return false, err
	}

	// Parse the response
	if respMap, ok := response.(map[string]interface{}); ok {
		if status, ok := respMap["status"].(string); ok {
			return status == "Accepted", nil
		}
	}

	return false, errors.New("invalid response format")
}

// Generic command for any OCPP action
func (cm *CommandManager) SendGenericCommand(chargePointID string, action string, payload map[string]interface{}) (interface{}, error) {
	return cm.SendCommand(chargePointID, action, payload)
}

// LockClients acquires the mutex to safely access the clients map
func (cm *CommandManager) LockClients() {
	cm.mutex.Lock()
}

// UnlockClients releases the mutex
func (cm *CommandManager) UnlockClients() {
	cm.mutex.Unlock()
}

// GetClient safely gets a client connection by ID (must call LockClients/UnlockClients around this)
func (cm *CommandManager) GetClient(chargePointID string) (*websocket.Conn, bool) {
	conn, exists := cm.clients[chargePointID]
	return conn, exists
}
