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
