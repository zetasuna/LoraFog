// Command lora runs the LoraFog system orchestrator.

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
	slog.Info("[System] Starting LoraFog runtime", "config", *cfgPath)

	// --- Validate config early ---
	cfg, err := loadAndValidateConfig(*cfgPath)
	if err != nil {
		slog.Error("[System] Invalid configuration", "error", err)
		os.Exit(1)
	}
	_ = cfg // not used directly (System loads internally)

	// --- Create system instance ---
	system, err := core.NewSystem(*cfgPath)
	if err != nil {
		slog.Error("[System] Failed to initialize system", "error", err)
		os.Exit(1)
	}

	// --- Setup signal handling for graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Start all components ---
	if err := system.Start(ctx); err != nil {
		slog.Error("[System] Failed to start", "error", err)
		os.Exit(1)
	}

	// --- Wait for termination signal ---
	slog.Info("[System] Running (press Ctrl+C to exit)")
	<-ctx.Done()

	// --- Graceful shutdown ---
	slog.Info("[System] Shutting down...")
	system.Stop()

	// Allow time for async cleanup/logs
	time.Sleep(500 * time.Millisecond)
	slog.Info("[System] Successfully terminated LoraFog")
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
