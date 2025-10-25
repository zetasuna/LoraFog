// Package model defines shared configuration structures used to initialize the LoraFog system.
// It includes global settings, gateway definitions, and vehicle definitions.
package model

// Config represents the root structure loaded from configs/config.yml.
// It contains global settings, gateway definitions and vehicle definitions.
type Config struct {
	Global         GlobalConfig        `yaml:"global"`
	Server         ServerConfig        `yaml:"server"`
	Gateways       []GatewayConfig     `yaml:"gateways"`
	Vehicles       []VehicleConfig     `yaml:"vehicles"`
	Arduinos       []ArduinoConfig     `yaml:"arduinos"`
	VirtualSerials VirtualSerialConfig `yaml:"virtual_serials"`
}

// GlobalConfig defines shared defaults across the system.
type GlobalConfig struct {
	WireFormat string `yaml:"wire_format"` // default wire format (csv/json)
}

// ServerConfig defines configuration for a server instance.
type ServerConfig struct {
	FogAddr  string            `yaml:"fog_addr"` // address for FogServer (e.g. ":10000") if blank server will not work
	AppAddr  string            `yaml:"app_addr"` // address for FogServer (e.g. ":10000") if blank server will not work
	Gateways []GatewayRegistry `yaml:"gateway_registry"`
}

// GatewayRegistry defines a gateway registration entry.
type GatewayRegistry struct {
	ID       string   `yaml:"id"`
	URL      string   `yaml:"url"`
	Vehicles []string `yaml:"vehicles"`
}

// GatewayConfig defines configuration for a single gateway instance.
type GatewayConfig struct {
	ID       string   `yaml:"id"`
	URL      string   `yaml:"url"`     // fog server endpoint
	FogURL   string   `yaml:"fog_url"` // fog server endpoint
	LoraDev  string   `yaml:"lora_device"`
	LoraBaud int      `yaml:"lora_baud"`
	WireIn   string   `yaml:"wire_in"`  // format received from vehicle
	WireOut  string   `yaml:"wire_out"` // format sent to fog
	Vehicles []string `yaml:"vehicles"`
}

// VehicleConfig defines configuration for a single vehicle agent.
type VehicleConfig struct {
	ID                  string `yaml:"id"`
	WireFormat          string `yaml:"wire_format"`
	TelemetryIntervalMs int    `yaml:"telemetry_interval_ms"`
	LoraDev             string `yaml:"lora_device"`
	LoraBaud            int    `yaml:"lora_baud"`
	ArduinoID           string `yaml:"arduino_id"`
	ArduinoDev          string `yaml:"arduino_device"`
	ArduinoBaud         int    `yaml:"arduino_baud"`
}

// ArduinoConfig defines serial setup for testing
type ArduinoConfig struct {
	ID   string `yaml:"id"`
	Dev  string `yaml:"device"`
	Baud int    `yaml:"baud"`
}

// GpsConfig defines serial setup for testing
type GpsConfig struct {
	ID   string `yaml:"id"`
	Dev  string `yaml:"device"`
	Baud int    `yaml:"baud"`
}

// VirtualPair defines a flexible pair of linked virtual serial endpoints.
type VirtualPair struct {
	Type  string `yaml:"type"`
	Left  string `yaml:"left"`
	Right string `yaml:"right"`
}

// VirtualSerialConfig defines optional virtual serial setup for testing.
type VirtualSerialConfig struct {
	Enabled bool          `yaml:"enabled"`
	Pairs   []VirtualPair `yaml:"pairs"`
}
