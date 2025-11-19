// Package util provides small utilities used by the system.
package util

import (
	"log/slog"
	"os"
)

// SetupLogger configures the global slog logger.
// Call once in main prior to starting System.
func SetupLogger() {
	// Use default handler (console) but include time and source.
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("logger initialized", "component", "util")
}
