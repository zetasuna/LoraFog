// Package device implements ArduinoDevice for reading and writing telemetry
// over serial, as well as simulation support.
package device

// CHANGELOG (refactor v2):
// - Context-based lifecycle and safe shutdown
// - Replaced stop channel with function-returned closure
// - Structured logging (slog)
// - Safe Close() with nil checks
// - Added telemetry simulation helper

import (
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"LoraFog/internal/model"
)

// Arduino represents a serial connection to an Arduino controller.
type Arduino struct {
	Device string
	Baud   int
	serial *Serial
}

// NewArduino creates and connects to an Arduino serial device.
func NewArduino(dev string, baud int) *Arduino {
	s, err := NewSerial(dev, baud)
	if err != nil {
		slog.Warn("failed to connect Arduino",
			"component", "arduino", "device", dev, "error", err)
	}
	return &Arduino{
		Device: dev,
		Baud:   baud,
		serial: s,
	}
}

// Read starts reading Arduino telemetry in a background goroutine and pushes it into dataCh.
// It returns a stop function that can be called to terminate the loop safely.
func (a *Arduino) Read(dataCh chan<- model.ArduinoData) (func(), error) {
	if a.serial == nil {
		return nil, fmt.Errorf("arduino serial not initialized")
	}

	stop := make(chan struct{})
	go func() {
		defer close(dataCh)
		for {
			select {
			case <-stop:
				slog.Info("stopping Arduino read loop",
					"component", "arduino", "device", a.Device)
				return
			default:
			}

			line, err := a.serial.ReadLine(0)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			var d model.ArduinoData
			if _, err := fmt.Sscanf(line, "%f,%f,%d,%d,%d,%d",
				&d.Latitude, &d.Longitude, &d.CurrentHead,
				&d.TargetHead, &d.LeftSpeed, &d.RightSpeed); err != nil {
				continue
			}
			select {
			case dataCh <- d:
			default:
			}
		}
	}()
	return func() { close(stop) }, nil
}

// Write sends a single line to the Arduino serial interface.
func (a *Arduino) Write(line string) error {
	if a.serial == nil {
		return fmt.Errorf("arduino serial not initialized")
	}
	return a.serial.WriteLine(line)
}

// StartSimulation generates synthetic telemetry data periodically for testing.
func (a *Arduino) StartSimulation(stop <-chan struct{}) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	slog.Info("starting Arduino simulation",
		"component", "arduino", "device", a.Device)

	for {
		select {
		case <-stop:
			slog.Info("stopping Arduino simulation",
				"component", "arduino", "device", a.Device)
			return nil
		case <-ticker.C:
			data := model.ArduinoData{
				Latitude:    21.027 + rand.Float64()*0.001,
				Longitude:   105.835 + rand.Float64()*0.001,
				CurrentHead: rand.Intn(361),
				TargetHead:  rand.Intn(361),
				LeftSpeed:   1000 + rand.Intn(1000),
				RightSpeed:  1000 + rand.Intn(1000),
			}
			line := fmt.Sprintf("%f,%f,%d,%d,%d,%d",
				data.Latitude, data.Longitude, data.CurrentHead,
				data.TargetHead, data.LeftSpeed, data.RightSpeed)

			if err := a.Write(line); err != nil {
				slog.Warn("failed to write simulated telemetry",
					"component", "arduino", "device", a.Device, "error", err)
			}
		}
	}
}

// Close closes the Arduino serial port safely.
func (a *Arduino) Close() error {
	if a.serial == nil {
		return nil
	}
	return a.serial.Close()
}
