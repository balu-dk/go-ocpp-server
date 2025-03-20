package database

import (
	"fmt"
	"log"
	"os"
	"time"

	env "ocpp-server/utils"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// DatabaseType represents the type of database to use
type DatabaseType string

const (
	// PostgreSQL database type
	PostgreSQL DatabaseType = "postgres"
	// GORM officially supports the databases MySQL, PostgreSQL, SQLite, SQL Server, and TiDB
	// Other database types can be imported as needed
)

// Config holds database configuration
type Config struct {
	Type         DatabaseType
	Host         string
	Port         int
	User         string
	Password     string
	DatabaseName string
	SSLMode      string
}

// NewConfig creates a new database configuration with values from environment variables
func NewConfig() *Config {
	return &Config{
		Type:         PostgreSQL,
		Host:         env.GetEnv("DB_HOST", "localhost"),
		Port:         env.GetEnvAsInt("DB_PORT", 5432),
		User:         env.GetEnv("DB_USER", "postgres"),
		Password:     env.GetEnv("DB_PASSWORD", "postgres"),
		DatabaseName: env.GetEnv("DB_NAME", "ocpp_server"),
		SSLMode:      env.GetEnv("DB_SSL_MODE", "disable"),
	}
}

// Service provides database operations
type Service struct {
	db       *gorm.DB
	dbConfig *Config
}

// NewService creates a new database service with PostgreSQL
func NewService(config *Config) (*Service, error) {
	var db *gorm.DB
	var err error

	// Configure logger
	newLogger := logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             time.Second,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  true,
		},
	)

	// Connect to PostgreSQL database
	dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		config.Host, config.Port, config.User, config.Password, config.DatabaseName, config.SSLMode)

	log.Printf("Connecting to PostgreSQL database at %s:%d...", config.Host, config.Port)

	db, err = gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: newLogger,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to connect to PostgreSQL database: %w", err)
	}

	log.Println("Successfully connected to PostgreSQL database")

	// Auto migrate the schema
	log.Println("Migrating database schema...")
	err = db.AutoMigrate(
		&ChargePoint{},
		&Connector{},
		&Transaction{},
		&MeterValue{},
		&Log{},
		&Authorization{},
		&RawMessageLog{}, // Add the new raw message log model
	)
	if err != nil {
		return nil, fmt.Errorf("failed to migrate database schema: %w", err)
	}
	log.Println("Database schema migration complete")

	return &Service{db: db, dbConfig: config}, nil
}

// GetDB returns the underlying GORM database
func (s *Service) GetDB() *gorm.DB {
	return s.db
}

// GetDatabaseType returns the type of database being used (always PostgreSQL)
func (s *Service) GetDatabaseType() DatabaseType {
	return PostgreSQL
}

// SaveChargePoint creates or updates a charge point in the database
func (s *Service) SaveChargePoint(cp *ChargePoint) error {
	result := s.db.Save(cp)
	return result.Error
}

// GetChargePoint retrieves a charge point by ID
func (s *Service) GetChargePoint(id string) (*ChargePoint, error) {
	var cp ChargePoint
	result := s.db.First(&cp, "id = ?", id)
	if result.Error != nil {
		return nil, result.Error
	}
	return &cp, nil
}

// ListChargePoints retrieves all charge points
func (s *Service) ListChargePoints() ([]ChargePoint, error) {
	var chargePoints []ChargePoint
	result := s.db.Find(&chargePoints)
	if result.Error != nil {
		return nil, result.Error
	}
	return chargePoints, nil
}

// SaveConnector creates or updates a connector in the database
func (s *Service) SaveConnector(connector *Connector) error {
	result := s.db.Save(connector)
	return result.Error
}

// GetConnector retrieves a connector by charge point ID and connector ID
func (s *Service) GetConnector(chargePointID string, connectorID int) (*Connector, error) {
	var connector Connector
	result := s.db.First(&connector, "charge_point_id = ? AND connector_id = ?", chargePointID, connectorID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &connector, nil
}

// ListConnectors retrieves all connectors for a charge point
func (s *Service) ListConnectors(chargePointID string) ([]Connector, error) {
	var connectors []Connector
	result := s.db.Find(&connectors, "charge_point_id = ?", chargePointID)
	if result.Error != nil {
		return nil, result.Error
	}
	return connectors, nil
}

// CreateTransaction creates a new transaction in the database
func (s *Service) CreateTransaction(transaction *Transaction) error {
	result := s.db.Create(transaction)
	return result.Error
}

// UpdateTransaction updates an existing transaction
func (s *Service) UpdateTransaction(transaction *Transaction) error {
	result := s.db.Save(transaction)
	return result.Error
}

// GetTransaction retrieves a transaction by transaction ID
func (s *Service) GetTransaction(transactionID int) (*Transaction, error) {
	var transaction Transaction
	result := s.db.First(&transaction, "transaction_id = ?", transactionID)
	if result.Error != nil {
		return nil, result.Error
	}
	return &transaction, nil
}

// SaveMeterValue adds a meter value to the database
func (s *Service) SaveMeterValue(meterValue *MeterValue) error {
	result := s.db.Create(meterValue)
	return result.Error
}

// GetMeterValues gets all meter values for a transaction
func (s *Service) GetMeterValues(transactionID int) ([]MeterValue, error) {
	var meterValues []MeterValue
	result := s.db.Find(&meterValues, "transaction_id = ?", transactionID)
	if result.Error != nil {
		return nil, result.Error
	}
	return meterValues, nil
}

// AddLog adds a log entry to the database
func (s *Service) AddLog(log *Log) error {
	result := s.db.Create(log)
	return result.Error
}

// SaveAuthorization creates or updates authorization in the database
func (s *Service) SaveAuthorization(auth *Authorization) error {
	result := s.db.Save(auth)
	return result.Error
}

// GetAuthorization retrieves authorization by ID tag
func (s *Service) GetAuthorization(idTag string) (*Authorization, error) {
	var auth Authorization
	result := s.db.First(&auth, "id_tag = ?", idTag)
	if result.Error != nil {
		return nil, result.Error
	}
	return &auth, nil
}

// ListAuthorizations retrieves all authorizations
func (s *Service) ListAuthorizations() ([]Authorization, error) {
	var authorizations []Authorization
	result := s.db.Find(&authorizations)
	if result.Error != nil {
		return nil, result.Error
	}
	return authorizations, nil
}

// DeleteAuthorization removes an authorization from the database
func (s *Service) DeleteAuthorization(idTag string) error {
	result := s.db.Delete(&Authorization{}, "id_tag = ?", idTag)
	return result.Error
}

// ListTransactions retrieves transactions with optional filters
func (s *Service) ListTransactions(chargePointID string, isComplete *bool) ([]Transaction, error) {
	db := s.db

	if chargePointID != "" {
		db = db.Where("charge_point_id = ?", chargePointID)
	}

	if isComplete != nil {
		db = db.Where("is_complete = ?", *isComplete)
	}

	var transactions []Transaction
	result := db.Order("start_timestamp desc").Find(&transactions)
	if result.Error != nil {
		return nil, result.Error
	}

	return transactions, nil
}

// GetLogs retrieves system logs with optional filters
func (s *Service) GetLogs(chargePointID string, level string, limit int, offset int) ([]Log, error) {
	db := s.db

	if chargePointID != "" {
		db = db.Where("charge_point_id = ?", chargePointID)
	}

	if level != "" {
		db = db.Where("level = ?", level)
	}

	var logs []Log
	result := db.Order("timestamp desc").Limit(limit).Offset(offset).Find(&logs)
	if result.Error != nil {
		return nil, result.Error
	}

	return logs, nil
}

// getEnv gets environment variable with fallback
func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// getEnvAsInt gets environment variable as int with fallback
func getEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	var value int
	_, err := fmt.Sscanf(valueStr, "%d", &value)
	if err != nil {
		return defaultValue
	}

	return value
}

// GetActiveTransactionForConnector finds the active transaction for a specific charge point and connector
func (s *Service) GetActiveTransactionForConnector(chargePointID string, connectorID int) (*Transaction, error) {
	var transaction Transaction

	result := s.db.Where("charge_point_id = ? AND connector_id = ? AND is_complete = ?",
		chargePointID, connectorID, false).First(&transaction)

	if result.Error != nil {
		return nil, fmt.Errorf("no active transaction found for charge point %s connector %d: %v",
			chargePointID, connectorID, result.Error)
	}

	return &transaction, nil
}

func (s *Service) GetLatestMeterValueForTransaction(transactionID int) ([]MeterValue, error) {
	var meterValues []MeterValue
	result := s.db.Where("transaction_id = ?", transactionID).
		Order("timestamp desc").
		Limit(20). // Get last 20 readings to ensure we have the right measurands
		Find(&meterValues)

	if result.Error != nil {
		return nil, result.Error
	}

	return meterValues, nil
}

// GetIncompleteTransactions gets all incomplete transactions for a charge point
func (s *Service) GetIncompleteTransactions(chargePointID string) ([]Transaction, error) {
	var transactions []Transaction
	result := s.db.Where("charge_point_id = ? AND is_complete = ?", chargePointID, false).
		Find(&transactions)

	if result.Error != nil {
		return nil, result.Error
	}

	return transactions, nil
}

func (s *Service) GetTransactionsForPeriod(startDate, endDate time.Time) ([]Transaction, error) {
	var transactions []Transaction

	result := s.db.Where("(start_timestamp BETWEEN ? AND ?) OR (stop_timestamp BETWEEN ? AND ?)",
		startDate, endDate, startDate, endDate).
		Order("start_timestamp").
		Find(&transactions)

	if result.Error != nil {
		return nil, result.Error
	}

	return transactions, nil
}

func (s *Service) GetAllIncompleteTransactions() ([]Transaction, error) {
	var transactions []Transaction
	result := s.db.Where("is_complete = ?", false).Find(&transactions)

	if result.Error != nil {
		return nil, result.Error
	}

	return transactions, nil
}

// MarkTransactionStopReason marks a transaction with a StopReason
func (s *Service) MarkTransactionStopReason(transactionID int, reason string) error {
	// Find the transaction first
	var transaction Transaction
	result := s.db.First(&transaction, "transaction_id = ?", transactionID)
	if result.Error != nil {
		return result.Error
	}

	// Update only StopReason – no other fields
	transaction.StopReason = reason

	// Save changes
	result = s.db.Model(&Transaction{}).
		Where("transaction_id = ?", transactionID).
		Update("stop_reason", reason)

	return result.Error
}

// SaveRawMessageLog saves a raw message log to the database
func (s *Service) SaveRawMessageLog(rawLog *RawMessageLog) error {
	// Check if raw message logging is enabled
	if !env.GetEnvAsBool("OCPP_RAW_LOGGING", true) {
		return nil
	}

	// Save to database
	result := s.db.Create(rawLog)
	return result.Error
}

// GetRawMessageLogs retrieves raw message logs with optional filters
func (s *Service) GetRawMessageLogs(chargePointID string, direction string, action string, limit int, offset int) ([]RawMessageLog, error) {
	db := s.db

	if chargePointID != "" {
		db = db.Where("charge_point_id = ?", chargePointID)
	}

	if direction != "" {
		db = db.Where("direction = ?", direction)
	}

	if action != "" {
		db = db.Where("action = ?", action)
	}

	var logs []RawMessageLog
	result := db.Order("timestamp desc").Limit(limit).Offset(offset).Find(&logs)
	if result.Error != nil {
		return nil, result.Error
	}

	return logs, nil
}

// GetRawMessageLogsForTransaction retrieves all raw message logs related to a specific transaction
func (s *Service) GetRawMessageLogsForTransaction(transactionID int) ([]RawMessageLog, error) {
	// First get the transaction to find its charge point and timeframe
	var transaction Transaction
	result := s.db.First(&transaction, "transaction_id = ?", transactionID)
	if result.Error != nil {
		return nil, result.Error
	}

	// Query logs for this transaction's timeframe and charge point
	var logs []RawMessageLog
	query := s.db.Where("charge_point_id = ? AND timestamp >= ?",
		transaction.ChargePointID, transaction.StartTimestamp)

	if !transaction.StopTimestamp.IsZero() {
		// If transaction is complete, add end time filter
		query = query.Where("timestamp <= ?", transaction.StopTimestamp)
	}

	// Execute query
	result = query.Order("timestamp").Find(&logs)
	if result.Error != nil {
		return nil, result.Error
	}

	return logs, nil
}

// CleanupOldRawMessageLogs removes raw message logs older than the specified retention period
func (s *Service) CleanupOldRawMessageLogs(retentionDays int) error {
	if retentionDays <= 0 {
		return nil // No cleanup if retention is disabled
	}

	cutoffDate := time.Now().AddDate(0, 0, -retentionDays)
	result := s.db.Where("timestamp < ?", cutoffDate).Delete(&RawMessageLog{})

	if result.Error != nil {
		return result.Error
	}

	log.Printf("Cleaned up %d old raw message logs (older than %d days)",
		result.RowsAffected, retentionDays)

	return nil
}
