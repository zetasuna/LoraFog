// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrLoraTimeout được dùng khi không nhận được frame trong thời gian quy định
var ErrLoraTimeout = errors.New("[Lora] Read timeout")

// Lora manages binary (CBOR) communication over a serial LoRa interface.
type Lora struct {
	Path   string
	Baud   int
	serial *Serial
}

// NewLora creates a new Lora.
func NewLora(path string, baud int) *Lora {
	serial, err := NewSerial(path, baud)
	if err != nil {
		slog.Warn("[Lora] Failed to connect Lora device",
			"device", path, "error", err)
	}
	return &Lora{
		Path:   path,
		Baud:   baud,
		serial: serial,
	}
}

// Read tries to read a CBOR frame with timeout.
// Timeout=0 means blocking indefinitely.
// (Logic goroutine/select đã được loại bỏ)
func (l *Lora) Read(timeout time.Duration) ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("[Lora] Serial not initialized")
	}

	// 1. Đọc 2 bytes header (độ dài), sử dụng timeout
	// (Chú ý: Đã thay thế io.ReadFull trên reader bằng l.serial.ReadBytes)
	header, err := l.serial.ReadBytes(2, timeout)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint16(header)
	if length == 0 || length > 512 { // Thêm check giới hạn kích thước
		return nil, fmt.Errorf("invalid frame length %d", length)
	}

	// 2. Đọc payload. Sử dụng timeout=0 (blocking) vì đã đọc được header,
	// ta muốn đợi đủ payload.
	payload, err := l.serial.ReadBytes(int(length), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}
	return payload, nil
}

func (l *Lora) Write(b []byte) error {
	if l.serial == nil {
		return fmt.Errorf("[Lora] Serial not initialized")
	}
	// prefix with 2-byte big endian length
	if len(b) == 0 || len(b) > 0xFFFF {
		return fmt.Errorf("invalid payload size %d", len(b))
	}
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(b)))

	// write header then payload
	if err := l.serial.WriteBytes(header); err != nil {
		return err
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
