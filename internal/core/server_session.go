package core

import (
	"log/slog"
	"sync"
	"time"
)

// Session represents a vehicle session on the fog server.
type Session struct {
	VehicleID   string
	GatewayID   string
	Key         []byte
	LeaseExpiry time.Time
	Seq         uint32
}

// sessionStore is a simple in-memory session storage with janitor.
type sessionStore struct {
	mu    sync.Mutex
	store map[string]*Session // vehicleID -> session
}

func newSessionStore() *sessionStore {
	return &sessionStore{store: make(map[string]*Session)}
}

func (s *sessionStore) Set(vehicle string, sess *Session) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.store[vehicle] = sess
}

func (s *sessionStore) Get(vehicle string) (*Session, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ss, ok := s.store[vehicle]
	return ss, ok
}

func (s *sessionStore) Delete(vehicle string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.store, vehicle)
}

// Sweep removes expired sessions.
func (s *sessionStore) Sweep() {
	now := time.Now()
	s.mu.Lock()
	for id, ss := range s.store {
		if now.After(ss.LeaseExpiry) {
			delete(s.store, id)
			slog.Info("session expired and removed", "component", "fog", "vehicle", id)
		}
	}
	s.mu.Unlock()
}
