// Package core implements the Fog server — registry, WebSocket, telemetry & control APIs.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"sort"
	"sync"
	"time"

	"LoraFog/internal/database"
	"LoraFog/internal/model"
)

const (
	TimeToLive     = 60 * time.Second
	SweeperCycle   = 10 * time.Second
	MaxGatewaySlot = 10
)

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
		httpClient:   &http.Client{Timeout: HTTPTimeout},
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

	server := &http.Server{Addr: s.Address, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("[Server] HTTP failed to start", "error", err)
		}
	}()
	slog.Info("[Server] Started", "address", s.Address)

	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), HTTPTimeout)
	defer cancel()
	_ = server.Shutdown(ctxShutdown)
	slog.Info("[Server] Stopped")
	return nil
}

// handleRegister: Xử lý yêu cầu đăng ký mới từ Gateway (Hello Packet)
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	slog.Info(
		"[Server] Received REGISTER",
		"gateway", req.GatewayAddress, "vehicle", req.VehicleID,
	)

	if req.VehicleID == "" || req.GatewayAddress == "" {
		http.Error(w, "vehicle and gateway required", http.StatusBadRequest)
		return
	}

	// 1. Logic Roaming/Trùng Session
	var oldGw string
	s.mutex.Lock()
	if oldSes, ok := s.sessions[req.VehicleID]; ok {
		oldGw = oldSes.GatewayAddress

		if oldGw == req.GatewayAddress {
			slog.Info(
				"[Server] Found session for vehicle that wanted to register",
				"gateway", req.GatewayAddress, "vehicle", req.VehicleID,
			)
			s.mutex.Unlock()
			return
		}

		delete(s.sessions, req.VehicleID)
		s.compactSlotsForGateway(oldGw)
	}
	s.mutex.Unlock()

	// 2. Nếu có session cũ → unregister DB + push update
	if oldGw != "" {
		// Update DB to clear gateway for vehicle (safeguard)
		_ = s.database.UpdateVehicleGateway(context.Background(), req.VehicleID, "")
		s.pushSlotUpdateToGateway(oldGw)
		slog.Info(
			"[Server] Roaming: Cleared old session",
			"vehicle", req.VehicleID, "old_gw", oldGw,
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
		TTL:            TimeToLive,
		Slot:           newSlot,
	}
	s.sessions[req.VehicleID] = newSession
	s.mutex.Unlock()
	slog.Info(
		"[Server] Created new session",
		"gateway", req.GatewayAddress, "vehicle", req.VehicleID, "slot", newSlot,
	)

	// 4. Cập nhật DB (Boat -> GatewayID)
	ctx, cancel := context.WithTimeout(context.Background(), ContextTimeout)
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

	// Respond with slot details
	resp := model.RegisterResponse{
		Slot:           newSlot,
		GatewayAddress: req.GatewayAddress,
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(&resp); err != nil {
		slog.Error("[Server] Failed to write register response", "error", err)
	}
}

// handleTelemetry: Xử lý dữ liệu Telemetry và Reset TTL
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	var telemetry model.VehicleData
	if err := json.NewDecoder(r.Body).Decode(&telemetry); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	slog.Info(
		"[Server] Received TELEMETRY",
		"vehicle", telemetry.VehicleID,
		"lat", telemetry.Latitude, "lon", telemetry.Longitude,
	)

	s.mutex.Lock()
	if ses, ok := s.sessions[telemetry.VehicleID]; ok {
		ses.CreatedAt = time.Now() // Reset TTL
		slog.Info("[Server] Reset TTL", "vehicle", telemetry.VehicleID)
		// forward to app server (if configured) - safe call with timeout
		if s.AppAddress != "" {
			go func(t model.VehicleData) {
				ctx, cancel := context.WithTimeout(context.Background(), ContextTimeout)
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
					slog.Warn("[Server] Failed to build forward request", "error", err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := s.httpClient.Do(req)
				if err != nil {
					slog.Warn("[Server] Failed to forward telemetry", "error", err)
					return
				}
				defer func() {
					_, _ = io.Copy(io.Discard, resp.Body)
					_ = resp.Body.Close()
				}()
			}(telemetry)
		}
	} else {
		// session unknown: log and drop
		slog.Warn("[Server] Received TELEMETRY for unknown session", "vehicle", telemetry.VehicleID)
	}
	s.mutex.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	var controlApp model.ControlApp
	if err := json.NewDecoder(r.Body).Decode(&controlApp); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	slog.Info("[Server] Received CONTROL")

	if controlApp.VehicleID == "" {
		http.Error(w, "vehicle id required", http.StatusBadRequest)
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
		ctx, cancel := context.WithTimeout(context.Background(), ContextTimeout)
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
		defer func() {
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}()
	}(controlData)

	w.WriteHeader(http.StatusOK)
}

// sweeper: Quét các Session đã hết hạn (TTL Expired)
func (s *Server) sweeper(ctx context.Context) {
	ticker := time.NewTicker(SweeperCycle)
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
					dirtyGateways[ses.GatewayAddress] = true

					// 3. Xóa session
					delete(s.sessions, vID)
				}
			}
			for gwID := range dirtyGateways {
				s.compactSlotsForGateway(gwID)
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
	for i := 1; i <= MaxGatewaySlot; i++ {
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

// Hàm này đảm bảo mảng slot luôn đặc, không bị thủng lỗ
func (s *Server) compactSlotsForGateway(gwAddr string) {
	// 1. Lấy danh sách session của gateway này
	var gwSessions []*Session
	for _, ses := range s.sessions {
		if ses.GatewayAddress == gwAddr {
			gwSessions = append(gwSessions, ses)
		}
	}

	if len(gwSessions) == 0 {
		// Nếu không còn session nào, reset map slot
		s.gatewaySlots[gwAddr] = make(map[int]bool)
		return
	}

	// 2. Sắp xếp các session theo Slot hiện tại (để giữ tính ổn định tương đối)
	// Ví dụ: Đang có [1, 5, 9] -> Sau khi sort và đánh lại sẽ thành [1, 2, 3]
	// Thuyền ở slot 1 vẫn là 1, slot 5 về 2, slot 9 về 3.
	sort.Slice(gwSessions, func(i, j int) bool {
		return gwSessions[i].Slot < gwSessions[j].Slot
	})

	// 3. Gán lại slot từ 1 -> N và cập nhật map
	newSlotMap := make(map[int]bool)
	for i, ses := range gwSessions {
		newSlot := i + 1

		// Update session
		ses.Slot = newSlot

		// Mark slot as used
		newSlotMap[newSlot] = true
	}

	// 4. Update lại GatewaySlots map
	s.gatewaySlots[gwAddr] = newSlotMap

	slog.Info("[Server] Compacted slots", "gateway", gwAddr, "count", len(gwSessions))
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
	ctx, cancel := context.WithTimeout(context.Background(), ContextTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+gwAddr+"/update_beacon",
		bytes.NewReader(body),
	)
	if err != nil {
		slog.Error(
			"[Server] Failed to build request for gateway",
			"gateway", gwAddr, "error", err,
		)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	slog.Info(
		"[Server] Pushing slot map to Gateway",
		"gateway", gwAddr, "count", len(slotMap),
	)
	resp, err := s.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"[Server] Failed to push slot update to Gateway",
			"gateway", gwAddr, "error", err,
		)
		return
	}
	if resp.StatusCode != http.StatusOK {
		slog.Warn(
			"[Server] Gateway rejected slot update",
			"gateway", gwAddr, "status", resp.Status,
		)
	} else {
		slog.Debug(
			"[Server] Successfully pushed slot map to Gateway",
			"gateway", gwAddr, "count", len(slotMap),
		)
	}
	// Ensure body closed
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
}
