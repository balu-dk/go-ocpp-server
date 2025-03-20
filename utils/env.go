package env

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"

	"github.com/joho/godotenv"
)

// Initialize loads environment variables from .env files
func Initialize() {
	// Get execution directory
	exPath, err := os.Executable()
	if err != nil {
		log.Printf("Warning: Could not determine executable path: %v", err)
		exPath = "."
	}
	exDir := filepath.Dir(exPath)

	// Possible .env file locations
	locations := []string{
		".env",                       // Current directory
		filepath.Join(exDir, ".env"), // Executable directory
		"/app/.env",                  // Docker container path
		"/etc/ocpp-server/.env",      // System config path
		filepath.Join(os.Getenv("HOME"), ".ocpp-server.env"), // User home directory
	}

	// Try to load from each location
	loaded := false
	for _, location := range locations {
		if _, err := os.Stat(location); err == nil {
			if err := godotenv.Load(location); err == nil {
				log.Printf("Loaded environment from %s", location)
				loaded = true
				break
			}
		}
	}

	// If no .env file found, try to load from .env.example if it exists
	if !loaded {
		if _, err := os.Stat(".env.example"); err == nil {
			if err := godotenv.Load(".env.example"); err == nil {
				log.Printf("Loaded environment from .env.example")
			}
		}
	}

	// Note: if no .env file is found, the application will continue with
	// environment variables from the system, which is fine for Docker
}

// GetEnv gets an environment variable with a default value
func GetEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}

// GetEnvAsBool gets an environment variable as a boolean
func GetEnvAsBool(key string, defaultValue bool) bool {
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

// GetEnvAsInt gets an environment variable as an integer
func GetEnvAsInt(key string, defaultValue int) int {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.Atoi(valueStr)
	if err != nil {
		return defaultValue
	}

	return value
}

// GetEnvAsInt64 gets an environment variable as a 64-bit integer
func GetEnvAsInt64(key string, defaultValue int64) int64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseInt(valueStr, 10, 64)
	if err != nil {
		return defaultValue
	}

	return value
}

// GetEnvAsFloat gets an environment variable as a float
func GetEnvAsFloat(key string, defaultValue float64) float64 {
	valueStr := os.Getenv(key)
	if valueStr == "" {
		return defaultValue
	}

	value, err := strconv.ParseFloat(valueStr, 64)
	if err != nil {
		return defaultValue
	}

	return value
}

// MustGetEnv gets an environment variable or panics if it doesn't exist
func MustGetEnv(key string) string {
	value := os.Getenv(key)
	if value == "" {
		panic(fmt.Sprintf("Required environment variable %s is not set", key))
	}
	return value
}
