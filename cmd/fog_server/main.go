// Fog server implements:
// - POST /register  : gateway registers itself and optionally provides vehicle list
// - POST /ingest    : gateway posts telemetry JSON (VehicleData)
// - GET  /ws        : websocket clients subscribe to telemetry
// - POST /control   : fog sends control targeting vehicle; fog routes to registered gateway
//
// Note: this is an in-memory registry. For production, persist registrations.
package main

import (
	"LoraFog/internal/model"
	"bytes"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// registry maps vehicleID -> gatewayURL
type registry struct {
	mu         sync.RWMutex
	vehicleMap map[string]string // vehicleID -> gatewayURL
	gatewayMap map[string]model.GatewayRegistration
}

func newRegistry() *registry {
	return &registry{
		vehicleMap: make(map[string]string),
		gatewayMap: make(map[string]model.GatewayRegistration),
	}
}

func (r *registry) register(g model.GatewayRegistration) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.gatewayMap[g.GatewayID] = g
	// map vehicles to gateway URL
	for _, v := range g.Vehicles {
		r.vehicleMap[v] = g.URL
	}
}

func (r *registry) gatewayForVehicle(vehicleID string) (string, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	url, ok := r.vehicleMap[vehicleID]
	return url, ok
}

func main() {
	addr := flag.String("addr", ":10000", "listen address")
	flag.Parse()

	reg := newRegistry()

	// in-memory websocket clients
	clients := make(map[*websocket.Conn]bool)
	var mu sync.Mutex

	// POST /register
	http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
		var g model.GatewayRegistration
		if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if g.GatewayID == "" || g.URL == "" {
			http.Error(w, "gateway_id and url required", http.StatusBadRequest)
			return
		}
		reg.register(g)
		log.Printf("gateway registered: %s -> %s (vehicles=%v)", g.GatewayID, g.URL, g.Vehicles)
		w.WriteHeader(http.StatusOK)
	})

	// POST /ingest
	http.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		var vd model.VehicleData
		if err := json.NewDecoder(r.Body).Decode(&vd); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		log.Printf("ingested: %s at %s", vd.VehicleID, vd.Timestamp)
		// broadcast to WS clients
		mu.Lock()
		for c := range clients {
			_ = c.WriteJSON(vd)
		}
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	})

	// GET /ws
	http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("ws upgrade err: %v", err)
			return
		}
		mu.Lock()
		clients[conn] = true
		mu.Unlock()
		// read loop to detect disconnect
		go func(c *websocket.Conn) {
			defer func() {
				mu.Lock()
				delete(clients, c)
				mu.Unlock()
				if err := c.Close(); err != nil {
					log.Printf("failed to close websocket: %v", err)
				}
			}()
			for {
				var v any
				if err := c.ReadJSON(&v); err != nil {
					break
				}
			}
		}(conn)
	})

	// POST /control
	http.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			VehicleID string `json:"vehicle_id"`
			Payload   string `json:"payload"`
			MsgID     string `json:"msg_id,omitempty"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		// find gateway for target vehicle
		gwURL, ok := reg.gatewayForVehicle(req.VehicleID)
		if !ok {
			http.Error(w, "no gateway for vehicle", http.StatusNotFound)
			return
		}
		ctl := model.ControlMessage{VehicleID: req.VehicleID, Payload: req.Payload, MsgID: req.MsgID}
		// forward to gateway /command
		go func() {
			b, _ := json.Marshal(ctl)
			_, err := http.Post(gwURL+"/command", "application/json", bytes.NewReader(b))
			if err != nil {
				log.Printf("forward to gateway err: %v", err)
			}
		}()
		w.WriteHeader(http.StatusAccepted)
	})

	log.Printf("fog server listening %s", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
