// Package device implements a simple wrapper for serial communication.
// It provides non-blocking read/write methods with optional timeout.
package device

// CHANGELOG (refactor v2):
// - Safe read/write with timeout
// - Added context support via external control
// - Structured logging (slog)
// - Safe Close() checks and standardized naming

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"time"

	"go.bug.st/serial"
)

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

// ReadBytes reads exactly n bytes from the serial port.
// It blocks until all bytes are received or an error occurs.
func (s *Serial) ReadBytes(n int) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("serial port not initialized")
	}
	buf := make([]byte, n)
	total := 0
	for total < n {
		readCount, err := io.ReadFull(s.reader, buf[total:])
		total += readCount
		if err != nil {
			return nil, err
		}
	}
	return buf, nil
}

func (s *Serial) ReadBytesTimeout(n int, timeout time.Duration) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("serial port not initialized")
	}
	deadline := time.Now().Add(timeout)
	buf := make([]byte, 0, n)
	tmp := make([]byte, 128)
	for len(buf) < n && time.Now().Before(deadline) {
		if err := s.Port.SetReadTimeout(time.Until(deadline)); err != nil {
			return nil, fmt.Errorf("SetReadTimeout failed: %w", err)
		}
		r, err := s.reader.Read(tmp)
		if r > 0 {
			if len(buf)+r > n {
				buf = append(buf, tmp[:n-len(buf)]...)
			} else {
				buf = append(buf, tmp[:r]...)
			}
			if len(buf) >= n {
				break
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) || strings.Contains(strings.ToLower(err.Error()), "timeout") {
				continue
			}
			return nil, err
		}
	}
	if len(buf) == 0 {
		return nil, errors.New("serial read timeout")
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
