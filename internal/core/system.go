// Package core orchestrates system startup and shutdown.
package core

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

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
		_ = sys.socatManager.CreatePair(
			vs.Left,
			vs.Right,
		)
	}
	time.Sleep(300 * time.Millisecond)

	// 1. Mở kết nối DB ở đây (trong main)
	db, err := sql.Open("mysql", "admin:admin@tcp(localhost:3006)/boat")
	if err != nil {
		slog.Error("failed to configure database", "error", err)
		// os.Exit(1)
	}

	// 2. Ping DB để xác nhận kết nối
	if err := db.Ping(); err != nil {
		slog.Error("failed to connect to database", "error", err)
		// os.Exit(1)
	}
	slog.Info("Database connection established")
	if cfg.Server.Addr != "" {
		sys.server = NewServer(
			cfg.Server.Addr,
			cfg.Server.AppAddr,
			db,
		)
	}
	for _, g := range cfg.Gateways {
		sys.gateways = append(sys.gateways, NewGateway(
			g.LoraDev,
			g.LoraBaud,
			g.Addr,
			g.ServerAddr,
		))
	}
	for _, v := range cfg.Vehicles {
		sys.vehicles = append(sys.vehicles, NewVehicle(
			v.ID,
			v.LoraDev,
			v.LoraBaud,
			v.ArduinoDev,
			v.ArduinoBaud,
		))
	}
	for _, a := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos, device.NewArduino(a.Dev, a.Baud))
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

// Shutdown gracefully stops all components.
func (s *System) Shutdown() {
	slog.Info("system shutting down")
	if s.cancel != nil {
		s.cancel()
	}
	for _, gw := range s.gateways {
		gw.Shutdown()
	}
	for _, vh := range s.vehicles {
		vh.Shutdown()
	}
	for _, ino := range s.arduinos {
		_ = ino.Close()
	}
	if s.server != nil {
		_ = s.server.Shutdown()
	}
	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}
	s.wg.Wait()
	slog.Info("shutdown complete")
}
