// Package core implements Vehicle node logic.
package core

import (
	"context"
	"crypto/rand"
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
	lora       *device.Lora
	arduino    *device.Arduino
	sessionKey []byte

	seq    uint8
	seqMu  sync.Mutex
	cancel context.CancelFunc
}

// NewVehicle creates a new Vehicle instance.
func NewVehicle(id, dev string, baud int, arDev string, arBaud int) *Vehicle {
	v := &Vehicle{
		ID:   id,
		lora: device.NewLora(dev, baud),
	}
	if arDev != "" {
		v.arduino = device.NewArduino(arDev, arBaud)
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

		// Try parse with current session key (if any). ParseFrame returns plaintext payload if key ok,
		// otherwise error. For plain frames, pass nil key.
		// Process each complete frame extracted by ReadFrames()
		for _, frame := range frames {
			var (
				typ     model.PacketType
				payload []byte
			)

			// If we have a session key, try decrypt/verify first.
			if v.sessionKey != nil {
				typ, _, _, payload, err = model.ParseFrame(frame, v.sessionKey)
				if err != nil {
					// try plain fallback (no key)
					typ, _, _, payload, err = model.ParseFrame(frame, nil)
					if err != nil {
						// unable to parse even as plain — skip this frame
						slog.Warn("failed to parse frame (secure and plain)", "vehicle", v.ID, "err", err)
						continue
					}
				}
			} else {
				// no key -> parse as plain
				typ, _, _, payload, err = model.ParseFrame(frame, nil)
				if err != nil {
					slog.Warn("failed to parse plain frame", "vehicle", v.ID, "err", err)
					continue
				}
			}
			switch typ {
			case model.TypeBeacon:
				v.handleBeacon()
			case model.TypeAuth:
				v.handleAuth(payload)
			case model.TypeControl:
				v.handleControl(payload)
			default:
				// ignore other types
				slog.Debug("ignoring unknown packet type", "vehicle", v.ID, "type", typ)
			}
		}
	}
}

// telemetryLoop reads Arduino data and sends telemetry frames.
func (v *Vehicle) telemetryLoop(ctx context.Context) {
	ch := make(chan model.ArduinoData, 4)
	stop, err := v.arduino.Read(ch)
	if err != nil {
		slog.Warn("arduino read failed", "vehicle", v.ID, "err", err)
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
	lat := int32(d.Latitude * 1e7)
	lon := int32(d.Longitude * 1e7)
	curHead := uint16(d.CurrentHead % 360)
	tarHead := uint16(d.CurrentHead % 360)
	lSpeed := uint16(d.LeftSpeed / 10) // conversion example; adjust per your mapping
	rSpeed := uint16(d.LeftSpeed / 10) // conversion example; adjust per your mapping

	payload := model.BuildTelemetryPacked(lat, lon, curHead, tarHead, lSpeed, rSpeed)

	v.seqMu.Lock()
	seq := v.seq
	v.seq++
	v.seqMu.Unlock()
	nonce := make([]byte, model.NonceLength)
	if _, err := rand.Read(nonce); err != nil { // fallback
		copy(nonce, []byte(time.Now().Format("15040506")))
	}

	var frame []byte
	var err error
	if v.sessionKey != nil {
		frame, err = model.BuildSecureFrame(model.TypeTelemetry, seq, nonce, payload, v.sessionKey)
	} else {
		frame, err = model.BuildPlainFrame(model.TypeTelemetry, seq, nonce, payload)
	}
	if err != nil {
		slog.Warn("build telemetry frame failed", "vehicle", v.ID, "err", err)
		return
	}
	if err := v.lora.WriteBytes(frame); err != nil {
		slog.Warn("failed to write telemetry frame", "vehicle", v.ID, "err", err)
		return
	}
	slog.Debug("telemetry sent", "vehicle", v.ID, "seq", seq, "nonce", fmt.Sprintf("%X", nonce[:4]), "len", len(frame))
}

// handleBeacon sends a HELLO frame to gateway when a beacon is seen.
func (v *Vehicle) handleBeacon() {
	msg := map[string]any{"type": "hello", "vehicle_id": v.ID}
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
