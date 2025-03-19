package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	ocppserver "ocpp-server/ocpp"
	"ocpp-server/server"
	"ocpp-server/server/database"
)

func main() {
	// Setup database configuration from environment variables
	dbConfig := database.NewConfig()
	log.Printf("Using database type: %s", dbConfig.Type)

	// Initialize database connection
	dbService, err := database.NewService(dbConfig)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	log.Println("Database connection established successfully")

	// Create OCPP server configuration
	ocppConfig := ocppserver.NewConfig()
	log.Printf("OCPP server configured with host: %s, WebSocket port: %d, API port: %d",
		ocppConfig.Host, ocppConfig.WebSocketPort, ocppConfig.APIPort)

	// Create central system handler with database integration
	var handler = ocppserver.NewCentralSystemHandlerWithDB(dbService)
	log.Println("OCPP handler created with database integration")

	// Create and initialize OCPP server
	ocppServer := ocppserver.NewOCPPServer(ocppConfig, handler)
	log.Println("OCPP server initialized")

	// Start background tasks for meter value collection and offline monitoring
	ocppServer.StartMeterValuePolling(dbService)
	ocppServer.StartOfflineTransactionCheck(dbService)
	ocppServer.StartMeterValueBackup(dbService)

	// Create and start API server with additional endpoints for database access
	apiServer := server.NewAPIServerWithDB(ocppServer, ocppConfig, dbService)
	if err := apiServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Wait a moment for server startup logs to complete
	time.Sleep(100 * time.Millisecond)

	// Display server information
	wsURL := fmt.Sprintf("ws://%s:%d", ocppConfig.Host, ocppConfig.WebSocketPort)
	apiURL := fmt.Sprintf("http://%s:%d/api", ocppConfig.Host, ocppConfig.APIPort)

	fmt.Println("\nOCPP Server started successfully")
	fmt.Println("=================================")

	fmt.Println("\nEnvironment variables used (if set):")
	fmt.Println("  For OCPP server: OCPP_HOST, OCPP_WEBSOCKET_PORT, OCPP_API_PORT, OCPP_SYSTEM_NAME")
	fmt.Println("  For Database: DB_TYPE, DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSL_MODE, DB_SQLITE_PATH")

	fmt.Println("\nServer endpoints:")
	fmt.Printf("  WebSocket endpoint: %s\n", wsURL)
	fmt.Printf("  HTTP API endpoint: %s\n", apiURL)

	fmt.Println("\nAvailable API endpoints:")
	fmt.Printf("  GET  %s/status                         - Server status\n", apiURL)
	fmt.Printf("  GET  %s/charge-points                  - List all charge points\n", apiURL)
	fmt.Printf("  GET  %s/charge-points/:id              - Get charge point details\n", apiURL)
	fmt.Printf("  GET  %s/transactions                   - List all transactions\n", apiURL)
	fmt.Printf("  GET  %s/logs                           - View system logs\n", apiURL)
	fmt.Printf("  POST %s/commands/remote-start          - Start charging remotely\n", apiURL)
	fmt.Printf("  POST %s/commands/remote-stop           - Stop charging remotely\n", apiURL)
	fmt.Printf("  POST %s/commands/reset                 - Reset a charge point\n", apiURL)
	fmt.Printf("  POST %s/commands/unlock-connector      - Unlock a connector\n", apiURL)
	fmt.Printf("  GET  %s/commands/get-configuration     - Get charge point configuration\n", apiURL)
	fmt.Printf("  POST %s/commands/change-configuration  - Change charge point configuration\n", apiURL)
	fmt.Printf("  POST %s/commands/clear-cache           - Clear charge point cache\n", apiURL)
	fmt.Printf("  POST %s/commands/trigger-message       - Trigger message from charge point\n", apiURL)
	fmt.Printf("  POST %s/commands/generic               - Send any OCPP command\n", apiURL)
	fmt.Printf("  POST %s/admin/close-transaction        - Administratively close a transaction\n", apiURL)
	fmt.Printf("  GET  %s/reports/elrefusion             - Generate tax refund report\n", apiURL)

	// Setup graceful shutdown
	setupGracefulShutdown(apiServer)

	// Keep server running until terminated
	apiServer.RunForever()
}

// setupGracefulShutdown configures graceful shutdown on system signals
func setupGracefulShutdown(apiServer *server.APIServerWithDB) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nShutting down OCPP server...")
		apiServer.Shutdown()
		os.Exit(0)
	}()
}
