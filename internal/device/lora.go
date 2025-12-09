// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// Lora manages binary (CBOR) communication over a serial LoRa interface.
type Lora struct {
	Path   string
	Baud   int
	serial *Serial
}

// NewLora creates a new Lora.
func NewLora(path string, baud int) (*Lora, error) {
	serial, err := NewSerial(path, baud)
	if err != nil {
		slog.Debug("[Lora] Failed to connect Lora device",
			"device", path, "error", err)
		return nil, fmt.Errorf("lora: failed to connect device %s: %w", path, err)
	}

	lora := &Lora{
		Path:   path,
		Baud:   baud,
		serial: serial,
	}
	return lora, nil
}

// ReadLine reads a line from Serial, decodes Base64, and returns raw bytes.
func (l *Lora) ReadLine(timeout time.Duration) ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("[Lora] Serial not initialized")
	}

	// Chuyển đổi duration sang ms
	timeoutMs := int64(timeout / time.Millisecond)

	// 1. Đọc 1 dòng text (đã được tách bởi \n ở tầng Serial)
	line, err := l.serial.ReadLine(timeoutMs)
	if err != nil {
		// Nếu lỗi là timeout nhưng vẫn đọc được chút dữ liệu rác -> trả lỗi Timeout chuẩn
		// Cần check xem thư viện serial trả lỗi gì, thường thì ta map về ErrTimeout
		if len(line) == 0 {
			return nil, ErrTimeout // Định nghĩa biến này ở đâu đó hoặc dùng context.DeadlineExceeded
		}
		return nil, err
	}

	// 2. Làm sạch chuỗi (CỰC KỲ QUAN TRỌNG CHO BASE64)
	cleanLine := strings.TrimSpace(line)

	if len(cleanLine) == 0 {
		return nil, nil // Dòng rỗng (do nhiễu hoặc xuống dòng thừa)
	}

	// 2. Giải mã Base64 -> Raw Bytes (CBOR)
	payload, err := base64.StdEncoding.DecodeString(line)
	if err != nil {
		// Nếu decode lỗi, có thể do nhiễu đường truyền làm hỏng chuỗi Base64
		// slog.Warn("[Lora] Base64 decode error", "line", line, "err", err)
		return nil, fmt.Errorf("corrupted packet (base64 invalid)")
	}

	return payload, nil
}

// WriteLine encodes raw bytes to Base64 and writes as a line.
func (l *Lora) WriteLine(b []byte) error {
	if l.serial == nil {
		return fmt.Errorf("[Lora] Serial not initialized")
	}
	if len(b) == 0 {
		return nil
	}

	// 1. Mã hóa Raw Bytes -> Base64 String
	// Việc này đảm bảo không bao giờ có ký tự \n nằm giữa gói tin
	b64Str := base64.StdEncoding.EncodeToString(b)

	// 2. Gửi chuỗi text xuống Serial (Hàm WriteLine sẽ tự thêm \n)
	return l.serial.WriteLine(b64Str)
}

// Close safely closes the LoRa serial device.
func (l *Lora) Close() error {
	if l.serial == nil {
		return nil
	}
	return l.serial.Close()
}
