// Vehicle agent (realistic): reads GPS from serial, sends CSV telemetry via LoRa (/dev/serial0),
// listens for control CSV and replies with ACKs. Resources are properly closed and checked.
package main

import (
	"LoraFog/internal/gps"
	"LoraFog/internal/lora"
	"LoraFog/internal/model"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"
)

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

	// GPS Channel
	gpsCh := make(chan model.GPSData, 10)
	// Goroutine for reading GPS module
	go func() {
		if err := gps.ReadStream(*gpsDev, *gpsBaud, gpsCh); err != nil {
			log.Printf("gps stream error: %v", err)
		}
	}()

	for {
		select {
		case <-stop:
			log.Println("vehicle stopping")
			return
			// case <-ticker.C:
			// 	lat, lon, err := gps.ReadGPSFromDevice(*gpsDev, *gpsBaud, 2*time.Second)
			// 	if err != nil {
			// 		log.Printf("gps read failed: %v; using fallback", err)
			// 		// lat = 21.028511
			// 		// lon = 105.804817
			// 		lat = 21.0285 + (rand.Float64()-0.5)*0.01
			// 		lon = 105.8048 + (rand.Float64()-0.5)*0.01
			// 	}
		case data := <-gpsCh:
			headCur := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
			headTar := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
			// left/right speed from local sensors (not implemented) — placeholder
			left := 12.0
			right := 12.0
			pid := 1.0
			v := fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f,%.2f,%.1f", *vehicleID, data.Lat, data.Lon, headCur, headTar, left, right, pid)
			if err := l.WriteLine(v); err != nil {
				log.Printf("lora write err: %v", err)
			} else {
				log.Printf("sent telemetry: %s", v)
			}
		}
	}
}
