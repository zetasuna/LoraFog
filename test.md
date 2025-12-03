
- /internal/core/system.go
```go
// Package core orchestrates system startup and shutdown.
package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"LoraFog/internal/database"
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
	serverDB     *database.ServerDB
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
		if err := sys.socatManager.CreatePair(
			vs.Left,
			vs.Right,
		); err != nil {
			slog.Error("[System] Failed to create socat pair", "left", vs.Left, "right", vs.Right, "error", err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	if cfg.Server.Address != "" {
		// 1. Mở kết nối DB ở đây (trong main)
		dsn := "admin:admin@tcp(localhost:3306)/boat_db"
		sys.serverDB, err = database.NewServerDB(dsn, 10, 5)
		if err != nil {
			slog.Error("[System] Database established fail", "error", err)
		} else {
			slog.Info("[System] Database established success")
		}
		sys.server = NewServer(
			cfg.Server.Address,
			cfg.Server.AppAddress,
			sys.serverDB,
		)
	}
	for _, g := range cfg.Gateways {
		sys.gateways = append(sys.gateways, NewGateway(
			g.Address,
			g.ServerAddress,
			g.LoraDevice,
			g.LoraBaud,
		))
	}
	for _, v := range cfg.Vehicles {
		sys.vehicles = append(sys.vehicles, NewVehicle(
			v.VehicleID,
			v.LoraDevice,
			v.LoraBaud,
			v.ArduinoDevice,
			v.ArduinoBaud,
		))
	}
	for _, a := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos, device.NewArduino(a.Device, a.Baud))
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

// Stop gracefully stops all components.
func (s *System) Stop() {
	slog.Info("[System] Stopping...")
	if s.cancel != nil {
		s.cancel()
	}
	for _, gw := range s.gateways {
		gw.Stop()
	}
	for _, vh := range s.vehicles {
		vh.Stop()
	}
	for _, ino := range s.arduinos {
		_ = ino.Close()
	}
	// if s.server != nil {
	// 	_ = s.server.Stop()
	// }
	if s.serverDB != nil {
		_ = s.serverDB.Close()
	}
	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}
	s.wg.Wait()
	slog.Info("[System] Shutdown complete")
}

```

- /internal/core/server.go
```go
// Package core implements the Fog server — registry, WebSocket, telemetry & control APIs.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"LoraFog/internal/database"
	"LoraFog/internal/model"
	// "github.com/gorilla/websocket"
)

// var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// Session đại diện cho một kết nối Vehicle đang hoạt động
type Session struct {
	VehicleID      string
	GatewayAddress string
	CreatedAt      time.Time
	TTL            time.Duration // Thời gian sống tối đa không nhận Telemetry
	Slot           int           // Slot TDMA được cấp phát
}

// Server là lõi quản lý của hệ thống Fog
type Server struct {
	Address    string
	AppAddress string
	database   *database.ServerDB // Giả lập Database
	httpClient *http.Client
	// httpServer *http.Server
	// mutexWS    sync.Mutex
	// clients    map[*websocket.Conn]bool

	mutex        sync.Mutex
	sessions     map[string]*Session     // Map: VehicleID -> Session
	gatewaySlots map[string]map[int]bool // Map: GwID -> (SlotIndex -> Used)
	// Map này giúp Server quản lý và cấp phát slot cho từng Gateway
}

// NewServer tạo một Server mới
func NewServer(
	address string,
	appAddress string,
	serverDB *database.ServerDB,
) *Server {
	return &Server{
		Address:      address,
		AppAddress:   appAddress,
		database:     serverDB,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		sessions:     make(map[string]*Session),
		gatewaySlots: make(map[string]map[int]bool),
	}
}

// Start khởi động Server và các tiến trình
func (s *Server) Start(ctx context.Context) error {
	// Khởi động Goroutine quét Session (TTL)
	go s.sweeper(ctx)

	// Khởi động HTTP Server
	mux := http.NewServeMux()
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/telemetry", s.handleTelemetry)
	mux.HandleFunc("/command", s.handleControl)
	// mux.HandleFunc("/ws", s.handleWS)

	server := &http.Server{Addr: s.Address, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("[Server] HTTP failed to start", "error", err)
		}
	}()
	slog.Info("[Server] Started", "address", s.Address)

	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctxShutdown)
	slog.Info("[Server] Stopped")
	return nil
}

// handleRegister: Xử lý yêu cầu đăng ký mới từ Gateway (Hello Packet)
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	slog.Info(
		"[Server] Received REGISTER",
		"gateway", req.GatewayAddress,
		"vehicle", req.VehicleID,
	)

	if req.VehicleID == "" || req.GatewayAddress == "" {
		http.Error(w,
			"vehicle and gateway required",
			http.StatusBadRequest,
		)
		return
	}

	// 1. Logic Roaming/Trùng Session
	var oldGw string
	var oldSlot int
	s.mutex.Lock()
	if oldSes, ok := s.sessions[req.VehicleID]; ok {
		oldGw = oldSes.GatewayAddress
		oldSlot = oldSes.Slot

		if oldGw == req.GatewayAddress {
			slog.Info("[Server] Vehicle wanted to register already had had session => Nothing change",
				"gateway", req.GatewayAddress, "vehicle", req.VehicleID)
			s.mutex.Unlock()
			return
		}

		delete(s.sessions, req.VehicleID)
		s.releaseSlot(oldGw, oldSlot)
	}
	s.mutex.Unlock()

	// 2. Nếu có session cũ → unregister DB + push update
	if oldGw != "" {
		// Update DB to clear gateway for vehicle (safeguard)
		_ = s.database.UpdateVehicleGateway(
			context.Background(),
			req.VehicleID,
			"",
		)
		s.pushSlotUpdateToGateway(oldGw)
		slog.Info(
			"[Server] Roaming: Cleared old session",
			"vehicle", req.VehicleID,
			"old_gw", oldGw,
		)
	}

	// 2. Cấp Slot mới tại Gateway mới
	s.mutex.Lock()
	newSlot := s.assignNewSlot(req.GatewayAddress)
	if newSlot == -1 {
		s.mutex.Unlock()
		http.Error(w, "No slot available", http.StatusServiceUnavailable)
		return
	}

	// 3. Tạo Session mới
	newSession := &Session{
		VehicleID:      req.VehicleID,
		GatewayAddress: req.GatewayAddress,
		CreatedAt:      time.Now(),
		TTL:            60 * time.Second, // Mặc định 60s TTL
		Slot:           newSlot,
	}
	s.sessions[req.VehicleID] = newSession
	s.mutex.Unlock()
	slog.Info("[Server] Created new session",
		"gateway", req.GatewayAddress,
		"vehicle", req.VehicleID,
		"slot", newSlot,
	)

	// 4. Cập nhật DB (Boat -> GatewayID)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()
	if err := s.database.UpdateVehicleGateway(
		ctx, req.VehicleID, req.GatewayAddress,
	); err != nil {
		slog.Error(
			"[Server] Failed to update DB during register",
			"vehicle", req.VehicleID, "error", err,
		)
		// try to rollback session
		s.mutex.Lock()
		delete(s.sessions, req.VehicleID)
		s.releaseSlot(req.GatewayAddress, newSlot)
		s.mutex.Unlock()
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// 5. Quan trọng: Push danh sách Slot mới xuống Gateway
	go s.pushSlotUpdateToGateway(req.GatewayAddress)

	// w.WriteHeader(http.StatusOK)
	// Respond with slot details
	resp := model.RegisterResponse{
		Slot:           newSlot,
		GatewayAddress: req.GatewayAddress,
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(&resp); err != nil {
		slog.Error("[Server] Failed to write register response", "err", err)
	}
}

// handleTelemetry: Xử lý dữ liệu Telemetry và Reset TTL
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var telem model.VehicleData
	if err := json.NewDecoder(r.Body).Decode(&telem); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	slog.Info(
		"[Server] Received TELEMETRY",
		"vehicle", telem.VehicleID,
		"lat", telem.Latitude,
		"lon", telem.Longitude,
	)

	s.mutex.Lock()
	if ses, ok := s.sessions[telem.VehicleID]; ok {
		ses.CreatedAt = time.Now() // Reset TTL
		slog.Info("[Server] Reset TTL", "vehicle", telem.VehicleID)
		// forward to app server (if configured) - safe call with timeout
		if s.AppAddress != "" {
			go func(t model.VehicleData) {
				ctx, cancel := context.WithTimeout(
					context.Background(),
					3*time.Second,
				)
				defer cancel()
				vehicleApp := model.VehicleApp{
					VehicleID:   t.VehicleID,
					Latitude:    t.Latitude,
					Longitude:   t.Longitude,
					CurrentHead: t.CurrentHead,
					TargetHead:  t.TargetHead,
					LeftSpeed:   t.LeftSpeed,
					RightSpeed:  t.RightSpeed,
				}
				body, _ := json.Marshal(vehicleApp)
				req, err := http.NewRequestWithContext(ctx,
					"POST",
					"http://"+s.AppAddress+"/api/telemetry",
					bytes.NewReader(body),
				)
				if err != nil {
					slog.Warn("[Server] Failed to build forward request", "err", err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := s.httpClient.Do(req)
				if err != nil {
					slog.Warn("[Server] Failed to forward telemetry", "err", err)
					return
				}
				_ = resp.Body.Close()
				// s.broadcast(string(body))
				// slog.Info("broadcast to websocket")
			}(telem)
		}
	} else {
		// session unknown: log and drop
		slog.Warn("[Server] Received TELEMETRY for unknown session", "vehicle", telem.VehicleID)
	}
	s.mutex.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var controlApp model.ControlApp
	if err := json.NewDecoder(r.Body).Decode(&controlApp); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	slog.Info("[Server] Received CONTROL")

	if controlApp.VehicleID == "" {
		http.Error(w,
			"vehicle id required",
			http.StatusBadRequest,
		)
		return
	}

	gatewayID, _ := s.database.GetVehicleGateway(context.Background(), controlApp.VehicleID)
	if gatewayID == "" {
		slog.Error("[Server] Control cannot reach because vehicle not belong to any gateway")
		return
	}

	controlData := model.ControlData{
		Type:      model.PacketControl,
		VehicleID: controlApp.VehicleID,
		Speed:     controlApp.Speed,
		Latitude:  controlApp.Latitude,
		Longitude: controlApp.Longitude,
		Kp:        controlApp.Kp,
		Ki:        controlApp.Ki,
		Kd:        controlApp.Kd,
	}
	go func(c model.ControlData) {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			3*time.Second,
		)
		defer cancel()
		body, _ := json.Marshal(c)
		req, err := http.NewRequestWithContext(ctx,
			"POST",
			"http://"+gatewayID+"/control",
			bytes.NewReader(body),
		)
		if err != nil {
			slog.Warn("[Server] Failed to build forward request", "err", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			slog.Warn("[Server] Failed to forward control", "err", err)
			return
		}
		_ = resp.Body.Close()
	}(controlData)
}

// sweeper: Quét các Session đã hết hạn (TTL Expired)
func (s *Server) sweeper(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.mutex.Lock()
			now := time.Now()
			dirtyGateways := make(map[string]bool)

			for vID, ses := range s.sessions {
				if now.Sub(ses.CreatedAt) > ses.TTL {
					slog.Info(
						"[Server] Session expired (TTL)",
						"gateway", ses.GatewayAddress, "vehicle", vID,
					)

					// 1. Cập nhật DB -> NULL/Empty
					// Update DB -> NULL/Empty
					go func(vehicle string) {
						_ = s.database.UpdateVehicleGateway(context.Background(), vehicle, "")
					}(vID)

					// 2. Release Slot và đánh dấu Gateway cần cập nhật
					s.releaseSlot(ses.GatewayAddress, ses.Slot)
					dirtyGateways[ses.GatewayAddress] = true

					// 3. Xóa session
					delete(s.sessions, vID)
				}
			}
			s.mutex.Unlock()

			// 4. Báo cho các Gateway có thay đổi session để cập nhật Beacon
			for gwID := range dirtyGateways {
				go s.pushSlotUpdateToGateway(gwID)
			}
		case <-ctx.Done():
			return
		}
	}
}

// assignNewSlot: Tìm và cấp phát slot mới cho Gateway (trong lock)
func (s *Server) assignNewSlot(gwAddr string) int {
	if _, ok := s.gatewaySlots[gwAddr]; !ok {
		s.gatewaySlots[gwAddr] = make(map[int]bool)
	}

	// Bắt đầu tìm từ slot 1, tối đa 20 slot
	for i := 1; i <= 20; i++ {
		if !s.gatewaySlots[gwAddr][i] {
			s.gatewaySlots[gwAddr][i] = true
			return i
		}
	}
	slog.Error("[Server] Max slots reached for gateway", "gateway", gwAddr)
	return -1 // Không thể cấp slot
}

// releaseSlot: Giải phóng slot (trong lock)
func (s *Server) releaseSlot(gwID string, slot int) {
	if slots, ok := s.gatewaySlots[gwID]; ok {
		delete(slots, slot)
	}
}

// pushSlotUpdateToGateway: Gửi danh sách Slot Map đầy đủ xuống Gateway
func (s *Server) pushSlotUpdateToGateway(gwAddr string) {
	s.mutex.Lock()
	// Tạo SlotMap chỉ chứa các vehicle thuộc Gateway này
	slotMap := make(map[string]int)
	for _, ses := range s.sessions {
		if ses.GatewayAddress == gwAddr {
			slotMap[ses.VehicleID] = ses.Slot
		}
	}
	s.mutex.Unlock()

	// Gửi POST xuống Gateway /update_beacon
	body, _ := json.Marshal(slotMap)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	// Lưu ý: Giả định gwAddr là địa chỉ HTTP (ví dụ: localhost:8081)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+gwAddr+"/update_beacon",
		bytes.NewReader(body),
	)
	if err != nil {
		slog.Error(
			"[Server] Failed to build request for gateway",
			"gateway", gwAddr, "err", err,
		)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	slog.Info("[Server] Pushing slot map to Gateway", "gateway", gwAddr, "count", len(slotMap))
	resp, err := s.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"[Server] Failed to push slot update to Gateway",
			"gateway", gwAddr, "error", err,
		)
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn("[Server] Gateway rejected slot update", "gateway", gwAddr, "status", resp.Status)
	} else {
		slog.Debug("[Server] Successfully pushed slot map to Gateway", "gateway", gwAddr, "count", len(slotMap))
	}
	// Ensure body closed
	// _ = resp.Body.Close()
	defer func() { _ = resp.Body.Close() }()
}

// // handleWS upgrades HTTP to websocket and registers the client for broadcasts.
// func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
// 	conn, err := upgrader.Upgrade(w, r, nil)
// 	if err != nil {
// 		return
// 	}
// 	s.mutexWS.Lock()
// 	s.clients[conn] = true
// 	s.mutexWS.Unlock()
//
// 	go func() {
// 		defer func() {
// 			s.mutexWS.Lock()
// 			delete(s.clients, conn)
// 			s.mutexWS.Unlock()
// 			if err := conn.Close(); err != nil {
// 				slog.Error("Failed to close websocket", "error", err)
// 			}
// 		}()
// 		for {
// 			if _, _, err := conn.ReadMessage(); err != nil {
// 				break
// 			}
// 		}
// 	}()
// }
//
// // broadcast sends a message to all connected websocket clients.
// func (s *Server) broadcast(msg string) {
// 	s.mutexWS.Lock()
// 	defer s.mutexWS.Unlock()
// 	for c := range s.clients {
// 		_ = c.WriteMessage(websocket.TextMessage, []byte(msg))
// 	}
// }

```

- /internal/core/vehicle.go
```go
// Package core implements the Vehicle agent
package core

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"

	"github.com/fxamacker/cbor/v2"
)

// VehicleState định nghĩa trạng thái của xe
type VehicleState int

const (
	StateIdle    VehicleState = 0 // Chờ Beacon, chưa có slot
	StateJoining VehicleState = 1 // Đã gửi Hello, chờ Beacon tiếp theo để confirm slot
	StateSending VehicleState = 2 // Đã có slot, gửi Telemetry định kỳ
)

// Vehicle là đại diện cho thiết bị thuyền/xe
type Vehicle struct {
	ID      string
	lora    *device.Lora
	arduino *device.Arduino // Có thể nil nếu dùng dữ liệu giả lập

	state          VehicleState
	currentGateway string
	assignedSlot   int

	// Cache dữ liệu telemetry mới nhất từ Arduino
	mutexTelemetry sync.Mutex
	lastTelemetry  model.ArduinoData

	mutexOffset sync.Mutex
	bestOffset  int64

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewVehicle tạo một Vehicle mới
func NewVehicle(
	id string,
	loraDev string, loraBaud int,
	arduinoDev string, arduinoBaud int,
) *Vehicle {
	lora := device.NewLora(loraDev, loraBaud)
	v := &Vehicle{
		ID:           id,
		lora:         lora,
		state:        StateIdle,
		assignedSlot: -1,
		bestOffset:   math.MaxInt64,
	}
	if arduinoDev != "" {
		v.arduino = device.NewArduino(arduinoDev, arduinoBaud)
	}
	return v
}

func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	slog.Info("[Vehicle] Started",
		"vehicle", v.ID)

	// 1. Goroutine đọc Arduino liên tục để update lastTelem
	if v.arduino != nil {
		v.wg.Add(1)
		go v.arduinoLoop(ctx)
	}

	// 2. Goroutine chính: LoRa Loop (State Machine)
	v.wg.Add(1)
	go v.loraLoop(ctx)

	return nil
}

func (v *Vehicle) Stop() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.lora != nil {
		if err := v.lora.Close(); err != nil {
			slog.Warn("[Vehicle] Failed to close LoRa device",
				"vehicle", v.ID, "error", err)
		}
	}
	if v.arduino != nil {
		if err := v.arduino.Close(); err != nil {
			slog.Warn("[Vehicle] Failed to close Arduino device",
				"vehicle", v.ID, "error", err)
		}
	}
	v.wg.Wait()
	slog.Info("[Vehicle] Stopped",
		"vehicle", v.ID)
}

// arduinoLoop đọc dữ liệu từ Arduino/Simulator
func (v *Vehicle) arduinoLoop(ctx context.Context) {
	defer v.wg.Done()
	dataCh := make(chan model.ArduinoData, 5)
	stop, err := v.arduino.Read(dataCh)
	if err != nil {
		slog.Warn("[Vehicle] Failed to read Arduino",
			"vehicle", v.ID, "err", err)
		return
	}
	defer stop()

	// Khởi tạo data giả nếu arduino nil
	if v.arduino == nil {
		v.mutexTelemetry.Lock()
		v.lastTelemetry = model.ArduinoData{
			Latitude:    21.0532 + rand.Float64()*0.001,
			Longitude:   105.8261 + rand.Float64()*0.001,
			CurrentHead: 0,    // + rand.Int63n(361),
			TargetHead:  0,    // + rand.Int63n(361),
			LeftSpeed:   1000, // + rand.Int63n(1000),
			RightSpeed:  1000, // + rand.Int63n(1000),
		}
		v.mutexTelemetry.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-dataCh:
			if !ok {
				return
			}
			v.mutexTelemetry.Lock()
			v.lastTelemetry = data
			v.mutexTelemetry.Unlock()
			// slog.Info("Vehicle received arduino data",
			// 	"vehicle", v.ID,
			// 	"lat", data.Latitude,
			// 	"lon", data.Longitude,
			// 	"curHead", data.CurrentHead,
			// 	"tarHead", data.TargetHead,
			// 	"leftSpeed", data.LeftSpeed,
			// 	"rightSpeed", data.RightSpeed,
			// )
		}
	}
}

// loraLoop là State Machine chính của Vehicle
func (v *Vehicle) loraLoop(ctx context.Context) {
	defer v.wg.Done()

	// Sử dụng giá trị mặc định cho timeout chờ beacon (ví dụ: 5 giây)
	var lastBeaconTime time.Time
	beaconTimeout := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		frame, err := v.lora.Read(beaconTimeout)
		if err != nil {
			// Nếu timeout hoặc lỗi sau khi đã từng có session -> Reset về IDLE
			if err == device.ErrSerialTimeout {
				// if we had a previous beacon and too long passed -> reset
				if !lastBeaconTime.IsZero() && time.Since(lastBeaconTime) > 2*beaconTimeout {
					if v.state != StateIdle {
						slog.Warn("[Vehicle] Lost beacon connection => State: IDLE",
							"vehicle", v.ID, "state", v.state)
					}
					v.state = StateIdle
					v.currentGateway = ""
					v.assignedSlot = -1
				}
				continue
			}
			// Nếu đã Idle thì cứ tiếp tục lắng nghe
			slog.Error("[Vehicle] Failed to read Lora",
				"vehicle", v.ID, "state", v.state, "err", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Phân loại gói tin
		var generic map[string]any
		if err := cbor.Unmarshal(frame, &generic); err != nil {
			slog.Warn("[Vehicle] Received unknown or corrupted CBOR packet",
				"vehicle", v.ID, "state", v.state, "err", err)
			continue
		}

		msgType, ok := generic["type"].(string)
		if !ok {
			slog.Warn("[Vehicle] Packet type missing",
				"vehicle", v.ID, "state", v.state)
			continue
		}

		switch msgType {
		case model.PacketBeacon:
			var beacon model.BeaconMessage
			if err := cbor.Unmarshal(frame, &beacon); err != nil {
				// Có thể là packet Control hoặc nhiễu
				slog.Info("[Vehicle] Received non-beacon frame or corrupted beacon",
					"vehicle", v.ID, "state", v.state, "error", err)
				continue
			}
			slog.Info("[Vehicle] Received BEACON",
				"vehicle", v.ID, "state", v.state, "source", beacon.GatewayAddress)

			// record last beacon time and timestamp (ms)
			lastBeaconTime = time.Now()
			v.handleBeacon(ctx, beacon)
		case model.PacketControl:
			if v.state == StateSending {
				var control model.ControlData
				if err := cbor.Unmarshal(frame, &control); err != nil {
					// Có thể là packet Control hoặc nhiễu
					slog.Info("[Vehicle] Received non-control frame or corrupted control",
						"vehicle", v.ID, "state", v.state, "error", err)
					continue
				}
				slog.Info("[Vehicle] Received CONTROL",
					"vehicle", v.ID, "state", v.state)
				arduinoControl := fmt.Sprintf("%d,%.6f,%.6f,%.6f,%.6f,%.6f",
					control.Speed, control.Latitude, control.Longitude,
					control.Kp, control.Ki, control.Kd)
				// Forward control data to Arduino
				if err := v.arduino.Write(arduinoControl); err != nil {
					slog.Error("[Vehicle] Failed to forward control to Arduino",
						"vehicle", v.ID, "state", v.state, "error", err)
				} else {
					slog.Info("[Vehicle] Forwarded CONTROL to Arduino",
						"vehicle", v.ID, "state", v.state)
				}
			}
		default:
			slog.Info("[Vehicle] Received unhandled message type",
				"vehicle", v.ID, "state", v.state, "type", msgType)
		}
	}
}

func (v *Vehicle) handleBeacon(ctx context.Context, b model.BeaconMessage) {
	newOffset := (time.Now().UnixNano() - b.CycleStart.UnixNano()) / int64(time.Millisecond)
	v.mutexOffset.Lock()
	v.bestOffset = min(v.bestOffset, newOffset)
	localCycleStart := b.CycleStart.Add(time.Duration(v.bestOffset) * time.Millisecond)
	v.mutexOffset.Unlock()

	beaconEnd := localCycleStart.Add(time.Duration(b.BeaconWindowMs) * time.Millisecond)
	controlEnd := beaconEnd.Add(time.Duration(b.ControlWindowMs) * time.Millisecond)

	// Logic Roaming: Nếu gateway ID khác với hiện tại và đã có slot
	if v.currentGateway != "" && v.currentGateway != b.GatewayAddress {
		v.currentGateway = b.GatewayAddress
		v.state = StateIdle // Reset về Idle để đăng ký lại với Gateway mới
		v.assignedSlot = -1
		// Reset trạng thái đồng bộ vì clock gateway mới khác clock cũ
		v.mutexOffset.Lock()
		v.bestOffset = math.MaxInt64 // Reset bộ lọc min-jitter
		v.mutexOffset.Unlock()
		slog.Info("[Vehicle] Roaming detected => Preparing to switch",
			"vehicle", v.ID, "state", v.state, "old", v.currentGateway, "new", b.GatewayAddress)

		// sleep until register window
		v.sleepUntil(ctx, controlEnd)
		// Re-check: maybe beacon or another goroutine assigned slot
		if v.assignedSlot != -1 {
			// already have slot -> go to sending
			v.state = StateSending
			slog.Info(
				"[Vehicle] Already assigned slot while waiting",
				"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot,
			)
			go v.performTDMA(ctx, b, localCycleStart)
			return
		}

		// Gửi lại Hello
		hello := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(hello)
		if err := v.lora.Write(payload); err != nil {
			slog.Warn("[Vehicle] Failed to send HELLO during roaming",
				"vehicle", v.ID, "state", v.state, "err", err)
		} else {
			v.state = StateJoining
			slog.Info("[Vehicle] Sent HELLO (Roaming)",
				"vehicle", v.ID, "state", v.state)
		}
		return
	}

	v.currentGateway = b.GatewayAddress
	// State vehicle
	switch v.state {
	case StateIdle:
		// sleep until register window
		v.sleepUntil(ctx, controlEnd)
		// Re-check: maybe beacon or another goroutine assigned slot
		if v.assignedSlot != -1 {
			// already have slot -> go to sending
			v.state = StateSending
			slog.Info(
				"[Vehicle] Already assigned slot while waiting",
				"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot,
			)
			go v.performTDMA(ctx, b, localCycleStart)
			return
		}

		// Gửi Hello
		msg := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(msg)
		if err := v.lora.Write(payload); err != nil {
			slog.Warn("[Vehicle] Failed to write HELLO",
				"vehicle", v.ID, "state", v.state, "err", err)
		} else {
			v.state = StateJoining
			slog.Info("[Vehicle] Sent HELLO",
				"vehicle", v.ID, "state", v.state)
		}

	case StateJoining:
		// Trạng thái CHUẨN BỊ GỬI: Kiểm tra xem trong Beacon mới có Slot cho mình chưa
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot
			v.state = StateSending
			slog.Info("[Vehicle] Joined successfully",
				"vehicle", v.ID, "state", v.state, "slot", slot)
			go v.performTDMA(ctx, b, localCycleStart)
		} else {
			// Chưa thấy tên mình, gói Hello có thể bị mất. Gửi lại Hello ở cuối chu kỳ này
			slog.Warn("[Vehicle] Waiting for slot assignment...",
				"vehicle", v.ID, "state", v.state)

			// sleep to send HELLO (retry)
			v.sleepUntil(ctx, controlEnd)
			if v.assignedSlot != -1 {
				v.state = StateSending
				slog.Info("[Vehicle] Slot assigned during wait => Skip resend",
					"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot)
				// start TDMA for this cycle if possible
				go v.performTDMA(ctx, b, localCycleStart)
				return
			}

			msg := model.HelloMessage{
				Type:      model.PacketHello,
				VehicleID: v.ID,
			}
			payload, _ := cbor.Marshal(msg)
			if err := v.lora.Write(payload); err != nil {
				slog.Warn("[Vehicle] Failed to write HELLO (retry)",
					"vehicle", v.ID, "state", v.state, "err", err)
			} else {
				slog.Info("[Vehicle] Sent HELLO (Retry)",
					"vehicle", v.ID, "state", v.state)
			}
			// State vẫn là Joining
		}

	case StateSending:
		// Trạng thái GỬI: Kiểm tra lại SlotMap xem còn được cấp phép không
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot // Cập nhật slot nếu Gateway thay đổi
			go v.performTDMA(ctx, b, localCycleStart)
		} else {
			v.state = StateIdle
			v.assignedSlot = -1
			slog.Warn("[Vehicle] Lost slot allocation",
				"vehicle", v.ID, "state", v.state)
		}
	}
}

// performTDMA now accepts ctx and uses sleepUntilSlot to schedule transmission
func (v *Vehicle) performTDMA(ctx context.Context, b model.BeaconMessage, localCycleStart time.Time) {
	if v.assignedSlot < 1 {
		slog.Warn("[Vehicle] Invalid slot => Skipping TDMA",
			"vehicle", v.ID, "state", v.state, "slot", v.assignedSlot)
		return
	}

	slotIndex := v.assignedSlot - 1
	slotStart := localCycleStart.Add(time.Duration(b.BeaconWindowMs+b.ControlWindowMs+b.RegisterWindowMs+b.SlotWindowMs*int64(slotIndex)) * time.Millisecond)
	v.sleepUntil(ctx, slotStart)

	// Re-check assigned slot hasn't changed
	if v.assignedSlot-1 != slotIndex {
		slog.Info("[Vehicle] Assigned slot changed before transmit => Skipping",
			"vehicle", v.ID, "state", v.state,
			"slotIndex", slotIndex,
			"currentSlot", v.assignedSlot,
		)
		return
	}

	// Lấy dữ liệu mới nhất
	v.mutexTelemetry.Lock()
	data := v.lastTelemetry
	v.mutexTelemetry.Unlock()

	// Đóng gói
	pkt := model.VehicleData{
		Type:        model.PacketTelemetry,
		VehicleID:   v.ID,
		Latitude:    data.Latitude,
		Longitude:   data.Longitude,
		CurrentHead: data.CurrentHead,
		TargetHead:  data.TargetHead,
		LeftSpeed:   data.LeftSpeed,
		RightSpeed:  data.RightSpeed,
	}
	payload, _ := cbor.Marshal(pkt)
	if err := v.lora.Write(payload); err != nil {
		slog.Warn("[Vehicle] Failed to send TELEMETRY", "vehicle", v.ID, "state", v.state, "err", err)
		return
	}
	slog.Info("[Vehicle] Sent TELEMETRY (TDMA)",
		"vehicle", v.ID,
		"state", v.state,
		"slot", v.assignedSlot,
		"lat", data.Latitude,
		"lon", data.Longitude,
	)
}

// Hàm phụ trợ giúp sleep chính xác và hỗ trợ cancel context
func (v *Vehicle) sleepUntil(ctx context.Context, target time.Time) {
	d := time.Until(target)
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}

```

- /internal/core/gateway.go
```go
// Package core defines the Gateway component responsible for
// bridging LoRa-connected vehicles with the FogServer
// using CBOR (for LoRa) and JSON (for HTTP).
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"

	"github.com/fxamacker/cbor/v2"
)

const (
	// Thông số TDMA cố định
	LoraDuration     = int64(1000)
	SlotWindowMs     = LoraDuration
	BeaconWindowMs   = LoraDuration
	ControlWindowMs  = LoraDuration * 2
	RegisterWindowMs = LoraDuration * 2
)

// Gateway là đại diện cho thiết bị Gateway LoRaWAN
type Gateway struct {
	Address       string // Địa chỉ HTTP/TCP của Gateway (dùng làm ID)
	ServerAddress string // Địa chỉ HTTP của Fog Server
	lora          *device.Lora
	httpClient    *http.Client

	slotMutex    sync.Mutex
	currentSlots map[string]int // Map VehicleID -> SlotIndex. Cập nhật từ Server.

	controlQueue chan model.ControlData

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewGateway tạo một Gateway mới
func NewGateway(
	address, serverAddress string,
	loraDev string, loraBaud int,
) *Gateway {
	return &Gateway{
		Address:       address,
		ServerAddress: serverAddress,
		lora:          device.NewLora(loraDev, loraBaud),
		httpClient:    &http.Client{Timeout: 5 * time.Second},
		currentSlots:  make(map[string]int),
		controlQueue:  make(chan model.ControlData, 50),
	}
}

// Start khởi động các tiến trình của Gateway
func (g *Gateway) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	// 1. Khởi động HTTP Server để nhận lệnh từ Server (Control & Update Beacon)
	g.wg.Add(1)
	go g.startHTTPServer(ctx)

	// 2. Khởi động vòng lặp Uplink
	g.wg.Add(1)
	go g.uplinkLoop(ctx)

	// 3. Khởi động vòng lặp Downlink
	g.wg.Add(1)
	go g.downlinkLoop(ctx)

	slog.Info("[Gateway] Started", "gateway", g.Address)
	return nil
}

// Stop gracefully stops the gateway and closes resources.
func (g *Gateway) Stop() {
	slog.Info("[Gateway] Stopping", "gateway", g.Address)

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn("[Gateway] Failed to close LoRa device",
				"gateway", g.Address, "error", err)
		}
	}

	if g.cancel != nil {
		g.cancel()
	}

	g.wg.Wait()
	slog.Info("[Gateway] Stopped", "address", g.Address)
}

// startHTTPServer khởi động server HTTP nội bộ để nhận lệnh từ Fog Server
func (g *Gateway) startHTTPServer(ctx context.Context) {
	defer g.wg.Done()
	mux := http.NewServeMux()

	// Endpoint nhận danh sách Slot Map mới từ Server
	mux.HandleFunc("/update_beacon", g.handleBeaconUpdate)
	mux.HandleFunc("/control", g.handleControl)

	server := &http.Server{Addr: g.Address, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("[Gateway] HTTP server error", "gateway", g.Address, "error", err)
		}
	}()

	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		slog.Error("[Gateway] HTTP server shutdown failed", "gateway", g.Address, "error", err)
	} else {
		slog.Info("[Gateway] HTTP server shutdown clean", "gateway", g.Address)
	}
}

// handleBeaconUpdate: Nhận Slot Map mới từ Server (khi có đăng ký/hết hạn session)
func (g *Gateway) handleBeaconUpdate(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var newMap map[string]int
	if err := json.NewDecoder(r.Body).Decode(&newMap); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	g.slotMutex.Lock()
	g.currentSlots = newMap
	g.slotMutex.Unlock()
	slog.Info("[Gateway] Beacon slots updated by Server",
		"gateway", g.Address,
		"count", len(newMap),
	)

	w.WriteHeader(http.StatusOK)
}

func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var controlJSON model.ControlData
	if err := json.NewDecoder(r.Body).Decode(&controlJSON); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Đưa vào queue để gửi ở CONTROL frame
	select {
	case g.controlQueue <- controlJSON:
		slog.Info("[Gateway] Queued CONTROL",
			"gateway", g.Address, "vehicle", controlJSON.VehicleID)
	default:
		slog.Warn("[Gateway] Control queue FULL, dropping control!")
	}

	w.WriteHeader(http.StatusOK)
}

// uplinkLoop: Lắng nghe gói Hello (Register) và Telemetry từ Vehicle
func (g *Gateway) uplinkLoop(ctx context.Context) {
	defer g.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// ReadFrameWithTimeout (50ms để không bị block lâu)
		frame, err := g.lora.Read(50 * time.Millisecond)
		if err != nil {
			if err == device.ErrSerialTimeout {
				continue // Tiếp tục vòng lặp
			}
			slog.Error("[Gateway] Lora read error",
				"gateway", g.Address, "error", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Phân loại gói tin
		var generic map[string]any
		if err := cbor.Unmarshal(frame, &generic); err != nil {
			slog.Warn("[Gateway] Received unknown or corrupted CBOR packet",
				"gateway", g.Address, "err", err)
			continue
		}

		msgType, ok := generic["type"].(string)
		if !ok {
			slog.Warn("[Gateway] Packet type missing", "gateway", g.Address)
			continue
		}

		switch msgType {
		case model.PacketHello:
			var hello model.HelloMessage
			if err := cbor.Unmarshal(frame, &hello); err == nil {
				slog.Info("[Gateway] Received HELLO",
					"gateway", g.Address, "source", hello.VehicleID)
				g.postRegisterToServer(hello.VehicleID)
			}
		case model.PacketTelemetry:
			var telemetry model.VehicleData
			if err := cbor.Unmarshal(frame, &telemetry); err == nil {
				// Lấy lock để đọc currentSlots
				g.slotMutex.Lock()
				_, exists := g.currentSlots[telemetry.VehicleID]
				g.slotMutex.Unlock()

				if !exists {
					// Vehicle không có slot → bỏ qua telemetry
					slog.Warn("[Gateway] Ignored TELEMETRY: vehicle has no active slot",
						"gateway", g.Address, "source", telemetry.VehicleID)
					continue
				}

				// Vehicle hợp lệ → gửi lên Server
				slog.Info("[Gateway] Received TELEMETRY",
					"gateway", g.Address, "source", telemetry.VehicleID)
				g.postTelemetryToServer(telemetry)
			}
		default:
			slog.Info("[Gateway] Received unhandled message type",
				"gateway", g.Address, "type", msgType)
		}
	}
}

// downlinkLoop: TDMA
func (g *Gateway) downlinkLoop(ctx context.Context) {
	defer g.wg.Done()

	// Dùng một biến theo dõi thời điểm bắt đầu dự kiến của chu kỳ tiếp theo
	nextCycleStart := time.Now()

	for {
		if time.Until(nextCycleStart) > 0 {
			g.sleepUntil(ctx, nextCycleStart)
		}

		// Kiểm tra nếu context đã bị hủy trong lúc ngủ thì thoát luôn
		if ctx.Err() != nil {
			return
		}

		// Snapshot số slot hiện tại
		g.slotMutex.Lock()
		slots := make(map[string]int)
		maps.Copy(slots, g.currentSlots)
		slotCount := len(slots)
		g.slotMutex.Unlock()

		cycleStart := time.Now()
		beaconEnd := cycleStart.Add(time.Duration(BeaconWindowMs) * time.Millisecond)
		controlEnd := beaconEnd.Add(time.Duration(ControlWindowMs) * time.Millisecond)
		registerEnd := controlEnd.Add(time.Duration(RegisterWindowMs) * time.Millisecond)
		slotsDuration := time.Duration(slotCount) * time.Duration(SlotWindowMs) * time.Millisecond
		slotsEnd := registerEnd.Add(slotsDuration)

		// Cập nhật thời điểm bắt đầu chu kỳ KẾ TIẾP
		nextCycleStart = slotsEnd

		// === BEACON WINDOW ===
		slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Beacon")
		g.sendBeacon(cycleStart, slots)
		g.sleepUntil(ctx, beaconEnd)

		// === CONTROL WINDOW ===
		slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Control")
		g.processControlQueue(ctx, controlEnd)

		// === REGISTER WINDOW ===
		slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Register")
		// g.listenUntil(ctx, registerEnd, model.PacketHello)
		g.sleepUntil(ctx, registerEnd)

		// === SLOT WINDOWS ===
		slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Slot")
		// g.listenUntil(ctx, slotsEnd, model.PacketTelemetry)
		g.sleepUntil(ctx, slotsEnd)

		// === END CYCLE ===
		slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "End")
	}
}

func (g *Gateway) sendBeacon(cycleStart time.Time, slots map[string]int) {
	beacon := model.BeaconMessage{
		Type:             model.PacketBeacon,
		GatewayAddress:   g.Address,
		CycleStart:       cycleStart,
		BeaconWindowMs:   BeaconWindowMs,
		ControlWindowMs:  ControlWindowMs,
		RegisterWindowMs: RegisterWindowMs,
		SlotWindowMs:     SlotWindowMs,
		SlotMap:          slots,
	}

	payload, err := cbor.Marshal(beacon)
	if err != nil {
		slog.Error("[Gateway] Failed to marshal BEACON",
			"gateway", g.Address, "error", err)
		return
	}

	if err := g.lora.Write(payload); err != nil {
		slog.Error("[Gateway] Failed to send BEACON",
			"gateway", g.Address, "error", err)
	} else {
		slog.Info("[Gateway] Beacon sent",
			"gateway", g.Address,
			"slotCount", len(slots),
		)
	}
}

// Hàm phụ trợ gửi gói Control (đã tách ra từ code cũ cho gọn)
func (g *Gateway) sendControl(ctrl model.ControlData) {
	payload, err := cbor.Marshal(ctrl)
	if err != nil {
		slog.Error("[Gateway] Control marshal error", "err", err)
		return
	}

	if err := g.lora.Write(payload); err != nil {
		slog.Error("[Gateway] Failed to send CONTROL", "vid", ctrl.VehicleID, "err", err)
	} else {
		slog.Info("[Gateway] Sent CONTROL", "vid", ctrl.VehicleID)
	}
}

// Hàm phụ trợ giúp sleep chính xác và hỗ trợ cancel context
func (g *Gateway) sleepUntil(ctx context.Context, target time.Time) {
	d := time.Until(target)
	if d <= 0 {
		return
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return
	case <-timer.C:
		return
	}
}

// processControlQueue: Gửi các lệnh trong hàng đợi cho đến khi hết giờ hoặc hết hàng đợi
func (g *Gateway) processControlQueue(ctx context.Context, deadline time.Time) {
	// 1. Vòng lặp gửi (Sending Loop)
	for {
		// Kiểm tra Context trước (để thoát nhanh nếu shutdown)
		select {
		case <-ctx.Done():
			return
		default:
		}

		// A. Kiểm tra ngân sách thời gian (Time Budget)
		// Nếu thời gian còn lại không đủ để gửi 1 gói tin (LoraDuration) -> Dừng gửi
		timeToSend := time.Duration(LoraDuration) * time.Millisecond
		if time.Now().Add(timeToSend).After(deadline) {
			// slog.Debug("[Gateway] Control window time up")
			break
		}

		// B. Kiểm tra hàng đợi (Non-blocking)
		select {
		case ctrl := <-g.controlQueue:
			// Có tin nhắn -> Gửi ngay
			g.sendControl(ctrl)

			// Tùy chọn: Nếu phần cứng LoRa là bất đồng bộ (Async), bạn có thể cần sleep
			// giả lập thời gian bay (Airtime) để tránh tràn buffer.
			// Nếu g.lora.Write là Blocking, thì không cần dòng dưới.
			// time.Sleep(timeToSend)

		default:
			// Hàng đợi rỗng -> Thoát vòng lặp gửi để chuyển sang trạng thái Ngủ
			goto WAIT_FOR_DEADLINE
		}
	}

WAIT_FOR_DEADLINE:
	// 2. Ngủ cho đến khi hết hẳn Control Window (để đồng bộ với Node)
	// Dùng sleepUntil để hỗ trợ cancel context
	g.sleepUntil(ctx, deadline)
}

// postRegisterToServer: Gửi yêu cầu đăng ký lên Server
func (g *Gateway) postRegisterToServer(vehicleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	register := model.RegisterRequest{
		GatewayAddress: g.Address,
		VehicleID:      vehicleID,
	}
	payload, _ := json.Marshal(register)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+g.ServerAddress+"/register",
		bytes.NewReader(payload),
	)
	if err != nil {
		slog.Error("[Gateway] Failed to build register request",
			"gateway", g.Address, "error", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	slog.Info(
		"[Gateway] Sending REGISTER",
		"gateway", g.Address,
		"vehicle", vehicleID,
	)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"[Gateway] Failed to POST register to Server",
			"gateway", g.Address, "server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("[Gateway] Server rejected registration",
			"gateway", g.Address, "status", resp.Status)
		return
	}

	// Optional: parse response (slot/gateway)
	var registerResponse model.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&registerResponse); err != nil && err != http.ErrBodyReadAfterClose {
		// decode error is non-fatal but log
		slog.Warn("[Gateway] Failed to parse register response",
			"gateway", g.Address, "error", err)
	} else {
		slog.Info(
			"[Gateway] Register accepted by server",
			"gateway", registerResponse.GatewayAddress,
			"vehicle", vehicleID, "slot", registerResponse.Slot,
		)
	}
}

// postTelemetryToServer: Gửi dữ liệu Telemetry lên Server
func (g *Gateway) postTelemetryToServer(data model.VehicleData) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Gửi VehicleData lên Server để reset TTL session
	body, _ := json.Marshal(data)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+g.ServerAddress+"/telemetry",
		bytes.NewReader(body),
	)
	if err != nil {
		slog.Error("[Gateway] Failed to build telemetry request",
			"gateway", g.Address, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	slog.Info(
		"[Gateway] Sending TELEMETRY",
		"gateway", g.Address,
		"vehicle", data.VehicleID,
		"lat", data.Latitude,
		"lon", data.Longitude,
	)
	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"[Gateway] Failed to POST telemetry to Server",
			"gateway", g.Address, "server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() { _ = resp.Body.Close() }()
}

```
