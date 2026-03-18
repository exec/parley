//go:build !stresstest

package main

import (
	"github.com/go-chi/chi/v5"
	"parley/internal/auth"
	"parley/internal/db"
)

// registerBenchRoutes is a no-op in production builds.
// The stresstest build tag compiles the real implementation.
func registerBenchRoutes(r chi.Router, repo *db.Repository, authService *auth.AuthService) {}
