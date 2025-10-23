package core

import (
	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"log"
	"math"
	"sync"
	"time"
)

// Vehicle represents a vehicle agent that reads GPS data and periodically
// sends telemetry via an underlying Device (e.g., LoRa serial).
type Vehicle struct {
	ID        string
	Device    device.Device
	GpsDevice *device.GpsDevice
	Parser    parser.Parser
	Interval  time.Duration

	stop       chan struct{}
	wg         sync.WaitGroup
	last       model.GpsData
	lastUpdate time.Time
	gpsFn      func()
}

// NewVehicle constructs a Vehicle with given identifiers, device paths and parser.
// It will attempt to open the LoRa serial device; if GpsDevice is provided a GPS device is created.
func NewVehicle(id, loraDev string, loraBaud int, gpsID string, gpsDev string, gpsBaud int, interval time.Duration, p parser.Parser) *Vehicle {
	dev, _ := device.NewSerialDevice(loraDev, loraBaud)
	v := &Vehicle{ID: id, Device: dev, Parser: p, Interval: interval, stop: make(chan struct{})}
	if gpsDev != "" {
		v.GpsDevice = device.NewGpsDevice(gpsID, gpsDev, gpsBaud)
	}
	return v
}

// Start initializes the vehicle data acquisition and telemetry loop.
// It starts reading GPS data and immediately sends telemetry upon new data arrival.
// Optionally, it may still include a periodic heartbeat if needed.
func (v *Vehicle) Start() error {
	// Start GPS reader if available
	if v.GpsDevice != nil {
		ch := make(chan model.GpsData, 5)

		// Start reading GPS asynchronously
		stop, err := v.GpsDevice.Read(ch)
		if err != nil {
			log.Printf("[vehicle %s] gps start err: %v", v.ID, err)
		} else {
			log.Printf("[vehicle %s] gps start: success", v.ID)
			v.gpsFn = stop
			v.wg.Add(1)
			go func() {
				defer v.wg.Done()
				for {
					select {
					case <-v.stop:
						log.Printf("[vehicle %s] stopping GPS loop", v.ID)
						return
					case g, ok := <-ch:
						if !ok {
							log.Printf("[vehicle %s] gps channel closed", v.ID)
							return
						}
						// Update last GPS reading
						v.last = g
						v.lastUpdate = time.Now()
						// Immediately send telemetry when new GPS data arrives
						v.sendTelemetry()
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
					// Only send heartbeat if no GPS data for a while
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

// Stop stops the vehicle goroutines, GPS provider and closes the device.
func (v *Vehicle) Stop() {
	// close stop channel (idempotent)
	select {
	case <-v.stop:
		// already closed
	default:
		close(v.stop)
	}
	if v.gpsFn != nil {
		v.gpsFn()
	}
	if v.Device != nil {
		_ = v.Device.Close()
	}
	v.wg.Wait()
}

// sendTelemetry builds a VehicleData from last GPS/fallback values and writes it to the Device.
func (v *Vehicle) sendTelemetry() {
	lat, lon := v.last.Lat, v.last.Lon
	if lat == 0 && lon == 0 {
		// fallback coordinate (Hanoi)
		lat, lon = 21.0285, 105.8048
	}
	headCur := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
	headTar := headCur
	vd := model.VehicleData{
		VehicleID: v.ID,
		Lat:       lat,
		Lon:       lon,
		HeadCur:   headCur,
		HeadTar:   headTar,
		LeftSpd:   100,
		RightSpd:  100,
		PID:       1.0,
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
