package auth

import (
	"log"
	"os"
	"time"
)

// Config holds JWT configuration settings
type Config struct {
	SecretKey   string
	TokenExpiry time.Duration
}

// DefaultConfig returns the default JWT configuration
func DefaultConfig() *Config {
	secretKey := os.Getenv("JWT_SECRET")
	if secretKey == "" {
		log.Fatal("JWT_SECRET environment variable is not set — refusing to start with an insecure default")
	}

	return &Config{
		SecretKey:   secretKey,
		TokenExpiry: 24 * time.Hour,
	}
}

// GetConfig returns the global config instance
func GetConfig() *Config {
	return defaultConfig
}

var defaultConfig = DefaultConfig()