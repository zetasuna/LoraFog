// Command lora runs the LoraFog system orchestrator.
//
// CHANGELOG (refactor v2):
// - Unified startup with structured logging and graceful shutdown
// - Integrated util.SetupLogger()
// - Uses context cancellation on SIGINT/SIGTERM
// - Validates YAML configuration before launch
// - Safe close sequence for System
// - Clean exit codes and consistent logs

package main

import (
	"context"
	"errors"
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
	cfgPath := flag.String("c", "configs/config.yml", "Path to YAML configuration file")
	flag.Parse()

	// --- Setup global logger ---
	util.SetupLogger()
	slog.Info("starting LoraFog runtime", "component", "main", "config", *cfgPath)

	// --- Validate config early ---
	cfg, err := loadAndValidateConfig(*cfgPath)
	if err != nil {
		slog.Error("invalid configuration", "component", "main", "error", err)
		os.Exit(1)
	}
	_ = cfg // not used directly (System loads internally)

	// --- Create system instance ---
	system, err := core.NewSystem(*cfgPath)
	if err != nil {
		slog.Error("failed to initialize system", "component", "main", "error", err)
		os.Exit(1)
	}

	// --- Setup signal handling for graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Start all components ---
	if err := system.Start(ctx); err != nil {
		slog.Error("system start failed", "component", "main", "error", err)
		os.Exit(1)
	}

	// --- Wait for termination signal ---
	slog.Info("system running (press Ctrl+C to exit)", "component", "main")
	<-ctx.Done()

	// --- Graceful shutdown ---
	slog.Info("shutting down system...", "component", "main")
	system.Stop()

	// Allow time for async cleanup/logs
	time.Sleep(500 * time.Millisecond)
	slog.Info("LoraFog terminated successfully", "component", "main")
}

// loadAndValidateConfig loads the YAML file and validates it.
func loadAndValidateConfig(path string) (*model.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, errors.New("configuration invalid: " + err.Error())
	}
	return &cfg, nil
}
