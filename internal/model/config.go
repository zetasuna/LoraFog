// Package model defines configuration structures used across LoraFog.
package model

// Config defines all system components loaded from YAML/JSON.
type Config struct {
	Server         ServerConfig          `yaml:"server" json:"server"`
	Gateways       []GatewayConfig       `yaml:"gateways" json:"gateways"`
	Vehicles       []VehicleConfig       `yaml:"vehicles" json:"vehicles"`
	Arduinos       []ArduinoConfig       `yaml:"arduinos" json:"arduinos"`
	VirtualSerials []VirtualSerialConfig `yaml:"virtual_serials" json:"virtual_serials"`
}

// ServerConfig configures the fog server and its exposed addresses.
type ServerConfig struct {
	Addr     string            `yaml:"address" json:"address"`
	AppAddr  string            `yaml:"app_address" json:"app_address"`
	Gateways []GatewayRegistry `yaml:"gateway_registry" json:"gateway_registry"`
}

// GatewayRegistry defines gateway entries for the fog registry.
type GatewayRegistry struct {
	ID   string `yaml:"id" json:"id"`
	Addr string `yaml:"address" json:"address"`
}

// GatewayConfig describes one gateway instance.
type GatewayConfig struct {
	Addr       string `yaml:"address" json:"address"`
	ServerAddr string `yaml:"server_address" json:"server_address"`
	LoraDev    string `yaml:"lora_device" json:"lora_device"`
	LoraBaud   int    `yaml:"lora_baud" json:"lora_baud"`
}

// VehicleConfig describes one LoRa vehicle.
type VehicleConfig struct {
	ID          string `yaml:"id" json:"id"`
	LoraDev     string `yaml:"lora_device" json:"lora_device"`
	LoraBaud    int    `yaml:"lora_baud" json:"lora_baud"`
	ArduinoDev  string `yaml:"arduino_device" json:"arduino_device"`
	ArduinoBaud int    `yaml:"arduino_baud" json:"arduino_baud"`
}

// ArduinoConfig defines a physical Arduino connected to serial port.
type ArduinoConfig struct {
	Dev  string `yaml:"device" json:"device"`
	Baud int    `yaml:"baud" json:"baud"`
}

// VirtualSerialConfig defines a linked virtual serial pair.
type VirtualSerialConfig struct {
	Left  string `yaml:"left" json:"left"`
	Right string `yaml:"right" json:"right"`
}
