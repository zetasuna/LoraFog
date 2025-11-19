
- /internal/model/config.go
```go
// Package model defines configuration and message structures used across LoraFog.
package model

// CHANGELOG (refactor v2):
// - Added JSON and CBOR tags to all serializable structs
// - Consolidated config structs and added Validate() for config validation
// - Clarified naming and godoc comments

import (
	"errors"
	"fmt"
)

// Config is the root configuration structure for the system.
type Config struct {
	Server         ServerConfig          `yaml:"server" json:"server" cbor:"server"`
	Gateways       []GatewayConfig       `yaml:"gateways" json:"gateways" cbor:"gateways"`
	Vehicles       []VehicleConfig       `yaml:"vehicles" json:"vehicles" cbor:"vehicles"`
	Arduinos       []ArduinoConfig       `yaml:"arduinos" json:"arduinos" cbor:"arduinos"`
	VirtualSerials []VirtualSerialConfig `yaml:"virtual_serials" json:"virtual_serials" cbor:"virtual_serials"`
}

// ServerConfig configures the fog server and app forwarder.
type ServerConfig struct {
	Addr     string            `yaml:"address" json:"address" cbor:"address"`
	AppAddr  string            `yaml:"app_address" json:"app_address" cbor:"app_address"`
	Gateways []GatewayRegistry `yaml:"gateway_registry" json:"gateway_registry" cbor:"gateway_registry"`
}

// GatewayRegistry is used to register gateways to the fog server on startup.
type GatewayRegistry struct {
	ID   string `yaml:"id" json:"id" cbor:"id"`
	Addr string `yaml:"address" json:"address" cbor:"address"`
	// Vehicles []string `yaml:"vehicles" json:"vehicles" cbor:"vehicles"`
}

// GatewayConfig config for a Gateway instance.
type GatewayConfig struct {
	ID         string `yaml:"id" json:"id" cbor:"id"`
	Addr       string `yaml:"address" json:"address" cbor:"address"`
	ServerAddr string `yaml:"server_address" json:"server_address" cbor:"server_address"`
	LoraDev    string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud   int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
}

// VehicleConfig config for a Vehicle agent.
type VehicleConfig struct {
	ID          string `yaml:"id" json:"id" cbor:"id"`
	LoraDev     string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud    int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
	ArduinoDev  string `yaml:"arduino_device" json:"arduino_device" cbor:"arduino_device"`
	ArduinoBaud int    `yaml:"arduino_baud" json:"arduino_baud" cbor:"arduino_baud"`
}

// ArduinoConfig defines a serial Arduino connection.
type ArduinoConfig struct {
	Dev  string `yaml:"device" json:"device" cbor:"device"`
	Baud int    `yaml:"baud" json:"baud" cbor:"baud"`
}

// VirtualSerialConfig defines a pair of linked virtual serial endpoints.
type VirtualSerialConfig struct {
	Left  string `yaml:"left" json:"left" cbor:"left"`
	Right string `yaml:"right" json:"right" cbor:"right"`
}

// Validate checks the configuration for basic correctness.
// It returns an error describing the first problem found.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.Server.Addr == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no server configured")
	}
	if c.Server.Addr == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no gateways configured")
	}
	if c.Server.Addr == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no vehicles configured")
	}
	ids := map[string]struct{}{}
	for i, gw := range c.Gateways {
		if gw.ID == "" {
			return fmt.Errorf("gateways[%d]: id is empty", i)
		}
		if gw.Addr == "" {
			return fmt.Errorf("gateways[%d]: address is empty", i)
		}
		if gw.ServerAddr == "" {
			return fmt.Errorf("gateways[%d]: server_address is empty", i)
		}
		if _, ok := ids[gw.ID]; ok {
			return fmt.Errorf("duplicate id in gateways: %s", gw.ID)
		}
		ids[gw.ID] = struct{}{}
	}
	// Validate vehicles
	for i, v := range c.Vehicles {
		if v.ID == "" {
			return fmt.Errorf("vehicles[%d]: id is empty", i)
		}
	}
	return nil
}

```

- /internal/model/message.go
```go
// Package model provides message definitions exchanged across components.
package model

// CHANGELOG (refactor v2):
// - Added json and cbor struct tags
// - Minor naming cleanup for clarity

// PacketType identifies packet category.
type PacketType string

const (
	PacketTelemetry PacketType = "t"
	PacketControl   PacketType = "c"
)

// Packet encapsulates a typed payload.
// type Packet struct {
// 	Type PacketType `json:"type" cbor:"type"`
// 	Data any        `json:"data" cbor:"data"`
// }

// VehicleData represents telemetry reported by a vehicle.
type VehicleData struct {
	VehicleID   string  `json:"vehicle_id" cbor:"vehicle_id"` // prefer small IDs in LoRa
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	CurrentHead int     `json:"current_head" cbor:"current_head"`
	TargetHead  int     `json:"target_head" cbor:"target_head"`
	LeftSpeed   int     `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int     `json:"right_speed" cbor:"right_speed"`
}

// ControlData represents a control command sent to a vehicle.
type ControlData struct {
	VehicleID string  `json:"vehicle_id" cbor:"vehicle_id"`
	Speed     int     `json:"speed" cbor:"speed"`
	Latitude  float64 `json:"latitude" cbor:"latitude"`
	Longitude float64 `json:"longitude" cbor:"longitude"`
	Kp        float64 `json:"kp" cbor:"kp"`
	Ki        float64 `json:"ki" cbor:"ki"`
	Kd        float64 `json:"kd" cbor:"kd"`
}

// ArduinoData represents raw telemetry from Arduino sensors.
type ArduinoData struct {
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	LeftSpeed   int     `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int     `json:"right_speed" cbor:"right_speed"`
	CurrentHead int     `json:"current_head" cbor:"current_head"`
	TargetHead  int     `json:"target_head" cbor:"target_head"`
}

// ArduinoControl is the format for commands forwarded to Arduino.
type ArduinoControl struct {
	CruiseSpeed int     `json:"cruise_speed" cbor:"cruise_speed"`
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	Kp          float64 `json:"kp" cbor:"kp"`
	Ki          float64 `json:"ki" cbor:"ki"`
	Kd          float64 `json:"kd" cbor:"kd"`
}

// GatewayRegistration is used when registering gateways to the fog server.
type GatewayRegistration struct {
	GatewayID string   `json:"gateway_id" cbor:"gateway_id"`
	Addr      string   `json:"address" cbor:"address"`
	Vehicles  []string `json:"vehicles" cbor:"vehicles"`
}

// BeaconMessage is broadcasted by Gateway to Vehicle.
type BeaconMessage struct {
	Type             string   `json:"type" cbor:"type"`
	Gateway          string   `json:"gateway" cbor:"gateway"`
	Timestamp        int64    `json:"timestamp" cbor:"timestamp"`                   // gateway local unix
	CycleStart       int64    `json:"cycle_start" cbor:"cycle_start"`               // unix seconds for the cycle start (sync)
	CyclePeriodSec   int64    `json:"cycle_period_sec" cbor:"cycle_period_sec"`     // total cycle length in seconds
	SlotDurationMs   int64    `json:"slot_duration_ms" cbor:"slot_duration_ms"`     // slot length in ms
	GuardMs          int64    `json:"guard_ms" cbor:"guard_ms"`                     // guard interval in ms
	RegisterWindowMs int64    `json:"register_window_ms" cbor:"register_window_ms"` // register window at cycle end
	Slots            []string `json:"slots" cbor:"slots"`                           // index->vehicleID ("" for empty)
}

// HelloMessage is sent by Vehicle to Gateway when receive beacon message.
type HelloMessage struct {
	Type      string `json:"type" cbor:"type"`
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	// NonceReply uint32 `json:"nonce_reply" cbor:"nonce_reply"`
}

// AuthMessage (relay from Fog -> Gateway -> Vehicle) contains key and TTL.
type AuthMessage struct {
	Type      string `json:"type" cbor:"type"`
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	TTL       int64  `json:"ttl" cbor:"ttl"` // seconds
	Slot      int    `json:"slot" cbor:"slot"`
}

```

- /internal/util/logger.go
```go
// Package util provides small utilities used by the system.
package util

// CHANGELOG (refactor v2):
// - Centralized slog setup for structured logging
// - Exported SetupLogger for callers to initialize global log behavior

import (
	"log/slog"
	"os"
)

// SetupLogger configures the global slog logger.
// Call once in main prior to starting System.
func SetupLogger() {
	// Use default handler (console) but include time and source.
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("logger initialized", "component", "util")
}

```

- /internal/util/socat.go
```go
// Package util provides helpers for virtual serial management using socat.
package util

// CHANGELOG (refactor v2):
// - Improved process tracking and safe cleanup
// - Added context-aware start with error handling
// - Standardized naming and structured logging

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// SocatManager manages socat processes used to create virtual serial pairs.
type SocatManager struct {
	mu      sync.Mutex
	cmds    []*exec.Cmd
	links   []string
	stopped bool
}

// NewSocatManager constructs an empty SocatManager.
func NewSocatManager() *SocatManager {
	return &SocatManager{}
}

// CreatePair starts a socat process to link two PTYs (left <-> right).
// It returns an error if socat cannot be started.
func (m *SocatManager) CreatePair(left, right string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return fmt.Errorf("socat manager is stopped")
	}

	cmd := exec.CommandContext(context.Background(),
		"socat", "-d", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", left),
		fmt.Sprintf("pty,raw,echo=0,link=%s", right),
	)
	// Direct logs to program stderr/stdout for visibility.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.Warn("failed to start socat", "component", "socat", "left", left, "right", right, "error", err)
		return fmt.Errorf("start socat: %w", err)
	}

	slog.Info("socat started", "component", "socat", "pid", cmd.Process.Pid, "left", left, "right", right)
	m.cmds = append(m.cmds, cmd)
	m.links = append(m.links, left, right)

	// Give socat some time to create the links
	timeout := time.After(500 * time.Millisecond)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return nil
		case <-ticker.C:
			if _, err := os.Stat(left); err == nil {
				return nil
			}
		}
	}
}

// Cleanup stops all started socat processes and removes links created.
func (m *SocatManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.stopped = true

	for _, cmd := range m.cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		p := cmd.Process
		slog.Info("killing socat process", "component", "socat", "pid", p.Pid)
		_ = p.Signal(syscall.SIGTERM)
		// wait with timeout
		done := make(chan error, 1)
		go func(c *exec.Cmd) { done <- c.Wait() }(cmd)
		select {
		case <-time.After(500 * time.Millisecond):
			_ = p.Kill()
		case <-done:
		}
	}

	// Remove links if exist
	for _, path := range m.links {
		if _, err := os.Lstat(path); err == nil {
			if err := os.Remove(path); err != nil {
				slog.Warn("failed to remove socat link", "component", "socat", "path", path, "error", err)
			} else {
				slog.Info("removed socat link", "component", "socat", "path", path)
			}
		}
	}

	// clear slices
	m.cmds = nil
	m.links = nil
	slog.Info("socat cleanup complete", "component", "socat")
}

// CleanupAll is a failsafe that attempts to kill any running socat globally.
func (m *SocatManager) CleanupAll() {
	// Best-effort: use pkill if available.
	_ = exec.Command("pkill", "-f", "socat").Run()
	slog.Info("socat global cleanup attempted", "component", "socat")
}

```

- /internal/core/system.go
```go
// Package core orchestrates system startup and shutdown.
package core

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

// System manages Fog server, gateways, and vehicles.
type System struct {
	configPath   string
	config       *model.Config
	server       *Server
	gateways     []*Gateway
	vehicles     []*Vehicle
	arduinos     []*device.Arduino
	socatManager *util.SocatManager

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSystem constructs a System from YAML configuration.
func NewSystem(path string) (*System, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	sys := &System{
		configPath:   path,
		config:       &cfg,
		socatManager: util.NewSocatManager(),
	}

	for _, vs := range cfg.VirtualSerials {
		_ = sys.socatManager.CreatePair(
			vs.Left,
			vs.Right,
		)
	}
	time.Sleep(300 * time.Millisecond)

	// 1. Mở kết nối DB ở đây (trong main)
	db, err := sql.Open("mysql", "admin:admin@tcp(localhost:3006)/boat")
	if err != nil {
		slog.Error("failed to configure database", "error", err)
		// os.Exit(1)
	}

	// 2. Ping DB để xác nhận kết nối
	if err := db.Ping(); err != nil {
		slog.Error("failed to connect to database", "error", err)
		// os.Exit(1)
	}
	slog.Info("Database connection established")
	if cfg.Server.Addr != "" {
		sys.server = NewServer(
			cfg.Server.Addr,
			cfg.Server.AppAddr,
			db,
		)
	}
	for _, g := range cfg.Gateways {
		sys.gateways = append(sys.gateways, NewGateway(
			g.LoraDev,
			g.LoraBaud,
			g.Addr,
			g.ServerAddr,
		))
	}
	for _, v := range cfg.Vehicles {
		sys.vehicles = append(sys.vehicles, NewVehicle(
			v.ID,
			v.LoraDev,
			v.LoraBaud,
			v.ArduinoDev,
			v.ArduinoBaud,
		))
	}
	for _, a := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos, device.NewArduino(a.Dev, a.Baud))
	}

	return sys, nil
}

// Start launches all system components.
func (s *System) Start(ctx context.Context) error {
	if s.server == nil && len(s.gateways) == 0 && len(s.vehicles) == 0 {
		return fmt.Errorf("no active components")
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	if s.server != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_ = s.server.Start(ctx)
		}()
	}
	for _, gw := range s.gateways {
		_ = gw.Start(ctx)
	}
	for _, vh := range s.vehicles {
		_ = vh.Start(ctx)
	}
	for _, ino := range s.arduinos {
		s.wg.Add(1)
		go func(ino *device.Arduino) {
			defer s.wg.Done()
			stop := make(chan struct{})
			go func() {
				<-ctx.Done()
				close(stop)
			}()
			_ = ino.StartSimulation(stop)
		}(ino)
	}
	return nil
}

// Shutdown gracefully stops all components.
func (s *System) Shutdown() {
	slog.Info("system shutting down")
	if s.cancel != nil {
		s.cancel()
	}
	for _, gw := range s.gateways {
		gw.Shutdown()
	}
	for _, vh := range s.vehicles {
		vh.Shutdown()
	}
	for _, ino := range s.arduinos {
		_ = ino.Close()
	}
	if s.server != nil {
		_ = s.server.Shutdown()
	}
	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}
	s.wg.Wait()
	slog.Info("shutdown complete")
}

```

- /internal/core/server.go
```go
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

```

- /internal/core/vehicle.go
```go
// Package core implements the Vehicle agent responsible for collecting telemetry
// from Arduino devices and sending CBOR-encoded data via LoRa.
package core

// CHANGELOG (refactor v2):
// - Removed parser dependency
// - Vehicle<->Gateway uses CBOR serialization
// - Context-based lifecycle management
// - Structured logging (slog)
// - Renamed methods to Start / Shutdown for consistency
// - Safe Close and consistent log keys

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"

	"github.com/fxamacker/cbor/v2"
)

// Vehicle represents a single autonomous vehicle communicating via LoRa.
type Vehicle struct {
	ID          string
	lora        *device.Lora
	arduino     *device.Arduino
	sessionKey  []byte
	leaseExpiry time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewVehicle constructs a Vehicle agent with LoRa and optional Arduino connection.
func NewVehicle(id, loraDev string, loraBaud int, arduinoDev string, arduinoBaud int) *Vehicle {
	lora := device.NewLora(loraDev, loraBaud)
	v := &Vehicle{
		ID:   id,
		lora: lora,
	}
	if arduinoDev != "" {
		v.arduino = device.NewArduino(arduinoDev, arduinoBaud)
	}
	return v
}

// Start begins the vehicle telemetry and control loops.
func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	// --- Arduino telemetry reader ---
	if v.arduino != nil {
		dataCh := make(chan model.ArduinoData, 5)
		stop, err := v.arduino.Read(dataCh)
		if err != nil {
			slog.Warn("failed to start Arduino reader",
				"component", "vehicle", "id", v.ID, "error", err)
		} else {
			slog.Info("Arduino telemetry reader started",
				"component", "vehicle", "id", v.ID)
			v.wg.Add(1)
			go func() {
				defer v.wg.Done()
				for {
					select {
					case <-ctx.Done():
						slog.Info("stopping Arduino telemetry loop",
							"component", "vehicle", "id", v.ID)
						return
					case data, ok := <-dataCh:
						if !ok {
							slog.Info("Arduino telemetry channel closed",
								"component", "vehicle", "id", v.ID)
							return
						}
						v.sendTelemetry(data)
					}
				}
			}()
			v.wg.Add(1)
			go func() {
				defer v.wg.Done()
				<-ctx.Done()
				stop()
			}()
		}
	}

	// --- LoRa control listener ---
	if v.lora != nil && v.arduino != nil {
		v.wg.Add(1)
		go func() {
			defer v.wg.Done()
			for {
				select {
				case <-ctx.Done():
					slog.Info("stopping LoRa control listener",
						"component", "vehicle", "id", v.ID)
					return
				default:
				}

				frame, err := v.lora.ReadFrame()
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}

				var ctl model.ControlData
				if err := cbor.Unmarshal(frame, &ctl); err != nil {
					slog.Warn("invalid control CBOR packet",
						"component", "vehicle", "id", v.ID, "error", err)
					continue
				}
				if ctl.VehicleID != v.ID {
					slog.Warn("control ignored (wrong target)",
						"component", "vehicle", "id", v.ID, "target", ctl.VehicleID)
					continue
				}

				out := fmt.Sprintf("%d,%.6f,%.6f,%.3f,%.3f,%.3f",
					ctl.Speed, ctl.Latitude, ctl.Longitude, ctl.Kp, ctl.Ki, ctl.Kd)
				if err := v.arduino.Write(out); err != nil {
					slog.Warn("failed to forward control to Arduino",
						"component", "vehicle", "id", v.ID, "error", err)
				} else {
					slog.Info("control forwarded to Arduino",
						"component", "vehicle", "id", v.ID)
				}
			}
		}()
	}

	// start beacon listener (reads frames and handles beacon/ auth)
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			frame, err := v.lora.ReadFrameWithTimeout(10 * time.Second)
			if err != nil {
				// timeout is normal
				continue
			}
			// try to detect message type
			var generic map[string]any
			if err := cbor.Unmarshal(frame, &generic); err != nil {
				slog.Warn("invalid cbor frame", "component", "vehicle", "id", v.ID, "error", err)
				continue
			}
			if t, ok := generic["type"].(string); ok {
				switch t {
				case "beacon":
					// respond with hello
					var b model.BeaconMessage
					_ = cbor.Unmarshal(frame, &b)
					hello := model.HelloMessage{VehicleID: v.ID}
					hb, _ := cbor.Marshal(hello)
					_ = v.lora.WriteFrame(hb)
					slog.Info("sent hello to gateway", "component", "vehicle", "id", v.ID, "gateway", b.Gateway)
				case "auth":
					// receive auth (key)
					var a model.AuthMessage
					_ = cbor.Unmarshal(frame, &a)
					// v.sessionKey = a.Key
					v.leaseExpiry = time.Now().Add(time.Duration(a.TTL) * time.Second)
					slog.Info("received auth and stored session key", "component", "vehicle", "id", v.ID, "ttl", a.TTL)
				default:
					// other types ignored here
				}
			}
		}
	}()

	// renew loop: if we have a key, periodically send a light "renew" (or telemetry) to keep lease alive
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if v.sessionKey != nil && time.Now().Before(v.leaseExpiry) {
					// send a short renew packet (could be telemetry too)
					msg := map[string]any{"type": "renew", "vehicle_id": v.ID}
					b, _ := cbor.Marshal(msg)
					_ = v.lora.WriteFrame(b)
					slog.Debug("sent renew", "component", "vehicle", "id", v.ID)
				} else if v.sessionKey != nil && time.Now().After(v.leaseExpiry) {
					// lease expired locally: drop key
					v.sessionKey = nil
					slog.Info("session expired locally, dropped key", "component", "vehicle", "id", v.ID)
				}
			}
		}
	}()

	return nil
}

// Shutdown stops all goroutines and closes devices safely.
func (v *Vehicle) Shutdown() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.lora != nil {
		if err := v.lora.Close(); err != nil {
			slog.Warn("failed to close LoRa device",
				"component", "vehicle", "id", v.ID, "error", err)
		}
	}
	if v.arduino != nil {
		if err := v.arduino.Close(); err != nil {
			slog.Warn("failed to close Arduino device",
				"component", "vehicle", "id", v.ID, "error", err)
		}
	}
	v.wg.Wait()
	slog.Info("vehicle stopped", "component", "vehicle", "id", v.ID)
}

// sendTelemetry encodes Arduino telemetry as CBOR and writes it via LoRa.
func (v *Vehicle) sendTelemetry(a model.ArduinoData) {
	data := model.VehicleData{
		VehicleID:   v.ID,
		Latitude:    a.Latitude,
		Longitude:   a.Longitude,
		CurrentHead: a.CurrentHead,
		TargetHead:  a.TargetHead,
		LeftSpeed:   a.LeftSpeed,
		RightSpeed:  a.RightSpeed,
	}

	payload, err := cbor.Marshal(data)
	if err != nil {
		slog.Warn("failed to encode telemetry CBOR",
			"component", "vehicle", "id", v.ID, "error", err)
		return
	}
	if v.lora != nil {
		if err := v.lora.WriteFrame(payload); err != nil {
			slog.Warn("failed to send telemetry",
				"component", "vehicle", "id", v.ID, "error", err)
		} else {
			slog.Debug("telemetry sent",
				"component", "vehicle", "id", v.ID)
		}
	}
}

```

- /internal/core/gateway.go
```go
// Package core defines the Gateway component responsible for bridging LoRa-connected
// vehicles with the FogServer using CBOR (for LoRa) and JSON (for HTTP).
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"

	"github.com/fxamacker/cbor/v2"
)

// Gateway represents a LoRa gateway that decodes CBOR messages from vehicles,
// re-encodes them as JSON, and forwards them to the FogServer.
type Gateway struct {
	Addr       string
	ServerAddr string
	slotMap    map[int]string
	lora       *device.Lora
	server     *http.Server
	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
}

// NewGateway creates a new Gateway instance bound to a LoRa serial device.
// func NewGateway(id, loraDev string, loraBaud int, addr, serverAddr string, vehicles []string) *Gateway {
func NewGateway(loraDev string, loraBaud int, addr, serverAddr string) *Gateway {
	lora := device.NewLora(loraDev, loraBaud)

	return &Gateway{
		lora:       lora,
		Addr:       addr,
		ServerAddr: serverAddr,
	}
}

// Start launches the gateway uplink (LoRa→Fog) and downlink (Fog→LoRa) handlers.
func (g *Gateway) Start(ctx context.Context) error {
	g.stopCtx, g.stopCancel = context.WithCancel(ctx)

	if g.lora == nil {
		slog.Warn("gateway running in headless mode (no serial device)",
			"component", "gateway", "addr", g.Addr)
		return nil
	}

	// Start beacon loop
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		beacon := model.BeaconMessage{
			Gateway:   g.Addr,
			Timestamp: time.Now().Unix(),
			// Nonce:     uint32(time.Now().UnixNano() & 0xffffffff),
		}
		// reuse same beacon object but update timestamp/nonce each tick inside BroadcastBeacon
		g.lora.BroadcastBeacon(ctx, 30*time.Second, beacon)
	}()

	// Start LoRa uplink loop and handle HELLO message
	g.wg.Add(1)
	go g.runUplink()

	// Start HTTP server for downlink control (Fog → Vehicle)
	mux := http.NewServeMux()
	mux.HandleFunc("/command", g.handleControlRequest)

	// addr := strings.TrimPrefix(strings.TrimPrefix(g.URL, "http://"), "https://")
	addr := g.Addr
	g.server = &http.Server{Addr: addr, Handler: mux}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		slog.Info("gateway HTTP server started",
			"component", "gateway", "id", g.Addr, "addr", addr)
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway HTTP server error",
				"component", "gateway", "id", g.Addr, "error", err)
		}
	}()

	return nil
}

// runUplink continuously reads telemetry from LoRa and forwards to FogServer.
func (g *Gateway) runUplink() {
	defer g.wg.Done()

	for {
		select {
		case <-g.stopCtx.Done():
			slog.Info("uplink loop stopped", "component", "gateway", "id", g.Addr)
			return
		default:
		}

		frame, err := g.lora.ReadFrameWithTimeout(5 * time.Second)
		if err != nil {
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// attempt to unmarshal as CBOR into generic map to detect type
		var generic map[string]any
		if err := cbor.Unmarshal(frame, &generic); err == nil {
			// determine message type
			if t, ok := generic["type"].(string); ok && t == "hello" {
				// parse HelloMessage
				var hello model.HelloMessage
				if err := cbor.Unmarshal(frame, &hello); err == nil {
					// call fog register API
					registerBody := map[string]string{
						"gateway_id": g.Addr,
						"vehicle_id": hello.VehicleID,
					}
					bodyB, _ := json.Marshal(registerBody)
					resp, err := http.Post("http://"+g.ServerAddr+"/api/register", "application/json", bytes.NewReader(bodyB))
					if err != nil {
						slog.Warn("register request failed", "component", "gateway", "id", g.Addr, "error", err)
						continue
					}
					if resp != nil && resp.Body != nil {
						_ = resp.Body.Close()
					}
					var regResp struct {
						VehicleID        string `json:"vehicle_id"`
						TTL              int64  `json:"ttl"`
						Slot             int    `json:"slot"` // CHANGED: server returns assigned slot
						CycleStart       int64  `json:"cycle_start"`
						CyclePeriod      int64  `json:"cycle_period_sec"`
						SlotDurMs        int64  `json:"slot_duration_ms"`
						GuardMs          int64  `json:"guard_ms"`
						RegisterWindowMs int64  `json:"register_window_ms"`
					}
					_ = json.NewDecoder(resp.Body).Decode(&regResp)
					_ = resp.Body.Close()

					// you probably want a proper slot map:
					if g.slotMap == nil {
						g.slotMap = make(map[int]string)
					}
					g.slotMap[regResp.Slot] = regResp.VehicleID

					// prepare auth relay (if key provided). CHANGED: Auth now contains slot/ttl
					auth := model.AuthMessage{
						Type:      "auth",
						VehicleID: regResp.VehicleID,
						TTL:       regResp.TTL,
						Slot:      regResp.Slot,
					}
					if err := g.lora.SendAuthRelay(auth); err != nil {
						slog.Warn("send auth to vehicle failed", "component", "gateway", "id", g.Addr, "vehicle", regResp.VehicleID, "error", err)
					} else {
						slog.Info("auth relayed to vehicle", "component", "gateway", "id", g.Addr, "vehicle", regResp.VehicleID)
					}
				}
			}
		}

		var telemetry model.VehicleData
		if err := cbor.Unmarshal(frame, &telemetry); err != nil {
			slog.Warn("failed to decode CBOR telemetry",
				"component", "gateway", "id", g.Addr, "error", err)
			continue
		}
		payloadJSON, _ := json.Marshal(telemetry)
		resp, err := http.Post("http://"+g.ServerAddr+"/api/telemetry", "application/json", bytes.NewReader(payloadJSON))
		if err != nil {
			slog.Warn("failed to forward telemetry",
				"component", "gateway", "id", g.Addr, "error", err)
			continue
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		slog.Info("uplink telemetry sent",
			"component", "gateway", "id", g.Addr, "vehicle", telemetry.VehicleID)
	}
}

// handleControlRequest receives control JSON and sends it via LoRa using CBOR.
func (g *Gateway) handleControlRequest(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close control request body",
					"component", "gateway", "id", g.Addr, "error", err)
			}
		}
	}()

	var control model.ControlData
	if err := json.NewDecoder(r.Body).Decode(&control); err != nil {
		http.Error(w, "invalid control JSON", http.StatusBadRequest)
		return
	}

	b, err := cbor.Marshal(control)
	if err != nil {
		http.Error(w, "failed to encode CBOR", http.StatusInternalServerError)
		return
	}

	if err := g.lora.WriteFrame(b); err != nil {
		http.Error(w, "failed to send control to vehicle", http.StatusInternalServerError)
		slog.Error("failed to write control to LoRa device",
			"component", "gateway", "id", g.Addr, "error", err)
		return
	}

	slog.Info("control sent to vehicle",
		"component", "gateway", "id", g.Addr, "vehicle", control.VehicleID)
	w.WriteHeader(http.StatusAccepted)
}

// Shutdown gracefully stops the gateway and closes resources.
func (g *Gateway) Shutdown() {
	slog.Info("stopping gateway", "component", "gateway", "id", g.Addr)

	if g.stopCancel != nil {
		g.stopCancel()
	}

	if g.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			slog.Warn("gateway HTTP server shutdown error",
				"component", "gateway", "id", g.Addr, "error", err)
		}
	}

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn("failed to close device",
				"component", "gateway", "id", g.Addr, "error", err)
		}
	}

	g.wg.Wait()
	slog.Info("gateway stopped", "component", "gateway", "id", g.Addr)
}

```

- /internal/device/arduino.go
```go
// Package device implements ArduinoDevice for reading and writing telemetry
// over serial, as well as simulation support.
package device

// CHANGELOG (refactor v2):
// - Context-based lifecycle and safe shutdown
// - Replaced stop channel with function-returned closure
// - Structured logging (slog)
// - Safe Close() with nil checks
// - Added telemetry simulation helper

import (
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"LoraFog/internal/model"
)

// Arduino represents a serial connection to an Arduino controller.
type Arduino struct {
	Device string
	Baud   int
	serial *Serial
}

// NewArduino creates and connects to an Arduino serial device.
func NewArduino(dev string, baud int) *Arduino {
	s, err := NewSerial(dev, baud)
	if err != nil {
		slog.Warn("failed to connect Arduino",
			"component", "arduino", "device", dev, "error", err)
	}
	return &Arduino{
		Device: dev,
		Baud:   baud,
		serial: s,
	}
}

// Read starts reading Arduino telemetry in a background goroutine and pushes it into dataCh.
// It returns a stop function that can be called to terminate the loop safely.
func (a *Arduino) Read(dataCh chan<- model.ArduinoData) (func(), error) {
	if a.serial == nil {
		return nil, fmt.Errorf("arduino serial not initialized")
	}

	stop := make(chan struct{})
	go func() {
		defer close(dataCh)
		for {
			select {
			case <-stop:
				slog.Info("stopping Arduino read loop",
					"component", "arduino", "device", a.Device)
				return
			default:
			}

			line, err := a.serial.ReadLine(0)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			var d model.ArduinoData
			if _, err := fmt.Sscanf(line, "%f,%f,%d,%d,%d,%d",
				&d.Latitude, &d.Longitude, &d.CurrentHead,
				&d.TargetHead, &d.LeftSpeed, &d.RightSpeed); err != nil {
				continue
			}
			select {
			case dataCh <- d:
			default:
			}
		}
	}()
	return func() { close(stop) }, nil
}

// Write sends a single line to the Arduino serial interface.
func (a *Arduino) Write(line string) error {
	if a.serial == nil {
		return fmt.Errorf("arduino serial not initialized")
	}
	return a.serial.WriteLine(line)
}

// StartSimulation generates synthetic telemetry data periodically for testing.
func (a *Arduino) StartSimulation(stop <-chan struct{}) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	slog.Info("starting Arduino simulation",
		"component", "arduino", "device", a.Device)

	for {
		select {
		case <-stop:
			slog.Info("stopping Arduino simulation",
				"component", "arduino", "device", a.Device)
			return nil
		case <-ticker.C:
			data := model.ArduinoData{
				Latitude:    21.027 + rand.Float64()*0.001,
				Longitude:   105.835 + rand.Float64()*0.001,
				CurrentHead: rand.Intn(361),
				TargetHead:  rand.Intn(361),
				LeftSpeed:   1000 + rand.Intn(1000),
				RightSpeed:  1000 + rand.Intn(1000),
			}
			line := fmt.Sprintf("%f,%f,%d,%d,%d,%d",
				data.Latitude, data.Longitude, data.CurrentHead,
				data.TargetHead, data.LeftSpeed, data.RightSpeed)

			if err := a.Write(line); err != nil {
				slog.Warn("failed to write simulated telemetry",
					"component", "arduino", "device", a.Device, "error", err)
			}
		}
	}
}

// Close closes the Arduino serial port safely.
func (a *Arduino) Close() error {
	if a.serial == nil {
		return nil
	}
	return a.serial.Close()
}

```

- /internal/device/lora.go
```go
// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

// CHANGELOG (refactor v2):
// - Added new binary-based LoraDevice (no simulation, unlike Arduino)
// - Uses Serial.ReadBytes() and WriteBytes() for CBOR payloads
// - Safe close and structured logging (slog)

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/fxamacker/cbor/v2"
)

// Lora manages binary (CBOR) communication over a serial LoRa interface.
type Lora struct {
	Path   string
	Baud   int
	serial *Serial
}

// NewLora creates a new Lora.
func NewLora(path string, baud int) *Lora {
	s, err := NewSerial(path, baud)
	if err != nil {
		slog.Warn("failed to connect Lora device",
			"component", "lora", "device", path, "error", err)
	}
	return &Lora{
		Path:   path,
		Baud:   baud,
		serial: s,
	}
}

// ReadFrame reads a binary frame from the LoRa serial interface.
// It expects a 2-byte length prefix followed by that many bytes of CBOR data.
func (l *Lora) ReadFrame() ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("lora serial not initialized")
	}

	header := make([]byte, 2)
	if _, err := io.ReadFull(l.serial.reader, header); err != nil {
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	length := binary.BigEndian.Uint16(header)
	if length == 0 {
		return nil, fmt.Errorf("invalid frame length 0")
	}

	payload, err := l.serial.ReadBytes(int(length))
	if err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}
	return payload, nil
}

// ReadFrameWithTimeout tries to read a CBOR frame with timeout.
func (l *Lora) ReadFrameWithTimeout(timeout time.Duration) ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("lora serial not initialized")
	}

	done := make(chan struct{})
	var result []byte
	var err error

	go func() {
		result, err = l.ReadFrame()
		close(done)
	}()

	select {
	case <-done:
		return result, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("read frame timeout after %s", timeout)
	}
}

// WriteFrame sends a binary CBOR frame with a 2-byte big-endian length prefix.
func (l *Lora) WriteFrame(payload []byte) error {
	if l.serial == nil {
		return fmt.Errorf("lora serial not initialized")
	}
	length := uint16(len(payload))
	frame := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], length)
	copy(frame[2:], payload)
	return l.serial.WriteBytes(frame)
}

func (l *Lora) WriteBytes(b []byte) error {
	return l.serial.WriteBytes(b)
}

// Close safely closes the LoRa serial device.
func (l *Lora) Close() error {
	if l.serial == nil {
		return nil
	}
	return l.serial.Close()
}

// BroadcastBeacon periodically writes a BeaconMessage as a CBOR frame.
// ctx controls lifecycle; interval defines broadcast frequency.
func (l *Lora) BroadcastBeacon(ctx context.Context, interval time.Duration, beacon any) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping beacon broadcast", "component", "lora", "device", l.Path)
			return
		case <-ticker.C:
			b, err := cbor.Marshal(beacon)
			if err != nil {
				slog.Warn("encode beacon failed", "component", "lora", "device", l.Path, "error", err)
				continue
			}
			if err := l.WriteFrame(b); err != nil {
				slog.Warn("write beacon frame failed", "component", "lora", "device", l.Path, "error", err)
				continue
			}
			slog.Debug("beacon broadcasted", "component", "lora", "device", l.Path)
		}
	}
}

// SendAuthRelay sends an AuthMessage CBOR frame (used by Gateway to relay server auth to vehicle).
func (l *Lora) SendAuthRelay(auth any) error {
	b, err := cbor.Marshal(auth)
	if err != nil {
		return err
	}
	return l.WriteFrame(b)
}

```

- /internal/device/serial.go
```go
// Package device implements a simple wrapper for serial communication.
// It provides non-blocking read/write methods with optional timeout.
package device

// CHANGELOG (refactor v2):
// - Safe read/write with timeout
// - Added context support via external control
// - Structured logging (slog)
// - Safe Close() checks and standardized naming

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"go.bug.st/serial"
)

// Serial represents a simple serial port connection.
type Serial struct {
	Port     serial.Port
	Path     string
	BaudRate int
	mu       sync.Mutex
	reader   *bufio.Reader
}

// NewSerial opens a serial port with the given path and baud rate.
func NewSerial(path string, baud int) (*Serial, error) {
	mode := &serial.Mode{BaudRate: baud}
	port, err := serial.Open(path, mode)
	if err != nil {
		return nil, fmt.Errorf("open serial port %s: %w", path, err)
	}
	s := &Serial{
		Port:     port,
		Path:     path,
		BaudRate: baud,
		reader:   bufio.NewReader(port),
	}
	slog.Info("serial port opened",
		"component", "serial", "path", path, "baud", baud)
	return s, nil
}

// ReadLine reads a line of data with an optional timeout (in milliseconds).
func (s *Serial) ReadLine(timeoutMs int) (string, error) {
	if s.Port == nil {
		return "", errors.New("serial port not initialized")
	}

	if timeoutMs > 0 {
		if err := s.Port.SetReadTimeout(time.Duration(timeoutMs) * time.Millisecond); err != nil {
			slog.Warn("failed to set read timeout",
				"component", "serial", "path", s.Path, "error", err)
		}
	}

	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return line, nil
}

// WriteLine writes a single line (with newline terminator) to the serial port.
func (s *Serial) WriteLine(data string) error {
	if s.Port == nil {
		return errors.New("serial port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.Port.Write([]byte(data + "\n")); err != nil {
		slog.Warn("failed to write to serial",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	return nil
}

// ReadBytes reads exactly n bytes from the serial port.
// It blocks until all bytes are received or an error occurs.
func (s *Serial) ReadBytes(n int) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("serial port not initialized")
	}
	buf := make([]byte, n)
	total := 0
	for total < n {
		readCount, err := io.ReadFull(s.reader, buf[total:])
		total += readCount
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

// WriteBytes writes raw binary data to the serial port without newline.
func (s *Serial) WriteBytes(b []byte) error {
	if s.Port == nil {
		return errors.New("serial port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Port.Write(b); err != nil {
		slog.Warn("failed to write bytes to serial",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	return nil
}

// Close closes the serial port safely.
func (s *Serial) Close() error {
	if s.Port == nil {
		return nil
	}
	if err := s.Port.Close(); err != nil {
		slog.Warn("failed to close serial port",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	slog.Info("serial port closed", "component", "serial", "path", s.Path)
	return nil
}

```
