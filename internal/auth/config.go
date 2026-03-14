package auth

import (
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
		secretKey = "parley-secret-key-change-in-production"
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