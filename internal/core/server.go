// Package core implements the Fog server — registry, WebSocket, telemetry & control APIs.
package core

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"log/slog"
	"net"
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

	server   *http.Server
	database *sql.DB

	// vehicleRegistry sync.Map // vehicleID -> gatewayURL

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
	Slot      int
}

// NewServer creates a new Fog HTTP server.
func NewServer(addr, appAddr string, db *sql.DB) *Server {
	return &Server{
		Addr:     addr,
		AppAddr:  appAddr,
		database: db,
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
	if s.database != nil {
		if err := s.database.Close(); err != nil {
			slog.Warn("failed to close database", "error", err)
			// Bạn có thể chọn trả về lỗi này hoặc lỗi server
		} else {
			slog.Info("Database connection closed")
		}
	}
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

	gatewayIP, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		slog.Warn("cannot parse gateway address", "remote", r.RemoteAddr, "err", err)
		gatewayIP = r.RemoteAddr
	}
	slog.Debug("received uplink from gateway", "ip", gatewayIP) // var exists bool

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

	// Validate vehicle identity against DB: boat.boatID must match telemetry.VehicleID
	if telemetry.VehicleID == "" {
		// Reject telemetry without vehicle id — require GW to forward vehicle id
		slog.Warn("telemetry without vehicle_id rejected", "from", gatewayIP)
		http.Error(w, "vehicle_id missing", http.StatusBadRequest)
		return
	}
	boatDBID, gwID, err := s.lookupBoatByBoatID(telemetry.VehicleID)
	if err != nil {
		slog.Error("db error lookup boat", "vehicle", telemetry.VehicleID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if boatDBID == 0 {
		// unknown boatID -> reject (possible spoofing)
		slog.Warn("telemetry for unknown boatID dropped", "boatID", telemetry.VehicleID, "from", gatewayIP)
		http.Error(w, "unknown vehicle", http.StatusNotFound)
		return
	}

	// Optional: check that the gateway IP matches the registered gateway for this boat (if gwID present)
	if gwID.Valid {
		var gwIP string
		if err := s.database.QueryRow("SELECT ip FROM gateway WHERE id = ?", gwID.Int64).Scan(&gwIP); err == nil {
			if gwIP != gatewayIP {
				slog.Warn("gateway IP mismatch for telemetry",
					"vehicle", telemetry.VehicleID, "expected_gw_ip", gwIP, "from", gatewayIP)
				// we still accept telemetry but log warning. If you want strict: reject here.
				// http.Error(w, "gateway mismatch", http.StatusForbidden); return
			}
		}
	}

	// Refresh or create session (extend TTL)
	s.sessionMu.Lock()
	if ses, ok := s.sessions[telemetry.VehicleID]; ok {
		ses.CreatedAt = time.Now()
		// TTL unchanged
	} else {
		// create with default TTL (5min)
		s.sessions[telemetry.VehicleID] = &Session{
			VehicleID: telemetry.VehicleID,
			GatewayID: gatewayIP,
			CreatedAt: time.Now(),
			TTL:       5 * time.Minute,
		}
	}
	s.sessionMu.Unlock()

	out, _ := json.Marshal(telemetry)
	s.broadcast(string(out))

	// Forward telemetry to App Server if configured
	if s.AppAddr != "" {
		go func() {
			resp, err := http.Post(
				s.AppAddr+"/api/telemetry",
				"application/json",
				bytes.NewReader(out),
			)
			if err != nil {
				slog.Warn(
					"failed to forward telemetry",
					"component", "fog",
					"app", s.AppAddr,
					"error", err,
				)
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
				slog.Warn(
					"failed to close register request body",
					"component", "server",
					"error", err,
				)
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
	if req.VehicleID == "" || req.GatewayID == "" {
		http.Error(w, "gateway_id or vehicle_id missing", http.StatusBadRequest)
		return
	}

	// Find gateway DB id by its IP/address (gateway table must be populated)
	var gwDBID int64
	err := s.database.QueryRow("SELECT id FROM gateway WHERE ip = ?", req.GatewayID).Scan(&gwDBID)
	if err == sql.ErrNoRows {
		// If gateway not found, insert it (basic)
		res, err := s.database.Exec("INSERT INTO gateway (name, ip) VALUES (?, ?)", req.GatewayID, req.GatewayID)
		if err != nil {
			slog.Error("db insert gateway failed", "gw", req.GatewayID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		gwDBID, _ = res.LastInsertId()
	} else if err != nil {
		slog.Error("db query gateway failed", "gw", req.GatewayID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// Upsert boat row: if exists update gwID, else insert.
	// MySQL-style upsert using UNIQUE constraint on boat.boatID assumed.
	_, err = s.database.Exec(
		"INSERT INTO boat (name, boatID, gwID) VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE gwID = VALUES(gwID)",
		req.VehicleID, req.VehicleID, gwDBID,
	)
	if err != nil {
		slog.Error("db upsert boat failed", "vehicle", req.VehicleID, "gw_id", gwDBID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	// create session for this vehicle
	s.sessionMu.Lock()
	slot := s.assignSlot(req.VehicleID)
	s.sessions[req.VehicleID] = &Session{
		VehicleID: req.VehicleID,
		GatewayID: req.GatewayID,
		CreatedAt: time.Now(),
		TTL:       5 * time.Minute,
		Slot:      slot,
	}
	s.sessionMu.Unlock()

	resp := map[string]any{
		"vehicle_id":         req.VehicleID,
		"ttl":                300,
		"slot":               slot,
		"cycle_start":        time.Now().Unix(), // gateway may use this as immediate cycle start
		"cycle_period_sec":   30,                // example: total cycle length (tune as needed)
		"slot_duration_ms":   4000,              // slot length in ms (example)
		"guard_ms":           300,
		"register_window_ms": 3000,
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.Warn("failed to encode register response", "error", err)
	}
	slog.Info(
		"registered vehicle",
		"vehicle", req.VehicleID,
		"gateway", req.GatewayID,
		"slot", slot,
	)
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
	if vehicleID == "" {
		http.Error(w, "vehicle_id missing", http.StatusBadRequest)
		return
	}

	// Query gateway IP by joining boat -> gateway
	var gatewayIP string
	err := s.database.QueryRow(
		`SELECT g.ip FROM gateway g 
         JOIN boat b ON b.gwID = g.id
         WHERE b.boatID = ?`, vehicleID,
	).Scan(&gatewayIP)
	if err == sql.ErrNoRows {
		http.Error(w, "gateway not found for vehicle", http.StatusNotFound)
		return
	}
	if err != nil {
		slog.Error("db error lookup gateway", "vehicle", vehicleID, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	payload, _ := json.Marshal(ctrl)
	go func() {
		resp, err := http.Post(gatewayIP+"/command",
			"application/json", bytes.NewReader(payload))
		if err != nil {
			slog.Warn("failed to send control to gateway",
				"component", "fog", "gateway", gatewayIP, "error", err)
			return
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		slog.Info("control forwarded",
			"component", "fog", "vehicle", vehicleID, "gateway", gatewayIP)
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
			var removed []string
			for k, v := range s.sessions {
				if now.Sub(v.CreatedAt) > v.TTL {
					delete(s.sessions, k)
					removed = append(removed, k)
					slog.Info("session expired", "vehicle", k)
				}
			}
			s.sessionMu.Unlock()
			// Optionally: inform gateways that slots freed (could send to all registered gateways).
			if len(removed) > 0 {
				slog.Debug("freed slots for vehicles", "vehicles", removed)
				// Implementation: iterate registered gateways and POST update; omitted here for brevity.
			}
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

// --- helper: check boat exists by boatID and get boat.id and gwID ---
func (s *Server) lookupBoatByBoatID(boatID string) (boatDBID int64, gwID sql.NullInt64, err error) {
	var id sql.NullInt64
	var gw sql.NullInt64
	// boat table: id, name, boatID, gwID
	err = s.database.QueryRow("SELECT id, gwID FROM boat WHERE boatID = ?", boatID).Scan(&id, &gw)
	if err == sql.ErrNoRows {
		return 0, sql.NullInt64{}, nil // not found
	}
	if err != nil {
		return 0, sql.NullInt64{}, err
	}
	return id.Int64, gw, nil
}

// --- new helper: assignSlot
// CHANGED: simple sequential slot allocator, reuses freed slots.
// In production you may want more robust allocation (per gateway/region).
func (s *Server) assignSlot(vehicleID string) int {
	// naive: find first unused slot in [0, MaxSlots)
	const MaxSlots = 64 // tune per your network
	used := make([]bool, MaxSlots)
	for _, ses := range s.sessions {
		if ses != nil {
			if ses.Slot >= 0 && ses.Slot < MaxSlots {
				used[ses.Slot] = true
			}
		}
	}
	// if already assigned, return existing
	if ses, ok := s.sessions[vehicleID]; ok {
		return ses.Slot
	}
	for i := range MaxSlots {
		if !used[i] {
			return i
		}
	}
	// fallback: modulo hash
	return int(time.Now().UnixNano() % MaxSlots)
}
