// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"
)

// Magic bytes: 0x4C 0x52 ('L', 'R')
const (
	MagicByte1 = 0x4C
	MagicByte2 = 0x52
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

// Read tìm kiếm Magic Bytes để đồng bộ, sau đó đọc Frame
func (l *Lora) Read(timeout time.Duration) ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("[Lora] Serial not initialized")
	}

	start := time.Now()

	// --- GIAI ĐOẠN 1: TÌM KIẾM MAGIC BYTES (SYNC) ---
	// Chúng ta đọc từng byte một cho đến khi khớp Header 0x4C 0x52
	// Việc này giúp bỏ qua toàn bộ log rác (như chữ "register", "info"...)

	syncState := 0 // 0: Tìm byte 1, 1: Tìm byte 2

	buf1 := make([]byte, 1)

	for {
		// Kiểm tra tổng thời gian timeout
		if time.Since(start) > timeout {
			return nil, ErrTimeout
		}

		// Đọc 1 byte với timeout ngắn (để check liên tục)
		// Timeout nhỏ cho mỗi byte giúp loop phản ứng nhanh
		n, err := l.serial.Port.Read(buf1)
		if err != nil {
			// Nếu lỗi không phải timeout/EOF thì return
			// Nhưng thường ta cứ continue để cố gắng sync
			continue
		}
		if n == 0 {
			continue // Chưa có dữ liệu
		}

		b := buf1[0]

		if syncState == 0 {
			if b == MagicByte1 {
				syncState = 1 // Tìm thấy 'L', tìm tiếp 'R'
			}
		} else if syncState == 1 {
			if b == MagicByte2 {
				// Đã tìm thấy 'L' và 'R' liên tiếp -> SYNCED!
				break
			} else {
				// Nếu byte này là 'L', có thể nó là bắt đầu mới
				if b == MagicByte1 {
					syncState = 1
				} else {
					syncState = 0 // Reset, tìm lại từ đầu
				}
			}
		}
	}

	// --- GIAI ĐOẠN 2: ĐỌC LENGTH VÀ PAYLOAD ---
	// Tính thời gian còn lại
	elapsed := time.Since(start)
	remaining := timeout - elapsed
	if remaining < 10*time.Millisecond {
		return nil, ErrTimeout
	}

	// Đọc 2 byte độ dài
	header, err := l.serial.ReadBytes(2, remaining)
	if err != nil {
		return nil, fmt.Errorf("failed to read length: %w", err)
	}

	length := binary.BigEndian.Uint16(header)

	// Kiểm tra độ dài hợp lý (ví dụ max 256 byte)
	if length == 0 || length > 256 {
		// Nếu độ dài vô lý, có thể là sync sai (giả mạo), thoát ra
		return nil, fmt.Errorf("invalid frame length %d", length)
	}

	// Cập nhật lại thời gian còn lại
	elapsed = time.Since(start)
	remaining = timeout - elapsed
	if remaining < 10*time.Millisecond {
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
	if len(b) == 0 || len(b) > 0xFFFF {
		return fmt.Errorf("invalid payload size %d", len(b))
	}

	// Frame Format: [Magic1][Magic2][LenHigh][LenLow][Payload...]
	// Tổng cộng overhead = 4 bytes
	frame := make([]byte, 4+len(b))

	frame[0] = MagicByte1
	frame[1] = MagicByte2
	binary.BigEndian.PutUint16(frame[2:4], uint16(len(b)))
	copy(frame[4:], b)

	return l.serial.WriteBytes(frame)
}

// Close safely closes the LoRa serial device.
func (l *Lora) Close() error {
	if l.serial == nil {
		return nil
	}
	return l.serial.Close()
}
