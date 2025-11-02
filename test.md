
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
	FogAddr  string            `yaml:"fog_addr" json:"fog_addr" cbor:"fog_addr"`
	AppAddr  string            `yaml:"app_addr" json:"app_addr" cbor:"app_addr"`
	Gateways []GatewayRegistry `yaml:"gateway_registry" json:"gateway_registry" cbor:"gateway_registry"`
}

// GatewayRegistry is used to register gateways to the fog server on startup.
type GatewayRegistry struct {
	ID       string   `yaml:"id" json:"id" cbor:"id"`
	URL      string   `yaml:"url" json:"url" cbor:"url"`
	Vehicles []string `yaml:"vehicles" json:"vehicles" cbor:"vehicles"`
}

// GatewayConfig config for a Gateway instance.
type GatewayConfig struct {
	ID       string   `yaml:"id" json:"id" cbor:"id"`
	URL      string   `yaml:"url" json:"url" cbor:"url"`
	FogURL   string   `yaml:"fog_url" json:"fog_url" cbor:"fog_url"`
	LoraDev  string   `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud int      `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
	Vehicles []string `yaml:"vehicles" json:"vehicles" cbor:"vehicles"`
}

// VehicleConfig config for a Vehicle agent.
type VehicleConfig struct {
	ID                  string `yaml:"id" json:"id" cbor:"id"`
	TelemetryIntervalMs int    `yaml:"telemetry_interval_ms" json:"telemetry_interval_ms" cbor:"telemetry_interval_ms"`
	LoraDev             string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud            int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
	ArduinoDev          string `yaml:"arduino_device" json:"arduino_device" cbor:"arduino_device"`
	ArduinoBaud         int    `yaml:"arduino_baud" json:"arduino_baud" cbor:"arduino_baud"`
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
	if c.Server.FogAddr == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no fog address and no gateways/vehicles configured")
	}
	ids := map[string]struct{}{}
	for i, gw := range c.Gateways {
		if gw.ID == "" {
			return fmt.Errorf("gateways[%d]: id is empty", i)
		}
		if gw.FogURL == "" {
			return fmt.Errorf("gateways[%d]: fog_url is empty", i)
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
type Packet struct {
	Type PacketType `json:"type" cbor:"type"`
	Data any        `json:"data" cbor:"data"`
}

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
	URL       string   `json:"url" cbor:"url"`
	Vehicles  []string `json:"vehicles" cbor:"vehicles"`
}

// BeaconMessage is broadcasted by Gateway to Vehicle.
type BeaconMessage struct {
	// GatewayID short id (nên dùng string ngắn hoặc uint16)
	GatewayID string `json:"gateway_id" cbor:"gateway_id"`
	Timestamp int64  `json:"timestamp" cbor:"timestamp"`
	// Nonce     uint32 `json:"nonce" cbor:"nonce"` // 4-byte nonce
}

// HelloMessage is sent by Vehicle to Gateway when receive beacon message.
type HelloMessage struct {
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	// NonceReply uint32 `json:"nonce_reply" cbor:"nonce_reply"`
}

// AuthMessage (relay from Fog -> Gateway -> Vehicle) contains key and TTL.
type AuthMessage struct {
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	// Key       []byte `json:"key" cbor:"key"`
	TTL int64 `json:"ttl" cbor:"ttl"` // seconds
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

- /internal/util/hmac.go
```go
package util

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
)

// GenerateRandomKey returns n random bytes (use n=16 for AES-128 / HMAC key).
func GenerateRandomKey(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return nil, fmt.Errorf("generate random key: %w", err)
	}
	return b, nil
}

// ComputeHMAC computes HMAC-SHA256 and returns the first tagLen bytes.
func ComputeHMAC(key, data []byte, tagLen int) []byte {
	m := hmac.New(sha256.New, key)
	_, _ = m.Write(data)
	full := m.Sum(nil)
	if tagLen <= 0 || tagLen > len(full) {
		tagLen = len(full)
	}
	return full[:tagLen]
}

// VerifyHMAC compares truncated HMAC in constant time.
func VerifyHMAC(key, data, tag []byte) bool {
	expected := ComputeHMAC(key, data, len(tag))
	return hmac.Equal(expected, tag)
}

```

- /internal/util/packet.go
```go
// internal/util/packet.go
// CHANGELOG:
// - New helpers for building and verifying wire frames used on LoRa:
//   Frame layout: [LEN(2)][NONCE(4)][HMAC(8)][PAYLOAD]
// - Uses util.ComputeHMAC / VerifyHMAC for HMAC-SHA256 truncated

package util

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Frame constants
const (
	NonceLen  = 4
	HMACLen   = 8
	PrefixLen = 2 // LEN field (uint16)
	HeaderLen = NonceLen + HMACLen
	MinFrame  = PrefixLen + HeaderLen
)

// BuildFrame creates a simple frame [LEN(2)][PAYLOAD].
func BuildFrame(payload []byte) ([]byte, error) {
	if len(payload) == 0 {
		return nil, errors.New("empty payload")
	}
	totalLen := PrefixLen + len(payload)
	if totalLen > 0xFFFF {
		return nil, fmt.Errorf("payload too large")
	}

	frame := make([]byte, totalLen)
	binary.BigEndian.PutUint16(frame[0:PrefixLen], uint16(len(payload)))
	copy(frame[PrefixLen:], payload)
	return frame, nil
}

// ParseFrame strips the 2-byte prefix and returns only the payload.
func ParseFrame(frame []byte) ([]byte, error) {
	if len(frame) < PrefixLen {
		return nil, errors.New("frame too short")
	}
	expectedLen := int(binary.BigEndian.Uint16(frame[:PrefixLen]))
	if expectedLen != len(frame)-PrefixLen {
		return nil, fmt.Errorf("length mismatch: got %d, expected %d", len(frame)-PrefixLen, expectedLen)
	}
	return frame[PrefixLen:], nil
}

// BuildFrameWithKey builds a wire frame from payload and a shared key.
// Returns full frame including 2-byte length prefix.
// Frame = [LEN:2][NONCE:4][HMAC:8][PAYLOAD]
func BuildFrameWithKey(key []byte, payload []byte) ([]byte, error) {
	if len(key) == 0 {
		return nil, errors.New("empty key")
	}
	// generate 4-byte nonce
	var nonce [NonceLen]byte
	if _, err := io.ReadFull(rand.Reader, nonce[:]); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}

	// compute HMAC over (nonce || payload)
	data := append(nonce[:], payload...)
	tag := ComputeHMAC(key, data, HMACLen)

	totalLen := PrefixLen + HeaderLen + len(payload)
	if totalLen > 0xFFFF {
		return nil, fmt.Errorf("payload too large")
	}

	frame := make([]byte, totalLen)
	// write length (big endian) at [0:2]
	binary.BigEndian.PutUint16(frame[0:PrefixLen], uint16(totalLen))
	// write nonce
	copy(frame[PrefixLen:PrefixLen+NonceLen], nonce[:])
	// write tag
	copy(frame[PrefixLen+NonceLen:PrefixLen+NonceLen+HMACLen], tag)
	// write payload
	copy(frame[PrefixLen+HeaderLen:], payload)

	return frame, nil
}

// ParseAndVerifyFrameWithKey parses a raw frame (as returned by LoraDevice.ReadFrame())
// and verifies HMAC using the provided key. If valid, returns payload and nonce.
func ParseAndVerifyFrameWithKey(key []byte, frame []byte) (payload []byte, nonce []byte, err error) {
	if len(frame) < HeaderLen {
		return nil, nil, errors.New("frame too short")
	}
	// Expectation: caller may pass in payload portion (without LEN) or full frame with LEN.
	// Accept both: if len(frame) >= MinFrame and the first two bytes look like length, strip prefix.
	if len(frame) >= MinFrame {
		// check whether first two bytes indicate the total length equals len(frame)
		pref := binary.BigEndian.Uint16(frame[0:PrefixLen])
		if int(pref) == len(frame) {
			// strip length prefix
			frame = frame[PrefixLen:]
		}
	}
	// Now frame layout: [NONCE(4)][HMAC(8)][PAYLOAD]
	if len(frame) < HeaderLen {
		return nil, nil, errors.New("payload too short after stripping prefix")
	}

	nonce = make([]byte, NonceLen)
	copy(nonce, frame[0:NonceLen])
	tag := frame[NonceLen : NonceLen+HMACLen]
	payload = make([]byte, len(frame)-HeaderLen)
	copy(payload, frame[HeaderLen:])

	// compute expected tag
	data := append(nonce, payload...)
	if !VerifyHMAC(key, data, tag) {
		return nil, nil, errors.New("hmac verification failed")
	}
	return payload, nonce, nil
}

```

- /internal/core/system.go
```go
// Package core implements the orchestration and communication logic for the LoraFog edge system.
package core

// CHANGELOG (refactor v2):
// - Removed parser layer and wire_format configuration
// - Unified goroutine lifecycle via context.Context
// - Introduced structured logging using log/slog
// - Renamed symbols to follow Go naming conventions (Start, Shutdown, socatManager, etc.)
// - Added nil-safe Close checks to suppress unchecked warnings
// - Improved documentation and function clarity for maintainability

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

// System manages the lifecycle of FogServer, Gateways, Vehicles, and Arduino devices.
type System struct {
	configPath   string
	config       *model.Config
	fogServer    *FogServer
	gateways     []*Gateway
	vehicles     []*Vehicle
	arduinos     []*device.Arduino
	socatManager *util.SocatManager

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSystem loads configuration from YAML and constructs the runtime components.
func NewSystem(configPath string) (*System, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	sys := &System{
		configPath:   configPath,
		config:       &cfg,
		socatManager: util.NewSocatManager(),
	}

	// Initialize virtual serial pairs if configured.
	for _, pair := range cfg.VirtualSerials {
		if err := sys.socatManager.CreatePair(pair.Left, pair.Right); err != nil {
			slog.Warn("failed to create virtual serial pair",
				"left", pair.Left, "right", pair.Right, "error", err)
		}
	}
	time.Sleep(2 * time.Second)

	// Construct FogServer
	if cfg.Server.FogAddr != "" {
		sys.fogServer = NewFogServer(cfg.Server.FogAddr, cfg.Server.AppAddr)
		for _, g := range cfg.Server.Gateways {
			sys.fogServer.RegisterGateway(g.ID, g.URL, g.Vehicles)
			slog.Info("registered gateway",
				"component", "fog",
				"id", g.ID,
				"url", g.URL,
				"vehicles", g.Vehicles)
		}
	}

	// Construct Gateways
	for _, gwCfg := range cfg.Gateways {
		gw := NewGateway(
			gwCfg.ID,
			gwCfg.LoraDev,
			gwCfg.LoraBaud,
			gwCfg.URL,
			gwCfg.FogURL,
			gwCfg.Vehicles,
		)
		sys.gateways = append(sys.gateways, gw)
	}

	// Construct Vehicles
	for _, vCfg := range cfg.Vehicles {
		v := NewVehicle(
			vCfg.ID,
			vCfg.LoraDev,
			vCfg.LoraBaud,
			vCfg.ArduinoDev,
			vCfg.ArduinoBaud,
			time.Duration(vCfg.TelemetryIntervalMs)*time.Millisecond,
		)
		sys.vehicles = append(sys.vehicles, v)
	}

	// Construct Arduino simulators
	for _, aCfg := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos,
			device.NewArduino(aCfg.Dev, aCfg.Baud))
	}

	return sys, nil
}

// Start launches all active components under a shared context.
func (s *System) Start(ctx context.Context) error {
	if s.fogServer == nil && len(s.gateways) == 0 && len(s.vehicles) == 0 {
		return errors.New("no components to start")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Start FogServer
	if s.fogServer != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			slog.Info("starting fog server",
				"component", "fog", "addr", s.fogServer.Addr)
			if err := s.fogServer.Start(ctx); err != nil {
				slog.Error("fog server stopped with error",
					"component", "fog", "error", err)
			}
		}()
	}

	// Start Gateways
	for _, gw := range s.gateways {
		if err := gw.Start(ctx); err != nil {
			slog.Warn("gateway start failed",
				"component", "gateway", "id", gw.ID, "error", err)
		} else {
			slog.Info("gateway started",
				"component", "gateway", "id", gw.ID)
		}
	}

	// Start Vehicles
	for _, v := range s.vehicles {
		if err := v.Start(ctx); err != nil {
			slog.Warn("vehicle start failed",
				"component", "vehicle", "id", v.ID, "error", err)
		} else {
			slog.Info("vehicle started",
				"component", "vehicle", "id", v.ID)
		}
	}

	// Start Arduino simulations
	for _, a := range s.arduinos {
		s.wg.Add(1)
		go func(a *device.Arduino) {
			defer s.wg.Done()
			slog.Info("starting arduino simulation",
				"component", "arduino", "device", a.Device)
			stop := make(chan struct{})
			go func() {
				<-ctx.Done()
				close(stop)
			}()
			if err := a.StartSimulation(stop); err != nil {
				slog.Error("arduino simulation failed",
					"component", "arduino", "device", a.Device, "error", err)
			}
		}(a)
	}

	return nil
}

// Shutdown gracefully stops all components and cleans up resources.
func (s *System) Shutdown() {
	if s.cancel != nil {
		slog.Info("initiating system shutdown", "component", "system")
		s.cancel()
	}

	for _, gw := range s.gateways {
		gw.Shutdown()
	}
	for _, v := range s.vehicles {
		v.Shutdown()
	}
	for _, a := range s.arduinos {
		if a != nil {
			if err := a.Close(); err != nil {
				slog.Warn("failed to close arduino",
					"component", "arduino", "device", a.Device, "error", err)
			}
		}
	}

	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}

	s.wg.Wait()
	slog.Info("system shutdown complete", "component", "system")
}

```

- /internal/core/server.go
```go
// Package core defines the FogServer, which bridges gateways and application layers
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

// FogServer acts as a lightweight fog layer that receives telemetry from gateways,
// broadcasts updates via WebSocket, and forwards control commands.
type FogServer struct {
	Addr            string
	AppAddr         string
	server          *http.Server
	sessions        *sessionStore
	vehicleRegistry sync.Map // vehicleID -> gatewayURL

	clientMu sync.Mutex
	clients  map[*websocket.Conn]bool
}

// NewFogServer initializes a new FogServer with the given listening and app addresses.
func NewFogServer(addr, appAddr string) *FogServer {
	return &FogServer{
		Addr:    addr,
		AppAddr: appAddr,
		clients: make(map[*websocket.Conn]bool),
	}
}

// RegisterGateway associates a gateway with its managed vehicles.
func (f *FogServer) RegisterGateway(gatewayID, url string, vehicles []string) {
	for _, v := range vehicles {
		f.vehicleRegistry.Store(v, url)
	}
	slog.Info("gateway registered",
		"component", "fog",
		"gateway", gatewayID,
		"vehicles", vehicles)
}

// Start runs the HTTP server until the provided context is cancelled.
func (f *FogServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/telemetry", f.handleTelemetry)
	mux.HandleFunc("/api/register", f.handleRegister)
	mux.HandleFunc("/api/control", f.handleControl)
	mux.HandleFunc("/ws", f.handleWebSocket)

	addr := strings.TrimPrefix(strings.TrimPrefix(f.Addr, "http://"), "https://")
	f.server = &http.Server{Addr: addr, Handler: mux}

	f.sessions = newSessionStore()
	go func() {
		t := time.NewTicker(30 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				f.sessions.Sweep()
			}
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("fog server listening", "component", "fog", "addr", addr)
		if err := f.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		slog.Info("fog server context cancelled", "component", "fog")
		return f.Shutdown()
	case err := <-errCh:
		return err
	}
}

// Shutdown gracefully stops the fog server.
func (f *FogServer) Shutdown() error {
	if f.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	slog.Info("shutting down fog server", "component", "fog")
	if err := f.server.Shutdown(ctx); err != nil {
		slog.Error("fog server shutdown error", "component", "fog", "error", err)
		return err
	}
	slog.Info("fog server stopped", "component", "fog")
	return nil
}

// handleTelemetryRequest processes telemetry JSON and broadcasts it to WebSocket clients.
func (f *FogServer) handleTelemetry(w http.ResponseWriter, r *http.Request) {
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
	f.broadcast(string(out))

	// Forward telemetry to App Server if configured
	if f.AppAddr != "" {
		go func() {
			resp, err := http.Post(f.AppAddr+"/api/telemetry",
				"application/json", bytes.NewReader(out))
			if err != nil {
				slog.Warn("failed to forward telemetry",
					"component", "fog", "app", f.AppAddr, "error", err)
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
func (f *FogServer) handleRegister(w http.ResponseWriter, r *http.Request) {
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
	f.sessions.Set(req.VehicleID, session)

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
func (f *FogServer) handleControl(w http.ResponseWriter, r *http.Request) {
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

	val, ok := f.vehicleRegistry.Load(control.VehicleID)
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
func (f *FogServer) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		slog.Warn("websocket upgrade failed", "component", "fog", "error", err)
		return
	}

	f.clientMu.Lock()
	f.clients[conn] = true
	f.clientMu.Unlock()

	go func() {
		defer func() {
			f.clientMu.Lock()
			delete(f.clients, conn)
			f.clientMu.Unlock()
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
func (f *FogServer) broadcast(msg string) {
	f.clientMu.Lock()
	defer f.clientMu.Unlock()
	for conn := range f.clients {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			slog.Warn("websocket send failed",
				"component", "fog", "error", err)
		}
	}
}

```

- /internal/core/server_session.go
```go
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
	"LoraFog/internal/util"

	"github.com/fxamacker/cbor/v2"
)

// Vehicle represents a single autonomous vehicle communicating via LoRa.
type Vehicle struct {
	ID            string
	lora          *device.Lora
	arduino       *device.Arduino
	sessionKey    []byte
	leaseExpiry   time.Time
	telemetryRate time.Duration

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewVehicle constructs a Vehicle agent with LoRa and optional Arduino connection.
func NewVehicle(id, loraDev string, loraBaud int, arduinoDev string, arduinoBaud int, interval time.Duration) *Vehicle {
	lora := device.NewLora(loraDev, loraBaud)
	v := &Vehicle{
		ID:            id,
		lora:          lora,
		telemetryRate: interval,
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
					slog.Info("sent hello to gateway", "component", "vehicle", "id", v.ID, "gateway", b.GatewayID)
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
		if v.sessionKey != nil {
			frame, err := util.BuildFrame(payload)
			if err != nil {
				slog.Warn("build frame failed", "component", "vehicle", "id", v.ID, "error", err)
				return
			}
			if err := v.lora.WriteBytes(frame); err != nil {
				slog.Warn("failed to send telemetry",
					"component", "vehicle", "id", v.ID, "error", err)
			} else {
				slog.Debug("telemetry sent",
					"component", "vehicle", "id", v.ID)
			}
		} else {
			if err := v.lora.WriteFrame(payload); err != nil {
				slog.Warn("failed to send telemetry",
					"component", "vehicle", "id", v.ID, "error", err)
			} else {
				slog.Debug("telemetry sent",
					"component", "vehicle", "id", v.ID)
			}
		}
	}
}

```

- /internal/core/gateway.go
```go
// Package core defines the Gateway component responsible for bridging LoRa-connected
// vehicles with the FogServer using CBOR (for LoRa) and JSON (for HTTP).
package core

// CHANGELOG (refactor v2):
// - Removed parser dependency; Vehicle<->Gateway uses CBOR serialization
// - Gateway<->FogServer uses JSON over HTTP
// - Context-based goroutine control and safe shutdown
// - Structured logging using slog
// - Renamed methods and variables to follow Go naming conventions
// - Safe Close checks and improved lifecycle management

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"github.com/fxamacker/cbor/v2"
)

// Gateway represents a LoRa gateway that decodes CBOR messages from vehicles,
// re-encodes them as JSON, and forwards them to the FogServer.
type Gateway struct {
	ID         string
	URL        string
	FogURL     string
	vehicles   map[string]struct{}
	keyStore   map[string][]byte // vehicleID -> session key
	lora       *device.Lora
	server     *http.Server
	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
}

// NewGateway creates a new Gateway instance bound to a LoRa serial device.
func NewGateway(id, loraDev string, loraBaud int, url, fogURL string, vehicles []string) *Gateway {
	lora := device.NewLora(loraDev, loraBaud)

	vmap := make(map[string]struct{}, len(vehicles))
	for _, v := range vehicles {
		vmap[v] = struct{}{}
	}

	return &Gateway{
		ID:       id,
		lora:     lora,
		URL:      url,
		FogURL:   fogURL,
		vehicles: vmap,
		keyStore: make(map[string][]byte),
	}
}

// Start launches the gateway uplink (LoRa→Fog) and downlink (Fog→LoRa) handlers.
func (g *Gateway) Start(ctx context.Context) error {
	g.stopCtx, g.stopCancel = context.WithCancel(ctx)

	if g.lora == nil {
		slog.Warn("gateway running in headless mode (no serial device)",
			"component", "gateway", "id", g.ID)
		return nil
	}

	// Start beacon loop
	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		beacon := model.BeaconMessage{
			GatewayID: g.ID,
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

	addr := strings.TrimPrefix(strings.TrimPrefix(g.URL, "http://"), "https://")
	g.server = &http.Server{Addr: addr, Handler: mux}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		slog.Info("gateway HTTP server started",
			"component", "gateway", "id", g.ID, "addr", addr)
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway HTTP server error",
				"component", "gateway", "id", g.ID, "error", err)
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
			slog.Info("uplink loop stopped", "component", "gateway", "id", g.ID)
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
						"gateway_id": g.ID,
						"vehicle_id": hello.VehicleID,
					}
					bodyB, _ := json.Marshal(registerBody)
					resp, err := http.Post(g.FogURL+"/api/register", "application/json", bytes.NewReader(bodyB))
					if err != nil {
						slog.Warn("register request failed", "component", "gateway", "id", g.ID, "error", err)
						continue
					}
					var regResp struct {
						VehicleID string `json:"vehicle_id"`
						KeyHex    string `json:"key_hex"`
						TTL       int64  `json:"ttl"`
					}
					_ = json.NewDecoder(resp.Body).Decode(&regResp)
					_ = resp.Body.Close()

					// decode key hex to bytes
					// key, _ := hex.DecodeString(regResp.KeyHex)
					if key, err := hex.DecodeString(regResp.KeyHex); err == nil {
						g.keyStore[regResp.VehicleID] = key

						// create auth message and relay to vehicle
						auth := model.AuthMessage{
							VehicleID: regResp.VehicleID,
							// Key:       key,
							TTL: regResp.TTL,
						}
						if err := g.lora.SendAuthRelay(auth); err != nil {
							slog.Warn("send auth to vehicle failed", "component", "gateway", "id", g.ID, "vehicle", hello.VehicleID, "error", err)
						} else {
							slog.Info("auth relayed to vehicle", "component", "gateway", "id", g.ID, "vehicle", hello.VehicleID)
						}
						continue
					}
				}
			}
		}

		// Otherwise assume telemetry frame => forward to Fog as before
		payload, err := util.ParseFrame(frame)
		if err != nil {
			slog.Warn("invalid frame", "component", "gateway", "id", g.ID, "error", err)
			continue
		}
		var telemetry model.VehicleData
		if err := cbor.Unmarshal(payload, &telemetry); err != nil {
			slog.Warn("failed to decode CBOR telemetry",
				"component", "gateway", "id", g.ID, "error", err)
			continue
		}
		payloadJSON, _ := json.Marshal(telemetry)
		resp, err := http.Post(g.FogURL+"/api/telemetry", "application/json", bytes.NewReader(payloadJSON))
		if err != nil {
			slog.Warn("failed to forward telemetry",
				"component", "gateway", "id", g.ID, "error", err)
			continue
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		slog.Info("uplink telemetry sent",
			"component", "gateway", "id", g.ID, "vehicle", telemetry.VehicleID)
	}
}

// handleControlRequest receives control JSON and sends it via LoRa using CBOR.
func (g *Gateway) handleControlRequest(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close control request body",
					"component", "gateway", "id", g.ID, "error", err)
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
			"component", "gateway", "id", g.ID, "error", err)
		return
	}

	slog.Info("control sent to vehicle",
		"component", "gateway", "id", g.ID, "vehicle", control.VehicleID)
	w.WriteHeader(http.StatusAccepted)
}

// Shutdown gracefully stops the gateway and closes resources.
func (g *Gateway) Shutdown() {
	slog.Info("stopping gateway", "component", "gateway", "id", g.ID)

	if g.stopCancel != nil {
		g.stopCancel()
	}

	if g.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			slog.Warn("gateway HTTP server shutdown error",
				"component", "gateway", "id", g.ID, "error", err)
		}
	}

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn("failed to close device",
				"component", "gateway", "id", g.ID, "error", err)
		}
	}

	g.wg.Wait()
	slog.Info("gateway stopped", "component", "gateway", "id", g.ID)
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
