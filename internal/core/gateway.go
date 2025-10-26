package core

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
)

// Gateway represents a LoRa gateway instance that reads wire lines from a Device,
// decodes using InParser, re-encodes using OutParser and forwards to FogServer.
type Gateway struct {
	ID         string
	Device     device.Device
	URL        string
	FogURL     string
	InParser   parser.Parser
	OutParser  parser.Parser
	WireIn     string // uplink Vehicle -> Gateway format
	WireOut    string // uplink Gateway -> Fog format
	Vehicles   []string
	VehicleSet map[string]struct{}
	server     *http.Server
	stop       chan struct{}
	wg         sync.WaitGroup
}

// NewGateway constructs a Gateway with device path and parsers.
// If opening the serial device fails, the Device field may be nil and Start will be a no-op.
func NewGateway(id, devPath string, baud int, URL string, fogURL string, wireIn string, wireOut string, in parser.Parser, out parser.Parser, vehicles []string) *Gateway {
	dev, err := device.NewSerialDevice(devPath, baud)
	if err != nil {
		// log but continue: user may run gateway without physical device (e.g., test)
		log.Printf("[gateway %s] open serial %s err: %v", id, devPath, err)
	} else {
		log.Printf("[gateway %s] open serial %s: success", id, devPath)
	}
	g := &Gateway{
		ID:         id,
		Device:     dev,
		URL:        URL,
		FogURL:     fogURL,
		WireIn:     wireIn,
		WireOut:    wireOut,
		InParser:   in,
		OutParser:  out,
		Vehicles:   vehicles,
		VehicleSet: make(map[string]struct{}, len(vehicles)),
		stop:       make(chan struct{}),
	}
	for _, v := range vehicles {
		g.VehicleSet[v] = struct{}{}
	}
	return g
}

// Start begins the gateway read/forward loop in a background goroutine.
// Returns nil even if the underlying device is nil (no-op for testing).
func (g *Gateway) Start() error {
	if g.Device == nil {
		log.Printf("[gateway %s] no serial device; running in headless mode", g.ID)
		return nil
	}

	// Start uplink loop (Vehicle → Fog)
	g.wg.Add(1)
	go g.loop()

	// Start downlink HTTP handler (Fog → Vehicle)
	mux := http.NewServeMux()
	mux.HandleFunc("/command", g.handleControl)
	// port := g.URL[strings.LastIndex(g.URL, ":"):]
	addr := g.URL
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	g.server = &http.Server{Addr: addr, Handler: mux}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		log.Printf("[gateway %s] HTTP listening at %s/command", g.ID, addr)
		if err := g.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("[gateway %s] HTTP error: %v", g.ID, err)
		}
	}()

	return nil
}

// loop continuously reads lines from the Device, decodes, re-encodes and posts to Fog.
func (g *Gateway) loop() {
	defer g.wg.Done()
	for {
		select {
		case <-g.stop:
			log.Printf("[gateway %s] stopping uplink loop", g.ID)
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
			log.Printf("[gateway %s] decode %s error: %v", g.ID, g.WireIn, err)
			continue
		} else {
			log.Printf("[gateway %s] decode %s: %s", g.ID, g.WireIn, line)
		}

		// check validation of packet that belong to vehicle managed by gateway
		if _, ok := g.VehicleSet[vd.VehicleID]; !ok {
			log.Printf("[gateway %s] skip telemetry from unmanaged vehicle %s", g.ID, vd.VehicleID)
			continue
		}

		// Encode for Fog using OutParser
		out, err := g.OutParser.EncodeTelemetry(vd)
		if err != nil {
			log.Printf("[gateway %s] encode %s err: %v", g.ID, g.WireOut, err)
			continue
		} else {
			log.Printf("[gateway %s] encode %s: %s", g.ID, g.WireOut, out)
		}

		// Determine content-type
		contentType := "text/plain"
		if g.WireOut == "json" {
			contentType = "application/json"
		}

		// send to Fog server
		resp, err := http.Post(g.FogURL+"/api/telemetry", contentType, strings.NewReader(out))
		if err != nil {
			log.Printf("[gateway %s] forward err: %v", g.ID, err)
			continue
		} else {
			log.Printf("[gateway %s] uplink %s → %s : %s", g.ID, g.WireIn, g.WireOut, out)
		}

		// Properly close response body (lint-safe)
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Printf("[gateway %s] warning: discard body: %v", g.ID, err)
		}
		if cerr := resp.Body.Close(); cerr != nil {
			log.Printf("[gateway %s] warning: close body: %v", g.ID, cerr)
		}
	}
}

// handleControl receives a control message from Fog (JSON or CSV),
// decodes into ControlData, re-encodes into wire_in format, and
// sends it downlink to the Vehicle via LoRa.
func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[gateway %s] warning: close control body: %v", g.ID, cerr)
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	line := strings.TrimSpace(string(body))
	if line == "" {
		http.Error(w, "empty control message", http.StatusBadRequest)
		return
	}

	// Step 1: decode incoming control message (Fog → Gateway)
	var ctl model.ControlData
	// Try JSON first
	if err := json.Unmarshal(body, &ctl); err != nil {
		// Try CSV fallback
		csvp := parser.NewCSVParser()
		ctl2, err2 := csvp.DecodeControl(line)
		if err2 != nil {
			http.Error(w, "invalid control message format", http.StatusBadRequest)
			log.Printf("[gateway %s] invalid control: %v", g.ID, err2)
			return
		}
		ctl = ctl2
	}

	// Step 2: encode message for downlink (Gateway → Vehicle)
	var downlink string
	switch strings.ToLower(g.WireIn) {
	case "json":
		b, err := json.Marshal(ctl)
		if err != nil {
			http.Error(w, "encode downlink error", http.StatusInternalServerError)
			return
		}
		downlink = string(b)
	default: // CSV
		csvp := parser.NewCSVParser()
		s, err := csvp.EncodeControl(ctl)
		if err != nil {
			http.Error(w, "encode downlink error", http.StatusInternalServerError)
			return
		}
		downlink = s
	}

	// Step 3: send to Vehicle via LoRa
	if err := g.Device.WriteLine(downlink); err != nil {
		http.Error(w, "failed to send to vehicle", http.StatusInternalServerError)
		log.Printf("[gateway %s] downlink send error: %v", g.ID, err)
		return
	}

	log.Printf("[gateway %s] downlink %s: %s", g.ID, g.WireIn, downlink)
	w.WriteHeader(http.StatusAccepted)
}

// Stop stops the gateway background loop and closes the device if present.
func (g *Gateway) Stop() {
	log.Printf("[gateway %s] stopping...", g.ID)

	// Đóng stop channel an toàn
	select {
	case <-g.stop:
	default:
		close(g.stop)
	}

	// Stop HTTP server
	if g.server != nil {
		log.Printf("[gateway %s] Shutting down web server...", g.ID)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := g.server.Shutdown(ctx); err != nil {
			log.Printf("[gateway %s] HTTP server shutdown error: %v", g.ID, err)
		} else {
			log.Printf("[gateway %s] Web server stopped cleanly", g.ID)
		}
	}

	// Close device
	if g.Device != nil {
		if err := g.Device.Close(); err != nil {
			log.Printf("[gateway %s] device close err: %v", g.ID, err)
		}
	}

	// Wait goroutine done
	done := make(chan struct{})
	go func() {
		g.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		log.Printf("[gateway %s] stopped cleanly", g.ID)
	case <-time.After(3 * time.Second):
		log.Printf("[gateway %s] stop timeout (forcing exit)", g.ID)
	}
}
