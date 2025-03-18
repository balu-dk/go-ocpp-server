package ocppserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/websocket"
)

// OCPP message types
const (
	BootNotificationMsg = "BootNotification"
	HeartbeatMsg        = "Heartbeat"
	AuthorizeMsg        = "Authorize"
)

// BootNotificationRequest repræsenterer anmodningen fra en ladestation
type BootNotificationRequest struct {
	ChargePointModel        string `json:"chargePointModel"`
	ChargePointVendor       string `json:"chargePointVendor"`
	ChargePointSerialNumber string `json:"chargePointSerialNumber,omitempty"`
	FirmwareVersion         string `json:"firmwareVersion,omitempty"`
}

// BootNotificationConfirmation repræsenterer svaret til en ladestation
type BootNotificationConfirmation struct {
	CurrentTime string `json:"currentTime"`
	Interval    int    `json:"interval"`
	Status      string `json:"status"`
}

// HeartbeatRequest repræsenterer en heartbeat anmodning
type HeartbeatRequest struct{}

// HeartbeatConfirmation repræsenterer svaret på en heartbeat
type HeartbeatConfirmation struct {
	CurrentTime string `json:"currentTime"`
}

// AuthorizeRequest repræsenterer en autorisationsanmodning
type AuthorizeRequest struct {
	IdTag string `json:"idTag"`
}

// IdTagInfo indeholder oplysninger om en IdTag
type IdTagInfo struct {
	Status string `json:"status"`
}

// AuthorizeConfirmation repræsenterer svaret på en autorisationsanmodning
type AuthorizeConfirmation struct {
	IdTagInfo IdTagInfo `json:"idTagInfo"`
}

// CentralSystemHandler håndterer OCPP-anmodninger fra ladestationer
type CentralSystemHandler struct {
	upgrader websocket.Upgrader
	clients  map[string]*websocket.Conn
}

// NewCentralSystemHandler opretter en ny handler for centralserveren
func NewCentralSystemHandler() *CentralSystemHandler {
	return &CentralSystemHandler{
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Tillad forbindelser fra alle origins
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		clients: make(map[string]*websocket.Conn),
	}
}

// HandleWebSocket håndterer WebSocket-forbindelser fra ladestationer
func (cs *CentralSystemHandler) HandleWebSocket(w http.ResponseWriter, r *http.Request) {
	conn, err := cs.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("Failed to upgrade connection: %v", err)
		return
	}

	// Få chargePointID direkte fra URL path
	// Format: localhost:9000/{id}
	chargePointID := strings.TrimPrefix(r.URL.Path, "/")

	// Håndter tilfælde hvor der kunne være flere skråstreger
	if strings.Contains(chargePointID, "/") {
		parts := strings.Split(chargePointID, "/")
		chargePointID = parts[0] // Tag kun den første del
	}

	if chargePointID == "" {
		chargePointID = "unknown"
	}

	// Gem forbindelsen
	cs.clients[chargePointID] = conn
	log.Printf("New connection from %s", chargePointID)

	// Håndter indgående beskeder - sørg for at chargePointID ikke indeholder '/'
	sanitizedID := chargePointID
	go cs.handleMessages(sanitizedID, conn)
}

// handleAuthorizeRequest håndterer en Authorize-anmodning
func (cs *CentralSystemHandler) handleAuthorizeRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	idTag, _ := payload["idTag"].(string)
	fmt.Printf("Authorize request from %s: idTag=%s\n", chargePointID, idTag)

	// Opret svar
	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": "Accepted",
		},
	}
}

// handleMessages håndterer beskeder fra en ladestation
func (cs *CentralSystemHandler) handleMessages(chargePointID string, conn *websocket.Conn) {
	defer func() {
		conn.Close()
		delete(cs.clients, chargePointID)
		log.Printf("Connection closed for %s", chargePointID)
	}()

	for {
		// Læs besked
		_, message, err := conn.ReadMessage()
		if err != nil {
			log.Printf("Error reading message: %v", err)
			break
		}

		// Parse OCPP besked
		log.Printf("Received message from %s: %s", chargePointID, message)

		// Forsøg at parse JSON-arrayet [MessageTypeId, UniqueId, Action, Payload]
		var ocppMsg []interface{}
		if err := json.Unmarshal(message, &ocppMsg); err != nil {
			log.Printf("Error parsing OCPP message: %v", err)
			continue
		}

		// Check at beskeden har det forventede format
		if len(ocppMsg) < 3 {
			log.Printf("Invalid OCPP message format: %s", message)
			continue
		}

		// Beskedtypen (2 = Request, 3 = Response, 4 = Error)
		msgTypeID, ok := ocppMsg[0].(float64)
		if !ok {
			log.Printf("Invalid message type ID: %v", ocppMsg[0])
			continue
		}

		// Beskedens unikke ID
		uniqueID, ok := ocppMsg[1].(string)
		if !ok {
			log.Printf("Invalid unique ID: %v", ocppMsg[1])
			continue
		}

		// Hvis det er en anmodning (msgTypeID == 2)
		if msgTypeID == 2 {
			action, ok := ocppMsg[2].(string)
			if !ok {
				log.Printf("Invalid action: %v", ocppMsg[2])
				continue
			}

			// Håndter de forskellige action typer
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
				response = map[string]interface{}{} // Send tomt svar for ukendte handlinger
			}

			// Send svar tilbage (CallResult)
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

// handleBootNotificationRequest håndterer en BootNotification-anmodning
func (cs *CentralSystemHandler) handleBootNotificationRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Log indkommende data
	chargePointModel, _ := payload["chargePointModel"].(string)
	chargePointVendor, _ := payload["chargePointVendor"].(string)

	fmt.Printf("BootNotification from %s: Model=%s, Vendor=%s\n",
		chargePointID, chargePointModel, chargePointVendor)

	// Opret svar
	return map[string]interface{}{
		"currentTime": time.Now().Format(time.RFC3339),
		"interval":    60,
		"status":      "Accepted",
	}
}

// handleHeartbeatRequest håndterer en Heartbeat-anmodning
func (cs *CentralSystemHandler) handleHeartbeatRequest(chargePointID string) map[string]interface{} {
	fmt.Printf("Heartbeat from %s\n", chargePointID)

	// Opret svar
	return map[string]interface{}{
		"currentTime": time.Now().Format(time.RFC3339),
	}
}

// handleStatusNotificationRequest håndterer en StatusNotification-anmodning
func (cs *CentralSystemHandler) handleStatusNotificationRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	// Udpak vigtige statusoplysninger
	status, _ := payload["status"].(string)
	connectorId, _ := payload["connectorId"].(float64)
	errorCode, _ := payload["errorCode"].(string)

	fmt.Printf("StatusNotification from %s: ConnectorId=%v, Status=%s, ErrorCode=%s\n",
		chargePointID, connectorId, status, errorCode)

	// StatusNotification kræver et tomt svar iflg. OCPP-specifikationen
	return map[string]interface{}{}
}

// handleStartTransactionRequest håndterer en StartTransaction-anmodning
func (cs *CentralSystemHandler) handleStartTransactionRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	idTag, _ := payload["idTag"].(string)
	connectorId, _ := payload["connectorId"].(float64)
	meterStart, _ := payload["meterStart"].(float64)

	fmt.Printf("StartTransaction from %s: ConnectorId=%v, IdTag=%s, MeterStart=%v\n",
		chargePointID, connectorId, idTag, meterStart)

	// Generer et tilfældigt transaktions-ID (i virkeligheden ville du bruge en database)
	transactionId := int(time.Now().Unix() % 10000)

	return map[string]interface{}{
		"idTagInfo": map[string]interface{}{
			"status": "Accepted",
		},
		"transactionId": transactionId,
	}
}

// handleStopTransactionRequest håndterer en StopTransaction-anmodning
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

// handleMeterValuesRequest håndterer en MeterValues-anmodning
func (cs *CentralSystemHandler) handleMeterValuesRequest(chargePointID string, payload map[string]interface{}) map[string]interface{} {
	connectorId, _ := payload["connectorId"].(float64)
	transactionId, _ := payload["transactionId"].(float64)

	fmt.Printf("MeterValues from %s: ConnectorId=%v, TransactionId=%v\n",
		chargePointID, connectorId, transactionId)

	// Log meterværdier hvis tilgængelige
	if meterValues, ok := payload["meterValue"].([]interface{}); ok && len(meterValues) > 0 {
		fmt.Printf("  Received %d meter values\n", len(meterValues))
	}

	// MeterValues kræver et tomt svar iflg. OCPP-specifikationen
	return map[string]interface{}{}
}
