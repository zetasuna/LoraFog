// Package core defines the Server, which bridges gateways and application layers
// via HTTP and WebSocket communication.
package core

// CHANGELOG (refactor v2):
// - Removed parser dependency; JSON only for external communication
// - Context-based lifecycle management
// - Structured logging via slog
// - Renamed registry map to vehicleRegistry (sync.Map)
// - Added safe Close checks to prevent unchecked warnings
// - Improved documentation, naming, and consistency

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"github.com/gorilla/websocket"
)

// Server acts as a lightweight fog layer that receives telemetry from gateways,
// broadcasts updates via WebSocket, and forwards control commands.
type Server struct {
	Addr            string
	AppAddr         string
	server          *http.Server
	sessions        *sessionStore
	vehicleRegistry sync.Map // vehicleID -> gatewayURL

	clientMu sync.Mutex
	clients  map[*websocket.Conn]bool
}

// NewServer initializes a new Server with the given listening and app addresses.
func NewServer(addr, appAddr string) *Server {
	return &Server{
		Addr:    addr,
		AppAddr: appAddr,
		clients: make(map[*websocket.Conn]bool),
	}
}

// RegisterGateway associates a gateway with its managed vehicles.
func (s *Server) RegisterGateway(gatewayID, url string, vehicles []string) {
	for _, v := range vehicles {
		s.vehicleRegistry.Store(v, url)
	}
	slog.Info("gateway registered",
		"component", "fog",
		"gateway", gatewayID,
		"vehicles", vehicles,
	)
}

// Start runs the HTTP server until the provided context is cancelled.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/telemetry", s.handleTelemetry)
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/control", s.handleControl)
	mux.HandleFunc("/ws", s.handleWebSocket)

	addr := strings.TrimPrefix(strings.TrimPrefix(s.Addr, "http://"), "https://")
	s.server = &http.Server{Addr: addr, Handler: mux}

	s.sessions = newSessionStore()
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.sessions.Sweep()
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("fog server listening", "component", "fog", "addr", addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("fog server context cancelled", "component", "fog")
		return s.Shutdown()
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the fog server.
func (s *Server) Shutdown() error {
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	slog.Info("shutting down fog server", "component", "fog")
	if err := s.server.Shutdown(ctx); err != nil {
		slog.Error("fog server shutdown error", "component", "fog", "error", err)
		return err
	}
	slog.Info("fog server stopped", "component", "fog")
	return nil
}

// handleTelemetryRequest processes telemetry JSON and broadcasts it to WebSocket clients.
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close telemetry body", "error", err)
			}
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var telemetry model.VehicleData
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

// handleRegister accepts gateway->fog register requests and returns key+ttl.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	type reqT struct {
		GatewayID string `json:"gateway_id"`
		VehicleID string `json:"vehicle_id"`
	}
	var req reqT
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	// generate 16-byte key
	key, err := util.GenerateRandomKey(16)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	ttl := int64(300) // 5 minutes lease; could be configurable

	session := &Session{
		VehicleID:   req.VehicleID,
		GatewayID:   req.GatewayID,
		Key:         key,
		LeaseExpiry: time.Now().Add(time.Duration(ttl) * time.Second),
		Seq:         0,
	}
	s.sessions.Set(req.VehicleID, session)

	// respond with key hex and ttl
	type respT struct {
		VehicleID string `json:"vehicle_id"`
		KeyHex    string `json:"key_hex"`
		TTL       int64  `json:"ttl"`
	}
	resp := respT{
		VehicleID: req.VehicleID,
		KeyHex:    hex.EncodeToString(key),
		TTL:       ttl,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)

	slog.Info("registered vehicle session", "component", "fog", "vehicle", req.VehicleID, "gateway", req.GatewayID)
}

// handleControlRequest receives control JSON and forwards it to the responsible gateway.
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close control body", "error", err)
			}
		}
	}()

	var control model.ControlData
	if err := json.NewDecoder(r.Body).Decode(&control); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	val, ok := s.vehicleRegistry.Load(control.VehicleID)
	if !ok {
		http.Error(w, "no gateway registered for vehicle", http.StatusNotFound)
		slog.Warn("control ignored: no gateway found",
			"component", "fog", "vehicle", control.VehicleID)
		return
	}
	gatewayURL := val.(string)

	payload, _ := json.Marshal(control)
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
			"component", "fog", "vehicle", control.VehicleID, "gateway", gatewayURL)
	}()

	w.WriteHeader(http.StatusAccepted)
}

// handleWebSocket upgrades an HTTP connection to WebSocket for live telemetry updates.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade failed", "component", "fog", "error", err)
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
				break
			}
		}
	}()
}

// broadcast sends a message to all connected WebSocket clients.
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
