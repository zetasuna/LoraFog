// Package core implements the FogServer component, which acts as a central hub between
// gateways and monitoring clients. It handles telemetry ingestion, control message routing,
// and websocket broadcasting.
package core

import (
	"LoraFog/internal/model"
	"LoraFog/internal/parser"
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// FogServer implements a lightweight in-memory fog server that accepts telemetry,
// broadcasts telemetry to websocket clients, and forwards control messages to gateways.
type FogServer struct {
	Addr    string
	reg     *registry
	clients map[*websocket.Conn]bool
	mu      sync.Mutex
	server  *http.Server
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
func NewFogServer(addr string) *FogServer {
	return &FogServer{Addr: addr, reg: newRegistry(), clients: map[*websocket.Conn]bool{}}
}

// RegisterGateway registers a gateway and maps its vehicle list in the registry.
func (f *FogServer) RegisterGateway(id, url string, vehicles []string) {
	for _, v := range vehicles {
		f.reg.set(v, url)
	}
}

// Start launches the HTTP server for telemetry, ws and control endpoints.
// This call blocks until the server stops or fails.
func (f *FogServer) Start() {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/telemetry", f.handleTelemetry)
	mux.HandleFunc("/ws", f.handleWS)
	mux.HandleFunc("/control", f.handleControl)
	f.server = &http.Server{Addr: f.Addr, Handler: mux}
	log.Printf("FogServer is listening on %s", f.Addr)
	if err := f.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatal(err)
	}
}

// Stop shuts down the HTTP server.
func (f *FogServer) Stop() {
	if f.server != nil {
		_ = f.server.Close()
	}
}

// handleTelemetry accepts telemetry posted by gateways in either JSON or CSV text.
// It decodes to VehicleData and broadcasts CSV lines to websocket clients.
func (f *FogServer) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	line := string(body)
	var vd model.VehicleData
	if err := json.Unmarshal(body, &vd); err != nil {
		csvp := parser.NewCSVParser()
		vd2, err2 := csvp.DecodeTelemetry(line)
		if err2 != nil {
			http.Error(w, "invalid telemetry", 400)
			return
		}
		vd = vd2
	}
	csv, _ := parser.NewCSVParser().EncodeTelemetry(vd)
	f.broadcast(csv)
	w.WriteHeader(200)
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

// handleControl accepts a structured ControlMessage and forwards it to the gateway that serves the target vehicle.
// The fog forwards the control as JSON to the gateway's /command endpoint.
func (f *FogServer) handleControl(w http.ResponseWriter, r *http.Request) {
	var c model.ControlMessage
	if err := json.NewDecoder(r.Body).Decode(&c); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	url, ok := f.reg.get(c.VehicleID)
	if !ok {
		http.Error(w, "no gateway for vehicle", 404)
		return
	}
	b, _ := json.Marshal(c)
	// go http.Post(url+"/command", "application/json", bytes.NewReader(b))
	go func() {
		resp, err := http.Post(url+"/command", "application/json", bytes.NewReader(b))
		if err != nil {
			log.Printf("failed to send control to %s: %v", url, err)
			return
		}
		defer func() {
			if cerr := resp.Body.Close(); cerr != nil {
				log.Printf("warning: close control response: %v", cerr)
			}
		}()
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			log.Printf("warning: discard control response: %v", err)
		}
	}()
	w.WriteHeader(202)
}
