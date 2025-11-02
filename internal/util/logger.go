// Package util provides small utilities used by the system.
package util

// CHANGELOG (refactor v2):
// - Centralized slog setup for structured logging
// - Exported SetupLogger for callers to initialize global log behavior

import (
	"log/slog"
	"os"
)

// SetupLogger configures the global slog logger.
// Call once in main prior to starting System.
func SetupLogger() {
	// Use default handler (console) but include time and source.
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("logger initialized", "component", "util")
}
