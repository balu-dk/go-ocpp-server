package ocppserver

import (
	"encoding/json"
	"log"
	"ocpp-server/server/database"
	"sync"
	"time"
)

// DatabaseMessageLogger logs OCPP messages to the database
type DatabaseMessageLogger struct {
	dbService     *database.Service
	mutex         sync.Mutex
	messageQueue  []database.RawMessageLog
	maxQueueSize  int
	flushInterval time.Duration
}

// NewDatabaseMessageLogger creates a new logger for raw OCPP messages
// that stores messages in the database immediately
func NewDatabaseMessageLogger(dbService *database.Service) *DatabaseMessageLogger {
	// Always enable logging

	// Small queue size for better immediacy (max 20 messages before forced flush)
	maxQueueSize := 20

	// Very short flush interval (2 seconds)
	flushIntervalSec := 2

	logger := &DatabaseMessageLogger{
		dbService:     dbService,
		messageQueue:  make([]database.RawMessageLog, 0, maxQueueSize),
		maxQueueSize:  maxQueueSize,
		flushInterval: time.Duration(flushIntervalSec) * time.Second,
	}

	// Start background goroutine for periodic flushing
	go logger.periodicFlush()

	// Log startup
	log.Printf("Database logging enabled with immediate write prioritization")

	return logger
}

// periodicFlush flushes the message queue periodically
func (l *DatabaseMessageLogger) periodicFlush() {
	ticker := time.NewTicker(l.flushInterval)
	defer ticker.Stop()

	for range ticker.C {
		l.flushQueue()
	}
}

// flushQueue writes all queued messages to the database
func (l *DatabaseMessageLogger) flushQueue() {
	l.mutex.Lock()
	defer l.mutex.Unlock()

	if len(l.messageQueue) == 0 {
		return
	}

	// Copy the queue and reset it
	messagesToSave := make([]database.RawMessageLog, len(l.messageQueue))
	copy(messagesToSave, l.messageQueue)
	l.messageQueue = l.messageQueue[:0]

	// Release the lock before DB operations
	l.mutex.Unlock()

	// Save all messages immediately, without batching
	for _, msg := range messagesToSave {
		if err := l.dbService.SaveRawMessageLog(&msg); err != nil {
			log.Printf("Error saving raw message log to database: %v", err)
		}
	}

	// Reacquire the lock
	l.mutex.Lock()
}

// LogRawMessage logs a raw OCPP message to the database immediately
func (l *DatabaseMessageLogger) LogRawMessage(direction string, chargePointID string, message []byte) error {
	// Extract metadata from the message for easier filtering
	var messageType, action, messageID string
	var msgObj []interface{}

	if err := json.Unmarshal(message, &msgObj); err == nil {
		if len(msgObj) >= 2 {
			// Get message type
			if msgTypeID, ok := msgObj[0].(float64); ok {
				switch int(msgTypeID) {
				case 2:
					messageType = "Request"
					// Get action for requests
					if len(msgObj) >= 3 {
						if actionStr, ok := msgObj[2].(string); ok {
							action = actionStr
						}
					}
				case 3:
					messageType = "Response"
				case 4:
					messageType = "Error"
				}
			}

			// Get message ID
			if msgID, ok := msgObj[1].(string); ok {
				messageID = msgID
			}
		}
	}

	// Create log entry
	logEntry := database.RawMessageLog{
		ChargePointID: chargePointID,
		Timestamp:     time.Now(),
		Direction:     direction,
		MessageType:   messageType,
		Action:        action,
		MessageID:     messageID,
		Message:       string(message),
	}

	// Try to save directly to database first for high-priority messages
	if direction == "RECV" && (action == "BootNotification" || action == "Heartbeat" || action == "StatusNotification") {
		// For important messages, save directly to DB
		if err := l.dbService.SaveRawMessageLog(&logEntry); err != nil {
			log.Printf("Error directly saving raw message log to database: %v", err)
			// Fall back to queue on error
			l.queueMessage(logEntry)
		}
		return nil
	}

	// For other messages, use the queue mechanism
	l.queueMessage(logEntry)
	return nil
}

// queueMessage adds a message to the queue
func (l *DatabaseMessageLogger) queueMessage(logEntry database.RawMessageLog) {
	// Add to queue
	l.mutex.Lock()
	defer l.mutex.Unlock()

	l.messageQueue = append(l.messageQueue, logEntry)

	// If queue is full, flush immediately
	if len(l.messageQueue) >= l.maxQueueSize {
		go l.flushQueue()
	}
}

// Close flushes any pending messages and closes the logger
func (l *DatabaseMessageLogger) Close() error {
	// Flush any remaining messages
	l.flushQueue()

	return nil
}
