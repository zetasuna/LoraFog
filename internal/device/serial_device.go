// Package device implements SerialDevice using go.bug.st/serial,
// which provides real serial communication support for devices like LoRa or sensors.
package device

import (
	"bufio"
	"errors"
	"fmt"
	"time"

	serial "go.bug.st/serial"
)

// SerialDevice implements Device using go.bug.st/serial.
type SerialDevice struct {
	port serial.Port
	r    *bufio.Reader
	dev  string
	baud int
}

// NewSerialDevice creates and opens a serial device with the given path and baudrate.
func NewSerialDevice(dev string, baud int) (*SerialDevice, error) {
	p, err := serial.Open(dev, &serial.Mode{BaudRate: baud})
	if err != nil {
		return nil, fmt.Errorf("failed to open serial %s: %w", dev, err)
	}
	return &SerialDevice{port: p, r: bufio.NewReader(p), dev: dev, baud: baud}, nil
}

// Open ensures that the serial port is ready for use.
func (s *SerialDevice) Open() error {
	if s.port != nil {
		return nil
	}
	p, err := serial.Open(s.dev, &serial.Mode{BaudRate: s.baud})
	if err != nil {
		return fmt.Errorf("reopen serial %s failed: %w", s.dev, err)
	}
	s.port = p
	s.r = bufio.NewReader(p)
	return nil
}

// Close closes the underlying serial connection.
func (s *SerialDevice) Close() error {
	if s.port == nil {
		return nil
	}
	err := s.port.Close()
	s.port = nil
	return err
}

// ReadLine reads a single line from the serial port, blocking until newline or timeout.
func (s *SerialDevice) ReadLine(timeout time.Duration) (string, error) {
	if s.port == nil {
		return "", errors.New("serial port not open")
	}

	ch := make(chan struct {
		line string
		err  error
	}, 1)

	go func() {
		line, err := s.r.ReadString('\n')
		ch <- struct {
			line string
			err  error
		}{line, err}
	}()

	if timeout <= 0 {
		res := <-ch
		return res.line, res.err
	}

	select {
	case res := <-ch:
		return res.line, res.err
	case <-time.After(timeout):
		return "", errors.New("read timeout")
	}
}

// WriteLine writes a single line followed by '\n' to the serial port.
func (s *SerialDevice) WriteLine(line string) error {
	if s.port == nil {
		return errors.New("serial port not open")
	}
	_, err := s.port.Write(append([]byte(line), '\n'))
	return err
}
