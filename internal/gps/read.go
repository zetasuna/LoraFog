// Package gps provides utilities for GPS.
package gps

import (
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"bufio"
	"fmt"
	"log"
	"strings"
	"time"

	serial "go.bug.st/serial"
)

// ReadStream continuously read GPS data from serial
func ReadStream(device string, baud int, out chan<- model.GPSData) error {
	port, err := serial.Open(device, &serial.Mode{BaudRate: baud})
	if err != nil {
		return fmt.Errorf("open serial failed: %w", err)
	}
	defer func() {
		if err := port.Close(); err != nil {
			log.Printf("warning: failed to close serial port: %v", err)
		}
	}()

	reader := bufio.NewReader(port)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			continue
		}
		line = strings.TrimSpace(line)
		log.Printf("RAW GPS: %s", line)

		// Chỉ xử lý câu NMEA hợp lệ
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

		out <- model.GPSData{Lat: lat, Lon: lon}
		log.Printf("gps data: %.6f, %.6f", lat, lon)
	}
}

// ReadGPSFromDevice reads NMEA sentence from GPS serial (device param).
func ReadGPSFromDevice(device string, baud int, timeout time.Duration) (float64, float64, error) {
	// Use go.bug.st/serial directly for GPS reading
	port, err := serial.Open(device, &serial.Mode{BaudRate: baud})
	if err != nil {
		return 0, 0, err
	}
	// ensure close
	defer func() {
		if cerr := port.Close(); cerr != nil {
			log.Printf("warning: close gps serial err: %v", cerr)
		}
	}()

	r := bufio.NewReader(port)
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		line, err := r.ReadString('\n')
		if err != nil {
			continue
		}
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "$GPGGA") || strings.HasPrefix(line, "$GNRMC") {
			parts := strings.Split(line, ",")
			// GPGGA: parts[2]=lat, parts[3]=N/S, parts[4]=lon, parts[5]=E/W
			if len(parts) >= 6 {
				lat, err1 := parser.ParseNMEACoord(parts[2], parts[3])
				lon, err2 := parser.ParseNMEACoord(parts[4], parts[5])
				if err1 == nil && err2 == nil {
					return lat, lon, nil
				}
			}
		}
	}
	return 0, 0, fmt.Errorf("no gps fix")
}
