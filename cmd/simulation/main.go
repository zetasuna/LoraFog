// Telemetry simulator: writes CSV telemetry lines to the specified serial device.
// Use this for local testing when you don't have real vehicle hardware.
package main

import (
	"LoraFog/internal/lora"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"time"
)

func main() {
	dev := flag.String("dev", "/dev/serial0", "serial device to write telemetry into")
	baud := flag.Int("baud", 9600, "baud rate")
	id := flag.String("id", "VEH_SIM_01", "simulated vehicle id")
	interval := flag.Int("interval", 1000, "ms between messages")
	flag.Parse()

	port, err := lora.New(*dev, *baud)
	if err != nil {
		log.Fatalf("open serial: %v", err)
	}
	defer func() {
		if cerr := port.Close(); cerr != nil {
			log.Printf("warning: close serial err: %v", cerr)
		}
	}()

	log.Printf("simulator sending to %s every %dms", *dev, *interval)
	tick := time.NewTicker(time.Duration(*interval) * time.Millisecond)
	defer tick.Stop()

	for range tick.C {
		lat := 21.0285 + (rand.Float64()-0.5)*0.01
		lon := 105.8048 + (rand.Float64()-0.5)*0.01
		head := rand.Float64() * 360.0
		left := 5.0 + rand.Float64()*20.0
		right := 5.0 + rand.Float64()*20.0
		line := fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f", *id, lat, lon, head, left, right)
		if err := port.WriteLine(line); err != nil {
			log.Printf("write err: %v", err)
		} else {
			log.Printf("sent: %s", line)
		}
	}
}
