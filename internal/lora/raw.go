package lora

import (
	serial "go.bug.st/serial"
)

// OpenRawSerial opens a raw serial.Port for devices like GPS where caller wants raw io.
// provides raw serial.Port which implements io.ReadWriteCloser
func OpenRawSerial(device string, baud int) (serial.Port, error) {
	return serial.Open(device, &serial.Mode{BaudRate: baud})
}
