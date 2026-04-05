package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"parley/internal/auth"
)

// ticketIssuer is the interface for issuing and consuming short-lived single-use
// WebSocket upgrade tickets. Two implementations exist: redisTicketStore (shared
// across all API nodes — used in production) and ticketStore (in-memory fallback).
type ticketIssuer interface {
	Issue(userID string) (string, error)
	Consume(ticket string) (string, bool)
}

// ----- Redis implementation (production) -----

// redisTicketStore stores WS tickets in Redis so they survive round-robin routing.
// Tickets are single-use: Consume atomically reads and deletes the key (GETDEL).
type redisTicketStore struct {
	rdb *goredis.Client
}

func newRedisTicketStore(rdb *goredis.Client) *redisTicketStore {
	return &redisTicketStore{rdb: rdb}
}

func (s *redisTicketStore) Issue(userID string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	ticket := hex.EncodeToString(b)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := s.rdb.Set(ctx, "parley:ticket:"+ticket, userID, 60*time.Second).Err(); err != nil {
		return "", err
	}
	return ticket, nil
}

func (s *redisTicketStore) Consume(ticket string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	userID, err := s.rdb.GetDel(ctx, "parley:ticket:"+ticket).Result()
	if err != nil {
		return "", false
	}
	return userID, true
}

// ----- In-memory implementation (dev / Redis unavailable fallback) -----

// ticketStore is an in-memory store of short-lived single-use WebSocket tickets.
// A ticket is issued via POST /api/ws-ticket (authenticated) and consumed once
// during the WebSocket upgrade. This keeps the JWT out of nginx access logs.
//
// WARNING: Not safe for multi-node deployments — use redisTicketStore in production.
type ticketStore struct {
	mu      sync.Mutex
	tickets map[string]ticketEntry
	done    chan struct{}
}

type ticketEntry struct {
	userID    string
	expiresAt time.Time
}

func newTicketStore() *ticketStore {
	ts := &ticketStore{
		tickets: make(map[string]ticketEntry),
		done:    make(chan struct{}),
	}
	go ts.cleanup()
	return ts
}

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
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ts.done:
			return
		case <-ticker.C:
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
}

// Stop signals the cleanup goroutine to exit.
func (ts *ticketStore) Stop() {
	close(ts.done)
}

// handleWsTicket handles POST /api/ws-ticket.
// Validates the caller's JWT and issues a short-lived single-use ticket for the WS upgrade.
func handleWsTicket(authService *auth.AuthService, store ticketIssuer) http.HandlerFunc {
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
