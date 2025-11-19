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
	CycleDurationMs  int64  `json:"cycle_dur_ms" cbor:"cycle_dur_ms"` // Tổng thời gian chu kỳ
	SlotDurationMs   int64  `json:"slot_dur_ms" cbor:"slot_dur_ms"`   // Thời gian 1 slot
	GuardTimeMs      int64  `json:"guard_ms" cbor:"guard_ms"`         // Thời gian nghỉ giữa các slot
	RegisterWindowMs int64  `json:"reg_win_ms" cbor:"reg_win_ms"`     // Thời gian cuối cho đăng ký
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
	Timestamp   int64   `json:"timestamp" cbor:"timestamp"`
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

// ControlData là gói tin điều khiển gửi từ Gateway xuống Vehicle.
type ControlData struct {
	Type      string `json:"type" cbor:"type"` // "control"
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	Command   string `json:"command" cbor:"command"` // e.g., "STOP_ENGINE", "SLOW_DOWN"
}
