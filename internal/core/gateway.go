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
	CycleDuration    = 10000 * time.Millisecond
	SlotDurationMs   = int64(2000)
	GuardTimeMs      = int64(500)
	RegisterWindowMs = int64(5000)
)

// Gateway là đại diện cho thiết bị Gateway LoRaWAN
type Gateway struct {
	Address       string // Địa chỉ HTTP/TCP của Gateway (dùng làm ID)
	ServerAddress string // Địa chỉ HTTP của Fog Server
	lora          *device.Lora
	httpClient    *http.Client

	slotMutex    sync.Mutex
	currentSlots map[string]int // Map VehicleID -> SlotIndex. Cập nhật từ Server.

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
	}
}

// Start khởi động các tiến trình của Gateway
func (g *Gateway) Start(ctx context.Context) error {
	// slog.Warn(">>> ENTER Gateway.Start() <<<")
	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	// 1. Khởi động HTTP Server để nhận lệnh từ Server (Control & Update Beacon)
	// slog.Warn(">>> BEFORE startHTTPServer <<<")
	g.wg.Add(1)
	go g.startHTTPServer(ctx)

	// 2. Khởi động vòng lặp phát Beacon
	// slog.Warn(">>> BEFORE beaconLoop <<<")
	g.wg.Add(1)
	go g.beaconLoop(ctx)

	// 3. Khởi động vòng lặp lắng nghe Uplink (Hello & Telemetry)
	// slog.Warn(">>> BEFORE uplinkLoop <<<")
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

// handleControl
func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var controlJSON model.ControlData
	if err := json.NewDecoder(r.Body).Decode(&controlJSON); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	controlCBOR, err := cbor.Marshal(controlJSON)
	if err != nil {
		slog.Error("[Gateway] Failed to marshal CONTROL",
			"gateway", g.Address, "error", err)
		return
	}
	if err := g.lora.Write(controlCBOR); err != nil {
		slog.Error("[Gateway] Failed to write CONTROL to LoRa",
			"gateway", g.Address, "err", err)
	} else {
		slog.Info("[Gateway] Send CONTROL", "gateway", g.Address)
	}
}

// beaconLoop: Phát Beacon TDMA định kỳ
func (g *Gateway) beaconLoop(ctx context.Context) {
	defer g.wg.Done()
	// Giả lập chu kỳ TDMA 2 giây
	ticker := time.NewTicker(CycleDuration)

	for {
		select {
		case <-ctx.Done():
			// slog.Info("CTX DEAD INSIDE BEACON LOOP")
			ticker.Stop()
			return
		case t := <-ticker.C:
			g.slotMutex.Lock()
			slots := make(map[string]int)
			// Copy map để đảm bảo Thread-safe khi broadcast
			maps.Copy(slots, g.currentSlots)
			g.slotMutex.Unlock()

			cycleStart := t.UnixNano() / int64(time.Millisecond)
			cycleDuration := int64(CycleDuration / time.Millisecond)
			nextCycleStart := cycleStart + cycleDuration
			beacon := model.BeaconMessage{
				Type:             model.PacketBeacon,
				GatewayAddress:   g.Address,
				CycleStart:       cycleStart,
				NextCycleStart:   nextCycleStart,
				CycleDurationMs:  cycleDuration,
				SlotDurationMs:   SlotDurationMs,
				GuardTimeMs:      GuardTimeMs,
				RegisterWindowMs: RegisterWindowMs,
				SlotMap:          slots,
			}

			payload, err := cbor.Marshal(beacon)
			if err != nil {
				slog.Error("[Gateway] Failed to marshal BEACON",
					"gateway", g.Address, "error", err)
				continue
			}

			// SYNC (tại thời điểm này)
			if err := g.lora.Write(payload); err != nil {
				slog.Error("[Gateway] Failed to write BEACON to LoRa",
					"gateway", g.Address, "err", err)
			} else {
				slog.Info(
					"[Gateway] Broadcast BEACON",
					"gateway", g.Address, "slots", len(slots),
				)
			}
		}
	}
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
			if err == device.ErrLoraTimeout {
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
			"gateway", g.Address, "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"[Gateway] Failed to POST register to Server",
			"gateway", g.Address, "server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info(
		"[Gateway] Send REGISTER",
		"gateway", g.Address,
		"vehicle", vehicleID,
	)

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
			"gateway", g.Address, "err", err)
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

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"[Gateway] Failed to POST telemetry to Server",
			"gateway", g.Address, "server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	slog.Info(
		"[Gateway] Send TELEMETRY",
		"gateway", g.Address,
		"vehicle", data.VehicleID,
		"lat", data.Latitude,
		"lon", data.Longitude,
	)
}
