// Package device defines a unified interface for communication devices such as LoRa or serial ports.
// It abstracts reading and writing line-based data with optional timeouts.
package device

import "time"

// Device defines an abstract interface for communication devices (e.g., LoRa, Serial).
// Implementations can provide ReadLine/WriteLine operations with optional timeout.
type Device interface {
	// ReadLine reads a single line terminated by '\n'.
	// If timeout > 0, it must return after timeout even if no data available.
	ReadLine(timeout time.Duration) (string, error)

	// WriteLine writes s followed by '\n' to the device.
	WriteLine(s string) error

	// Close closes the device and releases underlying resources.
	Close() error
}
