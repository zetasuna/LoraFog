// Package model defines the core data structures exchanged between vehicles,
// gateways, and the fog server, including telemetry and control messages.
package model

type PacketType string

const (
	PacketTelemetry PacketType = "t"
	PacketControl   PacketType = "c"
)

type Packet struct {
	Type PacketType `json:"type"`
	Data any        `json:"data"`
}

// VehicleData represents telemetry information reported by a vehicle.
// It is the common structure shared between vehicles, gateways and fog.
type VehicleData struct {
	VehicleID   string  `json:"vehicle_id"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	CurrentHead int     `json:"current_head"`
	TargetHead  int     `json:"target_head"`
	LeftSpeed   int     `json:"left_speed"`
	RightSpeed  int     `json:"right_speed"`
	PID         int     `json:"pid"`
}

// ControlData represents a control command sent from Fog to a vehicle.
// It can be encoded either as JSON or CSV depending on gateway configuration.
type ControlData struct {
	VehicleID string  `json:"vehicle_id"`
	Mode      int     `json:"mode"`
	Speed     int     `json:"speed"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Kp        float64 `json:"kp"`
	Ki        float64 `json:"ki"`
	Kd        float64 `json:"kd"`
}

// ArduinoData represents telemetry data collected by arduino
type ArduinoData struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	LeftSpeed   int     `json:"left_speed"`
	RightSpeed  int     `json:"right_speed"`
	CurrentHead int     `json:"current_head"`
	TargetHead  int     `json:"target_head"`
}

// ArduinoControl represents telemetry data collected by arduino
type ArduinoControl struct {
	CruiseSpeed int     `json:"cruise_speed"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Kp          float64 `json:"kp"`
	Ki          float64 `json:"ki"`
	Kd          float64 `json:"kd"`
}

// GpsData represents a simple latitude/longitude reading.
type GpsData struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

// GatewayRegistration represents information sent by a gateway
// to the fog when registering itself.
type GatewayRegistration struct {
	GatewayID string   `json:"gateway_id"`
	URL       string   `json:"url"`
	Vehicles  []string `json:"vehicles"`
}
