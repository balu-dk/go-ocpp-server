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
	env "ocpp-server/utils"
)

func main() {
	// Set up basic logging to stdout only (logs will primarily go to DB)
	log.SetOutput(os.Stdout)

	// Load environment variables from .env file
	env.Initialize()

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

	// Initialize the proxy manager with database service
	proxyManager := ocppserver.NewProxyManager(dbService)

	centralHandler := ocppserver.NewCentralSystemHandlerWithDB(dbService, proxyManager)

	proxyManager.SetCentralHandler(centralHandler)

	// Create and register the ID transformer for handling ID transformations in proxied messages
	idTransformer := ocppserver.NewIDTransformer(dbService, proxyManager)
	proxyManager.RegisterMessageProcessor(idTransformer)

	// Create central system handler with database integration and proxy manager
	var handler = ocppserver.NewCentralSystemHandlerWithDB(dbService, proxyManager)
	proxyManager.StartProxyHealthCheck()
	log.Println("OCPP handler created with database integration and proxy support")

	// Create and initialize OCPP server
	ocppServer := ocppserver.NewOCPPServer(ocppConfig, handler)
	log.Println("OCPP server initialized")

	// Start background tasks for meter value collection and offline monitoring
	ocppServer.StartMeterValuePolling(dbService)
	ocppServer.StartOfflineTransactionCheck(dbService)
	ocppServer.StartMeterValueBackup(dbService)

	// Start connection monitoring to detect disconnected charge points
	ocppServer.StartConnectionMonitoring(dbService)
	log.Println("Connection monitoring started")

	// Start status monitoring to regularly request updates from charge points
	ocppServer.StartStatusMonitoring(dbService)
	log.Println("Status monitoring started")

	// Start orphaned charging session detection
	ocppServer.StartOrphanedSessionDetection(dbService)
	log.Println("Orphaned charging session detection started")

	// Create and start API server with additional endpoints for database access
	apiServer := server.NewAPIServerWithDB(ocppServer, ocppConfig, dbService)
	if err := apiServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// After the server has started, connect all currently connected charge points to proxies
	go func() {
		// Give the server a moment to fully initialize
		time.Sleep(2 * time.Second)

		// Get all connected charge points
		connectedChargePoints, err := dbService.ListConnectedChargePoints()
		if err != nil {
			log.Printf("Error fetching connected charge points for proxy setup: %v", err)
			return
		}

		log.Printf("Setting up proxy connections for %d connected charge points", len(connectedChargePoints))

		// Connect each one to its configured proxies
		for _, cp := range connectedChargePoints {
			if err := proxyManager.ConnectToProxies(cp.ID); err != nil {
				log.Printf("Error connecting charge point %s to proxies: %v", cp.ID, err)
			} else {
				log.Printf("Successfully connected charge point %s to proxies", cp.ID)
			}
		}
	}()

	// Wait a moment for server startup logs to complete
	time.Sleep(100 * time.Millisecond)

	// Display server information
	protocol := "ws"
	apiProtocol := "http"
	if ocppConfig.UseTLS {
		protocol = "wss"
		apiProtocol = "https"
	}

	wsURL := fmt.Sprintf("%s://%s:%d", protocol, ocppConfig.Host, ocppConfig.WebSocketPort)
	apiURL := fmt.Sprintf("%s://%s:%d/api", apiProtocol, ocppConfig.Host, ocppConfig.APIPort)

	fmt.Println("\nOCPP Server started successfully")
	fmt.Println("=================================")

	fmt.Println("\nEnvironment variables used (if set):")
	fmt.Println("  For OCPP server: OCPP_HOST, OCPP_WEBSOCKET_PORT, OCPP_API_PORT, OCPP_SYSTEM_NAME, OCPP_USE_TLS, OCPP_CERT_FILE, OCPP_KEY_FILE")
	fmt.Println("  For Database: DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME, DB_SSL_MODE")

	fmt.Println("\nServer endpoints:")
	fmt.Printf("  WebSocket endpoint: %s\n", wsURL)
	fmt.Printf("  HTTP API endpoint: %s\n", apiURL)

	if ocppConfig.UseTLS {
		fmt.Println("\nTLS is enabled. Using certificate files:")
		fmt.Printf("  Certificate file: %s\n", ocppConfig.CertFile)
		fmt.Printf("  Key file: %s\n", ocppConfig.KeyFile)
	}

	fmt.Println("\nAvailable API endpoints:")
	fmt.Printf("  GET  %s/status                         - Server status\n", apiURL)
	fmt.Printf("  GET  %s/charge-points                  - List all charge points\n", apiURL)
	fmt.Printf("  GET  %s/charge-points/:id              - Get charge point details\n", apiURL)
	fmt.Printf("  GET  %s/transactions                   - List all transactions\n", apiURL)
	fmt.Printf("  GET  %s/logs                           - View system logs\n", apiURL)
	fmt.Printf("  POST %s/commands/remote-start          - Start charging remotely\n", apiURL)
	fmt.Printf("  POST %s/commands/remote-stop           - Stop charging remotely\n", apiURL)
	fmt.Printf("  POST %s/api/commands/force-stop        - Force stop charging remotely\n", apiURL)
	fmt.Printf("  POST %s/commands/reset                 - Reset a charge point\n", apiURL)
	fmt.Printf("  POST %s/commands/unlock-connector      - Unlock a connector\n", apiURL)
	fmt.Printf("  GET  %s/commands/get-configuration     - Get charge point configuration\n", apiURL)
	fmt.Printf("  POST %s/commands/change-configuration  - Change charge point configuration\n", apiURL)
	fmt.Printf("  POST %s/commands/clear-cache           - Clear charge point cache\n", apiURL)
	fmt.Printf("  POST %s/commands/trigger-message       - Trigger message from charge point\n", apiURL)
	fmt.Printf("  POST %s/commands/generic               - Send any OCPP command\n", apiURL)
	fmt.Printf("  POST %s/admin/close-transaction        - Administratively close a transaction\n", apiURL)
	fmt.Printf("  GET  %s/reports/energy                 - Generate energy consumption report\n", apiURL)

	// New proxy-related endpoints
	fmt.Println("\nProxy System API endpoints:")
	fmt.Printf("  POST     %s/proxy/connect                                           - Manually connect charge point\n", apiURL)
	fmt.Printf("  GET     %s/proxy/destinations                                       - List all proxy destinations\n", apiURL)
	fmt.Printf("  GET     %s/proxy/destinations?id=1                                  - Get details for a specific proxy destination\n", apiURL)
	fmt.Printf("  POST    %s/proxy/destinations                                       - Create or update a proxy destination\n", apiURL)
	fmt.Printf("  DELETE  %s/proxy/destinations?id=1                                  - Delete a proxy destination\n", apiURL)
	fmt.Printf("  GET     %s/proxy/charge-points?chargePointId=CP001                  - Get proxy config for a charge point\n", apiURL)
	fmt.Printf("  GET     %s/proxy/charge-points                                      - List all charge point proxy configurations\n", apiURL)
	fmt.Printf("  POST    %s/proxy/charge-points                                      - Enable/disable proxying for a charge point\n", apiURL)
	fmt.Printf("  GET     %s/proxy/mappings?chargePointId=CP001                       - List mappings for a charge point\n", apiURL)
	fmt.Printf("  GET     %s/proxy/mappings                                           - List all proxy mappings\n", apiURL)
	fmt.Printf("  POST    %s/proxy/mappings                                           - Create or update a mapping\n", apiURL)
	fmt.Printf("  DELETE  %s/proxy/mappings?id=1                                      - Delete a mapping by ID\n", apiURL)
	fmt.Printf("  DELETE  %s/proxy/mappings?chargePointId=CP001&proxyDestinationId=1  - Delete by charge point and destination\n", apiURL)
	fmt.Printf("  GET     %s/proxy/logs?chargePointId=CP001&direction=TO_PROXY        - View proxy message logs\n", apiURL)

	// Setup graceful shutdown
	setupGracefulShutdown(apiServer, proxyManager)

	// Keep server running until terminated
	apiServer.RunForever()
}

// setupGracefulShutdown configures graceful shutdown on system signals
func setupGracefulShutdown(apiServer *server.APIServerWithDB, proxyManager *ocppserver.ProxyManager) {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		fmt.Println("\nShutting down OCPP server...")

		// Disconnect all proxy connections
		if proxyManager != nil {
			log.Println("Disconnecting proxy connections...")
			proxyManager.DisconnectAllProxies()
		}

		// Shutdown API server
		apiServer.Shutdown()
		os.Exit(0)
	}()
}
