// Command lora runs the LoraFog system orchestrator.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"LoraFog/internal/core"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

func main() {
	// --- Parse CLI flags ---
	cfgPath := flag.String(
		"c",
		"configs/config.yml",
		"Path to YAML configuration file",
	)
	flag.Parse()

	// --- Setup global logger ---
	logConfig := util.LogConfig{
		Level:     slog.LevelDebug, // Log ở mức Debug khi phát triển
		IsJSON:    false,           // Dùng Text cho dễ đọc
		AddSource: true,            // Thêm file:line
	}
	util.SetupLogger(logConfig)
	slog.Info(
		"Logger initialized",
		"component", "main",
		"config", *cfgPath,
	)

	// --- Validate config early ---
	cfg, err := loadConfig(*cfgPath)
	if err != nil {
		slog.Error(
			"Configuration is invalid",
			"component", "main",
			"error", err,
		)
		os.Exit(1)
	}
	_ = cfg // not used directly (System loads internally)

	// --- Create system instance ---
	system, err := core.NewSystem(*cfgPath)
	if err != nil {
		slog.Error(
			"System failed to initialize",
			"component", "main",
			"error", err,
		)
		os.Exit(1)
	}

	// --- Setup signal handling for graceful shutdown ---
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	// --- Start all components ---
	if err := system.Start(ctx); err != nil {
		slog.Error(
			"System failed to start",
			"component", "main",
			"error", err,
		)
		os.Exit(1)
	}

	// --- Wait for termination signal ---
	slog.Info(
		"System is running (press Ctrl+C to exit)",
		"component", "main",
	)
	<-ctx.Done()

	// --- Graceful shutdown ---
	slog.Info(
		"System is shutting down ...",
		"component", "main",
	)
	system.Shutdown()

	// Allow time for async cleanup/logs
	time.Sleep(500 * time.Millisecond)
	slog.Info(
		"System terminated successfully",
		"component", "main",
	)
}

// loadAndValidateConfig loads the YAML file and validates it.
func loadConfig(path string) (*model.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}
