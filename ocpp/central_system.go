package ocppserver

import (
	"fmt"
	"log"
	"net/http"
	"ocpp-server/server/database"
	"time"
)

// OCPPServer represents an OCPP central server
type OCPPServer struct {
	config  *Config
	handler OCPPHandler
	server  *http.Server
}

// NewOCPPServer creates a new OCPP server with the given configuration and handler
func NewOCPPServer(config *Config, handler OCPPHandler) *OCPPServer {
	mux := http.NewServeMux()

	// Register WebSocket handler
	mux.Handle("/", handler)

	server := &http.Server{
		Addr:    fmt.Sprintf(":%d", config.WebSocketPort),
		Handler: mux,
	}

	return &OCPPServer{
		config:  config,
		handler: handler,
		server:  server,
	}
}

// Start initiates and starts the OCPP server
func (s *OCPPServer) Start() error {
	// Start the server in a goroutine
	go func() {
		protocol := "ws"
		if s.config.UseTLS {
			protocol = "wss"
		}

		serverAddr := fmt.Sprintf(":%d", s.config.WebSocketPort)
		wsURL := fmt.Sprintf("%s://%s%s", protocol, s.config.Host, serverAddr)
		log.Printf("OCPP Central System listening on %s", wsURL)

		var err error
		if s.config.UseTLS {
			err = s.server.ListenAndServeTLS(s.config.CertFile, s.config.KeyFile)
		} else {
			err = s.server.ListenAndServe()
		}

		if err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed: %v", err)
		}
	}()

	return nil
}

// GetHandler returns the handler used by the server
func (s *OCPPServer) GetHandler() OCPPHandler {
	return s.handler
}

// RunForever keeps the server running until terminated
func (s *OCPPServer) RunForever() {
	select {}
}

func (s *OCPPServer) StartMeterValuePolling(dbService *database.Service) {
	go func() {
		ticker := time.NewTicker(5 * time.Minute) // Poll every 5 minutes
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Find all active transactions
				incompleteTransactions, err := dbService.GetAllIncompleteTransactions()
				if err != nil {
					log.Printf("Error fetching incomplete transactions: %v", err)
					continue
				}

				// Group by charge point to avoid flooding with requests
				chargePointMap := make(map[string][]int)
				for _, tx := range incompleteTransactions {
					if _, exists := chargePointMap[tx.ChargePointID]; !exists {
						chargePointMap[tx.ChargePointID] = []int{}
					}
					chargePointMap[tx.ChargePointID] = append(chargePointMap[tx.ChargePointID], tx.ConnectorID)
				}

				// Request meter values for each active connector
				for chargePointID, connectorIDs := range chargePointMap {
					// Get the command manager
					cmdMgr := s.GetHandler().GetCommandManager()
					if cmdMgr == nil {
						continue
					}

					for _, connectorID := range connectorIDs {
						// Only request if charge point is connected
						if conn, exists := cmdMgr.clients[chargePointID]; exists && conn != nil {
							connID := connectorID // Create local copy for goroutine
							go func() {
								_, err := cmdMgr.TriggerMessage(chargePointID, "MeterValues", &connID)
								if err != nil {
									log.Printf("Failed to trigger meter values for %s connector %d: %v",
										chargePointID, connID, err)
								}
							}()
							// Small delay to prevent flooding the charge point
							time.Sleep(1 * time.Second)
						}
					}
				}
			}
		}
	}()
}

func (s *OCPPServer) StartOfflineTransactionCheck(dbService *database.Service) {
	go func() {
		ticker := time.NewTicker(15 * time.Minute) // Check every 15 minutes
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Get all charge points
				chargePoints, err := dbService.ListChargePoints()
				if err != nil {
					log.Printf("Error fetching charge points: %v", err)
					continue
				}

				now := time.Now()

				for _, cp := range chargePoints {
					// Skip if charge point is connected
					if cp.IsConnected {
						continue
					}

					// Check if offline for more than 1 hour
					offlineDuration := now.Sub(cp.LastHeartbeat)
					if offlineDuration < time.Hour {
						continue
					}

					// Check for incomplete transactions
					incompleteTransactions, err := dbService.GetIncompleteTransactions(cp.ID)
					if err != nil || len(incompleteTransactions) == 0 {
						continue
					}

					// Log that we have offline charge points with active transactions
					log.Printf("WARNING: Charge point %s has been offline for %v and has %d incomplete transactions",
						cp.ID, offlineDuration.Round(time.Minute), len(incompleteTransactions))

					// If offline for more than 24 hours, consider closing transactions administratively
					if offlineDuration > 24*time.Hour {
						log.Printf("NOTICE: Charge point %s has been offline for over 24 hours. Consider administrative transaction closing.",
							cp.ID)

						// Optional: add administrative closing logic here
					}
				}
			}
		}
	}()
}

// StartMeterValueBackup starts a background process to regularly store the latest meter values
// for all active transactions, providing data resiliency
func (s *OCPPServer) StartMeterValueBackup(dbService *database.Service) {
	go func() {
		ticker := time.NewTicker(30 * time.Minute) // Backup every 30 minutes
		defer ticker.Stop()

		for {
			select {
			case <-ticker.C:
				// Find all active transactions
				incompleteTransactions, err := dbService.GetAllIncompleteTransactions()
				if err != nil {
					log.Printf("Error fetching incomplete transactions for backup: %v", err)
					continue
				}

				for _, tx := range incompleteTransactions {
					// Get the latest meter values for this transaction
					meterValues, err := dbService.GetLatestMeterValueForTransaction(tx.TransactionID)
					if err != nil || len(meterValues) == 0 {
						continue
					}

					// Find the latest energy reading
					var latestEnergyValue float64
					var latestTimestamp time.Time

					for _, mv := range meterValues {
						if mv.Measurand == "Energy.Active.Import.Register" || mv.Measurand == "Energy.Active.Import.Interval" {
							if mv.Timestamp.After(latestTimestamp) {
								latestEnergyValue = mv.Value
								latestTimestamp = mv.Timestamp
							}
						}
					}

					if latestEnergyValue > 0 && !latestTimestamp.IsZero() {
						// Convert to Wh if necessary
						valueInWh := latestEnergyValue
						if meterValues[0].Unit == "kWh" {
							valueInWh *= 1000
						}

						// Store a backup reading with special source
						backupValue := &database.MeterValue{
							TransactionID: tx.TransactionID,
							ChargePointID: tx.ChargePointID,
							ConnectorID:   tx.ConnectorID,
							Timestamp:     time.Now(),
							Value:         latestEnergyValue,
							Unit:          meterValues[0].Unit,
							Measurand:     "Energy.Active.Import.Register",
							Source:        "Backup", // Mark as a backup reading
						}

						if err := dbService.SaveMeterValue(backupValue); err != nil {
							log.Printf("Error saving backup meter value for transaction %d: %v",
								tx.TransactionID, err)
						} else {
							log.Printf("Stored backup meter value for transaction %d: %.2f %s",
								tx.TransactionID, latestEnergyValue, backupValue.Unit)
						}
					}
				}
			}
		}
	}()
}
