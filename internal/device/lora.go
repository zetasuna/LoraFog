// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

import (
	"fmt"
	"log/slog"
	"time"
)

// Lora manages binary (CBOR) communication over a serial LoRa interface.
type Lora struct {
	Path   string
	Baud   int
	serial *Serial
}

// NewLora creates a new Lora.
func NewLora(path string, baud int) *Lora {
	s, err := NewSerial(path, baud)
	if err != nil {
		slog.Warn("failed to connect Lora device",
			"component", "lora", "device", path, "error", err)
	}
	return &Lora{
		Path:   path,
		Baud:   baud,
		serial: s,
	}
}

// ReadFrames continuously parses complete frames from serial input.
func (l *Lora) ReadFrames(timeout time.Duration) ([][]byte, error) {
	var frames [][]byte
	buf := make([]byte, 0, 512)
	deadline := time.Now().Add(timeout)

	for time.Now().Before(deadline) {
		chunk, _ := l.serial.ReadBytesTimeout(16, 50*time.Millisecond) // read chunk (non-blocking)
		if len(chunk) == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		buf = append(buf, chunk...)

		for len(buf) >= 2 {
			if buf[0] != 0xAA {
				// discard noise until preamble
				buf = buf[1:]
				continue
			}
			length := int(buf[1])
			if len(buf) < length {
				break // wait for more data
			}
			frame := buf[:length]
			frames = append(frames, frame)
			buf = buf[length:] // move to next potential frame
		}
	}
	return frames, nil
}

// WriteBytes writes a full frame to the serial device.
func (l *Lora) WriteBytes(b []byte) error {
	if l.serial == nil {
		return fmt.Errorf("lora serial not initialized")
	}
	return l.serial.WriteBytes(b)
}

// Close safely closes the LoRa serial device.
func (l *Lora) Close() error {
	if l.serial == nil {
		return nil
	}
	return l.serial.Close()
}
