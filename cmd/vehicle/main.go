// Vehicle agent (realistic): reads GPS from serial, sends CSV telemetry via LoRa (/dev/serial0),
// listens for control CSV and replies with ACKs. Resources are properly closed and checked.
package main

import (
	"LoraFog/internal/lora"
	"bufio"
	"flag"
	"fmt"
	"log"
	"math"
	"math/rand/v2"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// parseNMEACoord converts NMEA ddmm.mmmm to decimal degrees.
func parseNMEACoord(value string, dir string) (float64, error) {
	if len(value) < 4 {
		return 0, fmt.Errorf("invalid nmea coord")
	}
	var degPart, minPart string
	// latitude has 2 digit degrees vs lon 3 digits; detect by dir
	if dir == "N" || dir == "S" {
		degPart = value[:2]
		minPart = value[2:]
	} else {
		degPart = value[:3]
		minPart = value[3:]
	}
	deg, err := strconv.ParseFloat(degPart, 64)
	if err != nil {
		return 0, err
	}
	min, err := strconv.ParseFloat(minPart, 64)
	if err != nil {
		return 0, err
	}
	dec := deg + min/60.0
	if dir == "S" || dir == "W" {
		dec = -dec
	}
	return dec, nil
}

// readGPSFromDevice reads NMEA sentence from GPS serial (device param).
func readGPSFromDevice(device string, baud int, timeout time.Duration) (float64, float64, error) {
	// Use go.bug.st/serial directly for GPS reading
	port, err := lora.OpenRawSerial(device, baud)
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
				lat, err1 := parseNMEACoord(parts[2], parts[3])
				lon, err2 := parseNMEACoord(parts[4], parts[5])
				if err1 == nil && err2 == nil {
					return lat, lon, nil
				}
			}
		}
	}
	return 0, 0, fmt.Errorf("no gps fix")
}

func main() {
	vehicleID := flag.String("id", "00001", "vehicle id")
	gpsDev := flag.String("gps", "/dev/ttyS0", "gps serial device")
	gpsBaud := flag.Int("gpsbaud", 9600, "gps baudrate")
	loraDev := flag.String("lora", "/dev/serial0", "lora serial device")
	loraBaud := flag.Int("lorabaud", 9600, "lora baudrate")
	interval := flag.Int("interval", 3000, "telemetry send interval ms")
	flag.Parse()

	// open LoRa serial
	l, err := lora.New(*loraDev, *loraBaud)
	if err != nil {
		log.Fatalf("open lora: %v", err)
	}
	// ensure close on exit
	defer func() {
		if cerr := l.Close(); cerr != nil {
			log.Printf("warning: close lora err: %v", cerr)
		}
	}()

	// graceful shutdown
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	log.Printf("vehicle %s start: lora=%s gps=%s", *vehicleID, *loraDev, *gpsDev)

	// reader for incoming messages
	go func() {
		for {
			line, err := l.ReadLine(0)
			if err != nil {
				// non-fatal: wait and retry
				time.Sleep(100 * time.Millisecond)
				continue
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			// Expect control CSV: VEH_ID,PAYLOAD,MSGID or CTRL,left,right
			parts := strings.Split(line, ",")
			// If payload begins with CTRL -> control
			if len(parts) >= 1 && parts[0] == "CTRL" {
				// CTRL,leftSpeed,rightSpeed
				if len(parts) >= 3 {
					left, _ := strconv.ParseFloat(parts[1], 64)
					right, _ := strconv.ParseFloat(parts[2], 64)
					log.Printf("control cmd: left=%.2f right=%.2f", left, right)
					// TODO: apply to motor driver
				}
				// ack back
				_ = l.WriteLine("ACK,CTRL")
			} else {
				// maybe other format: log/ignore
				log.Printf("received: %s", line)
			}
		}
	}()

	ticker := time.NewTicker(time.Duration(*interval) * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			log.Println("vehicle stopping")
			return
		case <-ticker.C:
			lat, lon, err := readGPSFromDevice(*gpsDev, *gpsBaud, 2*time.Second)
			if err != nil {
				log.Printf("gps read failed: %v; using fallback", err)
				// lat = 21.028511
				// lon = 105.804817
				lat = 21.0285 + (rand.Float64()-0.5)*0.01
				lon = 105.8048 + (rand.Float64()-0.5)*0.01
			}
			headCur := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
			headTar := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
			// left/right speed from local sensors (not implemented) — placeholder
			left := 12.0
			right := 12.0
			pid := 1.0
			v := fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f,%.2f,%.1f", *vehicleID, lat, lon, headCur, headTar, left, right, pid)
			if err := l.WriteLine(v); err != nil {
				log.Printf("lora write err: %v", err)
			} else {
				log.Printf("sent telemetry: %s", v)
			}
		}
	}
}
