
- /internal/core/vehicle.go
```go
// Package core implements Vehicle node logic.
package core

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
)

// Vehicle represents one LoRa boat node.
type Vehicle struct {
	ID         string
	HWID       string
	lora       *device.Lora
	arduino    *device.Arduino
	sessionKey []byte

	seq    uint8
	seqMu  sync.Mutex
	cancel context.CancelFunc
}

// NewVehicle creates a new Vehicle instance.
func NewVehicle(
	id string,
	loraDevice string, loraBaud int,
	arduinoDevice string, arduinoBaud int,
) *Vehicle {
	// id, err := util.GenerateUUIDv4()
	// if err != nil {
	// 	slog.Warn(
	// 		"Failed to Generate UUID for vehicle",
	// 		"err", err,
	// 	)
	// }
	v := &Vehicle{
		ID:   id,
		lora: device.NewLora(loraDevice, loraBaud),
	}
	if arduinoDevice != "" {
		v.arduino = device.NewArduino(arduinoDevice, arduinoBaud)
	}
	return v
}

// Start begins LoRa and Arduino telemetry loop.
func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	go v.listenLoop(ctx)
	if v.arduino != nil {
		go v.telemetryLoop(ctx)
	}
	return nil
}

// Shutdown stops the vehicle operations.
func (v *Vehicle) Shutdown() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.lora != nil {
		_ = v.lora.Close()
	}
	if v.arduino != nil {
		_ = v.arduino.Close()
	}
	slog.Info("vehicle stopped", "id", v.ID)
}

// listenLoop handles incoming LoRa frames (beacon/auth/control).
func (v *Vehicle) listenLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// ReadFrames returns zero or more complete frames within timeout
		frames, err := v.lora.ReadFrames(10 * time.Second)
		if err != nil {
			// timeout or read error — continue listening
			continue
		}
		if len(frames) == 0 {
			// nothing read this cycle
			continue
		}

		// Try parse with current session key (if any).
		// ParseFrame returns plaintext payload if key ok,
		// otherwise error. For plain frames, pass nil key.
		// Process each complete frame extracted by ReadFrames()
		for _, frame := range frames {
			var (
				packetType model.PacketType
				payload    []byte
			)

			// If we have a session key, try decrypt/verify first.
			if v.sessionKey != nil {
				packetType, _, _, payload, err = model.ParseFrame(frame, v.sessionKey)
				if err != nil {
					// try plain fallback (no key)
					packetType, _, _, payload, err = model.ParseFrame(frame, nil)
					if err != nil {
						// unable to parse even as plain — skip this frame
						slog.Warn(
							"failed to parse frame (secure and plain)",
							"vehicle", v.ID, "err", err,
						)
						continue
					}
				}
			} else {
				// no key -> parse as plain
				packetType, _, _, payload, err = model.ParseFrame(frame, nil)
				if err != nil {
					slog.Warn(
						"failed to parse plain frame",
						"vehicle", v.ID, "err", err,
					)
					continue
				}
			}
			switch packetType {
			case model.TypeBeacon:
				v.handleBeacon()
			case model.TypeAuth:
				v.handleAuth(payload)
			case model.TypeControl:
				v.handleControl(payload)
			default:
				// ignore other types
				slog.Debug(
					"ignoring unknown packet type",
					"vehicle", v.ID, "type", packetType,
				)
			}
		}
	}
}

// telemetryLoop reads Arduino data and sends telemetry frames.
func (v *Vehicle) telemetryLoop(ctx context.Context) {
	ch := make(chan model.ArduinoData, 4)
	stop, err := v.arduino.Read(ch)
	if err != nil {
		slog.Warn(
			"arduino read failed",
			"vehicle", v.ID, "err", err,
		)
		return
	}
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-ch:
			if !ok {
				return
			}
			v.sendTelemetry(d)
		}
	}
}

// sendTelemetry packs and transmits telemetry data.
func (v *Vehicle) sendTelemetry(d model.ArduinoData) {
	latitude := int32(d.Latitude * 1e7)
	longitude := int32(d.Longitude * 1e7)
	currentHead := uint16(d.CurrentHead % 360)
	targetHead := uint16(d.TargetHead % 360)
	leftSpeed := uint16(d.LeftSpeed)
	rightSpeed := uint16(d.RightSpeed)

	payload := model.BuildTelemetryPacked(
		latitude,
		longitude,
		currentHead,
		targetHead,
		leftSpeed,
		rightSpeed,
	)

	v.seqMu.Lock()
	seq := v.seq
	v.seq++
	v.seqMu.Unlock()
	nonce := make([]byte, model.NonceLength)
	binary.BigEndian.PutUint32(nonce[0:4], uint32(seq))

	// phần còn lại random
	if _, err := rand.Read(nonce[4:]); err != nil {
		// fallback: dùng timestamp cho phần random
		binary.BigEndian.PutUint64(nonce[4:], uint64(time.Now().UnixNano()))
	}

	var frame []byte
	var err error
	if v.sessionKey != nil {
		frame, err = model.BuildSecureFrame(
			model.TypeTelemetry,
			seq,
			nonce,
			payload,
			v.sessionKey,
		)
	} else {
		frame, err = model.BuildPlainFrame(
			model.TypeTelemetry,
			seq,
			nonce,
			payload,
		)
	}
	if err != nil {
		slog.Warn("build telemetry frame failed", "vehicle", v.ID, "err", err)
		return
	}
	if err := v.lora.WriteBytes(frame); err != nil {
		slog.Warn("failed to write telemetry frame", "vehicle", v.ID, "err", err)
		return
	}
	slog.Debug(
		"telemetry sent",
		"vehicle", v.ID,
		"seq", seq,
		"nonce", fmt.Sprintf("%X", nonce[:4]),
		"len", len(frame),
	)
}

// handleControl forwards downlink control to Arduino or logs it.
func (v *Vehicle) handleControl(p []byte) {
	// Control payload may be JSON or raw bytes; try JSON decode for helpful log.
	var ctrl map[string]any
	if err := json.Unmarshal(p, &ctrl); err == nil {
		slog.Info("control received", "vehicle", v.ID, "cmd", ctrl)
	} else {
		slog.Info("control received (raw)", "vehicle", v.ID)
	}
	if v.arduino != nil {
		// forward raw bytes as string (Arduino expects line-based). Adapt as necessary.
		_ = v.arduino.Write(string(p))
	}
}

// handleBeacon sends a HELLO frame to gateway when a beacon is seen.
func (v *Vehicle) handleBeacon() {
	msg := map[string]any{
		"type":       "hello",
		"vehicle_id": v.ID,
	}
	b, _ := json.Marshal(msg)
	nonce := make([]byte, model.NonceLength)
	copy(nonce, []byte(time.Now().Format("15040506"))[:model.NonceLength])
	frame, _ := model.BuildPlainFrame(model.TypeHello, 0, nonce, b)
	_ = v.lora.WriteBytes(frame)
	slog.Info("HELLO sent", "vehicle", v.ID)
}

// handleAuth processes an auth frame payload (expects JSON with key_hex and ttl).
func (v *Vehicle) handleAuth(p []byte) {
	var am map[string]any
	if err := json.Unmarshal(p, &am); err != nil {
		slog.Warn("invalid auth payload", "vehicle", v.ID, "err", err)
		return
	}
	if kh, ok := am["key_hex"].(string); ok && kh != "" {
		kb, err := hex.DecodeString(kh)
		if err != nil {
			slog.Warn("invalid key hex", "vehicle", v.ID, "err", err)
		} else if len(kb) == 16 {
			v.sessionKey = kb
			slog.Info("session key applied", "vehicle", v.ID)
		}
	}
	if ttlf, ok := am["ttl"].(float64); ok {
		_ = ttlf // we could store expiry if needed
	}
}

// ApplyKeyHex sets AES key for secure transmission.
func (v *Vehicle) ApplyKeyHex(h string) error {
	k, err := hex.DecodeString(h)
	if err != nil {
		return err
	}
	if len(k) != 16 {
		return fmt.Errorf("invalid key length %d", len(k))
	}
	v.sessionKey = k
	slog.Info("applied session key", "vehicle", v.ID)
	return nil
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

	var telemetry model.TelemetryData
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
	s.sessions[req.VehicleID] = &Session{
		VehicleID: req.VehicleID,
		GatewayID: req.GatewayID,
		CreatedAt: time.Now(),
		TTL:       5 * time.Minute,
	}
	s.sessionMu.Unlock()

	resp := map[string]any{
		"vehicle_id": req.VehicleID,
		// "key_hex":    "",
		"ttl": 300,
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	slog.Info(
		"registered vehicle",
		"vehicle", req.VehicleID,
		"gateway", req.GatewayID,
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

```

- /internal/core/gateway.go
```go
// Package core implements Gateway: TDMA master, LoRa relay, auto-tuning.
package core

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
)

// Gateway manages LoRa radio, TDMA timing, and reports to Fog.
type Gateway struct {
	Addr       string
	ServerAddr string
	lora       *device.Lora

	keyMu sync.Mutex
	keys  map[string][]byte

	slotDur   time.Duration
	guardMs   int
	slotCount int

	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
	httpSrv    *http.Server
}

// NewGateway creates a gateway instance.
func NewGateway(dev string, baud int, addr, srv string) *Gateway {
	return &Gateway{
		Addr:       addr,
		ServerAddr: srv,
		lora:       device.NewLora(dev, baud),
		keys:       make(map[string][]byte),
		slotDur:    800 * time.Millisecond,
		guardMs:    200,
		slotCount:  8,
	}
}

// Start runs LoRa uplink/downlink and beacon broadcasting.
func (g *Gateway) Start(ctx context.Context) error {
	g.stopCtx, g.stopCancel = context.WithCancel(ctx)

	g.wg.Add(1)
	go g.beaconLoop()
	g.wg.Add(1)
	go g.uplinkLoop()
	// g.wg.Add(1)
	// go g.reportLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/command", g.handleControl)
	g.httpSrv = &http.Server{Addr: g.Addr, Handler: mux}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := g.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway http error", "err", err)
		}
	}()
	return nil
}

// Shutdown stops the gateway and closes LoRa interface.
func (g *Gateway) Shutdown() {
	if g.stopCancel != nil {
		g.stopCancel()
	}

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn("failed to close device",
				"component", "gateway", "addr", g.Addr, "error", err)
		}
	}

	if g.httpSrv != nil {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			2*time.Second,
		)
		defer cancel()
		if err := g.httpSrv.Shutdown(ctx); err != nil {
			slog.Warn("gateway HTTP server shutdown error",
				"component", "gateway", "addr", g.Addr, "error", err)
		}
	}

	g.wg.Wait()
	slog.Info("gateway stopped", "addr", g.Addr)
}

// beaconLoop periodically sends TDMA sync beacons.
func (g *Gateway) beaconLoop() {
	defer g.wg.Done()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-g.stopCtx.Done():
			return
		case <-t.C:
			beacon := map[string]any{
				"type": "beacon", "gw": g.Addr,
				"slot":  g.slotDur.Milliseconds(),
				"guard": g.guardMs,
			}
			b, _ := json.Marshal(beacon)
			frame, _ := model.BuildPlainFrame(
				model.TypeBeacon,
				0,
				make([]byte, model.NonceLength),
				b,
			)
			_ = g.lora.WriteBytes(frame)
			slog.Debug(
				"beacon sent",
				"gw", g.Addr,
			)
		}
	}
}

// uplinkLoop reads frames from LoRa and forwards telemetry.
func (g *Gateway) uplinkLoop() {
	defer g.wg.Done()
	for {
		select {
		case <-g.stopCtx.Done():
			return
		default:
		}

		// ReadFrames returns zero or more complete frames within timeout.
		frames, err := g.lora.ReadFrames(5 * time.Second)
		if err != nil {
			continue
		}
		if len(frames) == 0 {
			// nothing read this cycle
			continue
		}

		// Try secure parse using known keys
		g.keyMu.Lock()
		keys := make(map[string][]byte, len(g.keys))
		maps.Copy(keys, g.keys)
		g.keyMu.Unlock()

		// Process each extracted frame
		for _, frame := range frames {
			handled := false

			// 1) Try to decrypt/verify with each known key (fast path for secure telemetry)
			for vid, key := range keys {
				typ, _, _, payload, err := model.ParseFrame(frame, key)
				if err != nil {
					// decryption/verification failed with this key, try next
					continue
				}
				// parsed OK with this key
				if typ == model.TypeTelemetry {
					go g.postTelemetry(vid, payload)
				}
				handled = true
				break // don't try other keys for this frame
			}
			if handled {
				continue // next frame
			}

			// 2) Fallback: try plain (no key) parse
			typ, _, _, payload, err := model.ParseFrame(frame, nil)
			if err != nil {
				// cannot parse frame even as plain -> drop and continue
				slog.Warn(
					"unable to parse frame",
					"gw", g.Addr, "err", err,
				)
				continue
			}
			if typ == model.TypeTelemetry {
				// For plain telemetry we don't know vehicle id: post as generic telemetry.
				// If your plain payload contains vehicle id, modify postTelemetry to accept it.
				go g.postTelemetry("", payload)
			} else {
				// other plain types (hello/beacon/auth) may be ignored here or handled if needed
				slog.Debug(
					"received non-telemetry plain frame",
					"gw", g.Addr, "type", typ,
				)
			}
		}
	}
}

// postTelemetry forwards telemetry to Fog server.
func (g *Gateway) postTelemetry(vid string, data []byte) {
	t, err := model.ParseTelemetryPacked(data)
	if err != nil {
		slog.Warn("failed to decode telemetry", "gw", g.Addr, "err", err)
		return
	}

	telemetry := model.TelemetryData{
		VehicleID:   vid,
		Latitude:    float64(t.LatI32) / 1e7,
		Longitude:   float64(t.LonI32) / 1e7,
		CurrentHead: t.CurHeadU16,
		TargetHead:  t.TarHeadU16,
		LeftSpeed:   t.LSpeedU16,
		RightSpeed:  t.RSpeedU16,
	}

	b, _ := json.Marshal(telemetry)
	resp, err := http.Post(
		"http://"+g.ServerAddr+"/api/telemetry",
		"application/json",
		bytes.NewReader(b),
	)
	if err != nil {
		slog.Warn("failed to send telemetry",
			"component", "gateway", "id", g.Addr, "error", err)
		return
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// postRegisterToServer calls Fog /api/register and processes response.
// If server returns a key_hex, register the key for this vehicle locally.
func (g *Gateway) postRegisterToServer(vehicleHWID string) {
	reqBody := map[string]any{
		"gateway_id": g.Addr,
		"vehicle_id": vehicleHWID,
	}
	b, _ := json.Marshal(reqBody)
	resp, err := http.Post("http://"+g.ServerAddr+"/api/register", "application/json", bytes.NewReader(b))
	if err != nil {
		slog.Warn("register request to server failed", "gw", g.Addr, "vehicle", vehicleHWID, "err", err)
		return
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}

	// read response
	var r map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		slog.Warn("invalid register response", "gw", g.Addr, "vehicle", vehicleHWID, "err", err)
		return
	}

	// Optional key provision
	if kh, ok := r["key_hex"].(string); ok && kh != "" {
		if err := g.RegisterKey(vehicleHWID, kh); err != nil {
			slog.Warn("failed to register key", "gw", g.Addr, "vehicle", vehicleHWID, "err", err)
		} else {
			slog.Info("registered key for vehicle", "gw", g.Addr, "vehicle", vehicleHWID)
		}
	}

	// optional: store mapping to forward quickly (vehicle->gatewayURL)
	gURL := "http://" + g.Addr // gateway listens on g.Addr
	// In production you probably want to inform Fog about this mapping or use server DB.
	// store locally? (not necessary)
	_ = gURL
}

// handleControl handles HTTP control from Fog -> Vehicle.
func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close control request body",
					"component", "gateway", "id", g.Addr, "error", err)
			}
		}
	}()
	var ctrl map[string]any
	if err := json.NewDecoder(r.Body).Decode(&ctrl); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	vid, _ := ctrl["vehicle_id"].(string)
	if vid == "" {
		http.Error(w, "vehicle_id missing", http.StatusBadRequest)
		return
	}
	b, _ := json.Marshal(ctrl)
	nonce := make([]byte, model.NonceLength)
	g.keyMu.Lock()
	key := g.keys[vid]
	g.keyMu.Unlock()

	var frame []byte
	var err error
	if len(key) == 16 {
		frame, err = model.BuildSecureFrame(model.TypeControl, 0, nonce, b, key)
	} else {
		frame, err = model.BuildPlainFrame(model.TypeControl, 0, nonce, b)
	}
	if err == nil {
		_ = g.lora.WriteBytes(frame)
	}
	w.WriteHeader(http.StatusAccepted)
}

// reportLoop posts periodic stats and performs simple auto-tuning.
func (g *Gateway) reportLoop() {
	defer g.wg.Done()
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-g.stopCtx.Done():
			return
		case <-t.C:
			body := map[string]any{
				"gw_id":     g.Addr,
				"slotUsage": g.slotCount,
				"avgDelay":  80,
				"collision": 0,
			}
			payload, _ := json.Marshal(body)
			go func() {
				resp, err := http.Post(
					"http://"+g.ServerAddr+"/api/gw/report",
					"application/json",
					bytes.NewReader(payload),
				)
				if err != nil {
					slog.Warn(
						"failed to send report",
						"component", "gateway",
						"addr", g.Addr,
						"error", err,
					)
					return
				}
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
			}()

			// Simple auto-scaling: decrease slotCount if collisions high, increase if low usage
			// NOTE: demo heuristic — tune for your environment.
			if g.slotCount > 4 && body["collision"].(int) > 5 {
				g.slotCount -= 2
				slog.Info("autoscale: reduce slotCount due high collision", "gw", g.Addr, "slotCount", g.slotCount)
			} else if g.slotCount < 32 && body["slotUsage"].(int) < g.slotCount/2 {
				g.slotCount += 1
				slog.Info("autoscale: increase slotCount due low usage", "gw", g.Addr, "slotCount", g.slotCount)
			}
			// adjust slot duration proportionally (demo)
			g.slotDur = time.Duration(800+(g.slotCount/4)*100) * time.Millisecond
			// simple tuning logic
			if g.guardMs < 1000 {
				g.guardMs += 20
			}
		}
	}
}

// RegisterKey stores a 16-byte key for a vehicle.
func (g *Gateway) RegisterKey(vid string, hexKey string) error {
	k, err := hex.DecodeString(hexKey)
	if err != nil {
		return err
	}
	if len(k) != 16 {
		return fmt.Errorf("invalid key length")
	}
	g.keyMu.Lock()
	g.keys[vid] = k
	g.keyMu.Unlock()
	return nil
}

```
