package ocppserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// OCPP message types
const (
	BootNotificationMsg = "BootNotification"
	HeartbeatMsg        = "Heartbeat"
	AuthorizeMsg        = "Authorize"
)

// BootNotificationRequest represents a request from a charge point
type BootNotificationRequest struct {
	ChargePointModel        string `json:"chargePointModel"`
	ChargePointVendor       string `json:"chargePointVendor"`
	ChargePointSerialNumber string `json:"chargePointSerialNumber,omitempty"`
	FirmwareVersion         string `json:"firmwareVersion,omitempty"`
}

// BootNotificationConfirmation represents a response to a charge point
type BootNotificationConfirmation struct {
	CurrentTime string `json:"currentTime"`
	Interval    int    `json:"interval"`
	Status      string `json:"status"`
}

// HeartbeatRequest represents a heartbeat request
type HeartbeatRequest struct{}

// HeartbeatConfirmation represents a response to a heartbeat
type HeartbeatConfirmation struct {
	CurrentTime string `json:"currentTime"`
}

// AuthorizeRequest represents an authorization request
type AuthorizeRequest struct {
	IdTag string `json:"idTag"`
}

// IdTagInfo contains information about an IdTag
type IdTagInfo struct {
	Status string `json:"status"`
}

// AuthorizeConfirmation represents a response to an authorization request
type AuthorizeConfirmation struct {
	IdTagInfo IdTagInfo `json:"idTagInfo"`
}

// OCPPHandler defines the interface for OCPP message handlers
type OCPPHandler interface {
	http.Handler
	GetCommandManager() *CommandManager
}

// CentralSystemHandler handles OCPP requests from charge points
type CentralSystemHandler struct {
	upgrader       websocket.Upgrader
	clients        map[string]*websocket.Conn
	commandManager *CommandManager
	cmdMgrInit     sync.Once
}

// NewCentralSystemHandler creates a new handler for the central system
func NewCentralSystemHandler() *CentralSystemHandler {
	return &CentralSystemHandler{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow connections from all origins
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		clients: make(map[string]*websocket.Conn),
	}
}

// ServeHTTP implements the http.Handler interface
func (cs *CentralSystemHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cs.HandleWebSocket(w, r)
}

// GetCommandManager returns the command manager
func (cs *CentralSystemHandlerWithDB) GetCommandManager() *CommandManager {
	// Initialize command manager once if it doesn't exist
	cs.cmdMgrInit.Do(func() {
		cs.commandManager = NewCommandManager(cs.clients, cs.dbService)
	})
	return cs.commandManager
}

// GetCommandManager returns the command manager
func (cs *CentralSystemHandler) GetCommandManager() *CommandManager {
	// Initialize command manager once if it doesn't exist
	cs.cmdMgrInit.Do(func() {
		// Since CentralSystemHandler doesn't have a dbService, pass nil
		cs.commandManager = NewCommandManager(cs.clients, nil)
	})
	return cs.commandManager
}

// HandleWebSocket handles WebSocket connections from charge points
func (cs *CentralSystemHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := cs.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Get chargePointID directly from URL path
	// Format: localhost:9000/{id}
	chargePointID := strings.TrimPrefix(r.URL.Path, "/")

	// Handle case where there could be multiple slashes
	if strings.Contains(chargePointID, "/") {
		parts := strings.Split(chargePointID, "/")
		chargePointID = parts[0] // Take only the first part
	}

	if chargePointID == "" {
		chargePointID = "unknown"
	}

	// Store the connection
	cs.clients[chargePointID] = conn
	log.Printf("New connection from %s", chargePointID)

	// Handle incoming messages - ensure chargePointID doesn't contain '/'
	sanitizedID := chargePointID
	go cs.handleMessages(sanitizedID, conn)
}

// handleAuthorizeRequest handles an Authorize request
func (cs *CentralSystemHandler) handleAuthorizeRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	idTag, _ := payload["idTag"].(string)
	fmt.Printf("Authorize request from %s: idTag=%s\n", chargePointID, idTag)

	// Create response
	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": "Accepted",
		},
	}
}

// handleMessages handles messages from a charge point
func (cs *CentralSystemHandler) handleMessages(chargePointID string, conn *websocket.Conn) {
	defer func() {
		conn.Close()
		delete(cs.clients, chargePointID)
		log.Printf("Connection closed for %s", chargePointID)
	}()

	for {
		// Read message
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}

		// Parse OCPP message
		log.Printf("Received message from %s: %s", chargePointID, message)

		// Try to parse JSON array [MessageTypeId, UniqueId, Action, Payload]
		var ocppMsg []interface{}
		if err := json.Unmarshal(message, &ocppMsg); err != nil {
			log.Printf("Error parsing OCPP message: %v", err)
			continue
		}

		// Check that the message has the expected format
		if len(ocppMsg) < 3 {
			log.Printf("Invalid OCPP message format: %s", message)
			continue
		}

		// Message type (2 = Request, 3 = Response, 4 = Error)
		msgTypeID, ok := ocppMsg[0].(float64)
		if !ok {
			log.Printf("Invalid message type ID: %v", ocppMsg[0])
			continue
		}

		// Message unique ID
		uniqueID, ok := ocppMsg[1].(string)
		if !ok {
			log.Printf("Invalid unique ID: %v", ocppMsg[1])
			continue
		}

		// If it's a response (CallResult, msgTypeID == 3)
		if msgTypeID == 3 {
			// Process response to a command we sent
			if len(ocppMsg) > 2 {
				responsePayload, ok := ocppMsg[2].(map[string]interface{})
				if !ok && ocppMsg[2] != nil {
					log.Printf("Invalid response payload format: %v", ocppMsg[2])
					continue
				}

				// Pass to command manager to handle
				cs.GetCommandManager().HandleCommandResponse(uniqueID, responsePayload)
			}
			continue
		}

		// If it's an error response (CallError, msgTypeID == 4)
		if msgTypeID == 4 {
			// Parse error details
			if len(ocppMsg) >= 5 {
				errorCode, _ := ocppMsg[2].(string)
				errorDescription, _ := ocppMsg[3].(string)
				errorDetails, _ := ocppMsg[4].(map[string]interface{})

				log.Printf("Received error response: code=%s, description=%s, details=%v",
					errorCode, errorDescription, errorDetails)
			}
			continue
		}

		// If it's a request (msgTypeID == 2)
		if msgTypeID == 2 {
			action, ok := ocppMsg[2].(string)
			if !ok {
				log.Printf("Invalid action: %v", ocppMsg[2])
				continue
			}

			// Handle different action types
			var payload map[string]interface{}
			var response interface{}

			if len(ocppMsg) > 3 {
				payload, ok = ocppMsg[3].(map[string]interface{})
				if !ok {
					log.Printf("Invalid payload format: %v", ocppMsg[3])
					continue
				}
			}

			switch action {
			case "BootNotification":
				response = cs.handleBootNotificationRequest(chargePointID, payload)
			case "Heartbeat":
				response = cs.handleHeartbeatRequest(chargePointID)
			case "Authorize":
				response = cs.handleAuthorizeRequest(chargePointID, payload)
			case "StatusNotification":
				response = cs.handleStatusNotificationRequest(chargePointID, payload)
			case "StartTransaction":
				response = cs.handleStartTransactionRequest(chargePointID, payload)
			case "StopTransaction":
				response = cs.handleStopTransactionRequest(chargePointID, payload)
			case "MeterValues":
				response = cs.handleMeterValuesRequest(chargePointID, payload)
			default:
				log.Printf("Unsupported action: %s", action)
				response = map[string]interface{}{} // Send empty response for unknown actions
			}

			// Send response back (CallResult)
			callResult := []interface{}{3, uniqueID, response} // 3 = CallResult
			if err := conn.WriteJSON(callResult); err != nil {
				log.Printf("Error sending response: %v", err)
			} else {
				log.Printf("Sent response for %s: %+v", action, response)
			}
		} else {
			log.Printf("Received non-request message type: %v", msgTypeID)
		}
	}
}

// handleBootNotificationRequest handles a BootNotification request
func (cs *CentralSystemHandler) handleBootNotificationRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Log incoming data
	chargePointModel, _ := payload["chargePointModel"].(string)
	chargePointVendor, _ := payload["chargePointVendor"].(string)

	fmt.Printf("BootNotification from %s: Model=%s, Vendor=%s\n",
		chargePointID, chargePointModel, chargePointVendor)

	// Create response
	return map[string]interface{}{
		"currentTime": time.Now().Format(time.RFC3339),
		"interval":    60,
		"status":      "Accepted",
	}
}

// handleHeartbeatRequest handles a Heartbeat request
func (cs *CentralSystemHandler) handleHeartbeatRequest(chargePointID string) map[string]interface{} {
	fmt.Printf("Heartbeat from %s\n", chargePointID)

	// Create response
	return map[string]interface{}{
		"currentTime": time.Now().Format(time.RFC3339),
	}
}

// handleStatusNotificationRequest handles a StatusNotification request
func (cs *CentralSystemHandler) handleStatusNotificationRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Extract important status information
	status, _ := payload["status"].(string)
	connectorId, _ := payload["connectorId"].(float64)
	errorCode, _ := payload["errorCode"].(string)

	fmt.Printf("StatusNotification from %s: ConnectorId=%v, Status=%s, ErrorCode=%s\n",
		chargePointID, connectorId, status, errorCode)

	// StatusNotification requires an empty response according to the OCPP specification
	return map[string]interface{}{}
}

// handleStartTransactionRequest handles a StartTransaction request
func (cs *CentralSystemHandler) handleStartTransactionRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	idTag, _ := payload["idTag"].(string)
	connectorId, _ := payload["connectorId"].(float64)
	meterStart, _ := payload["meterStart"].(float64)

	fmt.Printf("StartTransaction from %s: ConnectorId=%v, IdTag=%s, MeterStart=%v\n",
		chargePointID, connectorId, idTag, meterStart)

	// Generate a random transaction ID (in reality you would use a database)
	transactionId := int(time.Now().Unix() % 10000)

	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": "Accepted",
		},
		"transactionId": transactionId,
	}
}

// handleStopTransactionRequest handles a StopTransaction request
func (cs *CentralSystemHandler) handleStopTransactionRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	transactionId, _ := payload["transactionId"].(float64)
	meterStop, _ := payload["meterStop"].(float64)
	reason, _ := payload["reason"].(string)

	fmt.Printf("StopTransaction from %s: TransactionId=%v, MeterStop=%v, Reason=%s\n",
		chargePointID, transactionId, meterStop, reason)

	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": "Accepted",
		},
	}
}

// handleMeterValuesRequest handles a MeterValues request
func (cs *CentralSystemHandler) handleMeterValuesRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	connectorId, _ := payload["connectorId"].(float64)
	transactionId, _ := payload["transactionId"].(float64)

	fmt.Printf("MeterValues from %s: ConnectorId=%v, TransactionId=%v\n",
		chargePointID, connectorId, transactionId)

	// Log meter values if available
	if meterValues, ok := payload["meterValue"].([]interface{}); ok && len(meterValues) > 0 {
		fmt.Printf("  Received %d meter values\n", len(meterValues))
	}

	// MeterValues requires an empty response according to the OCPP specification
	return map[string]interface{}{}
}
