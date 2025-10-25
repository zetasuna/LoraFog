// Package core contains the main runtime logic and orchestration layer for the LoraFog system.
// It defines the FogServer, Gateway, Vehicle, and System types that manage their lifecycle.
package core

import (
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

// System manages lifecycle of the main components (FogServer, Gateways, Vehicles).
// It loads configuration from a YAML file and constructs objects accordingly.
type System struct {
	cfgPath  string
	cfg      *model.Config
	parsers  map[string]parser.Parser
	Fog      *FogServer
	Gateways []*Gateway
	Vehicles []*Vehicle
	Arduinos []*device.ArduinoDevice
	SocatMgr *util.SocatManager

	stop      chan struct{}
	wg        sync.WaitGroup
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

	// Virtual Serial Setup
	virtMgr := util.NewSocatManager()
	s.SocatMgr = virtMgr
	for _, pair := range cfg.VirtualSerials.Pairs {
		if err := virtMgr.CreatePair(pair.Left, pair.Right); err != nil {
			log.Printf("[virt-serial] failed to create pair: %v", err)
		}
	}
	time.Sleep(2 * time.Second)

	// register parser formats
	s.parsers["csv"] = parser.NewCSVParser()
	s.parsers["json"] = parser.NewJSONParser()

	// construct FogServer from config
	if cfg.Server.FogAddr != "" {
		// s.Fog = NewFogServer(cfg.Server.FogAddr)
		s.Fog = NewFogServer(cfg.Server.FogAddr, cfg.Server.AppAddr)
		s.Fog.wireFmt = strings.ToLower(cfg.Global.WireFormat)

		for _, gw := range cfg.Server.Gateways {
			s.Fog.RegisterGateway(gw.ID, gw.URL, gw.Vehicles)
			log.Printf("[config] Registered gateway %s (%s) vehicles=%v",
				gw.ID, gw.URL, gw.Vehicles)
		}
	} else {
		log.Println("[config] Fog server disabled (no fog_addr configured)")
	}

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
			gcfg.LoraDev,
			gcfg.LoraBaud,
			gcfg.URL,
			gcfg.FogURL,
			gcfg.WireIn,
			gcfg.WireOut,
			s.parsers[inFmt],
			s.parsers[outFmt],
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
			vcfg.LoraDev,
			vcfg.LoraBaud,
			vcfg.ID,
			vcfg.ArduinoDev,
			vcfg.ArduinoBaud,
			time.Duration(vcfg.TelemetryIntervalMs)*time.Millisecond,
			p,
		)
		s.Vehicles = append(s.Vehicles, veh)
	}

	// construct arduino devices from config
	for _, arduinoCfg := range cfg.Arduinos {
		arduino := device.NewArduinoDevice(arduinoCfg.ID, arduinoCfg.Dev, arduinoCfg.Baud)
		s.Arduinos = append(s.Arduinos, arduino)
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
	s.started = true
	s.stop = make(chan struct{})

	// start fog server
	// go s.Fog.Start()
	if s.Fog != nil {
		log.Printf("[system] Starting fog server at %s ...", s.Fog.Addr)
		go func() {
			if err := s.Fog.Start(); err != nil {
				log.Printf("[system] Fog server error: %v", err)
			}
		}()
	} else {
		log.Println("[system] Fog server is disabled; skipping startup")
	}

	// start gateways and register them to fog registry
	for _, g := range s.Gateways {
		if err := g.Start(); err != nil {
			log.Printf("[gateway %s] start err: %v", g.ID, err)
		} else {
			log.Printf("[gateway %s] start: Success", g.ID)
			// s.Fog.RegisterGateway(g.ID, g.FogURL, g.Vehicles)
		}
	}

	// start vehicle agents
	for _, v := range s.Vehicles {
		if err := v.Start(); err != nil {
			log.Printf("[vehicle %s] start err: %v", v.ID, err)
		} else {
			log.Printf("[vehicle %s] start: Success", v.ID)
		}
	}

	// start arduino simulation
	for _, arduino := range s.Arduinos {
		s.wg.Add(1)
		go func(arduino *device.ArduinoDevice) {
			defer s.wg.Done()
			log.Printf("[system] starting arduino %s device %s (baud %d)", arduino.ID, arduino.Device, arduino.Baud)
			stop := make(chan struct{})
			go func() {
				<-s.stop
				close(stop)
			}()

			if err := arduino.StartSimulation(stop); err != nil {
				log.Printf("[arduino %s] simulate failed: %v", arduino.ID, err)
			} else {
				log.Printf("[arduino %s] simulation stopped", arduino.ID)
			}
		}(arduino)
	}
	return nil
}

// StopAll stops all running components gracefully.
func (s *System) StopAll() {
	s.startLock.Lock()
	defer s.startLock.Unlock()
	if !s.started {
		return
	}
	for _, g := range s.Gateways {
		g.Stop()
	}
	for _, v := range s.Vehicles {
		v.Stop()
	}
	for _, a := range s.Arduinos {
		if err := a.Close(); err != nil {
			log.Printf("[warning] failed to close arduino %s: %v", a.ID, err)
		}
	}
	if s.SocatMgr != nil {
		s.SocatMgr.Cleanup()
	}
	s.Fog.Stop()
	log.Println("[system] stopping all components...")
	close(s.stop)
	s.wg.Wait()
	s.started = false
	log.Println("[system] all components stopped.")
}
