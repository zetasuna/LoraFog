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
