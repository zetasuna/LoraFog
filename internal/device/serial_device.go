// Package device implements the SerialDevice type using go.bug.st/serial,
// providing read and write operations for physical serial communication ports.
package device

import (
	"bufio"
	"errors"
	"time"

	serial "go.bug.st/serial"
)

// SerialDevice implements Device using go.bug.st/serial package.
type SerialDevice struct {
	port serial.Port
	r    *bufio.Reader
}

// NewSerialDevice opens a serial device with given path and baudrate.
func NewSerialDevice(dev string, baud int) (*SerialDevice, error) {
	p, err := serial.Open(dev, &serial.Mode{BaudRate: baud})
	if err != nil {
		return nil, err
	}
	return &SerialDevice{port: p, r: bufio.NewReader(p)}, nil
}

// ReadLine reads a single line from the serial port with optional timeout.
func (s *SerialDevice) ReadLine(timeout time.Duration) (string, error) {
	ch := make(chan struct {
		line string
		err  error
	}, 1)

	// Reader goroutine
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

// WriteLine writes a line followed by newline.
func (s *SerialDevice) WriteLine(line string) error {
	_, err := s.port.Write(append([]byte(line), '\n'))
	return err
}

// Close closes the underlying serial port.
func (s *SerialDevice) Close() error {
	if s.port == nil {
		return nil
	}
	return s.port.Close()
}
