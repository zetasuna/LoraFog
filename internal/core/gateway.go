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
	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	// 1. Khởi động HTTP Server để nhận lệnh từ Server (Control & Update Beacon)
	g.wg.Add(1)
	go g.startHTTPServer(ctx)

	// 2. Khởi động vòng lặp phát Beacon
	g.wg.Add(1)
	go g.beaconLoop(ctx)

	// 3. Khởi động vòng lặp lắng nghe Uplink (Hello & Telemetry)
	g.wg.Add(1)
	go g.uplinkLoop(ctx)

	slog.Info("Gateway started", "address", g.Address)
	return nil
}

// Stop gracefully stops the gateway and closes resources.
func (g *Gateway) Stop() {
	slog.Info("Gateway is stopping", "address", g.Address)

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn("Failed to close LoRa device", "error", err)
		}
	}

	if g.cancel != nil {
		g.cancel()
	}

	g.wg.Wait()
	slog.Info("Gateway stopped", "address", g.Address)
}

// startHTTPServer khởi động server HTTP nội bộ để nhận lệnh từ Fog Server
func (g *Gateway) startHTTPServer(ctx context.Context) {
	defer g.wg.Done()
	mux := http.NewServeMux()

	// Endpoint nhận danh sách Slot Map mới từ Server
	mux.HandleFunc("/api/update_beacon", g.handleBeaconUpdate)

	server := &http.Server{Addr: g.Address, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("Gateway HTTP server error", "addr", g.Address, "error", err)
		}
	}()

	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		slog.Error("Gateway HTTP server shutdown failed", "addr", g.Address, "error", err)
	} else {
		slog.Info("Gateway HTTP server shutdown clean", "addr", g.Address)
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
	slog.Info("Beacon slots updated by Server", "count", len(newMap))

	w.WriteHeader(http.StatusOK)
}

// beaconLoop: Phát Beacon TDMA định kỳ
func (g *Gateway) beaconLoop(ctx context.Context) {
	defer g.wg.Done()
	// Giả lập chu kỳ TDMA 2 giây
	cycleDuration := 2000 * time.Millisecond
	ticker := time.NewTicker(cycleDuration)

	// Thông số TDMA cố định
	slotDurationMs := int64(50)
	guardTimeMs := int64(10)
	registerWindowMs := int64(300)

	for {
		select {
		case <-ctx.Done():
			ticker.Stop()
			return
		case t := <-ticker.C:
			g.slotMutex.Lock()
			slots := make(map[string]int)
			// Copy map để đảm bảo Thread-safe khi broadcast
			maps.Copy(slots, g.currentSlots)
			g.slotMutex.Unlock()

			beacon := model.BeaconMessage{
				Type:             model.PacketBeacon,
				GatewayAddress:   g.Address,
				Timestamp:        t.UnixNano() / int64(time.Millisecond),
				CycleDurationMs:  int64(cycleDuration / time.Millisecond),
				SlotDurationMs:   slotDurationMs,
				GuardTimeMs:      guardTimeMs,
				RegisterWindowMs: registerWindowMs,
				SlotMap:          slots,
			}

			payload, err := cbor.Marshal(beacon)
			if err != nil {
				slog.Error("Failed to marshal beacon", "error", err)
				continue
			}

			// SYNC (tại thời điểm này)
			if err := g.lora.Write(payload); err != nil {
				slog.Error("Failed to write beacon to LoRa", "err", err)
			} else {
				slog.Debug(
					"Beacon Broadcast",
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
			slog.Error("Lora read error", "error", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Phân loại gói tin
		var generic map[string]any
		if err := cbor.Unmarshal(frame, &generic); err != nil {
			slog.Warn("Received unknown or corrupted CBOR packet", "err", err)
			continue
		}

		msgType, ok := generic["type"].(string)
		if !ok {
			slog.Warn("Packet type missing")
			continue
		}

		switch msgType {
		case "hello":
			var hello model.HelloMessage
			if err := cbor.Unmarshal(frame, &hello); err == nil {
				slog.Info("Received Hello (Register)", "vehicle_id", hello.VehicleID)
				g.postRegisterToServer(hello.VehicleID)
			}
		case "telemetry":
			var telemetry model.VehicleData
			if err := cbor.Unmarshal(frame, &telemetry); err == nil {
				slog.Debug("Received Telemetry", "vehicle_id", telemetry.VehicleID)
				g.postTelemetryToServer(telemetry)
			}
		default:
			slog.Debug("Received unhandled message type", "type", msgType)
		}
	}
}

// postRegisterToServer: Gửi yêu cầu đăng ký lên Server
func (g *Gateway) postRegisterToServer(vehicleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	reqBody := map[string]string{
		"gateway_address": g.Address,
		"vehicle_id":      vehicleID,
	}
	payload, _ := json.Marshal(reqBody)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+g.ServerAddress+"/api/register",
		bytes.NewReader(payload),
	)
	if err != nil {
		slog.Error("Failed to build register request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"Failed to POST register to Server",
			"server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("Server rejected registration", "status", resp.Status)
		return
	}

	// Optional: parse response (slot/gateway)
	var registerResponse model.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&registerResponse); err != nil && err != http.ErrBodyReadAfterClose {
		// decode error is non-fatal but log
		slog.Warn("Failed to parse register response", "err", err)
	} else {
		slog.Info(
			"Register accepted by server",
			"vehicle", vehicleID,
			"slot", registerResponse.Slot,
			"gateway", registerResponse.GatewayAddress,
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
		"http://"+g.ServerAddress+"/api/telemetry",
		bytes.NewReader(body),
	)
	if err != nil {
		slog.Error("Failed to build telemetry request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"Failed to POST telemetry to Server",
			"server", g.ServerAddress, "error", err,
		)
		return
	}
	_ = resp.Body.Close()
}
