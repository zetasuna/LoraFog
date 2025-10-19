package core

import (
	"LoraFog/internal/device"
	"LoraFog/internal/parser"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"
)

// Gateway represents a LoRa gateway instance that reads wire lines from a Device,
// decodes using InParser, re-encodes using OutParser and forwards to FogServer.
type Gateway struct {
	ID        string
	Device    device.Device
	InParser  parser.Parser
	OutParser parser.Parser
	FogURL    string
	Vehicles  []string
	stop      chan struct{}
	wg        sync.WaitGroup
}

// NewGateway constructs a Gateway with device path and parsers.
// If opening the serial device fails, the Device field may be nil and Start will be a no-op.
func NewGateway(id, devPath string, baud int, in parser.Parser, out parser.Parser, fogURL string, vehicles []string) *Gateway {
	dev, err := device.NewSerialDevice(devPath, baud)
	if err != nil {
		// log but continue: user may run gateway without physical device (e.g., test)
		log.Printf("gateway %s: open serial failed: %v", id, err)
	}
	return &Gateway{
		ID:        id,
		Device:    dev,
		InParser:  in,
		OutParser: out,
		FogURL:    fogURL,
		Vehicles:  vehicles,
		stop:      make(chan struct{}),
	}
}

// Start begins the gateway read/forward loop in a background goroutine.
// Returns nil even if the underlying device is nil (no-op for testing).
func (g *Gateway) Start() error {
	if g.Device == nil {
		// nothing to start (allow headless/testing)
		return nil
	}
	g.wg.Add(1)
	go g.loop()
	return nil
}

// loop continuously reads lines from the Device, decodes, re-encodes and posts to Fog.
func (g *Gateway) loop() {
	defer g.wg.Done()
	for {
		select {
		case <-g.stop:
			return
		default:
		}
		line, err := g.Device.ReadLine(0)
		if err != nil {
			// transient error: wait and continue
			time.Sleep(100 * time.Millisecond)
			continue
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Decode input using InParser
		vd, err := g.InParser.DecodeTelemetry(line)
		if err != nil {
			log.Printf("gateway %s decode err: %v (%s)", g.ID, err, line)
			continue
		}
		// Encode for Fog using OutParser
		out, err := g.OutParser.EncodeTelemetry(vd)
		if err != nil {
			log.Printf("gateway %s encode err: %v", g.ID, err)
			continue
		}

		// send to Fog server
		resp, err := http.Post(g.FogURL+"/api/telemetry", "text/plain", strings.NewReader(out))
		if err != nil {
			log.Printf("gateway %s forward err: %v", g.ID, err)
			continue
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Printf("warning: failed to discard response body: %v", err)
		}
		if err := resp.Body.Close(); err != nil {
			log.Printf("warning: failed to close response body: %v", err)
		}

	}
}

// Stop stops the gateway background loop and closes the device if present.
func (g *Gateway) Stop() {
	// close stop channel (idempotent)
	select {
	case <-g.stop:
		// already closed
	default:
		close(g.stop)
	}
	if g.Device != nil {
		_ = g.Device.Close()
	}
	g.wg.Wait()
}
