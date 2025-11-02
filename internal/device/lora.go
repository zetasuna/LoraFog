// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

// CHANGELOG (refactor v2):
// - Added new binary-based LoraDevice (no simulation, unlike Arduino)
// - Uses Serial.ReadBytes() and WriteBytes() for CBOR payloads
// - Safe close and structured logging (slog)

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/fxamacker/cbor/v2"
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

// ReadFrame reads a binary frame from the LoRa serial interface.
// It expects a 2-byte length prefix followed by that many bytes of CBOR data.
func (l *Lora) ReadFrame() ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("lora serial not initialized")
	}

	header := make([]byte, 2)
	if _, err := io.ReadFull(l.serial.reader, header); err != nil {
		return nil, fmt.Errorf("failed to read frame header: %w", err)
	}

	length := binary.BigEndian.Uint16(header)
	if length == 0 {
		return nil, fmt.Errorf("invalid frame length 0")
	}

	payload, err := l.serial.ReadBytes(int(length))
	if err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}
	return payload, nil
}

// ReadFrameWithTimeout tries to read a CBOR frame with timeout.
func (l *Lora) ReadFrameWithTimeout(timeout time.Duration) ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("lora serial not initialized")
	}

	done := make(chan struct{})
	var result []byte
	var err error

	go func() {
		result, err = l.ReadFrame()
		close(done)
	}()

	select {
	case <-done:
		return result, err
	case <-time.After(timeout):
		return nil, fmt.Errorf("read frame timeout after %s", timeout)
	}
}

// WriteFrame sends a binary CBOR frame with a 2-byte big-endian length prefix.
func (l *Lora) WriteFrame(payload []byte) error {
	if l.serial == nil {
		return fmt.Errorf("lora serial not initialized")
	}
	length := uint16(len(payload))
	frame := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], length)
	copy(frame[2:], payload)
	return l.serial.WriteBytes(frame)
}

func (l *Lora) WriteBytes(b []byte) error {
	return l.serial.WriteBytes(b)
}

// Close safely closes the LoRa serial device.
func (l *Lora) Close() error {
	if l.serial == nil {
		return nil
	}
	return l.serial.Close()
}

// BroadcastBeacon periodically writes a BeaconMessage as a CBOR frame.
// ctx controls lifecycle; interval defines broadcast frequency.
func (l *Lora) BroadcastBeacon(ctx context.Context, interval time.Duration, beacon any) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("stopping beacon broadcast", "component", "lora", "device", l.Path)
			return
		case <-ticker.C:
			b, err := cbor.Marshal(beacon)
			if err != nil {
				slog.Warn("encode beacon failed", "component", "lora", "device", l.Path, "error", err)
				continue
			}
			if err := l.WriteFrame(b); err != nil {
				slog.Warn("write beacon frame failed", "component", "lora", "device", l.Path, "error", err)
				continue
			}
			slog.Debug("beacon broadcasted", "component", "lora", "device", l.Path)
		}
	}
}

// SendAuthRelay sends an AuthMessage CBOR frame (used by Gateway to relay server auth to vehicle).
func (l *Lora) SendAuthRelay(auth any) error {
	b, err := cbor.Marshal(auth)
	if err != nil {
		return err
	}
	return l.WriteFrame(b)
}
