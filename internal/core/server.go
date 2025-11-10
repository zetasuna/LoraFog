// Package core implements the Fog server — registry, WebSocket, telemetry & control APIs.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/model"

	"github.com/gorilla/websocket"
)

// Server is the lightweight Fog backend managing registry and telemetry.
type Server struct {
	Addr    string
	AppAddr string

	server *http.Server

	vehicleRegistry sync.Map // vehicleID -> gatewayURL

	clients   map[*websocket.Conn]bool
	clientMu  sync.Mutex
	sessionMu sync.Mutex
	sessions  map[string]*Session
}

// Session stores temporary authentication for a vehicle.
type Session struct {
	VehicleID string
	GatewayID string
	CreatedAt time.Time
	TTL       time.Duration
}

// NewServer creates a new Fog HTTP server.
func NewServer(addr, appAddr string) *Server {
	return &Server{
		Addr:     addr,
		AppAddr:  appAddr,
		clients:  make(map[*websocket.Conn]bool),
		sessions: make(map[string]*Session),
	}
}

// Start runs the HTTP server and session sweeper.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/telemetry", s.handleTelemetry)
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/control", s.handleControl)
	mux.HandleFunc("/api/gw/report", s.handleGatewayReport)
	mux.HandleFunc("/ws", s.handleWebSocket)

	addr := strings.TrimPrefix(strings.TrimPrefix(s.Addr, "http://"), "https://")
	s.server = &http.Server{Addr: addr, Handler: mux}

	go s.sweeper(ctx)
	slog.Info("Fog server started", "addr", addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown stops the HTTP server.
func (s *Server) Shutdown() error {
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	slog.Info("Stopping Fog server")
	return s.server.Shutdown(ctx)
}

// handleTelemetry accepts uplink telemetry and relays to app/websocket.
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close telemetry request body",
					"component", "server", "error", err)
			}
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var telemetry model.TelemetryData
	if err := json.Unmarshal(body, &telemetry); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	out, _ := json.Marshal(telemetry)
	s.broadcast(string(out))

	// Forward telemetry to App Server if configured
	if s.AppAddr != "" {
		go func() {
			resp, err := http.Post(s.AppAddr+"/api/telemetry",
				"application/json", bytes.NewReader(out))
			if err != nil {
				slog.Warn("failed to forward telemetry",
					"component", "fog", "app", s.AppAddr, "error", err)
				return
			}
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
		}()
	}
	w.WriteHeader(http.StatusOK)
}

// handleRegister registers a vehicle and returns session info.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close register request body",
					"component", "server", "error", err)
			}
		}
	}()

	var req struct {
		GatewayID string `json:"gateway_id"`
		VehicleID string `json:"vehicle_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.sessionMu.Lock()
	s.sessions[req.VehicleID] = &Session{
		VehicleID: req.VehicleID,
		GatewayID: req.GatewayID,
		CreatedAt: time.Now(),
		TTL:       5 * time.Minute,
	}
	s.sessionMu.Unlock()

	resp := map[string]any{"vehicle_id": req.VehicleID, "key_hex": "", "ttl": 300}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	slog.Info("registered vehicle", "vehicle", req.VehicleID, "gateway", req.GatewayID)
}

// handleControl forwards control messages to the target gateway.
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close control request body",
					"component", "server", "error", err)
			}
		}
	}()

	var ctrl map[string]any
	if err := json.NewDecoder(r.Body).Decode(&ctrl); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	vehicleID, _ := ctrl["vehicle_id"].(string)
	val, ok := s.vehicleRegistry.Load(vehicleID)
	if !ok {
		http.Error(w, "gateway not found", http.StatusNotFound)
		return
	}
	gatewayURL := val.(string)
	payload, _ := json.Marshal(ctrl)
	go func() {
		resp, err := http.Post(gatewayURL+"/command",
			"application/json", bytes.NewReader(payload))
		if err != nil {
			slog.Warn("failed to send control to gateway",
				"component", "fog", "gateway", gatewayURL, "error", err)
			return
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		slog.Info("control forwarded",
			"component", "fog", "vehicle", vehicleID, "gateway", gatewayURL)
	}()
	w.WriteHeader(http.StatusAccepted)
}

// handleWebSocket provides live telemetry streaming to dashboard clients.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.clientMu.Lock()
	s.clients[conn] = true
	s.clientMu.Unlock()

	go func() {
		defer func() {
			s.clientMu.Lock()
			delete(s.clients, conn)
			s.clientMu.Unlock()
			if err := conn.Close(); err != nil {
				slog.Warn("failed to close websocket", "error", err)
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

// handleGatewayReport logs periodic gateway reports.
func (s *Server) handleGatewayReport(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close report request body",
					"component", "server", "error", err)
			}
		}
	}()

	var rep struct {
		GWID      string `json:"gw_id"`
		Region    string `json:"region"`
		SlotUsage int    `json:"slotUsage"`
		AvgDelay  int    `json:"avgDelay"`
		Collision int    `json:"collision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	slog.Info("gateway report", "gw", rep.GWID, "usage", rep.SlotUsage, "delay", rep.AvgDelay, "coll", rep.Collision)
	w.WriteHeader(http.StatusOK)
}

// sweeper cleans expired sessions.
func (s *Server) sweeper(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := time.Now()
			s.sessionMu.Lock()
			for k, v := range s.sessions {
				if now.Sub(v.CreatedAt) > v.TTL {
					delete(s.sessions, k)
					slog.Info("session expired", "vehicle", k)
				}
			}
			s.sessionMu.Unlock()
		}
	}
}

// broadcast sends message to all connected WebSocket clients.
func (s *Server) broadcast(msg string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for conn := range s.clients {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			slog.Warn("websocket send failed",
				"component", "fog", "error", err)
		}
	}
}
