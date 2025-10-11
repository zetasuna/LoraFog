// Package util provides helper functions for logging events
package util

import (
	"fmt"
	"log"
	"time"
)

// Info prints general system information messages with timestamp.
func Info(msg string, args ...any) {
	log.Printf("[INFO] %s | %s", time.Now().Format(time.RFC3339), fmt.Sprintf(msg, args...))
}

// Error prints error messages with timestamp.
func Error(msg string, args ...any) {
	log.Printf("[ERROR] %s | %s", time.Now().Format(time.RFC3339), fmt.Sprintf(msg, args...))
}
