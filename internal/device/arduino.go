// Package device implements an Arduino serial reader,
// which exchanges telemetry data such as motor speed and heading.
package device

import (
	"bufio"
	"errors"
	"fmt"
	"log"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"LoraFog/internal/model"
)

// ArduinoDevice represents a serial-connected Arduino
// that transmits telemetry data (latitude, longitude, motor speeds, etc.).
type ArduinoDevice struct {
	ID     string
	Device string
	Baud   int
	Serial *SerialDevice
}

// NewArduinoDevice creates a new Arduino device handler.
func NewArduinoDevice(id, device string, baud int) *ArduinoDevice {
	return &ArduinoDevice{ID: id, Device: device, Baud: baud}
}

// --- Implementation of Device interface ---

// Open initializes the Arduino serial connection.
func (arduino *ArduinoDevice) Open() error {
	if arduino.Serial != nil {
		return nil
	}
	serialDevice, err := NewSerialDevice(arduino.Device, arduino.Baud)
	if err != nil {
		return fmt.Errorf("open arduino serial failed: %w", err)
	}
	arduino.Serial = serialDevice
	return nil
}

// Close terminates the serial connection safely.
func (arduino *ArduinoDevice) Close() error {
	if arduino.Serial == nil {
		return nil
	}
	err := arduino.Serial.Close()
	arduino.Serial = nil
	return err
}

// ReadLine reads a single line of data from the Arduino.
func (arduino *ArduinoDevice) ReadLine(timeout time.Duration) (string, error) {
	if arduino.Serial == nil {
		return "", errors.New("arduino serial not open")
	}
	return arduino.Serial.ReadLine(timeout)
}

// WriteLine writes a command or message to the Arduino.
func (arduino *ArduinoDevice) WriteLine(line string) error {
	if arduino.Serial == nil {
		return errors.New("arduino serial not open")
	}
	return arduino.Serial.WriteLine(line)
}

// --- Additional behavior ---

// Read continuously parses JSON telemetry sent from the Arduino and pushes it into the channel.
// Each line is expected to contain a valid JSON object of type ArduinoData.
func (arduino *ArduinoDevice) Read(out chan<- model.ArduinoData) (func(), error) {
	if err := arduino.Open(); err != nil {
		return nil, err
	}

	stop := make(chan struct{})
	go func() {
		defer func() {
			_ = arduino.Close()
			close(out)
		}()

		reader := bufio.NewReader(arduino.Serial.port)
		for {
			select {
			case <-stop:
				return
			default:
			}

			dataIn, err := reader.ReadString('\n')
			if err != nil {
				time.Sleep(200 * time.Millisecond)
				continue
			}

			dataIn = strings.TrimSpace(dataIn)
			if dataIn == "" {
				continue
			}

			parts := strings.Split(dataIn, ",")
			if len(parts) > 5 {
				continue
			}
			latitude, _ := strconv.ParseFloat(parts[0], 64)
			longitude, _ := strconv.ParseFloat(parts[1], 64)
			leftSpeed, _ := strconv.ParseFloat(parts[2], 64)
			rightSpeed, _ := strconv.ParseFloat(parts[3], 64)
			currentHead, _ := strconv.ParseFloat(parts[4], 64)
			out <- model.ArduinoData{
				Latitude:    latitude,
				Longitude:   longitude,
				LeftSpeed:   int(leftSpeed),
				RightSpeed:  int(rightSpeed),
				CurrentHead: int(currentHead),
			}
		}
	}()
	return func() { close(stop) }, nil
}

// StartSimulation generates fake Arduino telemetry for testing.
// It writes mock JSON data over the serial interface until stop is closed.
func (arduino *ArduinoDevice) StartSimulation(stop <-chan struct{}) error {
	if err := arduino.Open(); err != nil {
		return err
	}
	defer func() {
		if err := arduino.Close(); err != nil {
			log.Printf("[warning] Failed to close arduino device: %v", err)
		}
	}()

	fmt.Printf("[arduino %s] Simulator started on %s (baud %d)\n", arduino.ID, arduino.Device, arduino.Baud)

	for {
		select {
		case <-stop:
			fmt.Printf("[arduino %s] Simulation stopped.\n", arduino.ID)
			return nil
		default:
		}

		arduinoData := model.ArduinoData{
			Latitude:    21.0285 + (rand.Float64()-0.5)*0.001,
			Longitude:   105.8048 + (rand.Float64()-0.5)*0.001,
			LeftSpeed:   1000,
			RightSpeed:  1000,
			CurrentHead: 0 + (rand.Intn(361)),
		}

		message := fmt.Sprintf("%.6f,%.6f,%d,%d,%d",
			arduinoData.Latitude, arduinoData.Longitude, arduinoData.LeftSpeed, arduinoData.RightSpeed, arduinoData.CurrentHead)
		if err := arduino.WriteLine(message); err != nil {
			log.Printf("[arduino %s] simulate write error: %v", arduino.ID, err)
		} else {
			log.Printf("[arduino %s] simulate write: %s", arduino.ID, message)
		}

		time.Sleep(1 * time.Second)
	}
}
