// Package model defines shared message structures for LoraFog.
package model

// VehicleData represents telemetry parsed from CSV and forwarded as JSON.
type VehicleData struct {
	VehicleID string  `json:"vehicle_id"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	HeadCur   float64 `json:"head_current"`
	HeadTar   float64 `json:"head_target"`
	LeftSpd   float64 `json:"left_speed"`
	RightSpd  float64 `json:"right_speed"`
	PID       float64 `json:"pid"`
}

// ControlMessage is used by Fog -> Gateway (JSON). Gateway converts to CSV for LoRa.
type ControlMessage struct {
	VehicleID string  `json:"vehicle_id"`
	Mode      float64 `json:"mode"`
	Spd       float64 `json:"speed"`
	Lat       float64 `json:"lat"`
	Lon       float64 `json:"lon"`
	Kp        float64 `json:"kp"`
	Ki        float64 `json:"ki"`
	Kd        float64 `json:"kd"`
}

// GPSData is used by GPS
type GPSData struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
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
