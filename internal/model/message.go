// Package model defines the core data structures exchanged between vehicles,
// gateways, and the fog server, including telemetry and control messages.
package model

// VehicleData represents telemetry information reported by a vehicle.
// It is the common structure shared between vehicles, gateways and fog.
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

// ControlMessage represents a control command sent from Fog to a vehicle.
// It can be encoded either as JSON or CSV depending on gateway configuration.
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

// GPSData represents a simple latitude/longitude reading.
type GPSData struct {
	Lat float64 `json:"lat"`
	Lon float64 `json:"lon"`
}

// GatewayRegistration represents information sent by a gateway
// to the fog when registering itself.
type GatewayRegistration struct {
	GatewayID string   `json:"gateway_id"`
	URL       string   `json:"url"`
	Vehicles  []string `json:"vehicles"`
}
