// Package device defines unified interfaces for all hardware communication devices,
// such as LoRa modules, GPS receivers, or virtual serial ports.
// It abstracts read/write operations and optionally supports simulation for test environments.
package device

import "time"

// Device defines the common behavior of any communication-capable device.
// Implementations must support opening, closing, and line-based read/write.
type Device interface {
	// Open initializes and prepares the device for data communication.
	Open() error

	// Close gracefully closes the device and releases all underlying resources.
	Close() error

	// ReadLine reads a single line terminated by '\n'.
	// If timeout > 0, it must return after the given timeout even if no data arrives.
	ReadLine(timeout time.Duration) (string, error)

	// WriteLine writes a string followed by '\n' to the device.
	WriteLine(s string) error
}

// Simulatable extends Device with the ability to generate mock data.
// It is typically implemented by GPS or sensor devices for testing.
type Simulatable interface {
	Device
	// Simulate generates mock output continuously until stop is closed.
	Simulate(stop <-chan struct{}) error
}
