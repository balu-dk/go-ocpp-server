package ocppserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"ocpp-server/server/database"

	"github.com/gorilla/websocket"
)

// CentralSystemHandlerWithDB extends the CentralSystemHandler with database integration
type CentralSystemHandlerWithDB struct {
	upgrader       websocket.Upgrader
	clients        map[string]*websocket.Conn
	dbService      *database.Service
	commandManager *CommandManager
	cmdMgrInit     sync.Once
}

// NewCentralSystemHandlerWithDB creates a new handler with database integration
func NewCentralSystemHandlerWithDB(dbService *database.Service) *CentralSystemHandlerWithDB {
	return &CentralSystemHandlerWithDB{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow connections from all origins
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		clients:   make(map[string]*websocket.Conn),
		dbService: dbService,
	}
}

// ServeHTTP implements the http.Handler interface
func (cs *CentralSystemHandlerWithDB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cs.HandleWebSocket(w, r)
}

// GetCommandManager returns the command manager
func (cs *CentralSystemHandlerWithDB) GetCommandManager() *CommandManager {
	// Initialize command manager once if it doesn't exist
	cs.cmdMgrInit.Do(func() {
		cs.commandManager = NewCommandManager(cs.clients)
	})
	return cs.commandManager
}

// HandleWebSocket handles WebSocket connections from charge points
func (cs *CentralSystemHandlerWithDB) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := cs.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Extract charge point ID from the URL path
	chargePointID := extractChargePointIDFromURL(r.URL.Path)

	// Log the new connection
	cs.logEvent(chargePointID, "INFO", "System", fmt.Sprintf("New connection established from %s", chargePointID))

	// Update charge point connection status in the database
	cs.updateChargePointConnection(chargePointID, true)

	// Store the connection
	cs.clients[chargePointID] = conn

	// Handle incoming messages
	go cs.handleMessagesWithDB(chargePointID, conn)
}

// extractChargePointIDFromURL extracts the charge point ID from the URL path
func extractChargePointIDFromURL(path string) string {
	// Extract chargePointID from URL path
	chargePointID := strings.TrimPrefix(path, "/")
	if strings.Contains(chargePointID, "/") {
		parts := strings.Split(chargePointID, "/")
		chargePointID = parts[0]
	}
	if chargePointID == "" {
		chargePointID = "unknown"
	}
	return chargePointID
}

// handleMessagesWithDB handles messages with database integration
func (cs *CentralSystemHandlerWithDB) handleMessagesWithDB(chargePointID string, conn *websocket.Conn) {
	defer func() {
		conn.Close()
		delete(cs.clients, chargePointID)
		cs.updateChargePointConnection(chargePointID, false)
		cs.logEvent(chargePointID, "INFO", "System", fmt.Sprintf("Connection closed for %s", chargePointID))
	}()

	for {
		// Read message
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error reading message: %v", err))
			break
		}

		// Parse OCPP message
		log.Printf("Received message from %s: %s", chargePointID, message)

		// Parse the JSON array [MessageTypeId, UniqueId, Action, Payload]
		var ocppMsg []interface{}
		if err := json.Unmarshal(message, &ocppMsg); err != nil {
			log.Printf("Error parsing OCPP message: %v", err)
			cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error parsing OCPP message: %v", err))
			continue
		}

		// Check message format
		if len(ocppMsg) < 3 {
			log.Printf("Invalid OCPP message format: %s", message)
			cs.logEvent(chargePointID, "ERROR", "System", "Invalid OCPP message format")
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

				cs.logEvent(chargePointID, "ERROR", "ChargePoint",
					fmt.Sprintf("Error response for message %s: %s - %s",
						uniqueID, errorCode, errorDescription))
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

			// Log the incoming request
			cs.logEvent(chargePointID, "INFO", "ChargePoint", fmt.Sprintf("Received %s request", action))

			switch action {
			case "BootNotification":
				response = cs.handleBootNotificationRequestWithDB(chargePointID, payload)
			case "Heartbeat":
				response = cs.handleHeartbeatRequestWithDB(chargePointID)
			case "Authorize":
				response = cs.handleAuthorizeRequestWithDB(chargePointID, payload)
			case "StatusNotification":
				response = cs.handleStatusNotificationRequestWithDB(chargePointID, payload)
			case "StartTransaction":
				response = cs.handleStartTransactionRequestWithDB(chargePointID, payload)
			case "StopTransaction":
				response = cs.handleStopTransactionRequestWithDB(chargePointID, payload)
			case "MeterValues":
				response = cs.handleMeterValuesRequestWithDB(chargePointID, payload)
			default:
				log.Printf("Unsupported action: %s", action)
				cs.logEvent(chargePointID, "WARNING", "System", fmt.Sprintf("Unsupported action: %s", action))
				response = map[string]interface{}{} // Empty response for unknown actions
			}

			// Send response back (CallResult)
			callResult := []interface{}{3, uniqueID, response} // 3 = CallResult
			if err := conn.WriteJSON(callResult); err != nil {
				log.Printf("Error sending response: %v", err)
				cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error sending response: %v", err))
			} else {
				log.Printf("Sent response for %s: %+v", action, response)
				cs.logEvent(chargePointID, "INFO", "System", fmt.Sprintf("Sent response for %s", action))
			}
		} else {
			log.Printf("Received non-request message type: %v", msgTypeID)
		}
	}
}

// logEvent logs an event to the database
func (cs *CentralSystemHandlerWithDB) logEvent(chargePointID, level, source, message string) {
	logEntry := &database.Log{
		ChargePointID: chargePointID,
		Timestamp:     time.Now(),
		Level:         level,
		Source:        source,
		Message:       message,
	}

	if err := cs.dbService.AddLog(logEntry); err != nil {
		log.Printf("Error saving log to database: %v", err)
	}
}

// updateChargePointConnection updates the charge point connection status
func (cs *CentralSystemHandlerWithDB) updateChargePointConnection(chargePointID string, isConnected bool) {
	// Try to get existing charge point
	cp, err := cs.dbService.GetChargePoint(chargePointID)
	if err != nil {
		// If not found, create a new one with minimal info
		cp = &database.ChargePoint{
			ID:          chargePointID,
			Status:      "Unknown",
			IsConnected: isConnected,
			CreatedAt:   time.Now(),
			UpdatedAt:   time.Now(),
		}
	} else {
		// Update existing charge point
		cp.IsConnected = isConnected
		cp.UpdatedAt = time.Now()
	}

	if err := cs.dbService.SaveChargePoint(cp); err != nil {
		log.Printf("Error updating charge point connection status: %v", err)
	}
}

// handleBootNotificationRequestWithDB handles a BootNotification request with database integration
func (cs *CentralSystemHandlerWithDB) handleBootNotificationRequestWithDB(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Extract data from payload
	chargePointModel, _ := payload["chargePointModel"].(string)
	chargePointVendor, _ := payload["chargePointVendor"].(string)
	serialNumber, _ := payload["chargePointSerialNumber"].(string)
	firmwareVersion, _ := payload["firmwareVersion"].(string)

	// Log the information
	fmt.Printf("BootNotification from %s: Model=%s, Vendor=%s\n",
		chargePointID, chargePointModel, chargePointVendor)

	// Set default heartbeat interval
	heartbeatInterval := 60 // Default to 60 seconds if not configured

	// Get the heartbeat interval from config if available
	// This could be from environment variables or other configuration sources
	configHeartbeatInterval := cs.getConfiguredHeartbeatInterval()
	if configHeartbeatInterval > 0 {
		heartbeatInterval = configHeartbeatInterval
	}

	// Update or create charge point in database
	now := time.Now()
	cp, err := cs.dbService.GetChargePoint(chargePointID)

	if err != nil {
		// Create new charge point if not found
		cp = &database.ChargePoint{
			ID:                   chargePointID,
			Model:                chargePointModel,
			Vendor:               chargePointVendor,
			SerialNumber:         serialNumber,
			FirmwareVersion:      firmwareVersion,
			Status:               "Available",
			LastBootNotification: now,
			LastHeartbeat:        now,
			HeartbeatInterval:    heartbeatInterval, // Set the configured interval
			IsConnected:          true,
			CreatedAt:            now,
			UpdatedAt:            now,
		}
	} else {
		// Update existing charge point but preserve its heartbeat interval if set
		// Only update from config if the existing value is 0 or invalid
		if cp.HeartbeatInterval <= 0 {
			cp.HeartbeatInterval = heartbeatInterval
		}

		cp.Model = chargePointModel
		cp.Vendor = chargePointVendor
		cp.SerialNumber = serialNumber
		cp.FirmwareVersion = firmwareVersion
		cp.LastBootNotification = now
		cp.UpdatedAt = now
		cp.IsConnected = true
	}

	if err := cs.dbService.SaveChargePoint(cp); err != nil {
		log.Printf("Error saving charge point to database: %v", err)
		cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error saving charge point to database: %v", err))
	}

	// Log the heartbeat interval that's being sent
	log.Printf("Using heartbeat interval of %d seconds for charge point %s", cp.HeartbeatInterval, chargePointID)

	// Create response with the correct interval
	return map[string]interface{}{
		"currentTime": now.Format(time.RFC3339),
		"interval":    cp.HeartbeatInterval,
		"status":      "Accepted",
	}
}

// getConfiguredHeartbeatInterval retrieves the heartbeat interval from configuration
func (cs *CentralSystemHandlerWithDB) getConfiguredHeartbeatInterval() int {
	// Get from environment variable if set
	intervalStr := os.Getenv("OCPP_HEARTBEAT_INTERVAL")
	if intervalStr != "" {
		interval, err := strconv.Atoi(intervalStr)
		if err == nil && interval > 0 {
			return interval
		}
	}

	// Could add additional sources like config files here

	// Return default interval value
	return 60 // Default to 60 seconds
}

// handleHeartbeatRequestWithDB handles a Heartbeat request with database integration
func (cs *CentralSystemHandlerWithDB) handleHeartbeatRequestWithDB(chargePointID string) map[string]interface{} {
	fmt.Printf("Heartbeat from %s\n", chargePointID)

	// Update charge point heartbeat time
	now := time.Now()
	cp, err := cs.dbService.GetChargePoint(chargePointID)
	if err == nil {
		cp.LastHeartbeat = now
		cp.UpdatedAt = now
		if err := cs.dbService.SaveChargePoint(cp); err != nil {
			log.Printf("Error updating charge point heartbeat: %v", err)
			cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error updating charge point heartbeat: %v", err))
		}
	} else {
		log.Printf("Charge point not found for heartbeat: %s", chargePointID)
		cs.logEvent(chargePointID, "WARNING", "System", fmt.Sprintf("Charge point not found for heartbeat: %s", chargePointID))
	}

	// Create response
	return map[string]interface{}{
		"currentTime": now.Format(time.RFC3339),
	}
}

// handleAuthorizeRequestWithDB handles an Authorize request with database integration
func (cs *CentralSystemHandlerWithDB) handleAuthorizeRequestWithDB(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	idTag, _ := payload["idTag"].(string)
	fmt.Printf("Authorize request from %s: idTag=%s\n", chargePointID, idTag)

	// Check if the ID tag is authorized
	authStatus := "Accepted" // Default is to accept

	auth, err := cs.dbService.GetAuthorization(idTag)
	if err == nil {
		// ID tag found in database
		authStatus = auth.Status

		// Check if expired
		if auth.ExpiryDate.Before(time.Now()) && !auth.ExpiryDate.IsZero() {
			authStatus = "Expired"
			cs.logEvent(chargePointID, "INFO", "System", fmt.Sprintf("Authorization expired for idTag: %s", idTag))
		}
	} else {
		// ID tag not found, add it to database as accepted
		auth = &database.Authorization{
			IdTag:     idTag,
			Status:    "Accepted",
			CreatedAt: time.Now(),
			UpdatedAt: time.Now(),
		}
		if err := cs.dbService.SaveAuthorization(auth); err != nil {
			log.Printf("Error saving authorization to database: %v", err)
			cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error saving authorization to database: %v", err))
		}
	}

	// Log the authorization attempt
	cs.logEvent(chargePointID, "INFO", "System", fmt.Sprintf("Authorization attempt for idTag: %s, status: %s", idTag, authStatus))

	// Create response
	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": authStatus,
		},
	}
}

// handleStatusNotificationRequestWithDB handles a StatusNotification request with database integration
func (cs *CentralSystemHandlerWithDB) handleStatusNotificationRequestWithDB(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Extract status information
	status, _ := payload["status"].(string)
	connectorIdFloat, _ := payload["connectorId"].(float64)
	connectorId := int(connectorIdFloat)
	errorCode, _ := payload["errorCode"].(string)

	fmt.Printf("StatusNotification from %s: ConnectorId=%v, Status=%s, ErrorCode=%s\n",
		chargePointID, connectorId, status, errorCode)

	// Update connector status in database
	if connectorId == 0 {
		// Connector 0 is the charge point itself
		cp, err := cs.dbService.GetChargePoint(chargePointID)
		if err == nil {
			cp.Status = status
			cp.UpdatedAt = time.Now()
			if err := cs.dbService.SaveChargePoint(cp); err != nil {
				log.Printf("Error updating charge point status: %v", err)
				cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error updating charge point status: %v", err))
			}
		} else {
			log.Printf("Charge point not found for status update: %s", chargePointID)
			cs.logEvent(chargePointID, "WARNING", "System", fmt.Sprintf("Charge point not found for status update: %s", chargePointID))
		}
	} else {
		// Update or create connector
		connector, err := cs.dbService.GetConnector(chargePointID, connectorId)
		if err != nil {
			// Create new connector
			connector = &database.Connector{
				ChargePointID: chargePointID,
				ConnectorID:   connectorId,
				Status:        status,
				ErrorCode:     errorCode,
				UpdatedAt:     time.Now(),
			}
		} else {
			// Update existing connector
			connector.Status = status
			connector.ErrorCode = errorCode
			connector.UpdatedAt = time.Now()
		}

		if err := cs.dbService.SaveConnector(connector); err != nil {
			log.Printf("Error saving connector status: %v", err)
			cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error saving connector status: %v", err))
		}
	}

	// Log the status change
	cs.logEvent(chargePointID, "INFO", "ChargePoint",
		fmt.Sprintf("Status change for connector %d: %s, ErrorCode: %s", connectorId, status, errorCode))

	// StatusNotification requires an empty response according to the OCPP spec
	return map[string]interface{}{}
}

// handleStartTransactionRequestWithDB handles a StartTransaction request with database integration
func (cs *CentralSystemHandlerWithDB) handleStartTransactionRequestWithDB(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Extract transaction data
	idTag, _ := payload["idTag"].(string)
	connectorIdFloat, _ := payload["connectorId"].(float64)
	connectorId := int(connectorIdFloat)
	meterStartFloat, _ := payload["meterStart"].(float64)
	meterStart := int(meterStartFloat)
	timestampStr, _ := payload["timestamp"].(string)

	var timestamp time.Time
	var err error

	if timestampStr != "" {
		timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			timestamp = time.Now()
			log.Printf("Error parsing timestamp: %v, using current time", err)
		}
	} else {
		timestamp = time.Now()
	}

	fmt.Printf("StartTransaction from %s: ConnectorId=%v, IdTag=%s, MeterStart=%v\n",
		chargePointID, connectorId, idTag, meterStart)

	// Check authorization status
	authStatus := "Accepted"
	auth, err := cs.dbService.GetAuthorization(idTag)
	if err == nil {
		authStatus = auth.Status
		if auth.ExpiryDate.Before(time.Now()) && !auth.ExpiryDate.IsZero() {
			authStatus = "Expired"
		}
	}

	// Generate a transaction ID
	transactionId := int(time.Now().Unix() % 100000)

	// Create new transaction in database
	transaction := &database.Transaction{
		TransactionID:  transactionId,
		ChargePointID:  chargePointID,
		ConnectorID:    connectorId,
		IdTag:          idTag,
		StartTimestamp: timestamp,
		MeterStart:     meterStart,
		IsComplete:     false,
	}

	if err := cs.dbService.CreateTransaction(transaction); err != nil {
		log.Printf("Error saving transaction to database: %v", err)
		cs.logEvent(chargePointID, "ERROR", "System", fmt.Sprintf("Error saving transaction to database: %v", err))
	}

	// Update connector status to indicate it's charging
	connector, err := cs.dbService.GetConnector(chargePointID, connectorId)
	if err == nil {
		connector.Status = "Charging"
		connector.UpdatedAt = time.Now()
		if err := cs.dbService.SaveConnector(connector); err != nil {
			log.Printf("Error updating connector status: %v", err)
		}
	}

	// Log the transaction start
	cs.logEvent(chargePointID, "INFO", "System",
		fmt.Sprintf("Transaction %d started on connector %d with idTag %s", transactionId, connectorId, idTag))

	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": authStatus,
		},
		"transactionId": transactionId,
	}
}

// handleStopTransactionRequestWithDB handles a StopTransaction request with database integration
func (cs *CentralSystemHandlerWithDB) handleStopTransactionRequestWithDB(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Extract data from payload
	transactionIdFloat, _ := payload["transactionId"].(float64)
	transactionId := int(transactionIdFloat)
	meterStopFloat, _ := payload["meterStop"].(float64)
	meterStop := int(meterStopFloat)
	reason, _ := payload["reason"].(string)
	timestampStr, _ := payload["timestamp"].(string)
	idTag, _ := payload["idTag"].(string)

	var timestamp time.Time
	var err error

	if timestampStr != "" {
		timestamp, err = time.Parse(time.RFC3339, timestampStr)
		if err != nil {
			timestamp = time.Now()
			log.Printf("Error parsing timestamp: %v, using current time", err)
		}
	} else {
		timestamp = time.Now()
	}

	fmt.Printf("StopTransaction from %s: TransactionId=%v, MeterStop=%v, Reason=%s\n",
		chargePointID, transactionId, meterStop, reason)

	// Update transaction in database
	transaction, err := cs.dbService.GetTransaction(transactionId)
	if err != nil {
		log.Printf("Transaction %d not found: %v", transactionId, err)
		cs.logEvent(chargePointID, "ERROR", "System",
			fmt.Sprintf("Transaction %d not found when stopping: %v", transactionId, err))
	} else {
		transaction.StopTimestamp = timestamp
		transaction.MeterStop = meterStop
		transaction.StopReason = reason
		transaction.IsComplete = true

		// Calculate energy delivered in kWh
		energyWh := float64(meterStop - transaction.MeterStart)
		transaction.EnergyDelivered = energyWh / 1000.0 // Convert Wh to kWh

		if err := cs.dbService.UpdateTransaction(transaction); err != nil {
			log.Printf("Error updating transaction: %v", err)
			cs.logEvent(chargePointID, "ERROR", "System",
				fmt.Sprintf("Error updating transaction %d: %v", transactionId, err))
		}

		// Update connector status back to available
		connector, err := cs.dbService.GetConnector(chargePointID, transaction.ConnectorID)
		if err == nil {
			connector.Status = "Available"
			connector.UpdatedAt = time.Now()
			if err := cs.dbService.SaveConnector(connector); err != nil {
				log.Printf("Error updating connector status: %v", err)
			}
		}

		// Log the transaction stop
		cs.logEvent(chargePointID, "INFO", "System",
			fmt.Sprintf("Transaction %d stopped. Energy delivered: %.2f kWh, Reason: %s",
				transactionId, transaction.EnergyDelivered, reason))
	}

	// Check authorization status for the provided idTag (if any)
	authStatus := "Accepted"
	if idTag != "" {
		auth, err := cs.dbService.GetAuthorization(idTag)
		if err == nil {
			authStatus = auth.Status
			if auth.ExpiryDate.Before(time.Now()) && !auth.ExpiryDate.IsZero() {
				authStatus = "Expired"
			}
		}
	}

	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": authStatus,
		},
	}
}

// handleMeterValuesRequestWithDB handles a MeterValues request with database integration
func (cs *CentralSystemHandlerWithDB) handleMeterValuesRequestWithDB(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Extract data from payload
	connectorIdFloat, _ := payload["connectorId"].(float64)
	connectorId := int(connectorIdFloat)
	transactionIdFloat, _ := payload["transactionId"].(float64)
	transactionId := int(transactionIdFloat)

	fmt.Printf("MeterValues from %s: ConnectorId=%v, TransactionId=%v\n",
		chargePointID, connectorId, transactionId)

	// Process meter values if available
	if meterValues, ok := payload["meterValue"].([]interface{}); ok && len(meterValues) > 0 {
		for _, meterValueInterface := range meterValues {
			if meterValue, ok := meterValueInterface.(map[string]interface{}); ok {
				// Extract timestamp
				timestampStr, _ := meterValue["timestamp"].(string)
				var timestamp time.Time
				var err error

				if timestampStr != "" {
					timestamp, err = time.Parse(time.RFC3339, timestampStr)
					if err != nil {
						timestamp = time.Now()
						log.Printf("Error parsing meter value timestamp: %v, using current time", err)
					}
				} else {
					timestamp = time.Now()
				}

				// Process sampledValue array
				if sampledValues, ok := meterValue["sampledValue"].([]interface{}); ok {
					for _, sampledValueInterface := range sampledValues {
						if sampledValue, ok := sampledValueInterface.(map[string]interface{}); ok {
							// Extract value and metadata
							valueStr, _ := sampledValue["value"].(string)
							var value float64
							fmt.Sscanf(valueStr, "%f", &value)

							unit, _ := sampledValue["unit"].(string)
							if unit == "" {
								unit = "Wh" // Default unit according to OCPP
							}

							measurand, _ := sampledValue["measurand"].(string)
							if measurand == "" {
								measurand = "Energy.Active.Import.Register" // Default measurand according to OCPP
							}

							// Save meter value to database
							meterValueRecord := &database.MeterValue{
								TransactionID: transactionId,
								ChargePointID: chargePointID,
								ConnectorID:   connectorId,
								Timestamp:     timestamp,
								Value:         value,
								Unit:          unit,
								Measurand:     measurand,
							}

							if err := cs.dbService.SaveMeterValue(meterValueRecord); err != nil {
								log.Printf("Error saving meter value: %v", err)
								cs.logEvent(chargePointID, "ERROR", "System",
									fmt.Sprintf("Error saving meter value for transaction %d: %v", transactionId, err))
							}
						}
					}
				}
			}
		}

		cs.logEvent(chargePointID, "INFO", "ChargePoint",
			fmt.Sprintf("Received meter values for transaction %d, connector %d", transactionId, connectorId))
	}

	// MeterValues requires an empty response according to the OCPP spec
	return map[string]interface{}{}
}
