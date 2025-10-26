// Package core implements the FogServer component, which acts as a central hub between
// gateways and monitoring clients. It handles telemetry ingestion, control message routing,
// and websocket broadcasting.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/model"
	"LoraFog/internal/parser"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// FogServer implements a lightweight in-memory fog server that accepts telemetry,
// broadcasts telemetry to websocket clients, and forwards control messages to gateways.
type FogServer struct {
	Addr    string
	AppAddr string
	reg     *registry
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
	server  *http.Server
	wireFmt string // wire format: "csv" or "json"
}

// registry maps vehicle IDs to gateway URLs.
type registry struct {
	mu         sync.RWMutex
	vehicleMap map[string]string
}

// newRegistry creates an empty registry.
func newRegistry() *registry {
	return &registry{vehicleMap: map[string]string{}}
}

// set associates a vehicle ID with a gateway URL.
func (r *registry) set(v string, gw string) { r.mu.Lock(); r.vehicleMap[v] = gw; r.mu.Unlock() }

// get retrieves gateway URL for a vehicle ID.
func (r *registry) get(v string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	x, ok := r.vehicleMap[v]
	return x, ok
}

// NewFogServer constructs a FogServer listening on addr.
func NewFogServer(addr string, appAddr string) *FogServer {
	return &FogServer{
		Addr:    addr,
		AppAddr: appAddr,
		reg:     newRegistry(),
		clients: map[*websocket.Conn]bool{},
	}
}

// RegisterGateway registers a gateway and maps its vehicle list in the registry.
func (f *FogServer) RegisterGateway(id, url string, vehicles []string) {
	for _, v := range vehicles {
		f.reg.set(v, url)
	}
}

// Start launches the HTTP server for telemetry, ws and control endpoints.
// This call blocks until the server stops or fails.
func (f *FogServer) Start() error {
	if f.Addr == "" {
		log.Println("[fog] fog server not started (empty address)")
		return nil
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/telemetry", f.handleTelemetry)
	mux.HandleFunc("/api/control", f.handleControl)
	mux.HandleFunc("/ws", f.handleWS)
	addr := f.Addr
	addr = strings.TrimPrefix(addr, "http://")
	addr = strings.TrimPrefix(addr, "https://")
	f.server = &http.Server{Addr: addr, Handler: mux}
	log.Printf("[fog] listening on %s", addr)
	return f.server.ListenAndServe()
	// log.Printf("FogServer is listening on %s", f.Addr)
	// if err := f.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
	// 	log.Fatal(err)
	// }
}

// Stop shuts down the HTTP server.
func (f *FogServer) Stop() {
	if f.server != nil {
		log.Println("[fog] Shutting down web server...")
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := f.server.Shutdown(ctx); err != nil {
			log.Printf("[fog] HTTP server shutdown error: %v", err)
		} else {
			log.Println("[fog] Web server stopped cleanly")
		}
	}
}

// handleTelemetry accepts telemetry posted by gateways in either JSON or CSV text.
// It decodes to VehicleData and broadcasts CSV lines to websocket clients.
func (f *FogServer) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[fog] warning: failed to close request body: %v", cerr)
		}
	}()

	line := strings.TrimSpace(string(body))
	if line == "" {
		http.Error(w, "empty telemetry", http.StatusBadRequest)
		return
	}

	var vd model.VehicleData
	// Try decode as JSON first
	if err := json.Unmarshal(body, &vd); err != nil {
		// Try CSV fallback
		csvp := parser.NewCSVParser()
		vd2, err2 := csvp.DecodeTelemetry(line)
		if err2 != nil {
			log.Printf("[fog] invalid telemetry: cannot decode JSON or CSV: %v", err2)
			http.Error(w, "invalid telemetry", http.StatusBadRequest)
			return
		}
		vd = vd2
	}

	// Encode to broadcast format
	var out string
	var payload []byte
	var contentType string
	switch f.wireFmt {
	case "json":
		contentType = "application/json"
		payload, err = json.Marshal(vd)
		if err != nil {
			log.Printf("[fog] encode json err: %v", err)
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
		out = string(payload)
	default: // csv (default)
		contentType = "text/plain"
		csvp := parser.NewCSVParser()
		out, err = csvp.EncodeTelemetry(vd)
		if err != nil {
			log.Printf("[fog] encode csv err: %v", err)
			http.Error(w, "encode error", http.StatusInternalServerError)
			return
		}
		payload = []byte(out)
	}
	f.broadcast(out)
	log.Printf("[fog] broadcast %s telemetry: %s", strings.ToUpper(f.wireFmt), out)

	// Forward to App Server if enabled
	if f.AppAddr != "" {
		go func(v model.VehicleData) {
			resp, err := http.Post(f.AppAddr+"/api/telemetry", contentType, bytes.NewReader(payload))
			if err != nil {
				log.Printf("[fog] forward to app failed: %v", err)
				return
			}
			defer func() {
				if cerr := resp.Body.Close(); cerr != nil {
					log.Printf("[fog] warning: close app response: %v", cerr)
				}
			}()
			// Discard the body to complete the HTTP exchange cleanly
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				log.Printf("[fog] warning: discard control response: %v", err)
			}
			log.Printf("[fog] forwarded telemetry to app (%s): %s", f.AppAddr, v.VehicleID)
		}(vd)
	}
	w.WriteHeader(http.StatusOK)
}

// handleWS upgrades HTTP to websocket and registers the client for broadcasts.
func (f *FogServer) handleWS(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	f.mu.Lock()
	f.clients[conn] = true
	f.mu.Unlock()

	go func() {
		defer func() {
			f.mu.Lock()
			delete(f.clients, conn)
			f.mu.Unlock()
			if err := conn.Close(); err != nil {
				log.Printf("warning: failed to close websocket: %v", err)
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				break
			}
		}
	}()
}

// broadcast sends a message to all connected websocket clients.
func (f *FogServer) broadcast(msg string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for c := range f.clients {
		_ = c.WriteMessage(websocket.TextMessage, []byte(msg))
	}
}

// handleControl receives a control message from the cloud or admin,
// finds the gateway responsible for the target vehicle, and forwards
// the message in the format specified by the global wire_format.
func (f *FogServer) handleControl(w http.ResponseWriter, r *http.Request) {
	// Always close request body safely
	defer func() {
		if cerr := r.Body.Close(); cerr != nil {
			log.Printf("[fog] warning: close control request body: %v", cerr)
		}
	}()

	// Decode control message (JSON input only for API)
	body, berr := io.ReadAll(r.Body)
	if berr != nil {
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
			// log.Printf("[gateway %s] invalid control: %v", g.ID, err2)
			return
		}
		ctl = ctl2
	}

	// Lookup gateway by vehicle ID
	url, ok := f.reg.get(ctl.VehicleID)
	if !ok {
		http.Error(w, "no gateway registered for vehicle", http.StatusNotFound)
		log.Printf("[fog] control ignored: no gateway for vehicle %s", ctl.VehicleID)
		return
	}

	// Encode control message according to configured wire format
	var payload []byte
	var contentType string
	var err error

	switch f.wireFmt {
	case "csv":
		csvp := parser.NewCSVParser()
		line, encErr := csvp.EncodeControl(ctl)
		if encErr != nil {
			http.Error(w, "failed to encode control message (csv)", http.StatusInternalServerError)
			log.Printf("[fog] control encode csv error: %v", encErr)
			return
		}
		payload = []byte(line)
		contentType = "text/plain"

	default: // json
		payload, err = json.Marshal(ctl)
		if err != nil {
			http.Error(w, "failed to encode control message (json)", http.StatusInternalServerError)
			log.Printf("[fog] control encode json error: %v", err)
			return
		}
		contentType = "application/json"
	}

	// Send asynchronously to the gateway
	go func() {
		resp, err := http.Post(url+"/command", contentType, bytes.NewReader(payload))
		if err != nil {
			log.Printf("[fog] failed to send control to gateway %s: %v", url, err)
			return
		}

		// Always close response body safely
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				log.Printf("[fog] warning: close control response: %v", cerr)
			}
		}()

		// Discard the body to complete the HTTP exchange cleanly
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Printf("[fog] warning: discard control response: %v", err)
		}

		log.Printf("[fog] control forwarded to %s (fmt=%s, vehicle=%s)", url, f.wireFmt, ctl.VehicleID)
	}()

	w.WriteHeader(http.StatusAccepted)
}
