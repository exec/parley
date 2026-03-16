package main

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"parley/internal/auth"
)

// ticketStore is an in-memory store of short-lived single-use WebSocket tickets.
// A ticket is issued via POST /api/ws-ticket (authenticated) and consumed once
// during the WebSocket upgrade. This keeps the JWT out of nginx access logs.
type ticketStore struct {
	mu      sync.Mutex
	tickets map[string]ticketEntry
}

type ticketEntry struct {
	userID    string
	expiresAt time.Time
}

func newTicketStore() *ticketStore {
	ts := &ticketStore{tickets: make(map[string]ticketEntry)}
	go ts.cleanup()
	return ts
}

// Issue generates a random 32-byte (64-char hex) single-use ticket for the given user.
func (ts *ticketStore) Issue(userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(b)
	ts.mu.Lock()
	ts.tickets[ticket] = ticketEntry{userID: userID, expiresAt: time.Now().Add(60 * time.Second)}
	ts.mu.Unlock()
	return ticket, nil
}

// Consume validates and removes a ticket, returning the associated userID.
// Returns ("", false) if the ticket is unknown or expired.
func (ts *ticketStore) Consume(ticket string) (string, bool) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	entry, ok := ts.tickets[ticket]
	if !ok || time.Now().After(entry.expiresAt) {
		delete(ts.tickets, ticket)
		return "", false
	}
	delete(ts.tickets, ticket)
	return entry.userID, true
}

func (ts *ticketStore) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		now := time.Now()
		ts.mu.Lock()
		for k, v := range ts.tickets {
			if now.After(v.expiresAt) {
				delete(ts.tickets, k)
			}
		}
		ts.mu.Unlock()
	}
}

// handleWsTicket handles POST /api/ws-ticket.
// Validates the caller's JWT and issues a short-lived single-use ticket for the WS upgrade.
func handleWsTicket(authService *auth.AuthService, store *ticketStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// The endpoint is registered inside the JWT middleware group, so userID is already validated.
		userID := auth.GetUserIDFromContext(r)
		if userID == "" {
			jsonError(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		ticket, err := store.Issue(userID)
		if err != nil {
			jsonError(w, "failed to issue ticket", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"ticket": ticket})
	}
}
