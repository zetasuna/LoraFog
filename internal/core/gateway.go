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
	GuardTimeMs      = LoraDuration / 2
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

	// 2. Khởi động vòng lặp Lora
	g.wg.Add(1)
	go g.loraLoop(ctx)

	// 3. Khởi động vòng lặp Up Link
	g.wg.Add(1)
	go g.uplinkLoop(ctx)

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

	loraTimeout := 5 * time.Second

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// ReadFrameWithTimeout (50ms để không bị block lâu)
		frame, err := g.lora.Read(loraTimeout)
		if err != nil {
			if err == device.ErrTimeout {
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
					"gateway", g.Address, "vehicle", hello.VehicleID)
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
						"gateway", g.Address, "vehicle", telemetry.VehicleID)
					continue
				}

				// Vehicle hợp lệ → gửi lên Server
				slog.Info("[Gateway] Received TELEMETRY",
					"gateway", g.Address, "vehicle", telemetry.VehicleID)
				g.postTelemetryToServer(telemetry)
			}
		default:
			slog.Info("[Gateway] Received unhandled message type",
				"gateway", g.Address, "type", msgType)
		}
	}
}

// loraLoop: TDMA
func (g *Gateway) loraLoop(ctx context.Context) {
	defer g.wg.Done()

	// Dùng một biến theo dõi thời điểm bắt đầu dự kiến của chu kỳ tiếp theo
	nextCycleStart := time.Now()

	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(time.Until(nextCycleStart)):
		}

		// if time.Until(nextCycleStart) > 0 {
		// 	g.sleepUntil(ctx, nextCycleStart)
		// }

		if time.Since(nextCycleStart) > 50*time.Millisecond {
			slog.Warn("[Gateway] Cycle lagging detected, resyncing...", "gateway", g.Address)
			nextCycleStart = time.Now()
		}

		// Snapshot số slot hiện tại
		g.slotMutex.Lock()
		slots := make(map[string]int)
		maps.Copy(slots, g.currentSlots)
		slotCount := len(slots)
		g.slotMutex.Unlock()

		// cycleStart := time.Now()
		cycleStart := nextCycleStart
		cycleStartMs := cycleStart.UnixNano() / int64(time.Millisecond)
		beaconEnd := cycleStart.Add(time.Duration(BeaconWindowMs) * time.Millisecond)
		controlEnd := beaconEnd.Add(time.Duration(ControlWindowMs) * time.Millisecond)
		registerEnd := controlEnd.Add(time.Duration(RegisterWindowMs) * time.Millisecond)
		slotsDuration := time.Duration(slotCount) * time.Duration(SlotWindowMs) * time.Millisecond
		slotsEnd := registerEnd.Add(slotsDuration)

		// Cập nhật thời điểm bắt đầu chu kỳ KẾ TIẾP
		nextCycleStart = slotsEnd

		// === BEACON WINDOW ===
		slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Beacon")
		g.sendBeacon(cycleStartMs, slots)
		g.sleepUntil(ctx, beaconEnd)

		// === CONTROL WINDOW ===
		slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Control")
		g.processControl(ctx, controlEnd)

		// === REGISTER WINDOW ===
		// slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Register")
		// g.listenUntil(ctx, registerEnd, model.PacketHello)
		// g.sleepUntil(ctx, registerEnd)

		// === SLOT WINDOWS ===
		// slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "Slot")
		// g.listenUntil(ctx, slotsEnd, model.PacketTelemetry)
		// g.sleepUntil(ctx, slotsEnd)

		// === END CYCLE ===
		// slog.Info("[Gateway] Cycle Status", "gateway", g.Address, "status", "End")
	}
}

func (g *Gateway) sendBeacon(cycleStartMs int64, slots map[string]int) {
	beacon := model.BeaconMessage{
		Type:             model.PacketBeacon,
		GatewayAddress:   g.Address,
		GuardTimeMs:      GuardTimeMs,
		CycleStartMs:     cycleStartMs,
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
		slog.Error("[Gateway] Control marshal error",
			"gateway", g.Address, "err", err)
		return
	}

	if err := g.lora.Write(payload); err != nil {
		slog.Error("[Gateway] Failed to send CONTROL",
			"gateway", g.Address, "vehicle", ctrl.VehicleID, "err", err)
	} else {
		slog.Info("[Gateway] Sent CONTROL",
			"gateway", g.Address, "vid", ctrl.VehicleID)
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

// listenUntil: Lắng nghe LoRa liên tục cho đến thời điểm deadline
func (g *Gateway) listenUntil(ctx context.Context, deadline time.Time, expectedType string) {
	for {
		// 1. Kiểm tra Context
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 2. Kiểm tra Deadline
		remaining := time.Until(deadline)
		if remaining < 100*time.Millisecond {
			g.sleepUntil(ctx, deadline)
			return // Hết cửa sổ -> Thoát ngay để chuyển sang trạng thái khác
		}

		// slog.Info("1")
		// 3. Đọc dữ liệu
		frame, err := g.lora.Read(remaining)
		if err != nil {
			// slog.Info("2")
			// Nếu timeout thì thử lại (vòng lặp tiếp theo)
			if err == device.ErrTimeout { // Nhớ dùng biến lỗi chung device.ErrTimeout
				continue
			}
			// Lỗi khác (IO error)
			slog.Error("[Gateway] Read error", "err", err)
			time.Sleep(100 * time.Millisecond) // Nghỉ chút tránh spam log
			continue
		}

		if len(frame) == 0 {
			continue
		}
		// slog.Info("3")

		// 4. Xử lý gói tin (Spawn Goroutine để không chặn luồng lắng nghe)
		payload := make([]byte, len(frame))
		copy(payload, frame)

		g.wg.Add(1)
		go func(data []byte) {
			defer g.wg.Done()
			g.processLora(data, expectedType)
		}(payload)
	}
}

// processUplink: Xử lý logic gói tin (tách từ uplinkLoop cũ)
func (g *Gateway) processLora(frame []byte, expectedType string) {
	var generic map[string]any
	if err := cbor.Unmarshal(frame, &generic); err != nil {
		slog.Warn("[Gateway] Corrupted packet", "err", err)
		return
	}

	msgType, ok := generic["type"].(string)
	if !ok {
		return
	}

	// Lọc gói tin theo Window (TDMA Enforcement)
	// Nếu đang ở Register Window mà nhận Telemetry -> Có thể Drop hoặc Warn

	if msgType != expectedType {
		slog.Warn("[Gateway] Received wrong packet type", "expect", expectedType, "receive", msgType)
		return
	}

	switch msgType {
	case model.PacketHello:
		var hello model.HelloMessage
		if err := cbor.Unmarshal(frame, &hello); err == nil {
			slog.Info("[Gateway] Received HELLO",
				"gateway", g.Address, "vehicle", hello.VehicleID)
			g.postRegisterToServer(hello.VehicleID)
		}
	case model.PacketTelemetry:
		var telemetry model.VehicleData
		if err := cbor.Unmarshal(frame, &telemetry); err == nil {
			g.slotMutex.Lock()
			_, exists := g.currentSlots[telemetry.VehicleID]
			g.slotMutex.Unlock()

			if exists {
				slog.Info("[Gateway] Received TELEMETRY",
					"gateway", g.Address, "vehicle", telemetry.VehicleID)
				g.postTelemetryToServer(telemetry)
			} else {
				slog.Warn("[Gateway] Ignored TELEMETRY (No Slot)",
					"gateway", g.Address, "vehicle", telemetry.VehicleID)
			}
		}
	}
}

// processControlQueue: Gửi các lệnh trong hàng đợi cho đến khi hết giờ hoặc hết hàng đợi
func (g *Gateway) processControl(ctx context.Context, deadline time.Time) {
	// packetDuration := time.Duration(LoraDuration) * time.Millisecond
	for {
		// 1. Tính thời gian còn lại trước khi hết giờ
		// controlEnd := time.Now().Add(packetDuration)
		remaining := time.Until(deadline)

		// 2. Kiểm tra ngân sách thời gian (Time Budget)
		// Nếu thời gian còn lại < thời gian cần gửi 1 gói -> Dừng ngay
		// if remaining < packetDuration {
		if remaining < 100*time.Millisecond {
			// slog.Debug("[Gateway] Not enough time for new packet, closing window")
			g.sleepUntil(ctx, deadline)
			return
		}

		select {
		case <-ctx.Done():
			return
		case ctrl := <-g.controlQueue:
			g.sendControl(ctrl)
			time.Sleep(100 * time.Millisecond)
			// g.sleepUntil(ctx, controlEnd)
		default:
			// g.sleepUntil(ctx, controlEnd)
			continue
		}
	}
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
