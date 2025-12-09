// Package device implements ArduinoDevice for
// reading and writing telemetry over serial
// as well as simulation support.
package device

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"LoraFog/internal/model"
	"LoraFog/internal/util"
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
		slog.Warn(
			"[Arduino] Failed to connect Arduino",
			"device", dev, "error", err,
		)
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
		return nil, fmt.Errorf("[Arduino] Serial not initialized")
	}

	stop := make(chan struct{})
	go func() {
		defer close(dataCh)
		for {
			select {
			case <-stop:
				slog.Info("[Arduino] Stopping read loop", "device", a.Device)
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
				&d.Latitude, &d.Longitude,
				&d.LeftSpeed, &d.RightSpeed,
				&d.CurrentHead, &d.TargetHead,
			); err != nil {
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
		return fmt.Errorf("[Arduino] Serial not initialized")
	}
	return a.serial.WriteLine(line)
}

// Close closes the Arduino serial port safely.
func (a *Arduino) Close() error {
	if a.serial == nil {
		return nil
	}
	return a.serial.Close()
}

// StartSimulation generates synthetic telemetry data periodically for testing.
func (a *Arduino) StartSimulation(stop <-chan struct{}) error {
	// --- Simulation State ---
	const (
		DistanceStop = 5.0
		PIDTimer     = 300 * time.Millisecond
	)
	var (
		latNow  = 21.050299
		lonNow  = 105.826633
		headNow = 0.0 // độ
	)

	// --- Server Command State ---
	var (
		baseSpeed = 1000.0
		targetLat = 21.050295
		targetLon = 105.826633
		Kp        = 0.5
		Ki        = 0.0
		Kd        = 0.0
	)

	var (
		integral float64 = 0
		lastErr  float64 = 0
	)

	readCh := make(chan string, 10)

	// Goroutine để đọc lệnh CSV server gửi xuống
	go func() {
		for {
			line, err := a.serial.ReadLine(0)
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			readCh <- line
		}
	}()

	slog.Info("[Arduino] Starting simulation", "device", a.Device)

	ticker := time.NewTicker(PIDTimer)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			slog.Info("[Arduino] Stopping simulation", "device", a.Device)
			return nil
		case line := <-readCh:
			// EXPECT: speed,lat,lon,Kp,Ki,Kd
			if _, err := fmt.Sscanf(line, "%f,%f,%f,%f,%f,%f",
				&baseSpeed, &targetLat, &targetLon, &Kp, &Ki, &Kd,
			); err == nil {
				slog.Info(
					"[Arduino] Simulation: Received new target",
					"speed", baseSpeed, "lat", targetLat, "lon", targetLon,
				)
			}
		case <-ticker.C:
			// --- 0. Khởi tạo giá trị motor mặc định (Dừng) ---
			left := 1000.0
			right := 1000.0
			// Fix ki vs kd
			Ki = 0.0
			Kd = 0.0

			// --- 1. Tính hướng cần đến ---
			targetHead := util.Bearing(latNow, lonNow, targetLat, targetLon)
			dist := util.DistanceMeters(latNow, lonNow, targetLat, targetLon)
			// LOGIC MỚI: Nếu gần đến đích (< 5m), ép tốc độ về 1000 (Dừng)
			if dist < DistanceStop {
				baseSpeed = 1000.0
			}

			// --- 2. Logic di chuyển ---
			// Chỉ di chuyển và tính PID nếu tốc độ > 1000
			if baseSpeed <= 1000 {
				// Nếu dừng, reset các tham số PID để tránh tích lũy sai số khi đứng yên
				integral = 0
				lastErr = 0
			} else {
				// --- PID Steering ---
				err := util.NormalizeAngle(targetHead - headNow)
				integral += err
				derivative := err - lastErr
				lastErr = err

				turn := Kp*err + Ki*integral + Kd*derivative

				// --- Tính motor ---
				left = baseSpeed + turn
				right = baseSpeed - turn

				left = util.Clamp(left, 1000, 2000)
				right = util.Clamp(right, 1000, 2000)

				// --- Mô phỏng tàu di chuyển một chút ---
				headNow = util.NormalizeAngle(headNow + turn*0.1)
				latNow += (math.Cos(util.Deg2rad(headNow)) * 0.00001)
				lonNow += (math.Sin(util.Deg2rad(headNow)) * 0.00001)
			}

			// --- 3. Gửi dữ liệu như Arduino thật ---
			line := fmt.Sprintf("%.6f,%.6f,%d,%d,%d,%d",
				latNow, lonNow,
				int(left), int(right),
				int(headNow), int(targetHead),
			)

			if err := a.Write(line); err != nil {
				slog.Warn(
					"[Arduino] Failed to write simulated telemetry",
					"device", a.Device, "error", err,
				)
			}
		}
	}
}
