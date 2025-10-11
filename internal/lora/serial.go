// Package lora provides a light wrapper over a serial port used for LoRa E32 modules.
// It reads/writes newline-delimited text lines.
package lora

import (
	"bufio"
	"errors"
	"time"

	serial "go.bug.st/serial"
)

// LoRa wraps serial.Port and a buffered reader.
type LoRa struct {
	port serial.Port
	r    *bufio.Reader
}

// New opens a serial device (e.g. /dev/serial0) with given baudrate.
func New(device string, baud int) (*LoRa, error) {
	p, err := serial.Open(device, &serial.Mode{BaudRate: baud})
	if err != nil {
		return nil, err
	}
	return &LoRa{port: p, r: bufio.NewReader(p)}, nil
}

// ReadLine reads a single line terminated by '\n'. If timeout > 0, it will return after timeout.
func (l *LoRa) ReadLine(timeout time.Duration) (string, error) {
	ch := make(chan struct {
		line string
		err  error
	}, 1)

	// reader goroutine
	go func() {
		line, err := l.r.ReadString('\n')
		if err != nil {
			// convert io.EOF to error for caller
			ch <- struct {
				line string
				err  error
			}{"", err}
			return
		}
		ch <- struct {
			line string
			err  error
		}{line, nil}
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

// WriteLine writes s + '\n' to serial.
func (l *LoRa) WriteLine(s string) error {
	_, err := l.port.Write(append([]byte(s), '\n'))
	return err
}

// Close closes the underlying port and returns error if any.
func (l *LoRa) Close() error {
	if l.port == nil {
		return nil
	}
	return l.port.Close()
}
