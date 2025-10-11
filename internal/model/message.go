// Package model defines shared message structures for LoraFog.
package model

// VehicleData represents telemetry parsed from CSV and forwarded as JSON.
type VehicleData struct {
	VehicleID string  `json:"vehicle_id"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Head      float64 `json:"head"`
	LeftSpd   float64 `json:"left_speed"`
	RightSpd  float64 `json:"right_speed"`
	Timestamp string  `json:"timestamp"` // ISO8601 formatted on gateway
}

// ControlMessage is used by Fog -> Gateway (JSON). Gateway converts to CSV for LoRa.
type ControlMessage struct {
	VehicleID string `json:"vehicle_id"`
	Payload   string `json:"payload"` // command payload (plain string)
	MsgID     string `json:"msg_id"`  // optional message id
}

// AckMessage is a simple ack structure.
type AckMessage struct {
	MsgID string `json:"msg_id"`
	Ack   bool   `json:"ack"`
}

// GatewayRegistration is used when a gateway registers to Fog.
type GatewayRegistration struct {
	GatewayID string   `json:"gateway_id"`
	URL       string   `json:"url"`      // public URL where Fog can reach gateway (HTTP)
	Vehicles  []string `json:"vehicles"` // optional list of vehicle IDs this gateway serves
}
