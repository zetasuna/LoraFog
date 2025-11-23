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

var ErrSerialTimeout = errors.New("serial read timeout")

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
	slog.Info("serial port opened",
		"component", "serial", "path", path, "baud", baud)
	return s, nil
}

// ReadLine reads a line of data with an optional timeout (in milliseconds).
func (s *Serial) ReadLine(timeoutMs int) (string, error) {
	if s.Port == nil {
		return "", errors.New("serial port not initialized")
	}

	if timeoutMs > 0 {
		if err := s.Port.SetReadTimeout(time.Duration(timeoutMs) * time.Millisecond); err != nil {
			slog.Warn("failed to set read timeout",
				"component", "serial", "path", s.Path, "error", err)
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
		return errors.New("serial port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.Port.Write([]byte(data + "\n")); err != nil {
		slog.Warn("failed to write to serial",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	return nil
}

// ReadBytes reads exactly n bytes from the serial port with a timeout.
// Timeout=0 means blocking indefinitely.
func (s *Serial) ReadBytes(n int, timeout time.Duration) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("serial port not initialized")
	}
	buf := make([]byte, n)

	// s.mu.Lock()
	// defer s.mu.Unlock()

	// 1. Thiết lập timeout cho Port trước khi đọc
	if err := s.Port.SetReadTimeout(timeout); err != nil {
		slog.Warn("failed to set read timeout", "component", "serial", "path", s.Path, "error", err)
	}

	// 2. Đọc từ bufio.Reader
	readCount, err := io.ReadFull(s.reader, buf)

	// 3. Reset timeout về blocking (0) sau khi đọc xong
	_ = s.Port.SetReadTimeout(0)

	if err != nil {
		// Chuẩn hóa lỗi Timeout
		if errors.Is(err, ErrSerialTimeout) || errors.Is(err, io.EOF) {
			return nil, ErrSerialTimeout
		}
		if errors.Is(err, io.ErrUnexpectedEOF) && readCount > 0 {
			return nil, fmt.Errorf("read incomplete: %w", err)
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
		return errors.New("serial port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Port.Write(b); err != nil {
		slog.Warn("failed to write bytes to serial",
			"component", "serial", "path", s.Path, "error", err)
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
		slog.Warn("failed to close serial port",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	slog.Info("serial port closed", "component", "serial", "path", s.Path)
	return nil
}
