// Package device implements a GPS device reader using NMEA protocol.
// It supports both real GPS serial reading and simulated output generation.
package device

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"time"

	"LoraFog/internal/model"
	"LoraFog/internal/util"
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
func NewGpsDevice(id string, device string, baud int) *GpsDevice {
	return &GpsDevice{ID: id, Device: device, Baud: baud}
}

// --- Implementation of Device interface ---

// Open opens the GPS serial port.
func (gps *GpsDevice) Open() error {
	if gps.Serial != nil {
		return nil
	}
	serialDevice, err := NewSerialDevice(gps.Device, gps.Baud)
	if err != nil {
		return fmt.Errorf("open gps serial failed: %w", err)
	}
	gps.Serial = serialDevice
	return nil
}

// Close closes the GPS serial port safely.
func (gps *GpsDevice) Close() error {
	if gps.Serial == nil {
		return nil
	}
	err := gps.Serial.Close()
	gps.Serial = nil
	return err
}

// ReadLine reads one NMEA line from the GPS.
func (gps *GpsDevice) ReadLine(timeout time.Duration) (string, error) {
	if gps.Serial == nil {
		return "", errors.New("gps serial not open")
	}
	return gps.Serial.ReadLine(timeout)
}

// WriteLine writes a string to the GPS port (rarely used, but provided for interface compatibility).
func (gps *GpsDevice) WriteLine(dataOut string) error {
	if gps.Serial == nil {
		return errors.New("gps serial not open")
	}
	return gps.Serial.WriteLine(dataOut)
}

// --- Additional functions ---

// Read continuously streams GPS data and pushes parsed coordinates to a channel.
// Returns a stop function to safely terminate the loop.
func (gps *GpsDevice) Read(out chan<- model.GpsData) (func(), error) {
	if err := gps.Open(); err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	go func() {
		defer func() {
			_ = gps.Close()
			close(out)
		}()

		for {
			select {
			case <-stop:
				return
			default:
			}

			dataIn, err := gps.ReadLine(0)
			if err != nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}
			dataIn = strings.TrimSpace(dataIn)
			if !strings.HasPrefix(dataIn, "$GPRMC") && !strings.HasPrefix(dataIn, "$GNRMC") {
				continue
			}
			parts := strings.Split(dataIn, ",")
			if len(parts) < 7 || parts[3] == "" || parts[5] == "" {
				continue
			}
			lat, err1 := util.ParseNMEACoord(parts[3], parts[4])
			lon, err2 := util.ParseNMEACoord(parts[5], parts[6])
			if err1 != nil || err2 != nil {
				log.Printf("[gps] Skip invalid coord: %s", dataIn)
				continue
			}
			out <- model.GpsData{Latitude: lat, Longitude: lon}
		}
	}()
	return func() { close(stop) }, nil
}

// --- Implementation of Simulatable interface ---

// StartSimulation continuously writes fake GPS NMEA sentences to the port until stop is closed.
func (gps *GpsDevice) StartSimulation(stop <-chan struct{}) error {
	if err := gps.Open(); err != nil {
		return err
	}
	defer func() {
		if err := gps.Close(); err != nil {
			log.Printf("[warning] Failed to close gps device: %v", err)
		}
	}()

	fmt.Printf("[gps %s] Simulator started on %s (baud %d)\n", gps.ID, gps.Device, gps.Baud)

	for {
		select {
		case <-stop:
			fmt.Printf("[gps %s] Simulation stopped.\n", gps.ID)
			return nil
		default:
		}

		lat := 21.0285 + (rand.Float64()-0.5)*0.001
		lon := 105.8048 + (rand.Float64()-0.5)*0.001
		latStr, latDir := util.ToNMEACoord(lat, true)
		lonStr, lonDir := util.ToNMEACoord(lon, false)
		timeUTC := time.Now().UTC().Format("150405.00")
		valid := "A"

		nmea := fmt.Sprintf("$GPRMC,%s,%s,%s,%s,%s,%s,3.332,272.24,241025,,,A*69\r\n",
			timeUTC, valid, latStr, latDir, lonStr, lonDir)

		if err := gps.WriteLine(nmea); err != nil {
			log.Printf("[gps %s] simulate write error: %v", gps.ID, err)
		} else {
			log.Printf("[gps %s] simulate write: %s", gps.ID, nmea)
		}
		time.Sleep(1 * time.Second)
	}
}
