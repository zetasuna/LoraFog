// Package core defines the Gateway component responsible for
// bridging LoRa-connected vehicles with the FogServer
// using CBOR (for LoRa) and JSON (for HTTP).
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
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
	LoraDuration     = int64(2000)
	GuardTimeMs      = LoraDuration
	SlotWindowMs     = LoraDuration
	BeaconWindowMs   = LoraDuration
	ControlWindowMs  = LoraDuration * 2
	RegisterWindowMs = LoraDuration * 2

	ReadTimeout    = 100 * time.Millisecond
	HTTPTimeout    = 5 * time.Second
	ContextTimeout = 3 * time.Second

	ControlQueue = 50

	PacketLossWindowSize = 10
)

type VehicleStats struct {
	SentCount int // Số gói mong đợi (số lần Gateway gửi Beacon và dành slot cho xe này)
	RecvCount int // Số gói thực tế nhận được (Telemetry)
}

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

	statsMutex sync.Mutex
	stats      map[string]*VehicleStats
}

// NewGateway tạo một Gateway mới
func NewGateway(
	address, serverAddress string,
	loraDev string, loraBaud int,
) (*Gateway, error) {
	lora, err := device.NewLora(loraDev, loraBaud)
	if err != nil {
		// Nếu không mở được cổng LoRa, trả về lỗi luôn
		slog.Error("Failed to initialize Lora", "error", err)
		return nil, err
	}

	gateway := &Gateway{
		Address:       address,
		ServerAddress: serverAddress,
		lora:          lora,
		httpClient:    &http.Client{Timeout: HTTPTimeout},
		currentSlots:  make(map[string]int),
		controlQueue:  make(chan model.ControlData, ControlQueue),
		// --- [CHANGE START] ---
		stats: make(map[string]*VehicleStats),
		// --- [CHANGE END] ---
	}
	return gateway, nil
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

	slog.Info("[Gateway] Started", "gateway", g.Address)
	return nil
}

// Stop gracefully stops the gateway and closes resources.
func (g *Gateway) Stop() {
	slog.Info("[Gateway] Stopping", "gateway", g.Address)

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn(
				"[Gateway] Failed to close LoRa device",
				"gateway", g.Address, "error", err,
			)
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
			slog.Error(
				"[Gateway] HTTP server error",
				"gateway", g.Address, "error", err,
			)
		}
	}()

	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), HTTPTimeout)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		slog.Error(
			"[Gateway] HTTP server shutdown failed",
			"gateway", g.Address, "error", err,
		)
	} else {
		slog.Info(
			"[Gateway] HTTP server shutdown clean",
			"gateway", g.Address,
		)
	}
}

// handleBeaconUpdate: Nhận Slot Map mới từ Server (khi có đăng ký/hết hạn session)
func (g *Gateway) handleBeaconUpdate(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()
	var newMap map[string]int
	if err := json.NewDecoder(r.Body).Decode(&newMap); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	g.slotMutex.Lock()
	g.currentSlots = newMap
	g.slotMutex.Unlock()
	slog.Info(
		"[Gateway] Updated beacon slots",
		"gateway", g.Address, "count", len(newMap),
	)

	w.WriteHeader(http.StatusOK)
}

func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		_, _ = io.Copy(io.Discard, r.Body)
		_ = r.Body.Close()
	}()

	var controlJSON model.ControlData
	if err := json.NewDecoder(r.Body).Decode(&controlJSON); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	// Đưa vào queue để gửi ở CONTROL frame
	select {
	case g.controlQueue <- controlJSON:
		slog.Info(
			"[Gateway] Queued CONTROL",
			"gateway", g.Address, "vehicle", controlJSON.VehicleID,
		)
	default:
		slog.Warn("[Gateway] Control queue FULL, dropping control!")
	}

	w.WriteHeader(http.StatusOK)
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
		default:
		}

		if time.Until(nextCycleStart) > 0 {
			slog.Debug("[Gateway] Sleep to next Cycle", "gateway", g.Address)
			g.sleepUntil(ctx, nextCycleStart)
		} else {
			slog.Debug("[Gateway] Resync Cycle", "gateway", g.Address)
			nextCycleStart = time.Now()
		}

		// Snapshot số slot hiện tại
		g.slotMutex.Lock()
		slots := make(map[string]int)
		maps.Copy(slots, g.currentSlots)
		slotCount := len(slots)
		g.slotMutex.Unlock()

		// --- [CHANGE START] ---
		// Bắt đầu chu kỳ mới: Cập nhật biến "số gói gửi" (Expected) cho các xe có trong slot
		g.updateExpectedStats(slots)
		// --- [CHANGE END] ---

		// cycleStart := time.Now()
		cycleStart := nextCycleStart
		cycleStartMs := cycleStart.UnixNano() / int64(time.Millisecond)
		beaconEnd := cycleStart.Add(time.Duration(BeaconWindowMs) * time.Millisecond)
		controlEnd := beaconEnd.Add(time.Duration(ControlWindowMs) * time.Millisecond)
		registerEnd := controlEnd.Add(time.Duration(GuardTimeMs+RegisterWindowMs+GuardTimeMs) * time.Millisecond)
		slotsEnd := registerEnd.Add(time.Duration(int64(slotCount)*(SlotWindowMs+GuardTimeMs)) * time.Millisecond)

		// Cập nhật thời điểm bắt đầu chu kỳ KẾ TIẾP
		nextCycleStart = slotsEnd

		// === BEACON WINDOW ===
		slog.Debug(
			"[Gateway] Cycle Status",
			"gateway", g.Address, "status", "Beacon",
		)
		g.sendBeacon(cycleStartMs, slots)
		g.sleepUntil(ctx, beaconEnd)

		// === CONTROL WINDOW ===
		slog.Debug(
			"[Gateway] Cycle Status",
			"gateway", g.Address, "status", "Control",
		)
		g.processControl(ctx, controlEnd)

		// === LORA LISTENING ===
		slog.Debug(
			"[Gateway] Cycle Status",
			"gateway", g.Address, "status", "Listen",
		)
		g.listenUntil(ctx, slotsEnd)
		// === END CYCLE ===
		slog.Debug(
			"[Gateway] Cycle Status",
			"gateway", g.Address, "status", "End",
		)
	}
}

func (g *Gateway) sendBeacon(cycleStartMs int64, slots map[string]int) {
	beacon := model.BeaconMessage{
		Type:             model.PacketBeacon,
		Timestamp:        time.Now().UnixNano() / int64(time.Millisecond),
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
		slog.Error(
			"[Gateway] Failed to marshal BEACON",
			"gateway", g.Address, "error", err,
		)
		return
	}

	if err := g.lora.WriteLine(payload); err != nil {
		slog.Error("[Gateway] Failed to send BEACON",
			"gateway", g.Address, "error", err)
	} else {
		slog.Info(
			"[Gateway] Sent BEACON",
			"gateway", g.Address, "slotCount", len(slots),
		)
	}
}

// Hàm phụ trợ gửi gói Control (đã tách ra từ code cũ cho gọn)
func (g *Gateway) sendControl(ctrl model.ControlData) {
	payload, err := cbor.Marshal(ctrl)
	if err != nil {
		slog.Error(
			"[Gateway] Control marshal error",
			"gateway", g.Address, "error", err,
		)
		return
	}

	if err := g.lora.WriteLine(payload); err != nil {
		slog.Error(
			"[Gateway] Failed to send CONTROL",
			"gateway", g.Address, "vehicle", ctrl.VehicleID, "error", err,
		)
	} else {
		slog.Info(
			"[Gateway] Sent CONTROL",
			"gateway", g.Address, "vehicle", ctrl.VehicleID,
		)
	}
}

// Hàm phụ trợ giúp sleep chính xác và hỗ trợ cancel context
func (g *Gateway) sleepUntil(ctx context.Context, deadline time.Time) {
	d := time.Until(deadline)
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
func (g *Gateway) listenUntil(ctx context.Context, deadline time.Time) {
	for {
		// 1. Kiểm tra Context
		select {
		case <-ctx.Done():
			return
		default:
		}

		// 2. Kiểm tra Deadline
		remaining := time.Until(deadline)
		slog.Debug("[Listen]", "remain", remaining)
		if remaining < ReadTimeout {
			if remaining > 0 {
				g.sleepUntil(ctx, deadline)
			}
			return // Hết cửa sổ -> Thlength := int(header[0])oát ngay để chuyển sang trạng thái khác
		}

		// 3. Đọc dữ liệu
		frame, err := g.lora.ReadLine(ReadTimeout)
		if err != nil {
			// Nếu timeout thì thử lại (vòng lặp tiếp theo)
			if err == device.ErrTimeout { // Nhớ dùng biến lỗi chung device.ErrTimeout
				slog.Debug("[Listen] End (timout)")
				continue
			}
			// Lỗi khác (IO error)
			slog.Error(
				"[Gateway] Fail to read Lora",
				"gateway", g.Address, "error", err,
			)
			time.Sleep(ReadTimeout / 4) // Nghỉ chút tránh spam log
			continue
		}

		if len(frame) == 0 {
			slog.Debug("[Listen] End (lenght 0)")
			continue
		}

		// 4. Xử lý gói tin (Spawn Goroutine để không chặn luồng lắng nghe)
		payload := make([]byte, len(frame))
		copy(payload, frame)
		g.wg.Add(1)
		go func(data []byte) {
			defer g.wg.Done()
			g.processLora(data)
		}(payload)
		slog.Debug("[Listen] End")
	}
}

// processLora: Xử lý logic gói tin
func (g *Gateway) processLora(frame []byte) {
	var generic map[string]any
	if err := cbor.Unmarshal(frame, &generic); err != nil {
		slog.Warn(
			"[Gateway] Corrupted packet",
			"gateway", g.Address, "error", err,
		)
		return
	}

	msgType, ok := generic["type"].(string)
	if !ok {
		return
	}

	// Lọc gói tin theo Window (TDMA Enforcement)
	switch msgType {
	case model.PacketHello:
		var hello model.HelloMessage
		if err := cbor.Unmarshal(frame, &hello); err == nil {
			slog.Info(
				"[Gateway] Received HELLO",
				"gateway", g.Address, "vehicle", hello.VehicleID,
			)
			g.postRegisterToServer(hello.VehicleID)
		}
	case model.PacketTelemetry:
		var telemetry model.VehicleData
		if err := cbor.Unmarshal(frame, &telemetry); err == nil {
			g.slotMutex.Lock()
			_, exists := g.currentSlots[telemetry.VehicleID]
			g.slotMutex.Unlock()

			if exists {
				slog.Info(
					"[Gateway] Received TELEMETRY",
					"gateway", g.Address, "vehicle", telemetry.VehicleID,
				)

				// --- [CHANGE START] ---
				// Gói tin hợp lệ từ xe đã được cấp slot -> Tăng biến đếm thành công
				g.statsMutex.Lock()
				if stat, ok := g.stats[telemetry.VehicleID]; ok {
					stat.RecvCount++
				}
				g.statsMutex.Unlock()
				// Kiểm tra xem đã đủ 10 chu kỳ để in log chưa
				g.checkAndLogStats(telemetry.VehicleID)
				// --- [CHANGE END] ---

				g.postTelemetryToServer(telemetry)
			} else {
				slog.Warn(
					"[Gateway] Ignored TELEMETRY (No Slot)",
					"gateway", g.Address, "vehicle", telemetry.VehicleID,
				)
			}
		}
	}
}

// processControl: Gửi các lệnh trong hàng đợi cho đến khi hết giờ hoặc hết hàng đợi
func (g *Gateway) processControl(ctx context.Context, deadline time.Time) {
	// Tạo timer để báo hết giờ
	// time.Until(deadline) trả về khoảng thời gian còn lại
	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()

	for {
		if time.Until(deadline) < ReadTimeout {
			slog.Debug("[Gateway] Control window closing (time budget)")
			// g.sleepUntil(ctx, deadline)
			return
		}

		select {
		case <-ctx.Done():
			return
		case ctrl := <-g.controlQueue:
			g.sendControl(ctrl)
			time.Sleep(ReadTimeout / 4)
		case <-timer.C:
			// Khi timer nổ (đúng thời điểm deadline), thoát vòng lặp
			// Thay thế cho việc dùng sleepUntil
			return
		}
	}
}

// Helper function để tái sử dụng code gửi request
func (g *Gateway) sendJSONRequest(ctx context.Context, method, url string, payload any) (*http.Response, error) {
	var bodyReader io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal error: %w", err)
		}
		bodyReader = bytes.NewReader(data)
	}

	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Thực hiện request
	return g.httpClient.Do(req)
}

// postRegisterToServer: Gửi yêu cầu đăng ký lên Server
func (g *Gateway) postRegisterToServer(vehicleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), ContextTimeout)
	defer cancel()

	register := model.RegisterRequest{
		GatewayAddress: g.Address,
		VehicleID:      vehicleID,
	}

	slog.Info(
		"[Gateway] Sending REGISTER",
		"gateway", g.Address, "vehicle", vehicleID,
	)
	url := fmt.Sprintf("http://%s/register", g.ServerAddress)
	resp, err := g.sendJSONRequest(ctx, "POST", url, register)
	if err != nil {
		slog.Error(
			"[Gateway] Failed to POST register to Server",
			"gateway", g.Address, "server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		slog.Warn(
			"[Gateway] Server rejected registration",
			"gateway", g.Address, "status", resp.Status,
		)
		return
	}

	// Optional: parse response (slot/gateway)
	var registerResponse model.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&registerResponse); err != nil && err != http.ErrBodyReadAfterClose {
		// decode error is non-fatal but log
		slog.Debug(
			"[Gateway] Failed to parse register response",
			"gateway", g.Address, "error", err,
		)
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
	ctx, cancel := context.WithTimeout(context.Background(), ContextTimeout)
	defer cancel()

	slog.Info(
		"[Gateway] Sending TELEMETRY",
		"gateway", g.Address, "vehicle", data.VehicleID,
		"lat", data.Latitude, "lon", data.Longitude,
	)
	url := fmt.Sprintf("http://%s/telemetry", g.ServerAddress)
	resp, err := g.sendJSONRequest(ctx, "POST", url, data)
	if err != nil {
		slog.Error(
			"[Gateway] Failed to POST telemetry to Server",
			"gateway", g.Address, "server", g.ServerAddress, "error", err,
		)
		return
	}

	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		slog.Warn(
			"[Gateway] Server rejected telemetry",
			"gateway", g.Address, "status", resp.Status,
		)
		return
	}
}

// updateExpectedStats: Tăng biến đếm Expected cho mỗi xe được cấp slot trong chu kỳ này
func (g *Gateway) updateExpectedStats(slots map[string]int) {
	g.statsMutex.Lock()
	defer g.statsMutex.Unlock()

	for vehicleID := range slots {
		if _, ok := g.stats[vehicleID]; !ok {
			g.stats[vehicleID] = &VehicleStats{}
		}
		g.stats[vehicleID].SentCount++ // Tăng số gói kỳ vọng (Sent count)
	}
}

// checkAndLogStats: In thông tin packet loss nếu đủ chu kỳ 10 gói
func (g *Gateway) checkAndLogStats(vehicleID string) {
	g.statsMutex.Lock()
	defer g.statsMutex.Unlock()

	stat, ok := g.stats[vehicleID]
	if !ok {
		return
	}

	// Nếu số gói kỳ vọng đạt ngưỡng (10 gói)
	if stat.SentCount >= PacketLossWindowSize {
		ratio := (float64(stat.RecvCount) / float64(stat.SentCount)) * 100.0

		slog.Info(
			"[Gateway] Packet Loss Stats (Window: 10)",
			"vehicle", vehicleID,
			"expected_sent", stat.SentCount,
			"actual_received", stat.RecvCount,
			"success_ratio_percent", fmt.Sprintf("%.2f%%", ratio),
		)

		// Reset sau khi in
		stat.SentCount = 0
		stat.RecvCount = 0
	}
}
