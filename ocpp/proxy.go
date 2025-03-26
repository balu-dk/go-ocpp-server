package ocppserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"ocpp-server/server/database"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// ProxyManager manages proxy connections to other central systems
type ProxyManager struct {
	dbService            *database.Service
	proxyConnections     map[uint]map[string]*websocket.Conn // Maps ProxyDestinationID -> ChargePointID -> Connection
	transformedIDs       map[string]string                   // Maps original ID -> transformed ID
	reverseIDMapping     map[string]string                   // Maps transformed ID -> original ID
	mutex                sync.RWMutex
	messageProcessors    []MessageProcessor
	centralHandler       *CentralSystemHandlerWithDB
	pendingProxyRequests map[string]chan []byte // Maps message ID -> response channel
	pendingMutex         sync.RWMutex           // Mutex for the pendingProxyRequests map
}

// MessageProcessor defines functions that can process messages before forwarding
type MessageProcessor interface {
	ProcessOutgoing(chargePointID string, message []byte) ([]byte, bool, error) // Returns: modified message, should block, error
	ProcessIncoming(chargePointID string, message []byte) ([]byte, bool, error) // Returns: modified message, should block, error
}

// NewProxyManager creates a new proxy manager
func NewProxyManager(dbService *database.Service) *ProxyManager {
	return &ProxyManager{
		dbService:            dbService,
		proxyConnections:     make(map[uint]map[string]*websocket.Conn),
		transformedIDs:       make(map[string]string),
		reverseIDMapping:     make(map[string]string),
		messageProcessors:    make([]MessageProcessor, 0),
		pendingProxyRequests: make(map[string]chan []byte),
	}
}

// RegisterMessageProcessor adds a message processor to the pipeline
func (pm *ProxyManager) RegisterMessageProcessor(processor MessageProcessor) {
	pm.messageProcessors = append(pm.messageProcessors, processor)
}

// GetTransformedID returns the transformed ID for a charge point
func (pm *ProxyManager) GetTransformedID(originalID string, proxyConfig *database.ChargePointProxy) string {
	if proxyConfig == nil || (proxyConfig.IDTransformPrefix == "" && proxyConfig.IDTransformSuffix == "") {
		return originalID
	}

	transformedID := proxyConfig.IDTransformPrefix + originalID + proxyConfig.IDTransformSuffix

	// Store the mapping
	pm.mutex.Lock()
	pm.transformedIDs[originalID] = transformedID
	pm.reverseIDMapping[transformedID] = originalID
	pm.mutex.Unlock()

	return transformedID
}

// GetOriginalID returns the original ID for a transformed ID
func (pm *ProxyManager) GetOriginalID(transformedID string) (string, bool) {
	pm.mutex.RLock()
	defer pm.mutex.RUnlock()

	originalID, exists := pm.reverseIDMapping[transformedID]
	return originalID, exists
}

// ConnectToProxies establishes connections to all proxy destinations for a charge point
func (pm *ProxyManager) ConnectToProxies(chargePointID string) error {
	// Check if proxying is enabled for this charge point
	proxyConfig, err := pm.dbService.GetChargePointProxy(chargePointID)
	if err != nil || !proxyConfig.ProxyEnabled {
		log.Printf("Proxying not enabled for charge point %s", chargePointID)
		return nil
	}

	// Get active proxy destinations for this charge point
	destinations, err := pm.dbService.GetActiveProxyDestinationsForChargePoint(chargePointID)
	if err != nil {
		return fmt.Errorf("failed to get proxy destinations: %v", err)
	}

	if len(destinations) == 0 {
		log.Printf("No active proxy destinations for charge point %s", chargePointID)
		return nil
	}

	// Get the transformed ID for this charge point
	transformedID := pm.GetTransformedID(chargePointID, proxyConfig)

	// Connect to each proxy destination
	for _, dest := range destinations {
		if err := pm.connectToProxy(chargePointID, transformedID, &dest); err != nil {
			log.Printf("Failed to connect to proxy %s: %v", dest.Name, err)
			// Continue to next proxy even if this one fails
			continue
		}
	}

	return nil
}

// connectToProxy establishes a connection to a single proxy destination
func (pm *ProxyManager) connectToProxy(originalID, transformedID string, destination *database.ProxyDestination) error {
	// Create the WebSocket URL with the transformed ID
	// For OCPP, the URL format is typically {baseURL}/{chargePointID}
	proxyURL := fmt.Sprintf("%s/%s", destination.URL, transformedID)

	log.Printf("Connecting to proxy %s at %s with ID %s", destination.Name, proxyURL, transformedID)

	// Set up custom headers if needed
	header := http.Header{}
	header.Add("Sec-WebSocket-Protocol", "ocpp1.6")

	// Connect to the proxy
	conn, _, err := websocket.DefaultDialer.Dial(proxyURL, header)
	if err != nil {
		return fmt.Errorf("failed to connect to proxy %s: %v", destination.Name, err)
	}

	// Store the connection
	pm.mutex.Lock()
	if _, exists := pm.proxyConnections[destination.ID]; !exists {
		pm.proxyConnections[destination.ID] = make(map[string]*websocket.Conn)
	}
	pm.proxyConnections[destination.ID][originalID] = conn
	pm.mutex.Unlock()

	// Log the connection
	log.Printf("Successfully connected to proxy %s for charge point %s", destination.Name, originalID)

	// Start a goroutine to handle incoming messages from this proxy
	go pm.handleProxyMessages(originalID, destination.ID, conn)

	return nil
}

func getMessageTypeLog(message []byte) string {
	var ocppMsg []interface{}
	if err := json.Unmarshal(message, &ocppMsg); err != nil {
		return "unknown (JSON parse error)"
	}

	if len(ocppMsg) < 3 {
		return "invalid (insufficient elements)"
	}

	msgTypeID, ok := ocppMsg[0].(float64)
	if !ok {
		return "unknown message type"
	}

	if msgTypeID == 2 && len(ocppMsg) >= 3 {
		// This is a Call message with an action
		action, ok := ocppMsg[2].(string)
		if ok {
			messageID, _ := ocppMsg[1].(string)
			return fmt.Sprintf("Call(%s) ID=%s", action, messageID)
		}
	}

	switch int(msgTypeID) {
	case 2:
		return "Call"
	case 3:
		return "CallResult"
	case 4:
		return "CallError"
	default:
		return fmt.Sprintf("Unknown(%v)", msgTypeID)
	}
}

func (pm *ProxyManager) LogTransactionCommand(chargePointID string, message []byte) {
	var ocppMsg []interface{}
	if err := json.Unmarshal(message, &ocppMsg); err != nil {
		log.Printf("Error parsing transaction command: %v", err)
		return
	}

	if len(ocppMsg) < 4 {
		return
	}

	msgTypeID, ok := ocppMsg[0].(float64)
	if !ok || msgTypeID != 2 {
		return // Not a call message
	}

	action, ok := ocppMsg[2].(string)
	if !ok {
		return
	}

	// Only process transaction-related commands
	if action != "RemoteStartTransaction" && action != "RemoteStopTransaction" {
		return
	}

	payload, ok := ocppMsg[3].(map[string]interface{})
	if !ok {
		return
	}

	// Log detailed information
	messageID, _ := ocppMsg[1].(string)
	log.Printf("Transaction command for %s: Action=%s, MessageID=%s", chargePointID, action, messageID)

	// Add specific handling based on command type
	if action == "RemoteStartTransaction" {
		idTag, _ := payload["idTag"].(string)
		connectorID, hasConnector := payload["connectorId"].(float64)

		if hasConnector {
			log.Printf("  RemoteStart: idTag=%s, connectorId=%v", idTag, connectorID)
		} else {
			log.Printf("  RemoteStart: idTag=%s, no specific connector", idTag)
		}

		// Check if we already have a pending remote start
		if pm.dbService != nil {
			var connID int
			if hasConnector {
				connID = int(connectorID)
			}

			pending, err := pm.dbService.GetPendingRemoteStart(chargePointID, connID)
			if err == nil && pending != nil {
				log.Printf("  Found existing pending remote start: idTag=%s, completed=%v, expired=%v",
					pending.IdTag, pending.Completed, pending.Expired)
			}
		}

	} else if action == "RemoteStopTransaction" {
		txID, hasTxID := payload["transactionId"].(float64)

		if hasTxID {
			log.Printf("  RemoteStop: transactionId=%v", txID)

			// Look up transaction in database for additional info
			if pm.dbService != nil {
				tx, err := pm.dbService.GetTransaction(int(txID))
				if err != nil {
					log.Printf("  Warning: Transaction %v not found in database: %v", txID, err)

					// Check for any incomplete transactions on this charge point
					incTx, err := pm.dbService.GetIncompleteTransactions(chargePointID)
					if err != nil {
						log.Printf("  Error getting incomplete transactions: %v", err)
					} else if len(incTx) > 0 {
						log.Printf("  Found %d incomplete transactions for %s:", len(incTx), chargePointID)
						for _, tx := range incTx {
							log.Printf("    TransactionID=%d, ConnectorID=%d, IdTag=%s",
								tx.TransactionID, tx.ConnectorID, tx.IdTag)
						}
					} else {
						log.Printf("  No incomplete transactions found for charge point %s", chargePointID)
					}

				} else {
					log.Printf("  Found transaction %v: CP=%s, Connector=%d, IdTag=%s, Complete=%v",
						txID, tx.ChargePointID, tx.ConnectorID, tx.IdTag, tx.IsComplete)

					// If transaction is for a different charge point, this could be a problem
					if tx.ChargePointID != chargePointID {
						log.Printf("  WARNING: Transaction %v belongs to charge point %s but command sent to %s",
							txID, tx.ChargePointID, chargePointID)
					}
				}
			}
		} else {
			log.Printf("  RemoteStop: No transactionId provided")
		}
	}
}

// handleProxyMessages processes messages coming from a proxy server
func (pm *ProxyManager) handleProxyMessages(chargePointID string, destinationID uint, conn *websocket.Conn) {
	defer func() {
		conn.Close()
		// Remove the connection from our map
		pm.mutex.Lock()
		delete(pm.proxyConnections[destinationID], chargePointID)
		pm.mutex.Unlock()
		log.Printf("Connection closed for charge point %s to proxy %d", chargePointID, destinationID)
	}()

	for {
		// Read message from proxy
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading from proxy: %v", err)
			break
		}

		// Log the raw message for debugging
		log.Printf("PROXY → CHARGER [%d→%s]: %s", destinationID, chargePointID, string(message))

		// Save to database for logging
		proxyLog := &database.ProxyMessageLog{
			ChargePointID:      chargePointID,
			ProxyDestinationID: destinationID,
			Timestamp:          time.Now(),
			Direction:          "FROM_PROXY",
			OriginalMessage:    string(message),
		}

		// Parse the message but don't modify it
		var ocppMsg []interface{}
		if err := json.Unmarshal(message, &ocppMsg); err != nil {
			log.Printf("Error parsing message from proxy (will forward anyway): %v", err)
		} else {
			// Just extract basic info for logging
			if len(ocppMsg) >= 3 {
				msgType, typeOk := ocppMsg[0].(float64)
				messageID, idOk := ocppMsg[1].(string)

				if typeOk && idOk && msgType == 2 && len(ocppMsg) >= 4 {
					// This is a command (Call) message
					action, actionOk := ocppMsg[2].(string)
					if actionOk {
						log.Printf("Proxy sent %s command (ID: %s) for charge point %s",
							action, messageID, chargePointID)
					}
				}
			}
		}

		// Process the message through ID transformer only - no other processors
		modifiedMessage := message
		for _, processor := range pm.messageProcessors {
			// Only apply ID transformer, skip other processors
			if _, ok := processor.(*IDTransformer); ok {
				var err error
				modifiedMessage, _, err = processor.ProcessIncoming(chargePointID, modifiedMessage)
				if err != nil {
					log.Printf("Error in ID transformation: %v", err)
					// Continue with original message
					modifiedMessage = message
					break
				}
			}
		}

		// Only log if the message was modified
		if string(message) != string(modifiedMessage) {
			proxyLog.WasModified = true
			proxyLog.TransformedMessage = string(modifiedMessage)
			log.Printf("Message was transformed for %s", chargePointID)
		}

		// Save to log database
		if err := pm.dbService.SaveProxyMessageLog(proxyLog); err != nil {
			log.Printf("Error saving proxy message log: %v", err)
		}

		// Forward directly to charge point regardless of message type
		if pm.centralHandler == nil {
			log.Printf("ERROR: Cannot forward message - no central handler available")
			continue
		}

		// Just forward the message directly to the charge point
		log.Printf("Forwarding message from proxy to charge point %s", chargePointID)
		err = pm.centralHandler.ForwardMessageToChargePoint(chargePointID, modifiedMessage)
		if err != nil {
			log.Printf("ERROR: Failed to forward message to charge point %s: %v", chargePointID, err)
		} else {
			log.Printf("Successfully forwarded message to charge point %s", chargePointID)
		}
	}
}

// ForwardToProxies forwards a message from a charge point to all proxy destinations
func (pm *ProxyManager) ForwardToProxies(chargePointID string, message []byte) {
	// Check if proxying is enabled for this charge point
	proxyConfig, err := pm.dbService.GetChargePointProxy(chargePointID)
	if err != nil || !proxyConfig.ProxyEnabled {
		return
	}

	// Get active proxy destinations for this charge point
	destinations, err := pm.dbService.GetActiveProxyDestinationsForChargePoint(chargePointID)
	if err != nil {
		log.Printf("Failed to get proxy destinations: %v", err)
		return
	}

	if len(destinations) == 0 {
		return
	}

	// Log the message for debugging
	log.Printf("CHARGER → PROXY [%s→]: %s", chargePointID, string(message))

	// Parse the message for ID transformation
	var ocppMsg []interface{}
	if err := json.Unmarshal(message, &ocppMsg); err != nil {
		log.Printf("Error parsing message for forwarding: %v", err)
		// Continue anyway with the original message
	}

	// Forward to each configured proxy destination
	for _, dest := range destinations {
		destID := dest.ID

		// Log the original message
		proxyLog := &database.ProxyMessageLog{
			ChargePointID:      chargePointID,
			ProxyDestinationID: destID,
			Timestamp:          time.Now(),
			Direction:          "TO_PROXY",
			OriginalMessage:    string(message),
		}

		// Process the message through transformers only
		modifiedMessage := message
		for _, processor := range pm.messageProcessors {
			// Only apply ID transformer, skip other processors
			if _, ok := processor.(*IDTransformer); ok {
				var err error
				modifiedMessage, _, err = processor.ProcessOutgoing(chargePointID, modifiedMessage)
				if err != nil {
					log.Printf("Error in ID transformation: %v", err)
					// Continue with original message
					modifiedMessage = message
					break
				}
			}
		}

		// Check if the message was modified
		wasModified := string(message) != string(modifiedMessage)
		if wasModified {
			proxyLog.WasModified = true
			proxyLog.TransformedMessage = string(modifiedMessage)
		}

		// Save to log database
		if err := pm.dbService.SaveProxyMessageLog(proxyLog); err != nil {
			log.Printf("Error saving proxy message log: %v", err)
		}

		// Get the connection
		pm.mutex.RLock()
		connections, exists := pm.proxyConnections[destID]
		if !exists {
			pm.mutex.RUnlock()
			log.Printf("No connections for proxy destination %d", destID)
			continue
		}

		conn, exists := connections[chargePointID]
		pm.mutex.RUnlock()

		if !exists || conn == nil {
			log.Printf("No connection for charge point %s to proxy %d", chargePointID, destID)

			// Try to reconnect
			if err := pm.connectToProxy(chargePointID, pm.GetTransformedID(chargePointID, proxyConfig), &dest); err != nil {
				log.Printf("Failed to reconnect to proxy: %v", err)
				continue
			}

			// Get the new connection
			pm.mutex.RLock()
			conn = pm.proxyConnections[destID][chargePointID]
			pm.mutex.RUnlock()

			if conn == nil {
				log.Printf("Still no connection after reconnect attempt")
				continue
			}
		}

		// Send the modified message to the proxy
		log.Printf("Sending message to proxy %s", dest.Name)
		if err := conn.WriteMessage(websocket.TextMessage, modifiedMessage); err != nil {
			log.Printf("Error writing to proxy: %v", err)

			// Close the connection on error
			conn.Close()

			// Remove from our map
			pm.mutex.Lock()
			delete(pm.proxyConnections[destID], chargePointID)
			pm.mutex.Unlock()
		} else {
			log.Printf("Successfully forwarded message to proxy %s", dest.Name)
		}
	}
}

// DisconnectAllProxies closes all proxy connections
func (pm *ProxyManager) DisconnectAllProxies() {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for destID, connections := range pm.proxyConnections {
		for chargePointID, conn := range connections {
			if conn != nil {
				conn.Close()
				log.Printf("Closed connection for charge point %s to proxy %d", chargePointID, destID)
			}
		}
	}

	// Clear the maps
	pm.proxyConnections = make(map[uint]map[string]*websocket.Conn)
}

// DisconnectProxiesForChargePoint closes all proxy connections for a specific charge point
func (pm *ProxyManager) DisconnectProxiesForChargePoint(chargePointID string) {
	pm.mutex.Lock()
	defer pm.mutex.Unlock()

	for destID, connections := range pm.proxyConnections {
		if conn, exists := connections[chargePointID]; exists && conn != nil {
			conn.Close()
			delete(connections, chargePointID)
			log.Printf("Closed connection for charge point %s to proxy %d", chargePointID, destID)
		}
	}
}

// IDTransformer transforms charge point IDs in OCPP messages
type IDTransformer struct {
	dbService *database.Service
	manager   *ProxyManager
}

// NewIDTransformer creates a new ID transformer
func NewIDTransformer(dbService *database.Service, manager *ProxyManager) *IDTransformer {
	return &IDTransformer{
		dbService: dbService,
		manager:   manager,
	}
}

// ProcessOutgoing transforms outgoing messages (from charge point to proxy)
// ProcessOutgoing transforms outgoing messages (from charge point to proxy)
func (t *IDTransformer) ProcessOutgoing(chargePointID string, message []byte) ([]byte, bool, error) {
	// Parse the OCPP message
	var ocppMsg []interface{}
	if err := json.Unmarshal(message, &ocppMsg); err != nil {
		return message, false, fmt.Errorf("error parsing OCPP message: %v", err)
	}

	// Only continue if it's a valid OCPP message
	if len(ocppMsg) < 3 {
		return message, false, nil
	}

	// Get message type (2 = Call, 3 = CallResult, 4 = CallError)
	msgTypeID, ok := ocppMsg[0].(float64)
	if !ok {
		return message, false, nil
	}

	// For StartTransaction requests, we might need to transform the idTag
	if msgTypeID == 2 && len(ocppMsg) >= 4 {
		action, ok := ocppMsg[2].(string)
		if !ok {
			return message, false, nil
		}

		// Get proxy config for this charge point
		proxyConfig, err := t.dbService.GetChargePointProxy(chargePointID)
		if err != nil || proxyConfig == nil {
			return message, false, nil
		}

		// Transform identifiers in specific messages
		if action == "StartTransaction" || action == "Authorize" {
			payload, ok := ocppMsg[3].(map[string]interface{})
			if !ok {
				return message, false, nil
			}

			// Transform idTag if present
			if idTag, ok := payload["idTag"].(string); ok {
				transformedID := proxyConfig.IDTransformPrefix + idTag + proxyConfig.IDTransformSuffix
				if transformedID != idTag {
					payload["idTag"] = transformedID

					// Store the mapping
					t.manager.mutex.Lock()
					t.manager.transformedIDs[idTag] = transformedID
					t.manager.reverseIDMapping[transformedID] = idTag
					t.manager.mutex.Unlock()
				}
			}

			// Replace the payload in the message
			ocppMsg[3] = payload

			// Reserialize the modified message
			modifiedMsg, err := json.Marshal(ocppMsg)
			if err != nil {
				return message, false, fmt.Errorf("error serializing modified message: %v", err)
			}

			return modifiedMsg, false, nil
		}
	}

	// For StopTransaction, we need to transform transaction IDs
	if msgTypeID == 2 && len(ocppMsg) >= 4 {
		action, ok := ocppMsg[2].(string)
		if !ok {
			return message, false, nil
		}

		if action == "StopTransaction" {
			payload, ok := ocppMsg[3].(map[string]interface{})
			if !ok {
				return message, false, nil
			}

			// Store mapping between transaction IDs if needed
			if txID, ok := payload["transactionId"].(float64); ok {
				log.Printf("Outgoing StopTransaction with transactionId %v", txID)
				// We could map transaction IDs here if needed
			}
		}
	}

	// Never block or modify other message types
	return message, false, nil
}

// ProcessIncoming transforms incoming messages (from proxy to charge point)
func (t *IDTransformer) ProcessIncoming(chargePointID string, message []byte) ([]byte, bool, error) {
	// Parse the OCPP message
	var ocppMsg []interface{}
	if err := json.Unmarshal(message, &ocppMsg); err != nil {
		return message, false, fmt.Errorf("error parsing OCPP message: %v", err)
	}

	// Only continue if it's a valid OCPP message
	if len(ocppMsg) < 3 {
		return message, false, nil
	}

	// Get message type (2 = Call, 3 = CallResult, 4 = CallError)
	msgTypeID, ok := ocppMsg[0].(float64)
	if !ok {
		return message, false, nil
	}

	// For requests from proxy (Call), we might need to transform
	if msgTypeID == 2 && len(ocppMsg) >= 4 {
		action, ok := ocppMsg[2].(string)
		if !ok {
			return message, false, nil
		}

		// Get proxy config for this charge point
		proxyConfig, err := t.dbService.GetChargePointProxy(chargePointID)
		if err != nil || proxyConfig == nil {
			log.Printf("Warning: No proxy config found for %s, using message as-is", chargePointID)
			return message, false, nil
		}

		// Special handling for various message types
		payload, ok := ocppMsg[3].(map[string]interface{})
		if !ok {
			log.Printf("Warning: Invalid payload format for %s action", action)
			return message, false, nil
		}

		// Transform IDs in the payload based on action
		switch action {
		case "RemoteStartTransaction":
			if idTag, ok := payload["idTag"].(string); ok {
				// Check if the idTag is a transformed ID
				originalID, exists := t.manager.GetOriginalID(idTag)
				if exists {
					payload["idTag"] = originalID
					log.Printf("Transformed proxy idTag %s to original %s", idTag, originalID)
				}
			}

		case "RemoteStopTransaction":
			if txID, ok := payload["transactionId"].(float64); ok {
				// Try to translate the transaction ID
				if t.manager != nil {
					localTxID, err := t.manager.TranslateProxyTransactionID(chargePointID, txID)
					if err != nil {
						log.Printf("Warning: Could not translate transaction ID: %v", err)
					} else if localTxID != txID {
						log.Printf("Translating external transaction ID %v to local transaction ID %v",
							txID, localTxID)
						payload["transactionId"] = localTxID
					}
				}
			}
		}

		// Replace the payload in the message
		ocppMsg[3] = payload

		// Reserialize the modified message
		modifiedMsg, err := json.Marshal(ocppMsg)
		if err != nil {
			return message, false, fmt.Errorf("error serializing modified message: %v", err)
		}

		return modifiedMsg, false, nil
	}

	// Never block or modify other message types
	return message, false, nil
}

// ManuallyConnectToProxies forces connection to all configured proxies for a charge point
func (pm *ProxyManager) ManuallyConnectToProxies(chargePointID string) error {
	log.Printf("Manually connecting charge point %s to proxies", chargePointID)

	// Disconnect any existing proxies first
	pm.DisconnectProxiesForChargePoint(chargePointID)

	// Then connect to all configured proxies
	return pm.ConnectToProxies(chargePointID)
}

func (pm *ProxyManager) StartProxyHealthCheck() {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			pm.mutex.RLock()
			chargePoints := make(map[string]bool)
			for _, connections := range pm.proxyConnections {
				for cpID := range connections {
					chargePoints[cpID] = true
				}
			}
			pm.mutex.RUnlock()

			// Reconnect all charge points that have proxy connections
			for cpID := range chargePoints {
				if err := pm.ConnectToProxies(cpID); err != nil {
					log.Printf("Health check: Failed to reconnect %s to proxies: %v", cpID, err)
				}
			}
		}
	}()
}

// TranslateProxyTransactionID attempts to find the correct transaction ID to use
// for a charge point when receiving a remote start/stop command
func (pm *ProxyManager) TranslateProxyTransactionID(chargePointID string, externalID float64) (float64, error) {
	// First, see if the transaction with this ID exists
	tx, err := pm.dbService.GetTransaction(int(externalID))
	if err == nil {
		// Transaction exists, check if it belongs to this charge point
		if tx.ChargePointID == chargePointID {
			log.Printf("Found matching transaction ID %d for charge point %s", int(externalID), chargePointID)
			return externalID, nil
		} else {
			log.Printf("WARNING: Transaction %d exists but belongs to charge point %s, not %s",
				int(externalID), tx.ChargePointID, chargePointID)
			// We'll continue looking for transactions on this charge point
		}
	}

	// Get all incomplete transactions for this charge point
	transactions, err := pm.dbService.GetIncompleteTransactions(chargePointID)
	if err != nil {
		return externalID, fmt.Errorf("could not get transactions for charge point %s: %v", chargePointID, err)
	}

	if len(transactions) == 0 {
		// No transactions to map to
		return externalID, fmt.Errorf("no active transactions found for charge point %s", chargePointID)
	}

	if len(transactions) == 1 {
		// There's only one active transaction, so we can safely assume this is the one
		log.Printf("Translating external transaction ID %v to local transaction ID %d for charge point %s",
			externalID, transactions[0].TransactionID, chargePointID)
		return float64(transactions[0].TransactionID), nil
	}

	// Multiple transactions - this is trickier
	log.Printf("WARNING: Multiple active transactions (%d) for charge point %s", len(transactions), chargePointID)
	for _, tx := range transactions {
		log.Printf("  Transaction ID: %d, Connector: %d, IdTag: %s",
			tx.TransactionID, tx.ConnectorID, tx.IdTag)
	}

	// Just return the original ID to be safe
	return externalID, fmt.Errorf("multiple active transactions, using original ID")
}
