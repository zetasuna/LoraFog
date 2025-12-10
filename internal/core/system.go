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

// System manages server, gateways, and vehicles.
type System struct {
	configPath   string
	config       *model.Config
	server       *Server
	gateways     []*Gateway
	vehicles     []*Vehicle
	arduinos     []*device.Arduino
	serverDB     *database.ServerDB
	socatManager *util.SocatManager
	hubs         []*util.LoRaHub

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
		time.Sleep(50 * time.Millisecond)
	}

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

	sys.CreateVirtualSystem()

	for _, a := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos, device.NewArduino(a.Device, a.Baud))
	}

	for _, v := range cfg.Vehicles {
		vehicle, err := NewVehicle(
			v.VehicleID,
			v.LoraDevice,
			v.LoraBaud,
			v.ArduinoDevice,
			v.ArduinoBaud,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to init vehicle %s: %w", v.VehicleID, err)
		}
		sys.vehicles = append(sys.vehicles, vehicle)
	}

	for _, g := range cfg.Gateways {
		gateway, err := NewGateway(
			g.Address,
			g.ServerAddress,
			g.LoraDevice,
			g.LoraBaud,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to init gateway %s: %w", g.Address, err)
		}
		sys.gateways = append(sys.gateways, gateway)
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

	for _, vh := range s.vehicles {
		_ = vh.Start(ctx)
	}

	for _, gw := range s.gateways {
		_ = gw.Start(ctx)
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
	if s.serverDB != nil {
		_ = s.serverDB.Close()
	}
	for _, hub := range s.hubs {
		if hub != nil {
			_ = hub.Close()
		}
	}
	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}
	s.wg.Wait()
	slog.Info("[System] Shutdown complete")
}

func (s *System) CreateVirtualSystem() {
	// --- NEW FEATURE: Virtual Gateway + Multiple Virtual Vehicles ---
	if s.config.Server.VirtualGateways > 0 &&
		s.config.Server.VehiclesPerGateway > 0 {

		for gwIndex := 1; gwIndex <= s.config.Server.VirtualGateways; gwIndex++ {
			// 1. Tạo một BUS LoRa chung cho Gateway này
			//    Gateway LoRa <-> Bus <-> Vehicles LoRa
			busName := fmt.Sprintf("/tmp/lora_hub_%d.sock", gwIndex)
			hub, err := s.socatManager.CreateHub(busName)
			if err != nil {
				slog.Error("[System] Failed to create LoRaHub", "bus", busName, "error", err)
				continue
			}
			s.hubs = append(s.hubs, hub)
			time.Sleep(50 * time.Millisecond)

			// 2. Tạo M Vehicle ảo
			for vIndex := 1; vIndex <= s.config.Server.VehiclesPerGateway; vIndex++ {
				vehicleID := fmt.Sprintf("V-%d-%d", gwIndex, vIndex)
				// LoRa device của vehicle kết nối vào BUS
				vLoRa := fmt.Sprintf("/tmp/vh_lora_%d_%d", gwIndex, vIndex)
				if err := s.socatManager.CreateConnector(vLoRa, busName); err != nil {
					slog.Error("[System] Vehicle cannot connect LoRa", "device", vLoRa)
					continue
				}

				// Arduino simulator
				vArduinoR := fmt.Sprintf("/tmp/vh_arduino_R_%d_%d", gwIndex, vIndex)
				vArduinoS := fmt.Sprintf("/tmp/vh_arduino_S_%d_%d", gwIndex, vIndex)
				if err := s.socatManager.CreatePair(vArduinoR, vArduinoS); err != nil {
					slog.Error("[System] Vehicle Arduino pair failed", "receive", vArduinoR, "send", vArduinoS)
					continue
				}
				time.Sleep(50 * time.Millisecond)

				s.arduinos = append(s.arduinos, device.NewArduino(vArduinoS, 9600))

				veh, err := NewVehicle(
					vehicleID,
					vLoRa, // LoRa vtty
					9600,
					vArduinoR, // Arduino vtty
					9600,
				)
				if err != nil {
					slog.Error("[System] Cannot create virtual vehicle", "id", vehicleID)
					continue
				}
				s.vehicles = append(s.vehicles, veh)
			}

			// 3. Tạo Gateway ảo
			gwLoRa := fmt.Sprintf("/tmp/gw_lora_%d", gwIndex) // gateway vtty
			if err := s.socatManager.CreateConnector(gwLoRa, busName); err != nil {
				slog.Error("[System] Failed to connect gateway to BUS", "gw", gwLoRa, "bus", busName)
			}
			time.Sleep(50 * time.Millisecond)

			gw, err := NewGateway(
				fmt.Sprintf("127.0.0.1:%d", 11000+gwIndex),
				s.config.Server.Address,
				gwLoRa,
				9600,
			)
			if err != nil {
				slog.Error("[System] Unable to create virtual GW", "index", gwIndex)
				continue
			}
			s.gateways = append(s.gateways, gw)
		}
	}
}
