
- /internal/model/config.go
```go
// Package model defines shared configuration structures used to initialize the LoraFog system.
// It includes global settings, gateway definitions, and vehicle definitions.
package model

// Config represents the root structure loaded from configs/config.yml.
// It contains global settings, gateway definitions and vehicle definitions.
type Config struct {
	Global         GlobalConfig        `yaml:"global"`
	Server         ServerConfig        `yaml:"server"`
	Gateways       []GatewayConfig     `yaml:"gateways"`
	Vehicles       []VehicleConfig     `yaml:"vehicles"`
	Arduinos       []ArduinoConfig     `yaml:"arduinos"`
	VirtualSerials VirtualSerialConfig `yaml:"virtual_serials"`
}

// GlobalConfig defines shared defaults across the system.
type GlobalConfig struct {
	WireFormat string `yaml:"wire_format"` // default wire format (csv/json)
}

// ServerConfig defines configuration for a server instance.
type ServerConfig struct {
	FogAddr  string            `yaml:"fog_addr"` // address for FogServer (e.g. ":10000") if blank server will not work
	AppAddr  string            `yaml:"app_addr"` // address for FogServer (e.g. ":10000") if blank server will not work
	Gateways []GatewayRegistry `yaml:"gateway_registry"`
}

// GatewayRegistry defines a gateway registration entry.
type GatewayRegistry struct {
	ID       string   `yaml:"id"`
	URL      string   `yaml:"url"`
	Vehicles []string `yaml:"vehicles"`
}

// GatewayConfig defines configuration for a single gateway instance.
type GatewayConfig struct {
	ID       string   `yaml:"id"`
	URL      string   `yaml:"url"`     // fog server endpoint
	FogURL   string   `yaml:"fog_url"` // fog server endpoint
	LoraDev  string   `yaml:"lora_device"`
	LoraBaud int      `yaml:"lora_baud"`
	WireIn   string   `yaml:"wire_in"`  // format received from vehicle
	WireOut  string   `yaml:"wire_out"` // format sent to fog
	Vehicles []string `yaml:"vehicles"`
}

// VehicleConfig defines configuration for a single vehicle agent.
type VehicleConfig struct {
	ID                  string `yaml:"id"`
	WireFormat          string `yaml:"wire_format"`
	TelemetryIntervalMs int    `yaml:"telemetry_interval_ms"`
	LoraDev             string `yaml:"lora_device"`
	LoraBaud            int    `yaml:"lora_baud"`
	ArduinoID           string `yaml:"arduino_id"`
	ArduinoDev          string `yaml:"arduino_device"`
	ArduinoBaud         int    `yaml:"arduino_baud"`
}

// ArduinoConfig defines serial setup for testing
type ArduinoConfig struct {
	ID   string `yaml:"id"`
	Dev  string `yaml:"device"`
	Baud int    `yaml:"baud"`
}

// GpsConfig defines serial setup for testing
type GpsConfig struct {
	ID   string `yaml:"id"`
	Dev  string `yaml:"device"`
	Baud int    `yaml:"baud"`
}

// VirtualPair defines a flexible pair of linked virtual serial endpoints.
type VirtualPair struct {
	Type  string `yaml:"type"`
	Left  string `yaml:"left"`
	Right string `yaml:"right"`
}

// VirtualSerialConfig defines optional virtual serial setup for testing.
type VirtualSerialConfig struct {
	Enabled bool          `yaml:"enabled"`
	Pairs   []VirtualPair `yaml:"pairs"`
}

```

- /internal/model/message.go
```go
// Package model defines the core data structures exchanged between vehicles,
// gateways, and the fog server, including telemetry and control messages.
package model

type PacketType string

const (
	PacketTelemetry PacketType = "t"
	PacketControl   PacketType = "c"
)

type Packet struct {
	Type PacketType `json:"type"`
	Data any        `json:"data"`
}

// VehicleData represents telemetry information reported by a vehicle.
// It is the common structure shared between vehicles, gateways and fog.
type VehicleData struct {
	VehicleID   string  `json:"boatId"`
	Latitude    float64 `json:"lat"`
	Longitude   float64 `json:"lon"`
	CurrentHead int     `json:"head"`
	TargetHead  int     `json:"targetHead"`
	LeftSpeed   int     `json:"leftSpeed"`
	RightSpeed  int     `json:"rightSpeed"`
}

// ControlData represents a control command sent from Fog to a vehicle.
// It can be encoded either as JSON or CSV depending on gateway configuration.
type ControlData struct {
	VehicleID string  `json:"boatId"`
	Speed     int     `json:"speed"`
	Latitude  float64 `json:"targetLat"`
	Longitude float64 `json:"targetLon"`
	Kp        float64 `json:"kp"`
	Ki        float64 `json:"ki"`
	Kd        float64 `json:"kd"`
}

// ArduinoData represents telemetry data collected by arduino
type ArduinoData struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	LeftSpeed   int     `json:"left_speed"`
	RightSpeed  int     `json:"right_speed"`
	CurrentHead int     `json:"current_head"`
	TargetHead  int     `json:"target_head"`
}

// ArduinoControl represents telemetry data collected by arduino
type ArduinoControl struct {
	CruiseSpeed int     `json:"cruise_speed"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Kp          float64 `json:"kp"`
	Ki          float64 `json:"ki"`
	Kd          float64 `json:"kd"`
}

// GpsData represents a simple latitude/longitude reading.
type GpsData struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// GatewayRegistration represents information sent by a gateway
// to the fog when registering itself.
type GatewayRegistration struct {
	GatewayID string   `json:"gateway_id"`
	URL       string   `json:"url"`
	Vehicles  []string `json:"vehicles"`
}

```

- /internal/util/logger.go
```go
// Package util provides utility functions for setting up application-wide logging,
// including timestamped logs with file and line information.
package util

import (
	"log"
	"os"
)

// SetupLogger configures the standard logger with timestamp and short file info.
// It writes logs to stdout.
func SetupLogger() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("[Logger] Initialized")
}

```

- /internal/util/nmea.go
```go
// Package util provides NMEA coordinate conversion utilities for GPS data.
// It supports parsing from ddmm.mmmm format and conversion to decimal degrees.
package util

import (
	"fmt"
	"strconv"
)

// ParseNMEACoord converts NMEA ddmm.mmmm format to decimal degrees.
// For example, 2101.7102,N -> 21.0285033
func ParseNMEACoord(value string, dir string) (float64, error) {
	if len(value) < 4 {
		return 0, fmt.Errorf("invalid NMEA coord")
	}
	var degPart, minPart string
	if dir == "N" || dir == "S" {
		degPart = value[:2]
		minPart = value[2:]
	} else {
		degPart = value[:3]
		minPart = value[3:]
	}
	deg, err := strconv.ParseFloat(degPart, 64)
	if err != nil {
		return 0, err
	}
	min, err := strconv.ParseFloat(minPart, 64)
	if err != nil {
		return 0, err
	}
	dec := deg + min/60.0
	if dir == "S" || dir == "W" {
		dec = -dec
	}
	return dec, nil
}

// ToNMEACoord converts decimal degrees to ddmm.mmmm string format.
func ToNMEACoord(dec float64, isLat bool) (string, string) {
	dir := "N"
	if !isLat {
		dir = "E"
	}
	if dec < 0 {
		dec = -dec
		if isLat {
			dir = "S"
		} else {
			dir = "W"
		}
	}
	deg := int(dec)
	min := (dec - float64(deg)) * 60
	if isLat {
		return fmt.Sprintf("%02d%06.3f", deg, min), dir
	}
	return fmt.Sprintf("%03d%06.3f", deg, min), dir
}

```

- /internal/util/socat.go
```go
// Package util provides helpers for virtual serial management using socat.
package util

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
)

// SocatManager manages lifecycle of socat-created virtual serial pairs.
type SocatManager struct {
	mu     sync.Mutex
	cmds   []*exec.Cmd
	links  []string
	closed bool
}

// NewSocatManager initializes an empty manager.
func NewSocatManager() *SocatManager {
	return &SocatManager{}
}

// CreatePair starts a socat process that links two PTYs (bidirectional).
func (m *SocatManager) CreatePair(left, right string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.Command(
		"socat", "-d", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", left),
		fmt.Sprintf("pty,raw,echo=0,link=%s", right),
	)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start socat: %w", err)
	}

	log.Printf("[virt-serial] started socat (pid=%d): %s <-> %s", cmd.Process.Pid, left, right)

	m.cmds = append(m.cmds, cmd)
	m.links = append(m.links, left, right)
	return nil
}

// Cleanup stops all socat processes and removes created links.
func (m *SocatManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true

	for _, cmd := range m.cmds {
		if cmd.Process != nil {
			log.Printf("[virt-serial] killing socat pid=%d", cmd.Process.Pid)
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}

	for _, path := range m.links {
		if _, err := os.Lstat(path); err == nil {
			_ = os.Remove(path)
			log.Printf("[virt-serial] removed link: %s", path)
		}
	}

	log.Printf("[virt-serial] cleanup complete (%d pairs)", len(m.links)/2)
}

// CleanupAll forcibly kills all socat processes (failsafe).
func (m *SocatManager) CleanupAll() {
	_ = exec.Command("pkill", "-f", "socat").Run()
	// matches, _ := filepath.Glob("/tmp/tty*")
	// for _, m := range matches {
	// 	info, err := os.Lstat(m)
	// 	if err == nil && info.Mode()&os.ModeSymlink != 0 {
	// 		_ = os.Remove(m)
	// 	}
	// }
	log.Println("[virt-serial] global cleanup done")
}

```

- /internal/core/fog_server.go
```go
// Package core implements the FogServer component, which acts as a central hub between
// gateways and monitoring clients. It handles telemetry ingestion, control message routing,
// and websocket broadcasting.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/model"
	"LoraFog/internal/parser"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// FogServer implements a lightweight in-memory fog server that accepts telemetry,
// broadcasts telemetry to websocket clients, and forwards control messages to gateways.
type FogServer struct {
	Addr    string
	AppAddr string
	reg     *registry
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
	server  *http.Server
	wireFmt string // wire format: "csv" or "json"
}

// registry maps vehicle IDs to gateway URLs.
type registry struct {
	mu         sync.RWMutex
	vehicleMap map[string]string
}

// newRegistry creates an empty registry.
func newRegistry() *registry {
	return &registry{vehicleMap: map[string]string{}}
}

// set associates a vehicle ID with a gateway URL.
func (r *registry) set(v string, gw string) { r.mu.Lock(); r.vehicleMap[v] = gw; r.mu.Unlock() }

// get retrieves gateway URL for a vehicle ID.
func (r *registry) get(v string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.vehicleMap[v]
	return x, ok
}

// NewFogServer constructs a FogServer listening on addr.
func NewFogServer(addr string, appAddr string) *FogServer {
	return &FogServer{
		Addr:    addr,
		AppAddr: appAddr,
		reg:     newRegistry(),
		clients: map[*websocket.Conn]bool{},
	}
}

// RegisterGateway registers a gateway and maps its vehicle list in the registry.
func (f *FogServer) RegisterGateway(id, url string, vehicles []string) {
	for _, v := range vehicles {
		f.reg.set(v, url)
	}
}

// Start launches the HTTP server for telemetry, ws and control endpoints.
// This call blocks until the server stops or fails.
func (f *FogServer) Start() error {
	if f.Addr == "" {
		log.Println("[fog] fog server not started (empty address)")
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/telemetry", f.handleTelemetry)
	mux.HandleFunc("/api/control", f.handleControl)
	mux.HandleFunc("/ws", f.handleWS)
	addr := f.Addr
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	f.server = &http.Server{Addr: addr, Handler: mux}
	log.Printf("[fog] listening on %s", addr)
	return f.server.ListenAndServe()
	// log.Printf("FogServer is listening on %s", f.Addr)
	// if err := f.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
	// 	log.Fatal(err)
	// }
}

// Stop shuts down the HTTP server.
func (f *FogServer) Stop() {
	if f.server != nil {
		log.Println("[fog] Shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := f.server.Shutdown(ctx); err != nil {
			log.Printf("[fog] HTTP server shutdown error: %v", err)
		} else {
			log.Println("[fog] Web server stopped cleanly")
		}
	}
}

// handleTelemetry accepts telemetry posted by gateways in either JSON or CSV text.
// It decodes to VehicleData and broadcasts CSV lines to websocket clients.
func (f *FogServer) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[fog] warning: failed to close request body: %v", cerr)
		}
	}()

	line := strings.TrimSpace(string(body))
	if line == "" {
		http.Error(w, "empty telemetry", http.StatusBadRequest)
		return
	}

	var vd model.VehicleData
	// Try decode as JSON first
	if err := json.Unmarshal(body, &vd); err != nil {
		// Try CSV fallback
		csvp := parser.NewCSVParser()
		vd2, err2 := csvp.DecodeTelemetry(line)
		if err2 != nil {
			log.Printf("[fog] invalid telemetry: cannot decode JSON or CSV: %v", err2)
			http.Error(w, "invalid telemetry", http.StatusBadRequest)
			return
		}
		vd = vd2
	}

	// Encode to broadcast format
	var out string
	var payload []byte
	var contentType string
	switch f.wireFmt {
	case "json":
		contentType = "application/json"
		payload, err = json.Marshal(vd)
		if err != nil {
			log.Printf("[fog] encode json err: %v", err)
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
		out = string(payload)
	default: // csv (default)
		contentType = "text/plain"
		csvp := parser.NewCSVParser()
		out, err = csvp.EncodeTelemetry(vd)
		if err != nil {
			log.Printf("[fog] encode csv err: %v", err)
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
		payload = []byte(out)
	}
	f.broadcast(out)
	log.Printf("[fog] broadcast %s telemetry: %s", strings.ToUpper(f.wireFmt), out)

	// Forward to App Server if enabled
	payloadJSON, err := json.Marshal(vd)
	contentType = "application/json"
	if err != nil {
		log.Printf("[fog] encode json err: %v", err)
		http.Error(w, "encode error", http.StatusInternalServerError)
		return
	}
	if f.AppAddr != "" {
		go func(v model.VehicleData) {
			resp, err := http.Post(f.AppAddr+"/api/telemetry", contentType, bytes.NewReader(payloadJSON))
			if err != nil {
				log.Printf("[fog] forward to app failed: %v", err)
				return
			}
			defer func() {
				if cerr := resp.Body.Close(); cerr != nil {
					log.Printf("[fog] warning: close app response: %v", cerr)
				}
			}()
			// Discard the body to complete the HTTP exchange cleanly
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				log.Printf("[fog] warning: discard control response: %v", err)
			}
			log.Printf("[fog] forwarded telemetry to app (%s): %s", f.AppAddr, v.VehicleID)
		}(vd)
	}
	w.WriteHeader(http.StatusOK)
}

// handleWS upgrades HTTP to websocket and registers the client for broadcasts.
func (f *FogServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.clients[conn] = true
	f.mu.Unlock()

	go func() {
		defer func() {
			f.mu.Lock()
			delete(f.clients, conn)
			f.mu.Unlock()
			if err := conn.Close(); err != nil {
				log.Printf("warning: failed to close websocket: %v", err)
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// broadcast sends a message to all connected websocket clients.
func (f *FogServer) broadcast(msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for c := range f.clients {
		_ = c.WriteMessage(websocket.TextMessage, []byte(msg))
	}
}

// handleControl receives a control message from the cloud or admin,
// finds the gateway responsible for the target vehicle, and forwards
// the message in the format specified by the global wire_format.
func (f *FogServer) handleControl(w http.ResponseWriter, r *http.Request) {
	// Always close request body safely
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[fog] warning: close control request body: %v", cerr)
		}
	}()

	// Decode control message (JSON input only for API)
	body, berr := io.ReadAll(r.Body)
	if berr != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	line := strings.TrimSpace(string(body))
	if line == "" {
		http.Error(w, "empty control message", http.StatusBadRequest)
		return
	}

	// Step 1: decode incoming control message (Fog → Gateway)
	var ctl model.ControlData
	// Try JSON first
	if err := json.Unmarshal(body, &ctl); err != nil {
		// Try CSV fallback
		csvp := parser.NewCSVParser()
		ctl2, err2 := csvp.DecodeControl(line)
		if err2 != nil {
			http.Error(w, "invalid control message format", http.StatusBadRequest)
			// log.Printf("[gateway %s] invalid control: %v", g.ID, err2)
			return
		}
		ctl = ctl2
	}

	// Lookup gateway by vehicle ID
	url, ok := f.reg.get(ctl.VehicleID)
	if !ok {
		http.Error(w, "no gateway registered for vehicle", http.StatusNotFound)
		log.Printf("[fog] control ignored: no gateway for vehicle %s", ctl.VehicleID)
		return
	}

	// Encode control message according to configured wire format
	var payload []byte
	var contentType string
	var err error

	switch f.wireFmt {
	case "csv":
		csvp := parser.NewCSVParser()
		line, encErr := csvp.EncodeControl(ctl)
		if encErr != nil {
			http.Error(w, "failed to encode control message (csv)", http.StatusInternalServerError)
			log.Printf("[fog] control encode csv error: %v", encErr)
			return
		}
		payload = []byte(line)
		contentType = "text/plain"

	default: // json
		payload, err = json.Marshal(ctl)
		if err != nil {
			http.Error(w, "failed to encode control message (json)", http.StatusInternalServerError)
			log.Printf("[fog] control encode json error: %v", err)
			return
		}
		contentType = "application/json"
	}

	// Send asynchronously to the gateway
	go func() {
		resp, err := http.Post(url+"/command", contentType, bytes.NewReader(payload))
		if err != nil {
			log.Printf("[fog] failed to send control to gateway %s: %v", url, err)
			return
		}

		// Always close response body safely
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				log.Printf("[fog] warning: close control response: %v", cerr)
			}
		}()

		// Discard the body to complete the HTTP exchange cleanly
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Printf("[fog] warning: discard control response: %v", err)
		}

		log.Printf("[fog] control forwarded to %s (fmt=%s, vehicle=%s)", url, f.wireFmt, ctl.VehicleID)
	}()

	w.WriteHeader(http.StatusAccepted)
}

```

- /internal/core/gateway.go
```go
package core

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
)

// Gateway represents a LoRa gateway instance that reads wire lines from a Device,
// decodes using InParser, re-encodes using OutParser and forwards to FogServer.
type Gateway struct {
	ID         string
	Device     device.Device
	URL        string
	FogURL     string
	InParser   parser.Parser
	OutParser  parser.Parser
	WireIn     string // uplink Vehicle -> Gateway format
	WireOut    string // uplink Gateway -> Fog format
	Vehicles   []string
	VehicleSet map[string]struct{}
	server     *http.Server
	stop       chan struct{}
	wg         sync.WaitGroup
}

// NewGateway constructs a Gateway with device path and parsers.
// If opening the serial device fails, the Device field may be nil and Start will be a no-op.
func NewGateway(id, devPath string, baud int, URL string, fogURL string, wireIn string, wireOut string, in parser.Parser, out parser.Parser, vehicles []string) *Gateway {
	dev, err := device.NewSerialDevice(devPath, baud)
	if err != nil {
		// log but continue: user may run gateway without physical device (e.g., test)
		log.Printf("[gateway %s] open serial %s err: %v", id, devPath, err)
	} else {
		log.Printf("[gateway %s] open serial %s: success", id, devPath)
	}
	g := &Gateway{
		ID:         id,
		Device:     dev,
		URL:        URL,
		FogURL:     fogURL,
		WireIn:     wireIn,
		WireOut:    wireOut,
		InParser:   in,
		OutParser:  out,
		Vehicles:   vehicles,
		VehicleSet: make(map[string]struct{}, len(vehicles)),
		stop:       make(chan struct{}),
	}
	for _, v := range vehicles {
		g.VehicleSet[v] = struct{}{}
	}
	return g
}

// Start begins the gateway read/forward loop in a background goroutine.
// Returns nil even if the underlying device is nil (no-op for testing).
func (g *Gateway) Start() error {
	if g.Device == nil {
		log.Printf("[gateway %s] no serial device; running in headless mode", g.ID)
		return nil
	}

	// Start uplink loop (Vehicle → Fog)
	g.wg.Add(1)
	go g.loop()

	// Start downlink HTTP handler (Fog → Vehicle)
	mux := http.NewServeMux()
	mux.HandleFunc("/command", g.handleControl)
	// port := g.URL[strings.LastIndex(g.URL, ":"):]
	addr := g.URL
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	g.server = &http.Server{Addr: addr, Handler: mux}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		log.Printf("[gateway %s] HTTP listening at %s/command", g.ID, addr)
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[gateway %s] HTTP error: %v", g.ID, err)
		}
	}()

	return nil
}

// loop continuously reads lines from the Device, decodes, re-encodes and posts to Fog.
func (g *Gateway) loop() {
	defer g.wg.Done()
	for {
		select {
		case <-g.stop:
			log.Printf("[gateway %s] stopping uplink loop", g.ID)
			return
		default:
		}

		line, err := g.Device.ReadLine(0)
		if err != nil {
			// transient error: wait and continue
			time.Sleep(100 * time.Millisecond)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Decode input using InParser
		vd, err := g.InParser.DecodeTelemetry(line)
		if err != nil {
			log.Printf("[gateway %s] decode %s error: %v", g.ID, g.WireIn, err)
			continue
		} else {
			log.Printf("[gateway %s] decode %s: %s", g.ID, g.WireIn, line)
		}

		// check validation of packet that belong to vehicle managed by gateway
		if _, ok := g.VehicleSet[vd.VehicleID]; !ok {
			log.Printf("[gateway %s] skip telemetry from unmanaged vehicle %s", g.ID, vd.VehicleID)
			continue
		}

		// Encode for Fog using OutParser
		out, err := g.OutParser.EncodeTelemetry(vd)
		if err != nil {
			log.Printf("[gateway %s] encode %s err: %v", g.ID, g.WireOut, err)
			continue
		} else {
			log.Printf("[gateway %s] encode %s: %s", g.ID, g.WireOut, out)
		}

		// Determine content-type
		contentType := "text/plain"
		if g.WireOut == "json" {
			contentType = "application/json"
		}

		// send to Fog server
		resp, err := http.Post(g.FogURL+"/api/telemetry", contentType, strings.NewReader(out))
		if err != nil {
			log.Printf("[gateway %s] forward err: %v", g.ID, err)
			continue
		} else {
			log.Printf("[gateway %s] uplink %s → %s : %s", g.ID, g.WireIn, g.WireOut, out)
		}

		// Properly close response body (lint-safe)
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Printf("[gateway %s] warning: discard body: %v", g.ID, err)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[gateway %s] warning: close body: %v", g.ID, cerr)
		}
	}
}

// handleControl receives a control message from Fog (JSON or CSV),
// decodes into ControlData, re-encodes into wire_in format, and
// sends it downlink to the Vehicle via LoRa.
func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[gateway %s] warning: close control body: %v", g.ID, cerr)
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	line := strings.TrimSpace(string(body))
	if line == "" {
		http.Error(w, "empty control message", http.StatusBadRequest)
		return
	}

	// Step 1: decode incoming control message (Fog → Gateway)
	var ctl model.ControlData
	// Try JSON first
	if err := json.Unmarshal(body, &ctl); err != nil {
		// Try CSV fallback
		csvp := parser.NewCSVParser()
		ctl2, err2 := csvp.DecodeControl(line)
		if err2 != nil {
			http.Error(w, "invalid control message format", http.StatusBadRequest)
			log.Printf("[gateway %s] invalid control: %v", g.ID, err2)
			return
		}
		ctl = ctl2
	}

	// Step 2: encode message for downlink (Gateway → Vehicle)
	var downlink string
	switch strings.ToLower(g.WireIn) {
	case "json":
		b, err := json.Marshal(ctl)
		if err != nil {
			http.Error(w, "encode downlink error", http.StatusInternalServerError)
			return
		}
		downlink = string(b)
	default: // CSV
		csvp := parser.NewCSVParser()
		s, err := csvp.EncodeControl(ctl)
		if err != nil {
			http.Error(w, "encode downlink error", http.StatusInternalServerError)
			return
		}
		downlink = s
	}

	// Step 3: send to Vehicle via LoRa
	if err := g.Device.WriteLine(downlink); err != nil {
		http.Error(w, "failed to send to vehicle", http.StatusInternalServerError)
		log.Printf("[gateway %s] downlink send error: %v", g.ID, err)
		return
	}

	log.Printf("[gateway %s] downlink %s: %s", g.ID, g.WireIn, downlink)
	w.WriteHeader(http.StatusAccepted)
}

// Stop stops the gateway background loop and closes the device if present.
func (g *Gateway) Stop() {
	log.Printf("[gateway %s] stopping...", g.ID)

	// Đóng stop channel an toàn
	select {
	case <-g.stop:
	default:
		close(g.stop)
	}

	// Stop HTTP server
	if g.server != nil {
		log.Printf("[gateway %s] Shutting down web server...", g.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			log.Printf("[gateway %s] HTTP server shutdown error: %v", g.ID, err)
		} else {
			log.Printf("[gateway %s] Web server stopped cleanly", g.ID)
		}
	}

	// Close device
	if g.Device != nil {
		if err := g.Device.Close(); err != nil {
			log.Printf("[gateway %s] device close err: %v", g.ID, err)
		}
	}

	// Wait goroutine done
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[gateway %s] stopped cleanly", g.ID)
	case <-time.After(3 * time.Second):
		log.Printf("[gateway %s] stop timeout (forcing exit)", g.ID)
	}
}

```

- /internal/core/system.go
```go
// Package core contains the main runtime logic and orchestration layer for the LoraFog system.
// It defines the FogServer, Gateway, Vehicle, and System types that manage their lifecycle.
package core

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

// System manages lifecycle of the main components (FogServer, Gateways, Vehicles).
// It loads configuration from a YAML file and constructs objects accordingly.
type System struct {
	cfgPath  string
	cfg      *model.Config
	parsers  map[string]parser.Parser
	Fog      *FogServer
	Gateways []*Gateway
	Vehicles []*Vehicle
	Arduinos []*device.ArduinoDevice
	SocatMgr *util.SocatManager

	stop      chan struct{}
	wg        sync.WaitGroup
	started   bool
	startLock sync.Mutex
}

// NewSystem reads the YAML configuration at cfgPath and creates a System instance.
// It also registers available parsers (csv/json) and constructs Gateway and Vehicle objects.
func NewSystem(cfgPath string) (*System, error) {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg model.Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	s := &System{
		cfgPath: cfgPath,
		cfg:     &cfg,
		parsers: make(map[string]parser.Parser),
	}

	// Virtual Serial Setup
	virtMgr := util.NewSocatManager()
	s.SocatMgr = virtMgr
	for _, pair := range cfg.VirtualSerials.Pairs {
		if err := virtMgr.CreatePair(pair.Left, pair.Right); err != nil {
			log.Printf("[virt-serial] failed to create pair: %v", err)
		}
	}
	time.Sleep(2 * time.Second)

	// register parser formats
	s.parsers["csv"] = parser.NewCSVParser()
	s.parsers["json"] = parser.NewJSONParser()

	// construct FogServer from config
	if cfg.Server.FogAddr != "" {
		// s.Fog = NewFogServer(cfg.Server.FogAddr)
		s.Fog = NewFogServer(cfg.Server.FogAddr, cfg.Server.AppAddr)
		s.Fog.wireFmt = strings.ToLower(cfg.Global.WireFormat)

		for _, gw := range cfg.Server.Gateways {
			s.Fog.RegisterGateway(gw.ID, gw.URL, gw.Vehicles)
			log.Printf("[config] Registered gateway %s (%s) vehicles=%v",
				gw.ID, gw.URL, gw.Vehicles)
		}
	} else {
		log.Println("[config] Fog server disabled (no fog_addr configured)")
	}

	// construct gateways from config
	for _, gcfg := range cfg.Gateways {
		inFmt := gcfg.WireIn
		if inFmt == "" {
			inFmt = cfg.Global.WireFormat
		}
		outFmt := gcfg.WireOut
		if outFmt == "" {
			outFmt = cfg.Global.WireFormat
		}
		gw := NewGateway(
			gcfg.ID,
			gcfg.LoraDev,
			gcfg.LoraBaud,
			gcfg.URL,
			gcfg.FogURL,
			gcfg.WireIn,
			gcfg.WireOut,
			s.parsers[inFmt],
			s.parsers[outFmt],
			gcfg.Vehicles,
		)
		s.Gateways = append(s.Gateways, gw)
	}

	// construct vehicles from config
	for _, vcfg := range cfg.Vehicles {
		wf := vcfg.WireFormat
		if wf == "" {
			wf = cfg.Global.WireFormat
		}
		p := s.parsers[wf]
		veh := NewVehicle(
			vcfg.ID,
			vcfg.LoraDev,
			vcfg.LoraBaud,
			vcfg.ID,
			vcfg.ArduinoDev,
			vcfg.ArduinoBaud,
			time.Duration(vcfg.TelemetryIntervalMs)*time.Millisecond,
			p,
		)
		s.Vehicles = append(s.Vehicles, veh)
	}

	// construct arduino devices from config
	for _, arduinoCfg := range cfg.Arduinos {
		arduino := device.NewArduinoDevice(arduinoCfg.ID, arduinoCfg.Dev, arduinoCfg.Baud)
		s.Arduinos = append(s.Arduinos, arduino)
	}
	return s, nil
}

// StartAll starts the FogServer, all Gateways and all Vehicles concurrently.
// It registers gateways to the FogServer registry when a gateway is successfully started.
func (s *System) StartAll() error {
	s.startLock.Lock()
	defer s.startLock.Unlock()
	if s.started {
		return nil
	}
	s.started = true
	s.stop = make(chan struct{})

	// start fog server
	// go s.Fog.Start()
	if s.Fog != nil {
		log.Printf("[system] Starting fog server at %s ...", s.Fog.Addr)
		go func() {
			if err := s.Fog.Start(); err != nil {
				log.Printf("[system] Fog server error: %v", err)
			}
		}()
	} else {
		log.Println("[system] Fog server is disabled; skipping startup")
	}

	// start gateways and register them to fog registry
	for _, g := range s.Gateways {
		if err := g.Start(); err != nil {
			log.Printf("[gateway %s] start err: %v", g.ID, err)
		} else {
			log.Printf("[gateway %s] start: Success", g.ID)
			// s.Fog.RegisterGateway(g.ID, g.FogURL, g.Vehicles)
		}
	}

	// start vehicle agents
	for _, v := range s.Vehicles {
		if err := v.Start(); err != nil {
			log.Printf("[vehicle %s] start err: %v", v.ID, err)
		} else {
			log.Printf("[vehicle %s] start: Success", v.ID)
		}
	}

	// start arduino simulation
	for _, arduino := range s.Arduinos {
		s.wg.Add(1)
		go func(arduino *device.ArduinoDevice) {
			defer s.wg.Done()
			log.Printf("[system] starting arduino %s device %s (baud %d)", arduino.ID, arduino.Device, arduino.Baud)
			stop := make(chan struct{})
			go func() {
				<-s.stop
				close(stop)
			}()

			if err := arduino.StartSimulation(stop); err != nil {
				log.Printf("[arduino %s] simulate failed: %v", arduino.ID, err)
			} else {
				log.Printf("[arduino %s] simulation stopped", arduino.ID)
			}
		}(arduino)
	}
	return nil
}

// StopAll stops all running components gracefully.
func (s *System) StopAll() {
	s.startLock.Lock()
	defer s.startLock.Unlock()
	if !s.started {
		return
	}
	for _, g := range s.Gateways {
		g.Stop()
	}
	for _, v := range s.Vehicles {
		v.Stop()
	}
	for _, a := range s.Arduinos {
		if err := a.Close(); err != nil {
			log.Printf("[warning] failed to close arduino %s: %v", a.ID, err)
		}
	}
	if s.SocatMgr != nil {
		s.SocatMgr.Cleanup()
	}
	s.Fog.Stop()
	log.Println("[system] stopping all components...")
	close(s.stop)
	s.wg.Wait()
	s.started = false
	log.Println("[system] all components stopped.")
}

```

- /internal/core/vehicle.go
```go
package core

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
)

// Vehicle represents a vehicle agent that reads telemetry and periodically
// sends telemetry via an underlying Device (e.g., LoRa serial).
type Vehicle struct {
	ID            string
	Device        device.Device
	ArduinoDevice *device.ArduinoDevice
	Parser        parser.Parser
	Interval      time.Duration

	stop          chan struct{}
	wg            sync.WaitGroup
	lastTelemetry model.ArduinoData
	lastUpdate    time.Time
	arduinoFn     func()
}

// NewVehicle constructs a Vehicle with given identifiers, device paths and parser.
func NewVehicle(id, loraDev string, loraBaud int, arduinoID string, arduinoDev string, arduinoBaud int, interval time.Duration, p parser.Parser) *Vehicle {
	dev, _ := device.NewSerialDevice(loraDev, loraBaud)
	v := &Vehicle{ID: id, Device: dev, Parser: p, Interval: interval, stop: make(chan struct{})}
	if arduinoDev != "" {
		v.ArduinoDevice = device.NewArduinoDevice(arduinoID, arduinoDev, arduinoBaud)
	}
	return v
}

// Start initializes the vehicle data acquisition and telemetry loop.
// It starts reading Arduino data and immediately sends telemetry upon new data arrival.
// Optionally, it may still include a periodic heartbeat if needed.
func (v *Vehicle) Start() error {
	// --- 1. Start Arduino telemetry reader ---
	if v.ArduinoDevice != nil {
		ch := make(chan model.ArduinoData, 5)

		// Start reading Arduino asynchronously
		stop, err := v.ArduinoDevice.Read(ch)
		if err != nil {
			log.Printf("[vehicle %s] Arduino start err: %v", v.ID, err)
		} else {
			log.Printf("[vehicle %s] Arduino start: success", v.ID)
			v.arduinoFn = stop
			v.wg.Add(1)
			go func() {
				defer v.wg.Done()
				for {
					select {
					case <-v.stop:
						log.Printf("[vehicle %s] Stopping Arduino loop", v.ID)
						return
					case arduinoData, ok := <-ch:
						if !ok {
							log.Printf("[vehicle %s] Arduino channel closed", v.ID)
							return
						}
						// Update last Arduino reading
						log.Printf("[vehicle %s] received telemetry", v.ID)
						v.lastTelemetry = arduinoData
						v.lastUpdate = time.Now()
						v.sendTelemetry()
						log.Printf("[vehicle %s] sended telemetry", v.ID)
					}
				}
			}()
		}
	}

	// heartbeat ticker – periodic "alive" message
	if v.Interval > 0 {
		v.wg.Add(1)
		go func() {
			defer v.wg.Done()
			ticker := time.NewTicker(v.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-v.stop:
					log.Printf("[vehicle %s] stopping heartbeat", v.ID)
					return
				case <-ticker.C:
					// Only send heartbeat if no Arduino data for a while
					if time.Since(v.lastUpdate) > v.Interval {
						log.Printf("[vehicle %s] sending heartbeat", v.ID)
						v.sendTelemetry()
					}
				}
			}
		}()
	}

	// --- 2. Start LoRa control listener ---
	if v.Device != nil && v.ArduinoDevice != nil {
		v.wg.Add(1)
		go func() {
			defer v.wg.Done()
			for {
				select {
				case <-v.stop:
					log.Printf("[vehicle %s] stopping LoRa control listener", v.ID)
					return
				default:
				}

				dataIn, err := v.Device.ReadLine(0)
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}

				dataIn = strings.TrimSpace(dataIn)
				if dataIn == "" {
					continue
				}

				// Parse control packet
				control, err := v.Parser.DecodeControl(dataIn)
				if err != nil {
					log.Printf("[vehicle %s] invalid control packet: %v (%s)", v.ID, err, dataIn)
					continue
				}
				if control.VehicleID != v.ID {
					log.Printf("[vehicle %s] Reject control: %s", v.ID, dataIn)
					continue
				} else {
					log.Printf("[vehicle %s] Receive control packet: %s", v.ID, dataIn)
				}

				// targetHead := int(calculateBearing(v.lastTelemetry.Latitude,v.lastTelemetry.Longitude,control.Latitude,control.Longitude))
				arduinoControl := model.ArduinoControl{
					CruiseSpeed: control.Speed,
					Latitude:    control.Latitude,
					Longitude:   control.Longitude,
					Kp:          control.Kp,
					Ki:          control.Ki,
					Kd:          control.Kd,
				}
				dataOut := fmt.Sprintf("%d,%.6f,%.6f,%.6f,%.6f,%.6f",
					arduinoControl.CruiseSpeed,
					arduinoControl.Latitude,
					arduinoControl.Longitude,
					arduinoControl.Kp,
					arduinoControl.Ki,
					arduinoControl.Kd,
				)

				// Forward control data to Arduino
				if err := v.ArduinoDevice.WriteLine(dataOut); err != nil {
					log.Printf("[vehicle %s] failed to forward control to Arduino: %v", v.ID, err)
				} else {
					log.Printf("[vehicle %s] forwarded control to Arduino: %s", v.ID, dataOut)
				}
			}
		}()
	}

	return nil
}

// Stop stops the vehicle goroutines, Arduino provider and closes the device.
func (v *Vehicle) Stop() {
	// close LoRa serial
	if v.Device != nil {
		if err := v.Device.Close(); err != nil {
			log.Printf("[vehicle %s] device close err: %v", v.ID, err)
		}
	}

	// close Arduino serial
	if v.ArduinoDevice != nil {
		if err := v.ArduinoDevice.Close(); err != nil {
			log.Printf("[vehicle %s] arduino close err: %v", v.ID, err)
		}
	}
	// close stop channel (idempotent)
	select {
	case <-v.stop:
		// already closed
	default:
		close(v.stop)
	}
	if v.arduinoFn != nil {
		v.arduinoFn()
	}

	v.wg.Wait()
}

// sendTelemetry builds a VehicleData from last data/fallback values and writes it to the Device.
func (v *Vehicle) sendTelemetry() {
	latitude, longitude := v.lastTelemetry.Latitude, v.lastTelemetry.Longitude
	if latitude == 0 && longitude == 0 {
		// fallback coordinate (Hanoi)
		latitude, longitude = 21.0285, 105.8048
	}
	vd := model.VehicleData{
		VehicleID:   v.ID,
		Latitude:    latitude,
		Longitude:   longitude,
		CurrentHead: v.lastTelemetry.CurrentHead,
		TargetHead:  v.lastTelemetry.TargetHead,
		LeftSpeed:   v.lastTelemetry.LeftSpeed,
		RightSpeed:  v.lastTelemetry.RightSpeed,
	}
	line, err := v.Parser.EncodeTelemetry(vd)
	if err != nil {
		log.Printf("[vehicle %s] encode telemetry err: %v", v.ID, err)
		return
	} else {
		log.Printf("[vehicle %s] encode telemetry: %s", v.ID, line)
	}
	if v.Device != nil {
		if err := v.Device.WriteLine(line); err == nil {
			log.Printf("[vehicle %s] sent telemetry: %s", v.ID, line)
		} else {
			log.Printf("[vehicle %s] lora write err: %v", v.ID, err)
		}
	} else {
		log.Printf("[vehicle %s] device absent; telemetry not sent", v.ID)
	}
}

// func calculateBearing(currentLatitude, currentLongitude, targetLatitude, targetLongitude float64) float64 {
// 	// Convert degrees to radians
// 	currentLatitudeRadian := currentLatitude * math.Pi / 180.0
// 	currentLongitudeRadian := currentLongitude * math.Pi / 180.0
// 	targetLatitudeRadian := targetLatitude * math.Pi / 180.0
// 	targetLongitudeRadian := targetLongitude * math.Pi / 180.0
//
// 	// Calculate difference in longitude
// 	deltaLongitude := targetLongitudeRadian - currentLongitudeRadian
//
// 	// Bearing formula
// 	y := math.Sin(deltaLongitude) * math.Cos(targetLatitudeRadian)
// 	x := math.Cos(currentLatitudeRadian)*math.Sin(targetLatitudeRadian) - math.Sin(currentLatitudeRadian)*math.Cos(targetLatitudeRadian)*math.Cos(deltaLongitude)
//
// 	bearing := math.Atan2(y, x) * 180.0 / math.Pi
//
// 	// Normalize to [0, 360)
// 	bearing = math.Mod(bearing+360.0, 360.0)
//
// 	return bearing
// }

```

- /internal/device/arduino.go
```go
// Package device implements an Arduino serial reader,
// which exchanges telemetry data such as motor speed and heading.
package device

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"LoraFog/internal/model"
)

// ArduinoDevice represents a serial-connected Arduino
// that transmits telemetry data (latitude, longitude, motor speeds, etc.).
type ArduinoDevice struct {
	ID     string
	Device string
	Baud   int
	Serial *SerialDevice
}

// NewArduinoDevice creates a new Arduino device handler.
func NewArduinoDevice(id, device string, baud int) *ArduinoDevice {
	return &ArduinoDevice{ID: id, Device: device, Baud: baud}
}

// --- Implementation of Device interface ---

// Open initializes the Arduino serial connection.
func (arduino *ArduinoDevice) Open() error {
	if arduino.Serial != nil {
		return nil
	}
	serialDevice, err := NewSerialDevice(arduino.Device, arduino.Baud)
	if err != nil {
		return fmt.Errorf("open arduino serial failed: %w", err)
	}
	arduino.Serial = serialDevice
	return nil
}

// Close terminates the serial connection safely.
func (arduino *ArduinoDevice) Close() error {
	if arduino.Serial == nil {
		return nil
	}
	err := arduino.Serial.Close()
	arduino.Serial = nil
	return err
}

// ReadLine reads a single line of data from the Arduino.
func (arduino *ArduinoDevice) ReadLine(timeout time.Duration) (string, error) {
	if arduino.Serial == nil {
		return "", errors.New("arduino serial not open")
	}
	return arduino.Serial.ReadLine(timeout)
}

// WriteLine writes a command or message to the Arduino.
func (arduino *ArduinoDevice) WriteLine(line string) error {
	if arduino.Serial == nil {
		return errors.New("arduino serial not open")
	}
	return arduino.Serial.WriteLine(line)
}

// --- Additional behavior ---

// Read continuously parses JSON telemetry sent from the Arduino and pushes it into the channel.
// Each line is expected to contain a valid JSON object of type ArduinoData.
func (arduino *ArduinoDevice) Read(out chan<- model.ArduinoData) (func(), error) {
	if err := arduino.Open(); err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	go func() {
		defer func() {
			_ = arduino.Close()
			close(out)
		}()

		reader := bufio.NewReader(arduino.Serial.port)
		for {
			select {
			case <-stop:
				return
			default:
			}

			dataIn, err := reader.ReadString('\n')
			if err != nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}

			dataIn = strings.TrimSpace(dataIn)
			if dataIn == "" {
				continue
			}

			parts := strings.Split(dataIn, ",")
			if len(parts) > 6 {
				continue
			}
			latitude, _ := strconv.ParseFloat(parts[0], 64)
			longitude, _ := strconv.ParseFloat(parts[1], 64)
			leftSpeed, _ := strconv.ParseFloat(parts[2], 64)
			rightSpeed, _ := strconv.ParseFloat(parts[3], 64)
			currentHead, _ := strconv.ParseFloat(parts[4], 64)
			targetHead, _ := strconv.ParseFloat(parts[5], 64)
			out <- model.ArduinoData{
				Latitude:    latitude,
				Longitude:   longitude,
				LeftSpeed:   int(leftSpeed),
				RightSpeed:  int(rightSpeed),
				CurrentHead: int(currentHead),
				TargetHead:  int(targetHead),
			}
		}
	}()
	return func() { close(stop) }, nil
}

// StartSimulation generates fake Arduino telemetry for testing.
// It writes mock JSON data over the serial interface until stop is closed.
func (arduino *ArduinoDevice) StartSimulation(stop <-chan struct{}) error {
	if err := arduino.Open(); err != nil {
		return err
	}
	defer func() {
		if err := arduino.Close(); err != nil {
			log.Printf("[warning] Failed to close arduino device: %v", err)
		}
	}()

	fmt.Printf("[arduino %s] Simulator started on %s (baud %d)\n", arduino.ID, arduino.Device, arduino.Baud)

	for {
		select {
		case <-stop:
			fmt.Printf("[arduino %s] Simulation stopped.\n", arduino.ID)
			return nil
		default:
		}

		arduinoData := model.ArduinoData{
			Latitude:    21.0285 + (rand.Float64()-0.5)*0.001,
			Longitude:   105.8048 + (rand.Float64()-0.5)*0.001,
			LeftSpeed:   1000,
			RightSpeed:  1000,
			CurrentHead: 0 + (rand.Intn(361)),
			TargetHead:  0 + (rand.Intn(361)),
		}

		message := fmt.Sprintf("%.6f,%.6f,%d,%d,%d,%d",
			arduinoData.Latitude,
			arduinoData.Longitude,
			arduinoData.LeftSpeed,
			arduinoData.RightSpeed,
			arduinoData.CurrentHead,
			arduinoData.TargetHead)
		if err := arduino.WriteLine(message); err != nil {
			log.Printf("[arduino %s] simulate write error: %v", arduino.ID, err)
		} else {
			log.Printf("[arduino %s] simulate write: %s", arduino.ID, message)
		}

		time.Sleep(5 * time.Second)
	}
}

```

- /internal/device/device.go
```go
// Package device defines unified interfaces for all hardware communication devices,
// such as LoRa modules, GPS receivers, or virtual serial ports.
// It abstracts read/write operations and optionally supports simulation for test environments.
package device

import "time"

// Device defines the common behavior of any communication-capable device.
// Implementations must support opening, closing, and line-based read/write.
type Device interface {
	// Open initializes and prepares the device for data communication.
	Open() error

	// Close gracefully closes the device and releases all underlying resources.
	Close() error

	// ReadLine reads a single line terminated by '\n'.
	// If timeout > 0, it must return after the given timeout even if no data arrives.
	ReadLine(timeout time.Duration) (string, error)

	// WriteLine writes a string followed by '\n' to the device.
	WriteLine(s string) error
}

// Simulatable extends Device with the ability to generate mock data.
// It is typically implemented by GPS or sensor devices for testing.
type Simulatable interface {
	Device
	// Simulate generates mock output continuously until stop is closed.
	StartSimulation(stop <-chan struct{}) error
}

```

- /internal/device/gps.go
```go
// Package device implements a GPS device reader using NMEA protocol.
// It supports both real GPS serial reading and simulated output generation.
package device

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"LoraFog/internal/model"
	"LoraFog/internal/util"
)

// GpsDevice implements both Device and Simulatable interfaces.
// It can read real NMEA data from a serial GPS receiver or simulate GPS output for testing.
type GpsDevice struct {
	ID     string
	Device string
	Baud   int
	Serial *SerialDevice
}

// NewGpsDevice creates a new GPS device based on serial communication.
func NewGpsDevice(id string, device string, baud int) *GpsDevice {
	return &GpsDevice{ID: id, Device: device, Baud: baud}
}

// --- Implementation of Device interface ---

// Open opens the GPS serial port.
func (gps *GpsDevice) Open() error {
	if gps.Serial != nil {
		return nil
	}
	serialDevice, err := NewSerialDevice(gps.Device, gps.Baud)
	if err != nil {
		return fmt.Errorf("open gps serial failed: %w", err)
	}
	gps.Serial = serialDevice
	return nil
}

// Close closes the GPS serial port safely.
func (gps *GpsDevice) Close() error {
	if gps.Serial == nil {
		return nil
	}
	err := gps.Serial.Close()
	gps.Serial = nil
	return err
}

// ReadLine reads one NMEA line from the GPS.
func (gps *GpsDevice) ReadLine(timeout time.Duration) (string, error) {
	if gps.Serial == nil {
		return "", errors.New("gps serial not open")
	}
	return gps.Serial.ReadLine(timeout)
}

// WriteLine writes a string to the GPS port (rarely used, but provided for interface compatibility).
func (gps *GpsDevice) WriteLine(dataOut string) error {
	if gps.Serial == nil {
		return errors.New("gps serial not open")
	}
	return gps.Serial.WriteLine(dataOut)
}

// --- Additional functions ---

// Read continuously streams GPS data and pushes parsed coordinates to a channel.
// Returns a stop function to safely terminate the loop.
func (gps *GpsDevice) Read(out chan<- model.GpsData) (func(), error) {
	if err := gps.Open(); err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	go func() {
		defer func() {
			_ = gps.Close()
			close(out)
		}()

		for {
			select {
			case <-stop:
				return
			default:
			}

			dataIn, err := gps.ReadLine(0)
			if err != nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			dataIn = strings.TrimSpace(dataIn)
			if !strings.HasPrefix(dataIn, "$GPRMC") && !strings.HasPrefix(dataIn, "$GNRMC") {
				continue
			}
			parts := strings.Split(dataIn, ",")
			if len(parts) < 7 || parts[3] == "" || parts[5] == "" {
				continue
			}
			lat, err1 := util.ParseNMEACoord(parts[3], parts[4])
			lon, err2 := util.ParseNMEACoord(parts[5], parts[6])
			if err1 != nil || err2 != nil {
				log.Printf("[gps] Skip invalid coord: %s", dataIn)
				continue
			}
			out <- model.GpsData{Latitude: lat, Longitude: lon}
		}
	}()
	return func() { close(stop) }, nil
}

// --- Implementation of Simulatable interface ---

// StartSimulation continuously writes fake GPS NMEA sentences to the port until stop is closed.
func (gps *GpsDevice) StartSimulation(stop <-chan struct{}) error {
	if err := gps.Open(); err != nil {
		return err
	}
	defer func() {
		if err := gps.Close(); err != nil {
			log.Printf("[warning] Failed to close gps device: %v", err)
		}
	}()

	fmt.Printf("[gps %s] Simulator started on %s (baud %d)\n", gps.ID, gps.Device, gps.Baud)

	for {
		select {
		case <-stop:
			fmt.Printf("[gps %s] Simulation stopped.\n", gps.ID)
			return nil
		default:
		}

		lat := 21.0285 + (rand.Float64()-0.5)*0.001
		lon := 105.8048 + (rand.Float64()-0.5)*0.001
		latStr, latDir := util.ToNMEACoord(lat, true)
		lonStr, lonDir := util.ToNMEACoord(lon, false)
		timeUTC := time.Now().UTC().Format("150405.00")
		valid := "A"

		nmea := fmt.Sprintf("$GPRMC,%s,%s,%s,%s,%s,%s,3.332,272.24,241025,,,A*69\r\n",
			timeUTC, valid, latStr, latDir, lonStr, lonDir)

		if err := gps.WriteLine(nmea); err != nil {
			log.Printf("[gps %s] simulate write error: %v", gps.ID, err)
		} else {
			log.Printf("[gps %s] simulate write: %s", gps.ID, nmea)
		}
		time.Sleep(1 * time.Second)
	}
}

```

- /internal/device/serial.go
```go
// Package device implements SerialDevice using go.bug.st/serial,
// which provides real serial communication support for devices like LoRa or sensors.
package device

import (
	"bufio"
	"errors"
	"fmt"
	"time"

	serial "go.bug.st/serial"
)

// SerialDevice implements Device using go.bug.st/serial.
type SerialDevice struct {
	port serial.Port
	r    *bufio.Reader
	dev  string
	baud int
}

// NewSerialDevice creates and opens a serial device with the given path and baudrate.
func NewSerialDevice(dev string, baud int) (*SerialDevice, error) {
	p, err := serial.Open(dev, &serial.Mode{BaudRate: baud})
	if err != nil {
		return nil, fmt.Errorf("failed to open serial %s: %w", dev, err)
	}
	return &SerialDevice{port: p, r: bufio.NewReader(p), dev: dev, baud: baud}, nil
}

// Open ensures that the serial port is ready for use.
func (s *SerialDevice) Open() error {
	if s.port != nil {
		return nil
	}
	p, err := serial.Open(s.dev, &serial.Mode{BaudRate: s.baud})
	if err != nil {
		return fmt.Errorf("reopen serial %s failed: %w", s.dev, err)
	}
	s.port = p
	s.r = bufio.NewReader(p)
	return nil
}

// Close closes the underlying serial connection.
func (s *SerialDevice) Close() error {
	if s.port == nil {
		return nil
	}
	err := s.port.Close()
	s.port = nil
	return err
}

// ReadLine reads a single line from the serial port, blocking until newline or timeout.
func (s *SerialDevice) ReadLine(timeout time.Duration) (string, error) {
	if s.port == nil {
		return "", errors.New("serial port not open")
	}

	ch := make(chan struct {
		line string
		err  error
	}, 1)

	go func() {
		line, err := s.r.ReadString('\n')
		ch <- struct {
			line string
			err  error
		}{line, err}
	}()

	if timeout <= 0 {
		res := <-ch
		return res.line, res.err
	}

	select {
	case res := <-ch:
		return res.line, res.err
	case <-time.After(timeout):
		return "", errors.New("read timeout")
	}
}

// WriteLine writes a single line followed by '\n' to the serial port.
func (s *SerialDevice) WriteLine(line string) error {
	if s.port == nil {
		return errors.New("serial port not open")
	}
	_, err := s.port.Write(append([]byte(line), '\n'))
	return err
}

```

- /internal/app/app.go
```go
package app

import (
	"context"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.etcd.io/bbolt"
)

type App struct {
	DB     *bbolt.DB
	Tmpl   *template.Template
	Mux    *http.ServeMux
	Server *http.Server
}

// NewApp initializes the web app with templates, database, and routes.
func NewApp() (*App, error) {
	cwd, _ := os.Getwd()
	tmplPath := filepath.Join(cwd, "web", "templates", "*.html")

	tmpl := template.New("").Funcs(template.FuncMap{
		"year": func() int { return time.Now().Year() },
	})

	tmpl, err := tmpl.ParseGlob(tmplPath)
	if err != nil {
		return nil, fmt.Errorf("[app] failed to load templates: %w", err)
	}

	if err := os.MkdirAll("tmp", 0o755); err != nil {
		return nil, fmt.Errorf("[app] failed to create tmp/: %w", err)
	}

	dbPath := filepath.Join("tmp", "data.db")
	db, err := bbolt.Open(dbPath, 0o666, &bbolt.Options{Timeout: 1 * time.Second})
	if err != nil {
		return nil, fmt.Errorf("[app] failed to open BoltDB: %w", err)
	}

	app := &App{
		DB:   db,
		Tmpl: tmpl,
		Mux:  http.NewServeMux(),
	}

	app.registerRoutes()
	return app, nil
}

// Start launches the web server and blocks until stopped.
func (a *App) Start(addr string) error {
	if addr == "" {
		log.Println("[app] app server not started (empty address)")
		return nil
	}

	if a == nil {
		return fmt.Errorf("[app] Start called on nil receiver")
	}
	if a.Mux == nil {
		return fmt.Errorf("[app] nil HTTP mux — did you call registerRoutes()?")
	}

	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	if !strings.Contains(addr, ":") {
		addr = ":" + addr
	}

	a.Server = &http.Server{
		Addr:    addr,
		Handler: a.Mux,
	}

	log.Printf("[app] Web server listening at http://%s", addr)

	// Run server until Shutdown() is called
	if err := a.Server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("[app] HTTP server error: %w", err)
	}
	return nil
}

// Stop gracefully stops the web server and closes the DB.
func (a *App) Stop() {
	if a == nil {
		return
	}

	// Gracefully stop HTTP server
	if a.Server != nil {
		log.Println("[app] Shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := a.Server.Shutdown(ctx); err != nil {
			log.Printf("[app] HTTP server shutdown error: %v", err)
		} else {
			log.Println("[app] Web server stopped cleanly")
		}
	}

	// Close DB
	if a.DB != nil {
		if err := a.DB.Close(); err != nil {
			log.Printf("[app] error closing BoltDB: %v", err)
		} else {
			log.Println("[app] Closed BoltDB connection")
		}
	}
}

```

- /internal/app/handler_api.go
```go
// Package app implements the web server and API layer for the LoraFog dashboard.
package app

import (
	"bytes"
	"io"
	"log"
	"net/http"
	"time"

	"go.etcd.io/bbolt"
)

// handleTelemetry stores incoming telemetry data into BoltDB.
func (a *App) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read telemetry", http.StatusBadRequest)
		return
	}
	if cerr := r.Body.Close(); cerr != nil {
		log.Printf("[app] warning: failed to close telemetry body: %v", cerr)
	}

	timestamp := time.Now().Format(time.RFC3339Nano)
	err = a.DB.Update(func(tx *bbolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte("telemetry"))
		if err != nil {
			return err
		}
		return b.Put([]byte(timestamp), body)
	})
	if err != nil {
		http.Error(w, "failed to save telemetry", http.StatusInternalServerError)
		return
	}

	log.Printf("[app] received telemetry (%d bytes)", len(body))
	w.WriteHeader(http.StatusOK)
}

// handleLatest retrieves the latest telemetry entry.
func (a *App) handleLatest(w http.ResponseWriter, r *http.Request) {
	err := a.DB.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte("telemetry"))
		if b == nil {
			http.Error(w, "no telemetry data", http.StatusNotFound)
			return nil
		}
		c := b.Cursor()
		k, v := c.Last()
		if v == nil {
			http.Error(w, "no data available", http.StatusNotFound)
			return nil
		}
		w.Header().Set("Content-Type", "application/json")
		if _, werr := w.Write(v); werr != nil {
			log.Printf("[app] warning: failed to write telemetry: %v", werr)
		}
		log.Printf("[app] latest telemetry @ %s", string(k))
		return nil
	})
	if err != nil {
		http.Error(w, "failed to read telemetry", http.StatusInternalServerError)
	}
}

// handleControl forwards a control command to the Fog server.
func (a *App) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[app] warning: failed to close control body: %v", cerr)
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read control command", http.StatusBadRequest)
		return
	}

	// Forwards to FogServer (assuming it runs at :10000)
	resp, err := http.Post("http://localhost:10000/api/control", "application/json", bytes.NewReader(body))
	if err != nil {
		http.Error(w, "failed to forward control", http.StatusBadGateway)
		return
	}
	defer func() {
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[app] warning: failed to close fog response: %v", cerr)
		}
	}()

	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		log.Printf("[app] warning: failed to drain fog response body: %v", err)
	}

	w.WriteHeader(http.StatusAccepted)
}

```

- /internal/app/handler_auth.go
```go
package app

import (
	"html/template"
	"log"
	"net/http"
	"time"
)

// handleLogin displays a login form or processes login POST.
func (a *App) handleLogin(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		t, _ := template.ParseFiles("web/templates/login.html")
		_ = t.Execute(w, nil)
	case http.MethodPost:
		username := r.FormValue("username")
		password := r.FormValue("password")

		// Simple static check (can extend to DB user check)
		if username == "admin" && password == "1234" {
			http.SetCookie(w, &http.Cookie{
				Name:     "session_id",
				Value:    "admin",
				Path:     "/",
				Expires:  time.Now().Add(24 * time.Hour),
				HttpOnly: true,
			})
			http.Redirect(w, r, "/", http.StatusSeeOther)
			return
		}

		http.Redirect(w, r, "/login?err=1", http.StatusSeeOther)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleLogout clears session cookie.
func (a *App) handleLogout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "session_id",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
	http.Redirect(w, r, "/login", http.StatusSeeOther)
	log.Printf("[auth] user logged out")
}

```

- /internal/app/handler_dashboard.go
```go
package app

import (
	"log"
	"net/http"
)

// handleDashboard renders the main dashboard page.
func (a *App) handleDashboard(w http.ResponseWriter, r *http.Request) {
	log.Printf("[app] GET / (dashboard) from %s", r.RemoteAddr)
	data := map[string]any{
		"Title": "LoraFog Dashboard",
	}
	if err := a.Tmpl.ExecuteTemplate(w, "dashboard.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleGateways renders the list of gateways.
func (a *App) handleGateways(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Gateways"}
	if err := a.Tmpl.ExecuteTemplate(w, "gateways.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

// handleVehicles renders the list of vehicles.
func (a *App) handleVehicles(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Title": "Vehicles"}
	if err := a.Tmpl.ExecuteTemplate(w, "vehicles.html", data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

```

- /internal/app/middleware.go
```go
package app

import (
	"net/http"
)

// AuthMiddleware restricts access to logged-in users only.
func AuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie("session_id")
		if err != nil || cookie.Value == "" {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

```

- /internal/app/routes.go
```go
package app

import (
	"net/http"
)

// registerRoutes sets up all HTTP handlers for the application.
func (a *App) registerRoutes() {
	// Static files (CSS, JS)
	fs := http.FileServer(http.Dir("web/static"))
	a.Mux.Handle("/static/", http.StripPrefix("/static/", fs))

	// Public routes
	a.Mux.HandleFunc("/", a.handleDashboard)
	a.Mux.HandleFunc("/gateways", a.handleGateways)
	a.Mux.HandleFunc("/vehicles", a.handleVehicles)
	a.Mux.HandleFunc("/login", a.handleLogin)
	a.Mux.HandleFunc("/logout", a.handleLogout)

	// API routes
	a.Mux.HandleFunc("/api/telemetry", a.handleTelemetry)
	a.Mux.HandleFunc("/api/latest", a.handleLatest)
	a.Mux.HandleFunc("/api/control", a.handleControl)
}

```

- /internal/parser/csv.go
```go
// Package parser implements the CSVParser which handles encoding and decoding
// of telemetry and control data using comma-separated values format.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"LoraFog/internal/model"
)

// CSVParser implements Parser interface using CSV format.
// Example telemetry CSV: VEHICLE_ID,LAT,LON,HEAD_CUR,HEAD_TAR,LEFT,RIGHT,PID
type CSVParser struct{}

// NewCSVParser creates a new CSV parser instance.
func NewCSVParser() *CSVParser { return &CSVParser{} }

// EncodeTelemetry converts VehicleData into CSV string.
func (p *CSVParser) EncodeTelemetry(v model.VehicleData) (string, error) {
	line := fmt.Sprintf("%s,%.6f,%.6f,%d,%d,%d,%d",
		v.VehicleID, v.Latitude, v.Longitude, v.CurrentHead, v.TargetHead, v.LeftSpeed, v.RightSpeed)
	return line, nil
}

// DecodeTelemetry parses a CSV telemetry line into VehicleData struct.
func (p *CSVParser) DecodeTelemetry(line string) (model.VehicleData, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	// if len(fields) != 8 {
	// 	return model.VehicleData{}, fmt.Errorf("expected 8 fields, got %d", len(fields))
	// }

	latitude, _ := strconv.ParseFloat(fields[1], 64)
	longitude, _ := strconv.ParseFloat(fields[2], 64)
	currentHead, _ := strconv.ParseFloat(fields[3], 64)
	targetHead, _ := strconv.ParseFloat(fields[4], 64)
	leftSpeed, _ := strconv.ParseFloat(fields[5], 64)
	rightSpeed, _ := strconv.ParseFloat(fields[6], 64)
	// pid, _ := strconv.ParseFloat(fields[7], 64)

	return model.VehicleData{
		VehicleID:   fields[0],
		Latitude:    latitude,
		Longitude:   longitude,
		CurrentHead: int(currentHead),
		TargetHead:  int(targetHead),
		LeftSpeed:   int(leftSpeed),
		RightSpeed:  int(rightSpeed),
		// PID:         int(pid),
	}, nil
}

// EncodeControl converts a ControlData into CSV string.
func (p *CSVParser) EncodeControl(c model.ControlData) (string, error) {
	line := fmt.Sprintf("%s,%d,%.6f,%.6f,%.6f,%.6f,%.6f",
		c.VehicleID, c.Speed, c.Latitude, c.Longitude, c.Kp, c.Ki, c.Kd)
	return line, nil
}

// DecodeControl parses a CSV control message into ControlData struct.
func (p *CSVParser) DecodeControl(line string) (model.ControlData, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	// if len(fields) != 8 {
	// 	return model.ControlData{}, fmt.Errorf("expected 8 fields, got %d", len(fields))
	// }
	//
	// mode, _ := strconv.ParseFloat(fields[1], 64)
	speed, _ := strconv.ParseFloat(fields[1], 64)
	latitude, _ := strconv.ParseFloat(fields[2], 64)
	longitude, _ := strconv.ParseFloat(fields[3], 64)
	kp, _ := strconv.ParseFloat(fields[4], 64)
	ki, _ := strconv.ParseFloat(fields[5], 64)
	kd, _ := strconv.ParseFloat(fields[6], 64)

	return model.ControlData{
		VehicleID: fields[0],
		// Mode:      int(mode),
		Speed:     int(speed),
		Latitude:  latitude,
		Longitude: longitude,
		Kp:        kp,
		Ki:        ki,
		Kd:        kd,
	}, nil
}

```

- /internal/parser/json.go
```go
// Package parser implements the JSONParser which encodes and decodes telemetry
// and control data in JSON format.
package parser

import (
	"encoding/json"

	"LoraFog/internal/model"
)

// JSONParser implements Parser interface using JSON serialization.
type JSONParser struct{}

// NewJSONParser creates a new JSON parser.
func NewJSONParser() *JSONParser { return &JSONParser{} }

// EncodeTelemetry encodes VehicleData into JSON string.
func (p *JSONParser) EncodeTelemetry(v model.VehicleData) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// DecodeTelemetry decodes JSON string into VehicleData.
func (p *JSONParser) DecodeTelemetry(s string) (model.VehicleData, error) {
	var v model.VehicleData
	err := json.Unmarshal([]byte(s), &v)
	return v, err
}

// EncodeControl encodes ControlData into JSON string.
func (p *JSONParser) EncodeControl(c model.ControlData) (string, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

// DecodeControl decodes JSON string into ControlData.
func (p *JSONParser) DecodeControl(s string) (model.ControlData, error) {
	var c model.ControlData
	err := json.Unmarshal([]byte(s), &c)
	return c, err
}

```

- /internal/parser/parser.go
```go
// Package parser provides an abstraction layer for encoding and decoding data
// (telemetry and control messages) in multiple wire formats such as CSV and JSON.
package parser

import "LoraFog/internal/model"

// Parser defines a generic interface for encoding and decoding telemetry/control data.
// Different implementations support different wire formats such as CSV or JSON.
type Parser interface {
	// EncodeTelemetry converts a structured VehicleData into a wire string (CSV/JSON).
	EncodeTelemetry(model.VehicleData) (string, error)

	// DecodeTelemetry parses a raw string into a structured VehicleData.
	DecodeTelemetry(string) (model.VehicleData, error)

	// EncodeControl converts a ControlData into a wire string (CSV/JSON).
	EncodeControl(model.ControlData) (string, error)

	// DecodeControl parses a raw string into a structured ControlData.
	DecodeControl(string) (model.ControlData, error)
}

```
