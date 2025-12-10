// Package device implements a simple wrapper for serial communication.
package device

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"go.bug.st/serial"
)

var ErrTimeout = errors.New("[Serial] Read timeout")

const (
	Preamble       = 0xAA
	PayloadTimeout = 200 * time.Millisecond
)

// Serial represents a simple serial port connection.
type Serial struct {
	Port     serial.Port
	Path     string
	BaudRate int
}

// NewSerial opens a serial port with the given path and baud rate.
func NewSerial(path string, baud int) (*Serial, error) {
	mode := &serial.Mode{BaudRate: baud}
	port, err := serial.Open(path, mode)
	if err != nil {
		return nil, fmt.Errorf("open serial port %s: %w", path, err)
	}
	s := &Serial{
		Port:     port,
		Path:     path,
		BaudRate: baud,
	}
	slog.Info("[Serial] Opened port",
		"device", path, "baud", baud)
	return s, nil
}

// ReadLine reads a line of data with an optional timeout (in milliseconds).
func (s *Serial) ReadLine(timeout time.Duration) (string, error) {
	if s.Port == nil {
		return "", errors.New("[Serial] Port not initialized")
	}

	if err := s.Port.SetReadTimeout(timeout); err != nil {
		slog.Warn("[Serial] Failed to set read timeout",
			"device", s.Path, "error", err)
	}

	var lineBuf []byte
	oneByte := make([]byte, 1) // Buffer đọc từng byte
	// Đọc byte đầu tiên
	n, err := s.Port.Read(oneByte)
	if err != nil {
		// Hết duration mà không có gì -> Timeout thật sự
		return "", ErrTimeout
	}
	if n == 0 {
		return "", ErrTimeout
	}

	// Đã có dữ liệu! Gom vào buffer
	if oneByte[0] != '\n' && oneByte[0] != '\r' {
		lineBuf = append(lineBuf, oneByte[0])
	} else if oneByte[0] == '\n' {
		return "", nil // Gói tin rỗng chỉ có xuống dòng
	}

	_ = s.Port.SetReadTimeout(PayloadTimeout)

	for {
		// Đọc 1 byte từ cổng Serial
		n, err := s.Port.Read(oneByte)
		if err != nil {
			// Lỗi lúc này nghĩa là đang đọc dở thì đứt quãng (Inter-byte timeout)
			// Tuy nhiên, ta vẫn trả về những gì đã đọc được để thử cứu vớt gói tin
			// Hoặc return error nếu muốn chặt chẽ. Ở đây ta return những gì có.
			if len(lineBuf) > 0 {
				return string(lineBuf), nil
			}
			return "", err
		}

		if n == 0 {
			// Hết timeout inter mà không có byte tiếp theo -> Coi như hết gói
			continue
		}

		char := oneByte[0]

		// 4. Kiểm tra ký tự xuống dòng
		if char == '\n' {
			// Đã tìm thấy kết thúc dòng -> Thành công
			slog.Debug("[Serial] Receive bytes", "count", len(lineBuf))
			return string(lineBuf), nil
		}

		// Gom byte vào buffer (Bỏ qua \r nếu muốn sạch đẹp)
		if char != '\r' {
			lineBuf = append(lineBuf, char)
		}
	}
}

// WriteLine writes a single line (with newline terminator) to the serial port.
func (s *Serial) WriteLine(data string) error {
	if s.Port == nil {
		return errors.New("[Serial] Port not initialized")
	}

	if _, err := s.Port.Write([]byte(data + "\n")); err != nil {
		slog.Warn("[Serial] Failed to write to serial",
			"device", s.Path, "error", err)
		return err
	}
	return nil
}

// ReadBytes reads a packet with format: [Preamble (1b)] [Length (1b)] [Payload (Length bytes)]
// waitTimeout: Thời gian chờ tối đa để bắt được Preamble byte.
func (s *Serial) ReadBytes(timeout time.Duration) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("[Serial] Port not initialized")
	}

	// 1. Cấu hình timeout để chờ Preamble (Timeout dài)
	if err := s.Port.SetReadTimeout(timeout); err != nil {
		return nil, err
	}

	buf := make([]byte, 1)
	startTime := time.Now()

	// Vòng lặp tìm Preamble (để loại bỏ rác nếu có)
	for {
		n, err := s.Port.Read(buf)
		if err != nil {
			return nil, err // Timeout hoặc lỗi hardware
		}
		if n == 0 {
			// Một số thư viện trả về 0 thay vì error khi timeout
			if time.Since(startTime) > timeout {
				return nil, ErrTimeout
			}
			continue
		}

		// Nếu tìm thấy Preamble byte
		if buf[0] == Preamble {
			break
		}
		// Nếu đọc được byte nhưng không phải Preamble -> Rác, bỏ qua và tiếp tục chờ
	}

	// 2. Đã bắt được Preamble -> Chuyển sang Timeout ngắn để đọc nốt gói tin
	// (Tránh việc treo mãi nếu gói tin bị cụt)
	if err := s.Port.SetReadTimeout(PayloadTimeout); err != nil {
		return nil, err
	}

	// 3. Đọc byte độ dài (Length)
	// Dùng io.ReadFull để đảm bảo đọc đủ 1 byte
	if _, err := io.ReadFull(s.Port, buf); err != nil {
		return nil, fmt.Errorf("read length byte failed: %w", err)
	}
	length := int(buf[0])

	if length == 0 {
		return []byte{}, nil // Gói tin rỗng
	}

	// 4. Đọc Payload dựa trên độ dài
	payload := make([]byte, length)
	if _, err := io.ReadFull(s.Port, payload); err != nil {
		return nil, fmt.Errorf("read payload failed (expect %d bytes): %w", length, err)
	}

	slog.Debug("[Serial] Read packet success", "len", length)
	return payload, nil
}

// WriteBytes writes data with format: [Preamble] [Length] [Payload...]
func (s *Serial) WriteBytes(data []byte) error {
	if s.Port == nil {
		return errors.New("[Serial] Port not initialized")
	}

	length := len(data)
	if length > 255 {
		return fmt.Errorf("payload too large for 1-byte length prefix (max 255 bytes)")
	}

	// Tạo buffer: 1 byte Preamble + 1 byte Length + Data
	packet := make([]byte, 2+length)
	packet[0] = Preamble
	packet[1] = byte(length)
	copy(packet[2:], data)

	if _, err := s.Port.Write(packet); err != nil {
		return fmt.Errorf("write error: %w", err)
	}
	return nil
}

// Close closes the serial port safely.
func (s *Serial) Close() error {
	if s.Port == nil {
		return nil
	}
	if err := s.Port.Close(); err != nil {
		slog.Warn("[Serial] Failed to close serial port",
			"device", s.Path, "error", err)
		return err
	}
	slog.Info("[Serial] Closed port", "device", s.Path)
	return nil
}
