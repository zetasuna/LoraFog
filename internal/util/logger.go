// Package util provides small utilities used by the system.
package util

import (
	"log/slog"
	"os"
)

type LogConfig struct {
	Level     slog.Level // Mức log (Debug, Info, Warn, Error)
	IsJSON    bool       // true để dùng JSON, false để dùng Text
	AddSource bool       // Có thêm thông tin file:line hay không
}

// SetupLogger configures the global slog logger.
// Call once in main prior to starting System.
func SetupLogger(logConfig LogConfig) *slog.Logger {
	opts := &slog.HandlerOptions{
		AddSource: logConfig.AddSource,
		Level:     logConfig.Level,
	}
	var handler slog.Handler
	if logConfig.IsJSON {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}
	logger := slog.New(handler)
	slog.SetDefault(logger)
	return logger
}
