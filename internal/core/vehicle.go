package core

import (
	"log"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
)

// Vehicle represents a vehicle agent that reads telemetry and periodically
// sends telemetry via an underlying Device (e.g., LoRa serial).
type Vehicle struct {
	ID            string
	Device        device.Device
	ArduinoDevice *device.ArduinoDevice
	Parser        parser.Parser
	Interval      time.Duration

	stop          chan struct{}
	wg            sync.WaitGroup
	lastTelemetry model.ArduinoData
	lastUpdate    time.Time
	arduinoFn     func()
}

// NewVehicle constructs a Vehicle with given identifiers, device paths and parser.
func NewVehicle(id, loraDev string, loraBaud int, arduinoID string, arduinoDev string, arduinoBaud int, interval time.Duration, p parser.Parser) *Vehicle {
	dev, _ := device.NewSerialDevice(loraDev, loraBaud)
	v := &Vehicle{ID: id, Device: dev, Parser: p, Interval: interval, stop: make(chan struct{})}
	if arduinoDev != "" {
		v.ArduinoDevice = device.NewArduinoDevice(arduinoID, arduinoDev, arduinoBaud)
	}
	return v
}

// Start initializes the vehicle data acquisition and telemetry loop.
// It starts reading Arduino data and immediately sends telemetry upon new data arrival.
// Optionally, it may still include a periodic heartbeat if needed.
func (v *Vehicle) Start() error {
	// Start Arduino reader if available
	if v.ArduinoDevice != nil {
		ch := make(chan model.ArduinoData, 5)

		// Start reading Arduino asynchronously
		stop, err := v.ArduinoDevice.Read(ch)
		if err != nil {
			log.Printf("[vehicle %s] Arduino start err: %v", v.ID, err)
		} else {
			log.Printf("[vehicle %s] Arduino start: success", v.ID)
			v.arduinoFn = stop
			v.wg.Add(1)
			go func() {
				defer v.wg.Done()
				for {
					select {
					case <-v.stop:
						log.Printf("[vehicle %s] Stopping Arduino loop", v.ID)
						return
					case arduinoData, ok := <-ch:
						if !ok {
							log.Printf("[vehicle %s] Arduino channel closed", v.ID)
							return
						}
						// Update last Arduino reading
						v.lastTelemetry = arduinoData
						v.lastUpdate = time.Now()
						v.sendTelemetry()
						log.Printf("[vehicle %s] sended telemetry", v.ID)
					}
				}
			}()
		}
	}

	// (Optional) heartbeat ticker – useful if you still want periodic "alive" message
	if v.Interval > 0 {
		v.wg.Add(1)
		go func() {
			defer v.wg.Done()
			ticker := time.NewTicker(v.Interval)
			defer ticker.Stop()
			for {
				select {
				case <-v.stop:
					log.Printf("[vehicle %s] stopping heartbeat", v.ID)
					return
				case <-ticker.C:
					// Only send heartbeat if no Arduino data for a while
					if time.Since(v.lastUpdate) > v.Interval {
						log.Printf("[vehicle %s] sending heartbeat", v.ID)
						v.sendTelemetry()
					}
				}
			}
		}()
	}

	return nil
}

// Stop stops the vehicle goroutines, Arduino provider and closes the device.
func (v *Vehicle) Stop() {
	// close stop channel (idempotent)
	select {
	case <-v.stop:
		// already closed
	default:
		close(v.stop)
	}
	if v.arduinoFn != nil {
		v.arduinoFn()
	}
	if v.Device != nil {
		_ = v.Device.Close()
	}
	v.wg.Wait()
}

// sendTelemetry builds a VehicleData from last data/fallback values and writes it to the Device.
func (v *Vehicle) sendTelemetry() {
	latitude, longitude := v.lastTelemetry.Latitude, v.lastTelemetry.Longitude
	if latitude == 0 && longitude == 0 {
		// fallback coordinate (Hanoi)
		latitude, longitude = 21.0285, 105.8048
	}
	vd := model.VehicleData{
		VehicleID:   v.ID,
		Latitude:    latitude,
		Longitude:   longitude,
		CurrentHead: v.lastTelemetry.CurrentHead,
		TargetHead:  v.lastTelemetry.CurrentHead,
		LeftSpeed:   v.lastTelemetry.LeftSpeed,
		RightSpeed:  v.lastTelemetry.RightSpeed,
		PID:         1,
	}
	line, err := v.Parser.EncodeTelemetry(vd)
	if err != nil {
		log.Printf("[vehicle %s] encode telemetry err: %v", v.ID, err)
		return
	} else {
		log.Printf("[vehicle %s] encode telemetry: %s", v.ID, line)
	}
	if v.Device != nil {
		if err := v.Device.WriteLine(line); err == nil {
			log.Printf("[vehicle %s] sent telemetry: %s", v.ID, line)
		} else {
			log.Printf("[vehicle %s] lora write err: %v", v.ID, err)
		}
	} else {
		log.Printf("[vehicle %s] device absent; telemetry not sent", v.ID)
	}
}
