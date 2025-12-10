// Package model provides message definitions exchanged across components.
package model

const (
	PacketTelemetry string = "t"
	PacketControl   string = "c"
	PacketBeacon    string = "b"
	PacketHello     string = "h"
)

// BeaconMessage broadcast bởi Gateway đầu mỗi chu kỳ TDMA.
type BeaconMessage struct {
	Type             string `json:"type" cbor:"type"` // "beacon"
	GatewayAddress   string `json:"gateway_address" cbor:"gateway_address"`
	Timestamp        int64  `json:"timestamp" cbor:"timestamp"`
	GuardTimeMs      int64  `json:"guard_time_ms" cbor:"guard_time_ms"`
	CycleStartMs     int64  `json:"cycle_start_ms" cbor:"cycle_start_ms"`
	SlotWindowMs     int64  `json:"slot_window_ms" cbor:"slot_window_ms"`
	BeaconWindowMs   int64  `json:"beacon_window_ms" cbor:"beacon_window_ms"`
	ControlWindowMs  int64  `json:"control_window_ms" cbor:"control_window_ms"`
	RegisterWindowMs int64  `json:"register_window_ms" cbor:"register_window_ms"`
	// Map: VehicleID -> SlotIndex. Vehicle dựa vào đây để biết mình được gửi ở slot nào.
	SlotMap map[string]int `json:"slot_map" cbor:"slot_map"`
}

// HelloMessage gửi bởi Vehicle trong vùng Register Window để đăng ký.
type HelloMessage struct {
	Type      string `json:"type" cbor:"type"` // "hello"
	VehicleID string `json:"vehicle" cbor:"vehicle"`
}

// RegisterRequest là cấu trúc Server nhận từ Gateway
type RegisterRequest struct {
	GatewayAddress string `json:"gateway_address"`
	VehicleID      string `json:"vehicle_id"`
}

// RegisterResponse trả về slot cho Gateway (khi register thành công)
type RegisterResponse struct {
	Slot           int    `json:"slot"`
	GatewayAddress string `json:"gateway_address"`
}

// ArduinoData giả lập dữ liệu thô từ Arduino/Cảm biến.
type ArduinoData struct {
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	CurrentHead int64   `json:"current_head" cbor:"current_head"`
	TargetHead  int64   `json:"target_head" cbor:"target_head"`
	LeftSpeed   int64   `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int64   `json:"right_speed" cbor:"right_speed"`
}

// VehicleData là gói tin Telemetry gửi từ Vehicle lên Gateway.
type VehicleData struct {
	Type        string  `json:"type" cbor:"type"` // "telemetry"
	VehicleID   string  `json:"vehicle_id" cbor:"vehicle_id"`
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	CurrentHead int64   `json:"current_head" cbor:"current_head"`
	TargetHead  int64   `json:"target_head" cbor:"target_head"`
	LeftSpeed   int64   `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int64   `json:"right_speed" cbor:"right_speed"`
}

type VehicleApp struct {
	VehicleID   string  `json:"boatId" cbor:"vehicle_id"`
	Latitude    float64 `json:"lat" cbor:"latitude"`
	Longitude   float64 `json:"lon" cbor:"longitude"`
	CurrentHead int64   `json:"head" cbor:"current_head"`
	TargetHead  int64   `json:"targetHead" cbor:"target_head"`
	LeftSpeed   int64   `json:"leftSpeed" cbor:"left_speed"`
	RightSpeed  int64   `json:"rightSpeed" cbor:"right_speed"`
}

// ControlData là gói tin điều khiển gửi từ Gateway xuống Vehicle.
type ControlData struct {
	Type      string  `json:"type" cbor:"type"` // "control"
	VehicleID string  `json:"vehicle_id" cbor:"vehicle_id"`
	Speed     int     `json:"speed" cbor:"speed"`
	Latitude  float64 `json:"latitude" cbor:"latitude"`
	Longitude float64 `json:"longitude" cbor:"longitude"`
	Kp        float64 `json:"kp" cbor:"kp"`
	Ki        float64 `json:"ki" cbor:"ki"`
	Kd        float64 `json:"kd" cbor:"kd"`
}

type ControlApp struct {
	VehicleID string  `json:"boatId" cbor:"vehicle_id"`
	Speed     int     `json:"speed" cbor:"speed"`
	Latitude  float64 `json:"targetLat" cbor:"latitude"`
	Longitude float64 `json:"targetLon" cbor:"longitude"`
	Kp        float64 `json:"kp" cbor:"kp"`
	Ki        float64 `json:"ki" cbor:"ki"`
	Kd        float64 `json:"kd" cbor:"kd"`
}
