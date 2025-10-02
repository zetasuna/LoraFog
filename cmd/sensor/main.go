package main

import (
	"LoraFog/internal/sensor"
	"LoraFog/pkg/lorapkg"
	"encoding/json"
	"log"
	"net"
	"time"

	"github.com/brocaar/lorawan"
)

func main() {
	sensorConfigs := []struct {
		DevAddr  lorawan.DevAddr
		AppSKey  [16]byte
		NwkSKey  [16]byte
		FPort    uint8
		SensorID string
		TypeID   uint
		Interval uint
	}{
		{
			DevAddr:  lorawan.DevAddr{0x01, 0x00, 0x00, 0x01},
			AppSKey:  [16]byte{0x10, 0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F},
			NwkSKey:  [16]byte{0x20, 0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F},
			FPort:    10,
			SensorID: "sensor-1",
			TypeID:   1,
			Interval: 5,
		},
		{
			DevAddr:  lorawan.DevAddr{0x01, 0x00, 0x00, 0x02},
			AppSKey:  [16]byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20},
			NwkSKey:  [16]byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F, 0x30},
			FPort:    10,
			SensorID: "sensor-2",
			TypeID:   2,
			Interval: 5,
		},
		{
			DevAddr:  lorawan.DevAddr{0x01, 0x00, 0x00, 0x03},
			AppSKey:  [16]byte{0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18, 0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20, 0x21},
			NwkSKey:  [16]byte{0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28, 0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F, 0x30, 0x31},
			FPort:    10,
			SensorID: "sensor-3",
			TypeID:   3,
			Interval: 5,
		},
	}

	for _, config := range sensorConfigs {
		ctx := lorapkg.LoRaWANContext{
			DevAddr: config.DevAddr,
			AppSKey: config.AppSKey,
			NwkSKey: config.NwkSKey,
			FPort:   config.FPort,
		}
		s := sensor.NewSensor(config.SensorID, config.TypeID, config.Interval)
		go simulateSensor(ctx, s)
	}

	select {}
}

func simulateSensor(ctx lorapkg.LoRaWANContext, s *sensor.Sensor) {
	addr, err := net.ResolveUDPAddr("udp", "127.0.0.1:10001")
	if err != nil {
		log.Fatal(err)
	}

	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("failed to close connection: %v", err)
		}
	}()

	ticker := time.NewTicker(time.Duration(s.Interval) * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		ctx.FCnt++
		data := s.GenerateData()
		jsonData, err := json.Marshal(data)
		if err != nil {
			log.Println("json marshal error:", err)
			continue
		}

		encoded := lorapkg.Encode(ctx, jsonData)

		tmp, err := conn.Write([]byte(encoded))
		if err != nil {
			// 	log.Println("[Error] Fail to send data:", err)
			// } else {
			// 	log.Printf("[INFO] Data sent: %s\n", encoded)
			// }
			log.Printf("Sensor %s send error to %s: %v", s.ID, conn.RemoteAddr().String(), err)
		} else {
			log.Printf("Sensor %s sent %d bytes to %s", s.ID, tmp, conn.RemoteAddr().String())
		}
	}
}
