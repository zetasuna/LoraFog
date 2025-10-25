// Package util provides utility functions for setting up application-wide logging,
// including timestamped logs with file and line information.
package util

import (
	"log"
	"os"
)

// SetupLogger configures the standard logger with timestamp and short file info.
// It writes logs to stdout.
func SetupLogger() {
	log.SetOutput(os.Stdout)
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("[Logger] Initialized")
}
