// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

import (
	"encoding/base64"
	"fmt"
	"log/slog"
	"time"
)

const (
	MaxPacketSize = 255
	HeaderSize    = 2
	MagicByte     = 0x42
)

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

	if len(line) == 0 {
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

// Read tìm kiếm Magic Bytes để đồng bộ, sau đó đọc Frame
func (l *Lora) Read(timeout time.Duration) ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("[Lora] Serial not initialized")
	}

	start := time.Now()
	readTimeout := 50 * time.Millisecond

	// --- GIAI ĐOẠN 1: TÌM KIẾM MAGIC BYTES (SYNC) ---
	for {
		remaining := timeout - time.Since(start)
		if remaining < readTimeout {
			return nil, ErrTimeout
		}

		b, err := l.serial.ReadBytes(1, readTimeout)
		if err != nil {
			return nil, err
		}

		if b[0] == MagicByte {
			break // sync OK
		}

		// Nếu sai → bỏ byte và tiếp tục tìm MAGIC
	}
	// --- GIAI ĐOẠN 2: ĐỌC LENGTH VÀ PAYLOAD ---
	// Tính thời gian còn lại
	remaining := timeout - time.Since(start)
	if remaining < readTimeout {
		return nil, ErrTimeout
	}

	// Đọc 1 byte độ dài
	header, err := l.serial.ReadBytes(1, readTimeout)
	if err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	length := int(header[0])

	// Kiểm tra độ dài hợp lý (ví dụ max 256 byte)
	if length == 0 || length > MaxPacketSize {
		// Nếu độ dài vô lý, có thể là sync sai (giả mạo), thoát ra
		return nil, fmt.Errorf("invalid frame length %d", length)
	}

	// Cập nhật lại thời gian còn lại
	remaining = timeout - time.Since(start)
	if remaining < readTimeout {
		return nil, ErrTimeout
	}

	// Đọc Payload
	payload, err := l.serial.ReadBytes(int(length), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read payload: %w", err)
	}

	return payload, nil
}

func (l *Lora) Write(b []byte) error {
	if l.serial == nil {
		return fmt.Errorf("[Lora] Serial not initialized")
	}
	if len(b) == 0 || len(b) > MaxPacketSize {
		return fmt.Errorf("invalid payload size %d", len(b))
	}

	// Frame Format: [Magic][Length][[Payload...]
	// Tổng cộng overhead = 2 bytes
	frame := make([]byte, HeaderSize+len(b))

	frame[0] = MagicByte
	frame[1] = byte(len(b))
	copy(frame[2:], b)

	return l.serial.WriteBytes(frame)
}

// Close safely closes the LoRa serial device.
func (l *Lora) Close() error {
	if l.serial == nil {
		return nil
	}
	return l.serial.Close()
}
