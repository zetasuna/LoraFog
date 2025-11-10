// Package model defines the data structures
package model

// TelemetryData represents telemetry information reported by a vehicle.
// It is the common structure shared between vehicles, gateways and fog.
type TelemetryData struct {
	VehicleID   string  `json:"boatId"`
	Latitude    float64 `json:"lat"`
	Longitude   float64 `json:"lon"`
	CurrentHead uint16  `json:"head"`
	TargetHead  uint16  `json:"targetHead"`
	LeftSpeed   uint16  `json:"leftSpeed"`
	RightSpeed  uint16  `json:"rightSpeed"`
}

// ControlData represents a control command sent from Fog to a vehicle.
// It can be encoded either as JSON or CSV depending on gateway configuration.
type ControlData struct {
	VehicleID string  `json:"boatId"`
	Speed     int     `json:"speed"`
	Latitude  float64 `json:"targetLat"`
	Longitude float64 `json:"targetLon"`
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
