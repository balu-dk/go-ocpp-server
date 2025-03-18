package main

import (
	"fmt"
	"log"

	"github.com/balu-dk/go-ocpp-server/internal/server"
	"github.com/balu-dk/go-ocpp-server/pkg/ocppserver"
)

func main() {
	// Opret konfiguration
	config := ocppserver.NewConfig().
		WithListenAddress(":8887").
		WithSystemName("ocpp-central")

	// Opret central system handler
	handler := ocppserver.NewCentralSystemHandler()

	// Opret OCPP server
	ocppServer := ocppserver.NewOCPPServer(config, handler)

	// Opret og start API server (valgfrit - kan udelades hvis du kun ønsker OCPP-server)
	apiServer := server.NewAPIServer(ocppServer, ":8080")
	if err := apiServer.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}

	fmt.Println("OCPP server started successfully")
	fmt.Println("WebSocket endpoint: ws://localhost:8887")
	fmt.Println("HTTP API endpoint: http://localhost:8080/api/status")

	// Hold serveren kørende
	apiServer.RunForever()
}
