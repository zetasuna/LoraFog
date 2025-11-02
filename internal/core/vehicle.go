// Package core implements the Vehicle agent responsible for collecting telemetry
// from Arduino devices and sending CBOR-encoded data via LoRa.
package core

// CHANGELOG (refactor v2):
// - Removed parser dependency
// - Vehicle<->Gateway uses CBOR serialization
// - Context-based lifecycle management
// - Structured logging (slog)
// - Renamed methods to Start / Shutdown for consistency
// - Safe Close and consistent log keys

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"github.com/fxamacker/cbor/v2"
)

// Vehicle represents a single autonomous vehicle communicating via LoRa.
type Vehicle struct {
	ID          string
	lora        *device.Lora
	arduino     *device.Arduino
	sessionKey  []byte
	leaseExpiry time.Time

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewVehicle constructs a Vehicle agent with LoRa and optional Arduino connection.
func NewVehicle(id, loraDev string, loraBaud int, arduinoDev string, arduinoBaud int) *Vehicle {
	lora := device.NewLora(loraDev, loraBaud)
	v := &Vehicle{
		ID:   id,
		lora: lora,
	}
	if arduinoDev != "" {
		v.arduino = device.NewArduino(arduinoDev, arduinoBaud)
	}
	return v
}

// Start begins the vehicle telemetry and control loops.
func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	// --- Arduino telemetry reader ---
	if v.arduino != nil {
		dataCh := make(chan model.ArduinoData, 5)
		stop, err := v.arduino.Read(dataCh)
		if err != nil {
			slog.Warn("failed to start Arduino reader",
				"component", "vehicle", "id", v.ID, "error", err)
		} else {
			slog.Info("Arduino telemetry reader started",
				"component", "vehicle", "id", v.ID)
			v.wg.Add(1)
			go func() {
				defer v.wg.Done()
				for {
					select {
					case <-ctx.Done():
						slog.Info("stopping Arduino telemetry loop",
							"component", "vehicle", "id", v.ID)
						return
					case data, ok := <-dataCh:
						if !ok {
							slog.Info("Arduino telemetry channel closed",
								"component", "vehicle", "id", v.ID)
							return
						}
						v.sendTelemetry(data)
					}
				}
			}()
			v.wg.Add(1)
			go func() {
				defer v.wg.Done()
				<-ctx.Done()
				stop()
			}()
		}
	}

	// --- LoRa control listener ---
	if v.lora != nil && v.arduino != nil {
		v.wg.Add(1)
		go func() {
			defer v.wg.Done()
			for {
				select {
				case <-ctx.Done():
					slog.Info("stopping LoRa control listener",
						"component", "vehicle", "id", v.ID)
					return
				default:
				}

				frame, err := v.lora.ReadFrame()
				if err != nil {
					time.Sleep(200 * time.Millisecond)
					continue
				}

				var ctl model.ControlData
				if err := cbor.Unmarshal(frame, &ctl); err != nil {
					slog.Warn("invalid control CBOR packet",
						"component", "vehicle", "id", v.ID, "error", err)
					continue
				}
				if ctl.VehicleID != v.ID {
					slog.Warn("control ignored (wrong target)",
						"component", "vehicle", "id", v.ID, "target", ctl.VehicleID)
					continue
				}

				out := fmt.Sprintf("%d,%.6f,%.6f,%.3f,%.3f,%.3f",
					ctl.Speed, ctl.Latitude, ctl.Longitude, ctl.Kp, ctl.Ki, ctl.Kd)
				if err := v.arduino.Write(out); err != nil {
					slog.Warn("failed to forward control to Arduino",
						"component", "vehicle", "id", v.ID, "error", err)
				} else {
					slog.Info("control forwarded to Arduino",
						"component", "vehicle", "id", v.ID)
				}
			}
		}()
	}

	// start beacon listener (reads frames and handles beacon/ auth)
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			frame, err := v.lora.ReadFrameWithTimeout(10 * time.Second)
			if err != nil {
				// timeout is normal
				continue
			}
			// try to detect message type
			var generic map[string]any
			if err := cbor.Unmarshal(frame, &generic); err != nil {
				slog.Warn("invalid cbor frame", "component", "vehicle", "id", v.ID, "error", err)
				continue
			}
			if t, ok := generic["type"].(string); ok {
				switch t {
				case "beacon":
					// respond with hello
					var b model.BeaconMessage
					_ = cbor.Unmarshal(frame, &b)
					hello := model.HelloMessage{VehicleID: v.ID}
					hb, _ := cbor.Marshal(hello)
					_ = v.lora.WriteFrame(hb)
					slog.Info("sent hello to gateway", "component", "vehicle", "id", v.ID, "gateway", b.GatewayID)
				case "auth":
					// receive auth (key)
					var a model.AuthMessage
					_ = cbor.Unmarshal(frame, &a)
					// v.sessionKey = a.Key
					v.leaseExpiry = time.Now().Add(time.Duration(a.TTL) * time.Second)
					slog.Info("received auth and stored session key", "component", "vehicle", "id", v.ID, "ttl", a.TTL)
				default:
					// other types ignored here
				}
			}
		}
	}()

	// renew loop: if we have a key, periodically send a light "renew" (or telemetry) to keep lease alive
	v.wg.Add(1)
	go func() {
		defer v.wg.Done()
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if v.sessionKey != nil && time.Now().Before(v.leaseExpiry) {
					// send a short renew packet (could be telemetry too)
					msg := map[string]any{"type": "renew", "vehicle_id": v.ID}
					b, _ := cbor.Marshal(msg)
					_ = v.lora.WriteFrame(b)
					slog.Debug("sent renew", "component", "vehicle", "id", v.ID)
				} else if v.sessionKey != nil && time.Now().After(v.leaseExpiry) {
					// lease expired locally: drop key
					v.sessionKey = nil
					slog.Info("session expired locally, dropped key", "component", "vehicle", "id", v.ID)
				}
			}
		}
	}()

	return nil
}

// Shutdown stops all goroutines and closes devices safely.
func (v *Vehicle) Shutdown() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.lora != nil {
		if err := v.lora.Close(); err != nil {
			slog.Warn("failed to close LoRa device",
				"component", "vehicle", "id", v.ID, "error", err)
		}
	}
	if v.arduino != nil {
		if err := v.arduino.Close(); err != nil {
			slog.Warn("failed to close Arduino device",
				"component", "vehicle", "id", v.ID, "error", err)
		}
	}
	v.wg.Wait()
	slog.Info("vehicle stopped", "component", "vehicle", "id", v.ID)
}

// sendTelemetry encodes Arduino telemetry as CBOR and writes it via LoRa.
func (v *Vehicle) sendTelemetry(a model.ArduinoData) {
	data := model.VehicleData{
		VehicleID:   v.ID,
		Latitude:    a.Latitude,
		Longitude:   a.Longitude,
		CurrentHead: a.CurrentHead,
		TargetHead:  a.TargetHead,
		LeftSpeed:   a.LeftSpeed,
		RightSpeed:  a.RightSpeed,
	}

	payload, err := cbor.Marshal(data)
	if err != nil {
		slog.Warn("failed to encode telemetry CBOR",
			"component", "vehicle", "id", v.ID, "error", err)
		return
	}
	if v.lora != nil {
		if v.sessionKey != nil {
			frame, err := util.BuildFrame(payload)
			if err != nil {
				slog.Warn("build frame failed", "component", "vehicle", "id", v.ID, "error", err)
				return
			}
			if err := v.lora.WriteBytes(frame); err != nil {
				slog.Warn("failed to send telemetry",
					"component", "vehicle", "id", v.ID, "error", err)
			} else {
				slog.Debug("telemetry sent",
					"component", "vehicle", "id", v.ID)
			}
		} else {
			if err := v.lora.WriteFrame(payload); err != nil {
				slog.Warn("failed to send telemetry",
					"component", "vehicle", "id", v.ID, "error", err)
			} else {
				slog.Debug("telemetry sent",
					"component", "vehicle", "id", v.ID)
			}
		}
	}
}
