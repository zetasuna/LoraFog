// Package core orchestrates system startup and shutdown.
package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"LoraFog/internal/database"
	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

// System manages Fog server, gateways, and vehicles.
type System struct {
	configPath   string
	config       *model.Config
	server       *Server
	gateways     []*Gateway
	vehicles     []*Vehicle
	arduinos     []*device.Arduino
	serverDB     *database.ServerDB
	socatManager *util.SocatManager

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSystem constructs a System from YAML configuration.
func NewSystem(path string) (*System, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	sys := &System{
		configPath:   path,
		config:       &cfg,
		socatManager: util.NewSocatManager(),
	}

	for _, vs := range cfg.VirtualSerials {
		if err := sys.socatManager.CreatePair(
			vs.Left,
			vs.Right,
		); err != nil {
			slog.Error("[System] Failed to create socat pair", "left", vs.Left, "right", vs.Right, "error", err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	if cfg.Server.Address != "" {
		// 1. Mở kết nối DB ở đây (trong main)
		dsn := "admin:admin@tcp(localhost:3306)/boat_db"
		sys.serverDB, err = database.NewServerDB(dsn, 10, 5)
		if err != nil {
			slog.Error("[System] Database established fail", "error", err)
		} else {
			slog.Info("[System] Database established success")
		}
		sys.server = NewServer(
			cfg.Server.Address,
			cfg.Server.AppAddress,
			sys.serverDB,
		)
	}
	for _, g := range cfg.Gateways {
		sys.gateways = append(sys.gateways, NewGateway(
			g.Address,
			g.ServerAddress,
			g.LoraDevice,
			g.LoraBaud,
		))
	}
	for _, v := range cfg.Vehicles {
		sys.vehicles = append(sys.vehicles, NewVehicle(
			v.VehicleID,
			v.LoraDevice,
			v.LoraBaud,
			v.ArduinoDevice,
			v.ArduinoBaud,
		))
	}
	for _, a := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos, device.NewArduino(a.Device, a.Baud))
	}

	return sys, nil
}

// Start launches all system components.
func (s *System) Start(ctx context.Context) error {
	if s.server == nil && len(s.gateways) == 0 && len(s.vehicles) == 0 {
		return fmt.Errorf("no active components")
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	if s.server != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_ = s.server.Start(ctx)
		}()
	}
	for _, gw := range s.gateways {
		_ = gw.Start(ctx)
	}
	for _, vh := range s.vehicles {
		_ = vh.Start(ctx)
	}
	for _, ino := range s.arduinos {
		s.wg.Add(1)
		go func(ino *device.Arduino) {
			defer s.wg.Done()
			stop := make(chan struct{})
			go func() {
				<-ctx.Done()
				close(stop)
			}()
			_ = ino.StartSimulation(stop)
		}(ino)
	}
	return nil
}

// Stop gracefully stops all components.
func (s *System) Stop() {
	slog.Info("[System] Stopping...")
	if s.cancel != nil {
		s.cancel()
	}
	for _, gw := range s.gateways {
		gw.Stop()
	}
	for _, vh := range s.vehicles {
		vh.Stop()
	}
	for _, ino := range s.arduinos {
		_ = ino.Close()
	}
	// if s.server != nil {
	// 	_ = s.server.Stop()
	// }
	if s.serverDB != nil {
		_ = s.serverDB.Close()
	}
	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}
	s.wg.Wait()
	slog.Info("[System] Shutdown complete")
}
