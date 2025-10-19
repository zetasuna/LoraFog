// Package core contains the main runtime logic and orchestration layer for the LoraFog system.
// It defines the FogServer, Gateway, Vehicle, and System types that manage their lifecycle.
package core

import (
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"log"
	"os"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

// System manages lifecycle of the main components (FogServer, Gateways, Vehicles).
// It loads configuration from a YAML file and constructs objects accordingly.
type System struct {
	cfgPath  string
	cfg      *model.Config
	parsers  map[string]parser.Parser
	Gateways []*Gateway
	Vehicles []*Vehicle
	Fog      *FogServer

	started   bool
	startLock sync.Mutex
}

// NewSystem reads the YAML configuration at cfgPath and creates a System instance.
// It also registers available parsers (csv/json) and constructs Gateway and Vehicle objects.
func NewSystem(cfgPath string) (*System, error) {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, err
	}
	var cfg model.Config
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return nil, err
	}

	s := &System{
		cfgPath: cfgPath,
		cfg:     &cfg,
		parsers: make(map[string]parser.Parser),
	}

	// register parser formats
	s.parsers["csv"] = parser.NewCSVParser()
	s.parsers["json"] = parser.NewJSONParser()

	// construct FogServer with configured address or default
	fogAddr := cfg.Global.FogAddr
	if fogAddr == "" {
		fogAddr = ":10000"
	}
	s.Fog = NewFogServer(fogAddr)

	// construct gateways from config
	for _, gcfg := range cfg.Gateways {
		inFmt := gcfg.WireIn
		if inFmt == "" {
			inFmt = cfg.Global.WireFormat
		}
		outFmt := gcfg.WireOut
		if outFmt == "" {
			outFmt = cfg.Global.WireFormat
		}
		gw := NewGateway(
			gcfg.ID,
			gcfg.LoRaDev,
			gcfg.LoRaBaud,
			s.parsers[inFmt],
			s.parsers[outFmt],
			gcfg.FogURL,
			gcfg.Vehicles,
		)
		s.Gateways = append(s.Gateways, gw)
	}

	// construct vehicles from config
	for _, vcfg := range cfg.Vehicles {
		wf := vcfg.WireFormat
		if wf == "" {
			wf = cfg.Global.WireFormat
		}
		p := s.parsers[wf]
		veh := NewVehicle(
			vcfg.ID,
			vcfg.LoRaDev,
			vcfg.LoRaBaud,
			vcfg.GPSDev,
			vcfg.GPSBaud,
			time.Duration(vcfg.TelemetryIntervalMs)*time.Millisecond,
			p,
		)
		s.Vehicles = append(s.Vehicles, veh)
	}
	return s, nil
}

// StartAll starts the FogServer, all Gateways and all Vehicles concurrently.
// It registers gateways to the FogServer registry when a gateway is successfully started.
func (s *System) StartAll() error {
	s.startLock.Lock()
	defer s.startLock.Unlock()
	if s.started {
		return nil
	}
	// start fog server in background
	go s.Fog.Start()

	// start gateways and register them to fog registry
	for _, g := range s.Gateways {
		if err := g.Start(); err != nil {
			log.Printf("gateway %s start err: %v", g.ID, err)
		} else {
			s.Fog.RegisterGateway(g.ID, g.FogURL, g.Vehicles)
		}
	}

	// start vehicle agents
	for _, v := range s.Vehicles {
		if err := v.Start(); err != nil {
			log.Printf("vehicle %s start err: %v", v.ID, err)
		}
	}
	s.started = true
	return nil
}

// StopAll stops all running components gracefully.
func (s *System) StopAll() {
	s.startLock.Lock()
	defer s.startLock.Unlock()
	if !s.started {
		return
	}
	for _, v := range s.Vehicles {
		v.Stop()
	}
	for _, g := range s.Gateways {
		g.Stop()
	}
	s.Fog.Stop()
	s.started = false
}
