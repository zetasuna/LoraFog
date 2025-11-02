// Package core implements the orchestration and communication logic for the LoraFog edge system.
package core

// CHANGELOG (refactor v2):
// - Removed parser layer and wire_format configuration
// - Unified goroutine lifecycle via context.Context
// - Introduced structured logging using log/slog
// - Renamed symbols to follow Go naming conventions (Start, Shutdown, socatManager, etc.)
// - Added nil-safe Close checks to suppress unchecked warnings
// - Improved documentation and function clarity for maintainability

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

// System manages the lifecycle of FogServer, Gateways, Vehicles, and Arduino devices.
type System struct {
	configPath   string
	config       *model.Config
	server       *Server
	gateways     []*Gateway
	vehicles     []*Vehicle
	arduinos     []*device.Arduino
	socatManager *util.SocatManager

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSystem loads configuration from YAML and constructs the runtime components.
func NewSystem(configPath string) (*System, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, err
	}

	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	sys := &System{
		configPath:   configPath,
		config:       &cfg,
		socatManager: util.NewSocatManager(),
	}

	// Initialize virtual serial pairs if configured.
	for _, pair := range cfg.VirtualSerials {
		if err := sys.socatManager.CreatePair(pair.Left, pair.Right); err != nil {
			slog.Warn("failed to create virtual serial pair",
				"left", pair.Left, "right", pair.Right, "error", err)
		}
	}
	time.Sleep(2 * time.Second)

	// Construct Server
	if cfg.Server.Addr != "" {
		sys.server = NewServer(cfg.Server.Addr, cfg.Server.AppAddr)
		// for _, gr := range cfg.Server.Gateways {
		// 	sys.server.RegisterGateway(gr.ID, gr.Addr, gr.Vehicles)
		// 	slog.Info("registered gateway",
		// 		"component", "fog",
		// 		"id", gr.ID,
		// 		"url", gr.Addr,
		// 		"vehicles", gr.Vehicles,
		// 	)
		// }
	}

	// Construct Gateways
	for _, gwCfg := range cfg.Gateways {
		gw := NewGateway(
			gwCfg.ID,
			gwCfg.LoraDev,
			gwCfg.LoraBaud,
			gwCfg.Addr,
			gwCfg.ServerAddr,
			// gwCfg.Vehicles,
		)
		sys.gateways = append(sys.gateways, gw)
	}

	// Construct Vehicles
	for _, vhCfg := range cfg.Vehicles {
		vh := NewVehicle(
			vhCfg.ID,
			vhCfg.LoraDev,
			vhCfg.LoraBaud,
			vhCfg.ArduinoDev,
			vhCfg.ArduinoBaud,
		)
		sys.vehicles = append(sys.vehicles, vh)
	}

	// Construct Arduino simulators
	for _, arCfg := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos,
			device.NewArduino(arCfg.Dev, arCfg.Baud))
	}

	return sys, nil
}

// Start launches all active components under a shared context.
func (s *System) Start(ctx context.Context) error {
	if s.server == nil && len(s.gateways) == 0 && len(s.vehicles) == 0 {
		return errors.New("no components to start")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Start FogServer
	if s.server != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			slog.Info("starting fog server",
				"component", "fog", "addr", s.server.Addr)
			if err := s.server.Start(ctx); err != nil {
				slog.Error("fog server stopped with error",
					"component", "fog", "error", err)
			}
		}()
	}

	// Start Gateways
	for _, gw := range s.gateways {
		if err := gw.Start(ctx); err != nil {
			slog.Warn("gateway start failed",
				"component", "gateway", "id", gw.ID, "error", err)
		} else {
			slog.Info("gateway started",
				"component", "gateway", "id", gw.ID)
		}
	}

	// Start Vehicles
	for _, vh := range s.vehicles {
		if err := vh.Start(ctx); err != nil {
			slog.Warn("vehicle start failed",
				"component", "vehicle", "id", vh.ID, "error", err)
		} else {
			slog.Info("vehicle started",
				"component", "vehicle", "id", vh.ID)
		}
	}

	// Start Arduino simulations
	for _, ar := range s.arduinos {
		s.wg.Add(1)
		go func(ar *device.Arduino) {
			defer s.wg.Done()
			slog.Info("starting arduino simulation",
				"component", "arduino", "device", ar.Device)
			stop := make(chan struct{})
			go func() {
				<-ctx.Done()
				close(stop)
			}()
			if err := ar.StartSimulation(stop); err != nil {
				slog.Error("arduino simulation failed",
					"component", "arduino", "device", ar.Device, "error", err)
			}
		}(ar)
	}

	return nil
}

// Shutdown gracefully stops all components and cleans up resources.
func (s *System) Shutdown() {
	if s.cancel != nil {
		slog.Info("initiating system shutdown", "component", "system")
		s.cancel()
	}

	for _, gw := range s.gateways {
		gw.Shutdown()
	}
	for _, vh := range s.vehicles {
		vh.Shutdown()
	}
	for _, ar := range s.arduinos {
		if ar != nil {
			if err := ar.Close(); err != nil {
				slog.Warn("failed to close arduino",
					"component", "arduino", "device", ar.Device, "error", err)
			}
		}
	}

	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}

	s.wg.Wait()
	slog.Info("system shutdown complete", "component", "system")
}
