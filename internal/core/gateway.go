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
	Addr       string
	ServerAddr string
	// vehicles   map[string]struct{}
	keyStore   map[string][]byte // vehicleID -> session key
	lora       *device.Lora
	server     *http.Server
	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
}

// NewGateway creates a new Gateway instance bound to a LoRa serial device.
// func NewGateway(id, loraDev string, loraBaud int, addr, serverAddr string, vehicles []string) *Gateway {
func NewGateway(id, loraDev string, loraBaud int, addr, serverAddr string) *Gateway {
	lora := device.NewLora(loraDev, loraBaud)

	// vmap := make(map[string]struct{}, len(vehicles))
	// for _, v := range vehicles {
	// 	vmap[v] = struct{}{}
	// }

	return &Gateway{
		ID:         id,
		lora:       lora,
		Addr:       addr,
		ServerAddr: serverAddr,
		// vehicles:   vmap,
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

	// addr := strings.TrimPrefix(strings.TrimPrefix(g.URL, "http://"), "https://")
	addr := g.Addr
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
					resp, err := http.Post("http://"+g.ServerAddr+"/api/register", "application/json", bytes.NewReader(bodyB))
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
		resp, err := http.Post("http://"+g.ServerAddr+"/api/telemetry", "application/json", bytes.NewReader(payloadJSON))
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
