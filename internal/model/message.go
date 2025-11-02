// Package model provides message definitions exchanged across components.
package model

// CHANGELOG (refactor v2):
// - Added json and cbor struct tags
// - Minor naming cleanup for clarity

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
type BeaconMessage struct {
	// GatewayID short id (nên dùng string ngắn hoặc uint16)
	Type      string `json:"type" cbor:"type"`
	GatewayID string `json:"gateway_id" cbor:"gateway_id"`
	Timestamp int64  `json:"timestamp" cbor:"timestamp"`
	// Nonce     uint32 `json:"nonce" cbor:"nonce"` // 4-byte nonce
}

// HelloMessage is sent by Vehicle to Gateway when receive beacon message.
type HelloMessage struct {
	Type      string `json:"type" cbor:"type"`
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	// NonceReply uint32 `json:"nonce_reply" cbor:"nonce_reply"`
}

// AuthMessage (relay from Fog -> Gateway -> Vehicle) contains key and TTL.
type AuthMessage struct {
	Type      string `json:"type" cbor:"type"`
	VehicleID string `json:"vehicle_id" cbor:"vehicle_id"`
	// Key       []byte `json:"key" cbor:"key"`
	TTL int64 `json:"ttl" cbor:"ttl"` // seconds
}
