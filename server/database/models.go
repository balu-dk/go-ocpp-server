package database

import (
	"time"
)

// ChargePoint represents a charging station in the database
type ChargePoint struct {
	ID                   string    `gorm:"primaryKey" json:"id"`
	Model                string    `json:"model"`
	Vendor               string    `json:"vendor"`
	SerialNumber         string    `json:"serialNumber,omitempty"`
	FirmwareVersion      string    `json:"firmwareVersion,omitempty"`
	Status               string    `json:"status"` // Overall status of the charge point
	LastHeartbeat        time.Time `json:"lastHeartbeat"`
	LastBootNotification time.Time `json:"lastBootNotification"`
	HeartbeatInterval    int       `json:"heartbeatInterval"`
	IsConnected          bool      `json:"isConnected"`
	CreatedAt            time.Time `json:"createdAt"`
	UpdatedAt            time.Time `json:"updatedAt"`
}

// Connector represents a connector on a charge point
type Connector struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ChargePointID string    `json:"chargePointId"`
	ConnectorID   int       `json:"connectorId"` // The connector ID as reported by the charge point
	Status        string    `json:"status"`      // Available, Occupied, Reserved, Unavailable, Faulted
	ErrorCode     string    `json:"errorCode,omitempty"`
	CurrentPower  float64   `json:"currentPower"`
	UpdatedAt     time.Time `json:"updatedAt"`
}

// Transaction represents a charging transaction
type Transaction struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	TransactionID   int       `json:"transactionId"` // The transaction ID assigned by the system
	ChargePointID   string    `json:"chargePointId"`
	ConnectorID     int       `json:"connectorId"`
	IdTag           string    `json:"idTag"` // The ID of the card/token used
	StartTimestamp  time.Time `json:"startTimestamp"`
	StopTimestamp   time.Time `json:"stopTimestamp,omitempty"`
	MeterStart      int       `json:"meterStart"`                // Meter reading at start (Wh)
	MeterStop       int       `json:"meterStop,omitempty"`       // Meter reading at end (Wh)
	EnergyDelivered float64   `json:"energyDelivered,omitempty"` // Calculated energy delivered (kWh)
	StopReason      string    `json:"stopReason,omitempty"`
	IsComplete      bool      `json:"isComplete"`
}

// MeterValue represents a meter reading during a transaction
type MeterValue struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	TransactionID int       `json:"transactionId"`
	ChargePointID string    `json:"chargePointId"`
	ConnectorID   int       `json:"connectorId"`
	Timestamp     time.Time `json:"timestamp"`
	Value         float64   `json:"value"`            // The actual reading value
	Unit          string    `json:"unit"`             // Wh, kWh, W, A, V, etc.
	Measurand     string    `json:"measurand"`        // Energy.Active.Import.Register, Power.Active.Import, etc.
	Source        string    `json:"source,omitempty"` // Source of the meter value: ChargePoint, Backup, etc.
}

// Log represents a system log entry
type Log struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ChargePointID string    `json:"chargePointId,omitempty"`
	Timestamp     time.Time `json:"timestamp"`
	Level         string    `json:"level"`  // INFO, WARNING, ERROR, DEBUG
	Source        string    `json:"source"` // System, ChargePoint
	Message       string    `json:"message"`
}

// Authorization represents an authorized RFID card or token
type Authorization struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	IdTag      string    `gorm:"unique" json:"idTag"`
	Status     string    `json:"status"` // Accepted, Blocked, Expired
	ExpiryDate time.Time `json:"expiryDate,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// RawMessageLog represents a raw OCPP message log entry
type RawMessageLog struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ChargePointID string    `json:"chargePointId"`
	Timestamp     time.Time `json:"timestamp"`
	Direction     string    `json:"direction"`                // "SEND" or "RECV"
	MessageType   string    `json:"messageType,omitempty"`    // "Request", "Response", "Error"
	Action        string    `json:"action,omitempty"`         // OCPP action like "BootNotification", "Heartbeat", etc.
	MessageID     string    `json:"messageId,omitempty"`      // Unique ID of the message
	Message       string    `gorm:"type:text" json:"message"` // Full message content as JSON
}

// PendingRemoteStart tracks remote start requests that haven't been confirmed with a StartTransaction
type PendingRemoteStart struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ChargePointID string    `json:"chargePointId"`
	ConnectorID   int       `json:"connectorId"`
	IdTag         string    `json:"idTag"`
	RequestTime   time.Time `json:"requestTime"`
	Completed     bool      `json:"completed"`
	TransactionID *int      `json:"transactionId,omitempty"` // Once a transaction is created
	Expired       bool      `json:"expired"`                 // Set to true after expiration time
}

// ProxyDestination represents a central system to proxy requests to
type ProxyDestination struct {
	ID          uint      `gorm:"primaryKey" json:"id"`
	Name        string    `json:"name"`                  // Friendly name for this proxy destination
	URL         string    `json:"url"`                   // WebSocket URL of the central system
	Description string    `json:"description,omitempty"` // Optional description
	IsActive    bool      `json:"isActive"`              // Whether this proxy destination is active
	CreatedAt   time.Time `json:"createdAt"`
	UpdatedAt   time.Time `json:"updatedAt"`
}

// ChargePointProxy stores the proxy configuration for a specific charge point
type ChargePointProxy struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	ChargePointID     string    `json:"chargePointId"`     // ID of the charge point
	ProxyEnabled      bool      `json:"proxyEnabled"`      // Whether proxying is enabled for this charge point
	IDTransformPrefix string    `json:"idTransformPrefix"` // Prefix to add to the ID when proxying
	IDTransformSuffix string    `json:"idTransformSuffix"` // Suffix to add to the ID when proxying
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

// ChargePointProxyMapping maps a charge point to multiple proxy destinations
type ChargePointProxyMapping struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	ChargePointID      string    `json:"chargePointId"`      // ID of the charge point
	ProxyDestinationID uint      `json:"proxyDestinationId"` // ID of the proxy destination
	IsActive           bool      `json:"isActive"`           // Whether this specific mapping is active
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// ProxyMessageLog stores logs of proxied messages
type ProxyMessageLog struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	ChargePointID      string    `json:"chargePointId"`
	ProxyDestinationID uint      `json:"proxyDestinationId"`
	Timestamp          time.Time `json:"timestamp"`
	Direction          string    `json:"direction"` // "TO_PROXY" or "FROM_PROXY"
	OriginalMessage    string    `gorm:"type:text" json:"originalMessage"`
	TransformedMessage string    `gorm:"type:text" json:"transformedMessage,omitempty"`
	WasModified        bool      `json:"wasModified"`
	WasBlocked         bool      `json:"wasBlocked"`
}

type TransactionMapping struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ChargePointID string    `json:"chargePointId"`
	LocalTxID     int       `json:"localTxId"`    // Your transaction ID
	ExternalTxID  int       `json:"externalTxId"` // Proxy system's transaction ID
	ProxyID       uint      `json:"proxyId"`      // ID of the proxy destination
	CreatedAt     time.Time `json:"createdAt"`
}
