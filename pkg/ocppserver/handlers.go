package ocppserver

import (
	"fmt"
	"log"
	"net/http"
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
				return true
			},
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

	// Få chargePointID fra URL eller header
	chargePointID := r.URL.Path[1:] // Fjern den indledende '/'
	if chargePointID == "" {
		chargePointID = "unknown"
	}

	// Gem forbindelsen
	cs.clients[chargePointID] = conn
	log.Printf("New connection from %s", chargePointID)

	// Håndter indgående beskeder
	go cs.handleMessages(chargePointID, conn)
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

		// Parse og håndter besked
		log.Printf("Received message from %s: %s", chargePointID, message)

		// Håndter forskellige beskedtyper baseret på indhold
		// Dette er meget forenklet; en rigtig implementation ville
		// parse JSON-beskederne og validere dem
		if string(message) == BootNotificationMsg {
			cs.handleBootNotification(chargePointID, conn)
		} else if string(message) == HeartbeatMsg {
			cs.handleHeartbeat(chargePointID, conn)
		} else if string(message) == AuthorizeMsg {
			cs.handleAuthorize(chargePointID, conn)
		}
	}
}

// handleBootNotification håndterer BootNotification
func (cs *CentralSystemHandler) handleBootNotification(chargePointID string, conn *websocket.Conn) {
	fmt.Printf("BootNotification from %s\n", chargePointID)

	// Opret svar
	confirmation := BootNotificationConfirmation{
		CurrentTime: time.Now().Format(time.RFC3339),
		Interval:    60,
		Status:      "Accepted",
	}

	// Send svar
	if err := conn.WriteJSON(confirmation); err != nil {
		log.Printf("Error sending BootNotification response: %v", err)
	}
}

// handleHeartbeat håndterer Heartbeat
func (cs *CentralSystemHandler) handleHeartbeat(chargePointID string, conn *websocket.Conn) {
	fmt.Printf("Heartbeat from %s\n", chargePointID)

	// Opret svar
	confirmation := HeartbeatConfirmation{
		CurrentTime: time.Now().Format(time.RFC3339),
	}

	// Send svar
	if err := conn.WriteJSON(confirmation); err != nil {
		log.Printf("Error sending Heartbeat response: %v", err)
	}
}

// handleAuthorize håndterer Authorization
func (cs *CentralSystemHandler) handleAuthorize(chargePointID string, conn *websocket.Conn) {
	fmt.Printf("Authorize request from %s\n", chargePointID)

	// Opret svar
	confirmation := AuthorizeConfirmation{
		IdTagInfo: IdTagInfo{
			Status: "Accepted",
		},
	}

	// Send svar
	if err := conn.WriteJSON(confirmation); err != nil {
		log.Printf("Error sending Authorize response: %v", err)
	}
}
