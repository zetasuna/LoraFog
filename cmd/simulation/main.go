// Telemetry simulator: writes CSV telemetry lines to the specified serial device.
// Use this for local testing when you don't have real vehicle hardware.
package main

import (
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"time"

	"go.bug.st/serial"
)

func main() {
	device := flag.String("device", "/tmp/ttyV0", "serial device to write telemetry into")
	baud := flag.Int("baud", 9600, "baud rate")
	flag.Parse()
	port, err := serial.Open(*device, &serial.Mode{BaudRate: *baud})
	if err != nil {
		fmt.Printf("failed to open serial port %s: %v\n", port, err)
		os.Exit(1)
	}
	defer func() {
		if err := port.Close(); err != nil {
			log.Printf("warning: failed to close serial port: %v", err)
		}
	}()

	fmt.Printf("✅ GPS simulator started on %s (baud %d)\n", *device, *baud)

	for {
		// Giả lập vị trí Hà Nội (gần Hồ Gươm)
		lat := 21.0285 + (rand.Float64()-0.5)*0.001
		lon := 105.8048 + (rand.Float64()-0.5)*0.001
		// timeUTC := time.Now().UTC().Format("150405.00")

		// Chuỗi NMEA $GPGGA đơn giản
		nmea := fmt.Sprintf("$GPGGA,%.4f,N,%.4f,E,1,08,0.9,10.0,M,0.0,M,,*47\r\n",
			lat, lon)

		_, err = port.Write([]byte(nmea))
		if err != nil {
			fmt.Printf("write error: %v\n", err)
		} else {
			fmt.Printf("sent: %s", nmea)
		}

		time.Sleep(1 * time.Second)
	}
}
