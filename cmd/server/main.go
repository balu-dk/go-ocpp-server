package main

import (
	"fmt"
	"log"

	"ocpp-server/internal/server"
	"ocpp-server/pkg/ocppserver"
)

func main() {
	// Opret konfiguration baseret på miljøvariabler (eller standardværdier)
	config := ocppserver.NewConfig()

	// For at overskrive konfiguration programmatisk hvis nødvendigt:
	// config.WithHost("example.com").WithWebSocketPort(9876)

	// Opret central system handler
	handler := ocppserver.NewCentralSystemHandler()

	// Opret OCPP server
	ocppServer := ocppserver.NewOCPPServer(config, handler)

	// Opret og start API server
	apiServer := server.NewAPIServer(ocppServer, config)
	if err := apiServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	// Vis information om kørende server
	wsURL := fmt.Sprintf("ws://%s:%d", config.Host, config.WebSocketPort)
	apiURL := fmt.Sprintf("http://%s:%d/api/status", config.Host, config.APIPort)

	fmt.Println("OCPP server started successfully")
	fmt.Println("Environment variables used (if set):")
	fmt.Println("  OCPP_HOST, OCPP_WEBSOCKET_PORT, OCPP_API_PORT, OCPP_SYSTEM_NAME")
	fmt.Println("")
	fmt.Printf("WebSocket endpoint: %s\n", wsURL)
	fmt.Printf("HTTP API endpoint: %s\n", apiURL)

	// Hold serveren kørende
	apiServer.RunForever()
}
