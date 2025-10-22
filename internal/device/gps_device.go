// Package device implements a GPS device reader using NMEA protocol.
// It supports both real GPS serial reading and simulated output generation.
package device

import (
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"
)

// GpsDevice implements both Device and Simulatable interfaces.
// It can read real NMEA data from a serial GPS receiver or simulate GPS output for testing.
type GpsDevice struct {
	ID     string
	Device string
	Baud   int
	Serial *SerialDevice
}

// NewGpsDevice creates a new GPS device based on serial communication.
func NewGpsDevice(id string, dev string, baud int) *GpsDevice {
	return &GpsDevice{ID: id, Device: dev, Baud: baud}
}

// --- Implementation of Device interface ---

// Open opens the GPS serial port.
func (g *GpsDevice) Open() error {
	if g.Serial != nil {
		return nil
	}
	sd, err := NewSerialDevice(g.Device, g.Baud)
	if err != nil {
		return fmt.Errorf("open gps serial failed: %w", err)
	}
	g.Serial = sd
	return nil
}

// Close closes the GPS serial port safely.
func (g *GpsDevice) Close() error {
	if g.Serial == nil {
		return nil
	}
	err := g.Serial.Close()
	g.Serial = nil
	return err
}

// ReadLine reads one NMEA line from the GPS.
func (g *GpsDevice) ReadLine(timeout time.Duration) (string, error) {
	if g.Serial == nil {
		return "", errors.New("gps serial not open")
	}
	return g.Serial.ReadLine(timeout)
}

// WriteLine writes a string to the GPS port (rarely used, but provided for interface compatibility).
func (g *GpsDevice) WriteLine(s string) error {
	if g.Serial == nil {
		return errors.New("gps serial not open")
	}
	return g.Serial.WriteLine(s)
}

// --- Additional functions ---

// Read continuously streams GPS data and pushes parsed coordinates to a channel.
// Returns a stop function to safely terminate the loop.
func (g *GpsDevice) Read(out chan<- model.GpsData) (func(), error) {
	if err := g.Open(); err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	go func() {
		defer func() {
			_ = g.Close()
			close(out)
		}()

		for {
			select {
			case <-stop:
				return
			default:
			}

			line, err := g.ReadLine(0)
			if err != nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			line = strings.TrimSpace(line)
			if !strings.HasPrefix(line, "$GPGGA") && !strings.HasPrefix(line, "$GNRMC") {
				continue
			}
			parts := strings.Split(line, ",")
			if len(parts) < 6 {
				continue
			}
			lat, err1 := parser.ParseNMEACoord(parts[2], parts[3])
			lon, err2 := parser.ParseNMEACoord(parts[4], parts[5])
			if err1 != nil || err2 != nil {
				continue
			}
			out <- model.GpsData{Lat: lat, Lon: lon}
		}
	}()
	return func() { close(stop) }, nil
}

// --- Implementation of Simulatable interface ---

// Simulate continuously writes fake GPS NMEA sentences to the port until stop is closed.
func (g *GpsDevice) Simulate(stop <-chan struct{}) error {
	if err := g.Open(); err != nil {
		return err
	}
	defer func() {
		if err := g.Close(); err != nil {
			log.Printf("warning: failed to close gps device: %v", err)
		}
	}()

	fmt.Printf("GPS simulator started on %s (baud %d)\n", g.Device, g.Baud)

	for {
		select {
		case <-stop:
			fmt.Println("GPS simulation stopped.")
			return nil
		default:
		}

		lat := 21.0285 + (rand.Float64()-0.5)*0.001
		lon := 105.8048 + (rand.Float64()-0.5)*0.001
		latStr, latDir := parser.ToNMEACoord(lat, true)
		lonStr, lonDir := parser.ToNMEACoord(lon, false)
		timeUTC := time.Now().UTC().Format("150405.00")

		nmea := fmt.Sprintf("$GPGGA,%s,%s,%s,%s,%s,1,08,0.9,10.0,M,0.0,M,,*47\r\n",
			timeUTC, latStr, latDir, lonStr, lonDir)

		if err := g.WriteLine(nmea); err != nil {
			log.Printf("GPS simulate write error: %v", err)
		}
		time.Sleep(2 * time.Second)
	}
}
