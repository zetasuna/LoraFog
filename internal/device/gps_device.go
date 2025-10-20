// Package device provides an abstraction for reading GPS data from serial devices.
// It parses NMEA sentences and outputs latitude/longitude updates via channels.
package device

import (
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"bufio"
	"fmt"
	"log"
	"math/rand/v2"
	"strings"
	"time"

	serial "go.bug.st/serial"
)

// GpsDevice continuously reads GPS data from serial and sends parsed coordinates to channel.
type GpsDevice struct {
	Device string
	Baud   int
}

// NewSerialGpsDevice creates a GPS device using a serial device path and baudrate.
func NewSerialGpsDevice(dev string, baud int) *GpsDevice {
	return &GpsDevice{Device: dev, Baud: baud}
}

// Read begins reading NMEA sentences and sending valid coordinates to the provided channel.
// Returns a stop function that signals the reading loop to exit.
func (p *GpsDevice) Read(out chan<- model.GpsData) (func(), error) {
	port, err := serial.Open(p.Device, &serial.Mode{BaudRate: p.Baud})
	if err != nil {
		return nil, fmt.Errorf("open gps serial failed: %w", err)
	}
	stop := make(chan struct{})
	go func() {
		defer func() {
			if err := port.Close(); err != nil {
				log.Printf("warning: close gps serial: %v", err)
			}
			close(out)
		}()
		reader := bufio.NewReader(port)
		for {
			select {
			case <-stop:
				return
			default:
			}
			line, err := reader.ReadString('\n')
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

func (p *GpsDevice) WriteSimulation() error {
	port, err := serial.Open(p.Device, &serial.Mode{BaudRate: p.Baud})
	if err != nil {
		return fmt.Errorf("open gps serial failed: %w", err)
	}

	defer func() {
		if err := port.Close(); err != nil {
			log.Printf("warning: failed to close serial port: %v", err)
		}
	}()

	fmt.Printf("GPS simulator started on %s (baud %d)\n", p.Device, p.Baud)

	for {
		// Giả lập vị trí Hà Nội (gần Hồ Gươm)
		lat := 21.0285 + (rand.Float64()-0.5)*0.001
		lon := 105.8048 + (rand.Float64()-0.5)*0.001
		latStr, latDir := parser.ToNMEACoord(lat, true)
		lonStr, lonDir := parser.ToNMEACoord(lon, false)
		timeUTC := time.Now().UTC().Format("150405.00")

		// Chuỗi NMEA $GPGGA đơn giản
		// nmea := fmt.Sprintf("$GPGGA,%.4f,N,%.4f,E,1,08,0.9,10.0,M,0.0,M,,*47\r\n",lat, lon)
		nmea := fmt.Sprintf("$GPGGA,%s,%s,%s,%s,%s,1,08,0.9,10.0,M,0.0,M,,*47\r\n",
			timeUTC, latStr, latDir, lonStr, lonDir)

		_, err = port.Write([]byte(nmea))
		if err != nil {
			fmt.Printf("write error: %v\n", err)
		} else {
			fmt.Printf("sent: %s", nmea)
		}

		time.Sleep(2 * time.Second)
	}
}
