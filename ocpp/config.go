package ocppserver

import (
	"fmt"
	env "ocpp-server/utils"
	"os"
	"strconv"
)

// Config contains configuration for the OCPP server
type Config struct {
	// Host is the domain name or IP address the server should use
	Host string

	// WebSocketPort is the port on which the OCPP WebSocket server should listen
	WebSocketPort int

	// APIPort is the port on which the HTTP API server should listen
	APIPort int

	// SystemName is the name of the central server
	SystemName string

	// TLS configuration
	// UseTLS indicates whether to use TLS (HTTPS/WSS)
	UseTLS bool

	// CertFile is the path to the TLS certificate file
	CertFile string

	// KeyFile is the path to the TLS key file
	KeyFile string
}

// NewConfig creates a new configuration with default values and environment variables
func NewConfig() *Config {
	config := &Config{
		Host:          env.GetEnv("OCPP_HOST", "localhost"),
		WebSocketPort: env.GetEnvAsInt("OCPP_WEBSOCKET_PORT", 9000),
		APIPort:       env.GetEnvAsInt("OCPP_API_PORT", 9001),
		SystemName:    env.GetEnv("OCPP_SYSTEM_NAME", "ocpp-central"),
		UseTLS:        env.GetEnvAsBool("OCPP_USE_TLS", false),
		CertFile:      env.GetEnv("OCPP_CERT_FILE", "cert.pem"),
		KeyFile:       env.GetEnv("OCPP_KEY_FILE", "key.pem"),
	}

	return config
}

// WebSocketAddr returns the full address for the WebSocket server in "host:port" format
func (c *Config) WebSocketAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.WebSocketPort)
}

// APIAddr returns the full address for the API server in "host:port" format
func (c *Config) APIAddr() string {
	return fmt.Sprintf("%s:%d", c.Host, c.APIPort)
}

// WithHost sets the host or IP for the servers
func (c *Config) WithHost(host string) *Config {
	c.Host = host
	return c
}

// WithWebSocketPort sets the port for the WebSocket server
func (c *Config) WithWebSocketPort(port int) *Config {
	c.WebSocketPort = port
	return c
}

// WithAPIPort sets the port for the API server
func (c *Config) WithAPIPort(port int) *Config {
	c.APIPort = port
	return c
}

// WithSystemName sets the system name
func (c *Config) WithSystemName(name string) *Config {
	c.SystemName = name
	return c
}

// getEnv retrieves an environment variable with a default value if it doesn't exist
func getEnv(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

// getEnvAsInt retrieves an environment variable as an integer with a default value if it doesn't exist
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

// getEnvAsBool retrieves an environment variable as a boolean with a default value if it doesn't exist
func getEnvAsBool(key string, defaultValue bool) bool {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseBool(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}
