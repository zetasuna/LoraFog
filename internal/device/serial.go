// Package device implements a simple wrapper for serial communication.
package device

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"go.bug.st/serial"
)

var ErrSerialTimeout = errors.New("[Serial] Read timeout")

// Serial represents a simple serial port connection.
type Serial struct {
	Port     serial.Port
	Path     string
	BaudRate int
	mu       sync.Mutex
	reader   *bufio.Reader
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
		reader:   bufio.NewReader(port),
	}
	slog.Info("[Serial] Opened port",
		"device", path, "baud", baud)
	return s, nil
}

// ReadLine reads a line of data with an optional timeout (in milliseconds).
func (s *Serial) ReadLine(timeoutMs int) (string, error) {
	if s.Port == nil {
		return "", errors.New("[Serial] Port not initialized")
	}

	if timeoutMs > 0 {
		if err := s.Port.SetReadTimeout(time.Duration(timeoutMs) * time.Millisecond); err != nil {
			slog.Warn("[Serial] Failed to set read timeout",
				"device", s.Path, "error", err)
		}
	}

	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return line, nil
}

// WriteLine writes a single line (with newline terminator) to the serial port.
func (s *Serial) WriteLine(data string) error {
	if s.Port == nil {
		return errors.New("[Serial] Port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.Port.Write([]byte(data + "\n")); err != nil {
		slog.Warn("[Serial] Failed to write to serial",
			"device", s.Path, "error", err)
		return err
	}
	return nil
}

// ReadBytes reads exactly n bytes from the serial port with a timeout.
// Timeout=0 means blocking indefinitely.
func (s *Serial) ReadBytes(n int, timeout time.Duration) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("[Serial] Port not initialized")
	}
	buf := make([]byte, n)

	// s.mu.Lock()
	// defer s.mu.Unlock()

	// 1. Thiết lập timeout cho Port trước khi đọc
	if err := s.Port.SetReadTimeout(timeout); err != nil {
		slog.Warn("[Serial] Failed to set read timeout", "device", s.Path, "error", err)
	}

	// 2. Đọc từ bufio.Reader
	readCount, err := io.ReadFull(s.Port, buf)

	// 3. Reset timeout về blocking (0) sau khi đọc xong
	if timeout != 0 {
		_ = s.Port.SetReadTimeout(0)
	}

	if err != nil {
		// Chuẩn hóa lỗi Timeout
		// Nếu đọc được 0 byte và có lỗi -> Timeout
		if readCount == 0 {
			return nil, ErrSerialTimeout
		}
		// Nếu đọc dở dang (ví dụ cần 2 byte mà mới được 1)
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return nil, ErrSerialTimeout
		}
		return nil, err
	}

	if readCount != n {
		return nil, fmt.Errorf("read incomplete: expected %d, got %d", n, readCount)
	}
	return buf, nil
}

// WriteBytes writes raw binary data to the serial port without newline.
func (s *Serial) WriteBytes(b []byte) error {
	if s.Port == nil {
		return errors.New("[Serial] Port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Port.Write(b); err != nil {
		slog.Warn("[Serial] Failed to write bytes to serial",
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
