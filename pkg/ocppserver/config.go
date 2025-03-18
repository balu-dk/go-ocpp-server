package ocppserver

import (
	"fmt"
	"os"
)

// Config indeholder konfiguration til OCPP-serveren
type Config struct {
	// Host er domænenavn eller IP-adressen serveren skal bruge
	Host string

	// WebSocketPort er porten som OCPP WebSocket-serveren skal lytte på
	WebSocketPort int

	// APIPort er porten som HTTP API-serveren skal lytte på
	APIPort int

	// SystemName er navnet på centralserveren
	SystemName string
}

// NewConfig opretter en ny konfiguration med standardværdier og miljøvariabler
func NewConfig() *Config {
	config := &Config{
		Host:          getEnv("OCPP_HOST", "localhost"),
		WebSocketPort: getEnvAsInt("OCPP_WEBSOCKET_PORT", 9000),
		APIPort:       getEnvAsInt("OCPP_API_PORT", 9001),
		SystemName:    getEnv("OCPP_SYSTEM_NAME", "ocpp-central"),
	}

	return config
}

// WebSocketAddr returnerer den fulde adresse for WebSocket-serveren i format "host:port"
func (c *Config) WebSocketAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.WebSocketPort)
}

// APIAddr returnerer den fulde adresse for API-serveren i format "host:port"
func (c *Config) APIAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.APIPort)
}

// WithHost sætter host eller IP for serverene
func (c *Config) WithHost(host string) *Config {
	c.Host = host
	return c
}

// WithWebSocketPort sætter porten for WebSocket-serveren
func (c *Config) WithWebSocketPort(port int) *Config {
	c.WebSocketPort = port
	return c
}

// WithAPIPort sætter porten for API-serveren
func (c *Config) WithAPIPort(port int) *Config {
	c.APIPort = port
	return c
}

// WithSystemName sætter systemnavnet
func (c *Config) WithSystemName(name string) *Config {
	c.SystemName = name
	return c
}

// getEnv henter en miljøvariabel med en standardværdi hvis den ikke eksisterer
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getEnvAsInt henter en miljøvariabel som heltal med en standardværdi hvis den ikke eksisterer
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
