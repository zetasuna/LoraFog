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
		slog.Warn("[Arduino] Failed to connect Arduino",
			"device", dev, "error", err)
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
				slog.Info("[Arduino] Stopping read loop",
					"device", a.Device)
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
	var (
		latNow  = 21.027000
		lonNow  = 105.835000
		headNow = 0.0 // độ
	)

	// --- Server Command State ---
	var (
		baseSpeed = 1000.0
		targetLat = 21.0532
		targetLon = 105.8261
		Kp        = 0.0
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

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			slog.Info("[Arduino] Stopping simulation",
				"device", a.Device)
			return nil
		case line := <-readCh:
			// EXPECT: speed,lat,lon,Kp,Ki,Kd
			if _, err := fmt.Sscanf(line, "%f,%f,%f,%f,%f,%f",
				&baseSpeed, &targetLat, &targetLon, &Kp, &Ki, &Kd,
			); err == nil {
				slog.Info("[Arduino] Simulation: Received new target",
					"speed", baseSpeed, "lat", targetLat, "lon", targetLon)
			}
		case <-ticker.C:
			// --- 1. Tính hướng cần đến ---
			targetHead := bearing(latNow, lonNow, targetLat, targetLon)

			// --- 2. PID Steering ---
			err := normalizeAngle(targetHead - headNow)
			integral += err
			derivative := err - lastErr
			lastErr = err

			turn := Kp*err + Ki*integral + Kd*derivative

			// --- 3. Tính motor ---
			left := baseSpeed + turn
			right := baseSpeed - turn

			left = clamp(left, 1000, 2000)
			right = clamp(right, 1000, 2000)

			// --- 4. Mô phỏng tàu di chuyển một chút ---
			headNow = normalizeAngle(headNow + turn*0.1)
			latNow += (math.Cos(deg2rad(headNow)) * 0.00001)
			lonNow += (math.Sin(deg2rad(headNow)) * 0.00001)

			// --- 5. Gửi dữ liệu như Arduino thật ---
			line := fmt.Sprintf("%.6f,%.6f,%d,%d,%d,%d",
				latNow, lonNow,
				int(left), int(right),
				int(headNow), int(targetHead),
			)

			if err := a.Write(line); err != nil {
				slog.Warn("[Arduino] Failed to write simulated telemetry",
					"device", a.Device, "error", err)
			}
		}
	}
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	rad1 := deg2rad(lat1)
	rad2 := deg2rad(lat2)
	delta := deg2rad(lon2 - lon1)

	y := math.Sin(delta) * math.Cos(rad2)
	x := math.Cos(rad1)*math.Sin(rad2) -
		math.Sin(rad1)*math.Cos(rad2)*math.Cos(delta)

	ans := math.Atan2(y, x)
	return normalizeAngle(rad2deg(ans))
}

func deg2rad(d float64) float64 { return d * math.Pi / 180 }
func rad2deg(r float64) float64 { return r * 180 / math.Pi }

func normalizeAngle(a float64) float64 {
	a = math.Mod(a+360, 360)
	if a < 0 {
		a += 360
	}
	return a
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}
