// Package model provides message definitions exchanged across components.
package model

// PacketType identifies packet category.
type PacketType string

const (
	PacketTelemetry PacketType = "t"
	PacketControl   PacketType = "c"
)

// Packet encapsulates a typed payload.
// type Packet struct {
// 	Type PacketType `json:"type" cbor:"type"`
// 	Data any        `json:"data" cbor:"data"`
// }

// VehicleData represents telemetry reported by a vehicle.
type VehicleData struct {
	VehicleID   string  `json:"vehicle_id" cbor:"vehicle_id"` // prefer small IDs in LoRa
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	CurrentHead int     `json:"current_head" cbor:"current_head"`
	TargetHead  int     `json:"target_head" cbor:"target_head"`
	LeftSpeed   int     `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int     `json:"right_speed" cbor:"right_speed"`
}

// ControlData represents a control command sent to a vehicle.
type ControlData struct {
	VehicleID string  `json:"vehicle_id" cbor:"vehicle_id"`
	Speed     int     `json:"speed" cbor:"speed"`
	Latitude  float64 `json:"latitude" cbor:"latitude"`
	Longitude float64 `json:"longitude" cbor:"longitude"`
	Kp        float64 `json:"kp" cbor:"kp"`
	Ki        float64 `json:"ki" cbor:"ki"`
	Kd        float64 `json:"kd" cbor:"kd"`
}

// ArduinoData represents raw telemetry from Arduino sensors.
type ArduinoData struct {
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	LeftSpeed   int     `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int     `json:"right_speed" cbor:"right_speed"`
	CurrentHead int     `json:"current_head" cbor:"current_head"`
	TargetHead  int     `json:"target_head" cbor:"target_head"`
}

// ArduinoControl is the format for commands forwarded to Arduino.
type ArduinoControl struct {
	CruiseSpeed int     `json:"cruise_speed" cbor:"cruise_speed"`
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	Kp          float64 `json:"kp" cbor:"kp"`
	Ki          float64 `json:"ki" cbor:"ki"`
	Kd          float64 `json:"kd" cbor:"kd"`
}

// GatewayRegistration is used when registering gateways to the fog server.
type GatewayRegistration struct {
	GatewayID string   `json:"gateway_id" cbor:"gateway_id"`
	Addr      string   `json:"address" cbor:"address"`
	Vehicles  []string `json:"vehicles" cbor:"vehicles"`
}

// BeaconMessage is broadcasted by Gateway to Vehicle.
// type BeaconMessage struct {
// 	Type             string   `json:"type" cbor:"type"`
// 	Gateway          string   `json:"gateway" cbor:"gateway"`
// 	Timestamp        int64    `json:"timestamp" cbor:"timestamp"`                   // gateway local unix
// 	CycleStart       int64    `json:"cycle_start" cbor:"cycle_start"`               // unix seconds for the cycle start (sync)
// 	CyclePeriodSec   int64    `json:"cycle_period_sec" cbor:"cycle_period_sec"`     // total cycle length in seconds
// 	SlotDurationMs   int64    `json:"slot_duration_ms" cbor:"slot_duration_ms"`     // slot length in ms
// 	GuardMs          int64    `json:"guard_ms" cbor:"guard_ms"`                     // guard interval in ms
// 	RegisterWindowMs int64    `json:"register_window_ms" cbor:"register_window_ms"` // register window at cycle end
// 	SlotMap          map[string]int `json:"slot_map" cbor:"slot_map"`
// }

// BeaconMessage broadcast bởi Gateway đầu mỗi chu kỳ.
type BeaconMessage struct {
	Type             string         `json:"type" cbor:"type"` // luôn là "beacon"
	GatewayID        string         `json:"gateway_id" cbor:"gateway_id"`
	Timestamp        int64          `json:"timestamp" cbor:"timestamp"`
	CycleDurationMs  int64          `json:"cycle_dur_ms" cbor:"cycle_dur_ms"` // Tổng thời gian chu kỳ
	SlotDurationMs   int64          `json:"slot_dur_ms" cbor:"slot_dur_ms"`   // Thời gian 1 slot
	GuardTimeMs      int64          `json:"guard_ms" cbor:"guard_ms"`         // Thời gian nghỉ giữa các slot
	RegisterWindowMs int64          `json:"reg_win_ms" cbor:"reg_win_ms"`     // Thời gian cuối cho đăng ký
	SlotMap          map[string]int `json:"slot_map" cbor:"slot_map"`         // Map: VehicleID -> SlotIndex. Vehicle dựa vào đây để biết mình được gửi ở slot nào.
}

// HelloMessage is sent by Vehicle to Gateway when receive beacon message.
type HelloMessage struct {
	Type      string `json:"type" cbor:"type"`
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
}

// AuthMessage (relay from Fog -> Gateway -> Vehicle) contains key and TTL.
type AuthMessage struct {
	Type      string `json:"type" cbor:"type"`
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	TTL       int64  `json:"ttl" cbor:"ttl"` // seconds
	Slot      int    `json:"slot" cbor:"slot"`
}
