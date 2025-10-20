// Package model defines shared configuration structures used to initialize the LoraFog system.
// It includes global settings, gateway definitions, and vehicle definitions.
package model

// Config represents the root structure loaded from configs/config.yml.
// It contains global settings, gateway definitions and vehicle definitions.
type Config struct {
	Global   GlobalConfig    `yaml:"global"`
	Gateways []GatewayConfig `yaml:"gateways"`
	Vehicles []VehicleConfig `yaml:"vehicles"`
	GPSes    []GpsConfig     `yaml:"gpses"`
}

// GlobalConfig defines shared defaults across the system.
type GlobalConfig struct {
	WireFormat string `yaml:"wire_format"` // default wire format (csv/json)
	FogAddr    string `yaml:"fog_addr"`    // address for FogServer (e.g. ":10000") if blank server will not work
}

// GatewayConfig defines configuration for a single gateway instance.
type GatewayConfig struct {
	ID       string   `yaml:"id"`
	LoraDev  string   `yaml:"lora_device"`
	LoraBaud int      `yaml:"lora_baud"`
	WireIn   string   `yaml:"wire_in"`  // format received from vehicle
	WireOut  string   `yaml:"wire_out"` // format sent to fog
	FogURL   string   `yaml:"fog_url"`  // fog server endpoint
	Vehicles []string `yaml:"vehicles"` // vehicle IDs handled by this gateway
}

// VehicleConfig defines configuration for a single vehicle agent.
type VehicleConfig struct {
	ID                  string `yaml:"id"`
	LoraDev             string `yaml:"lora_device"`
	LoraBaud            int    `yaml:"lora_baud"`
	GpsDev              string `yaml:"gps_device"`
	GpsBaud             int    `yaml:"gps_baud"`
	TelemetryIntervalMs int    `yaml:"telemetry_interval_ms"`
	WireFormat          string `yaml:"wire_format"`
}

type GpsConfig struct {
	ID      string `yaml:"id"`
	GpsDev  string `yaml:"gps_device"`
	GpsBaud int    `yaml:"gps_baud"`
}
