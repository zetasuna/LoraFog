// Package device implements a simple wrapper for serial communication.
package device

import (
	"errors"
	"fmt"
	"log/slog"
	"time"

	"go.bug.st/serial"
)

var ErrTimeout = errors.New("[Serial] Read timeout")

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
func (s *Serial) ReadLine(timeoutMs int64) (string, error) {
	if s.Port == nil {
		return "", errors.New("[Serial] Port not initialized")
	}

	const packetTimeout = 200 * time.Millisecond
	waitTimeout := time.Duration(timeoutMs) * time.Millisecond
	if err := s.Port.SetReadTimeout(waitTimeout); err != nil {
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

	_ = s.Port.SetReadTimeout(packetTimeout)

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
