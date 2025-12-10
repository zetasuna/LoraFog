// Package model defines configuration and message structures used across LoraFog.
package model

import (
	"errors"
	"fmt"
)

// Config is the root configuration structure for the system.
type Config struct {
	Server         ServerConfig          `yaml:"server" json:"server" cbor:"server"`
	Gateways       []GatewayConfig       `yaml:"gateways" json:"gateways" cbor:"gateways"`
	Vehicles       []VehicleConfig       `yaml:"vehicles" json:"vehicles" cbor:"vehicles"`
	Arduinos       []ArduinoConfig       `yaml:"arduinos" json:"arduinos" cbor:"arduinos"`
	VirtualSerials []VirtualSerialConfig `yaml:"virtual_serials" json:"virtual_serials" cbor:"virtual_serials"`
}

// ServerConfig configures the fog server and app forwarder.
type ServerConfig struct {
	Address            string `yaml:"address" json:"address" cbor:"address"`
	AppAddress         string `yaml:"app_address" json:"app_address" cbor:"app_address"`
	VirtualGateways    int    `yaml:"virtual_gateways" json:"virtual_gateways" cbor:"virtual_gateways"`
	VehiclesPerGateway int    `yaml:"vehicles_per_gateway" json:"vehicles_per_gateway" cbor:"vehicles_per_gateway"`
}

// GatewayConfig config for a Gateway instance.
type GatewayConfig struct {
	Address       string `yaml:"address" json:"address" cbor:"address"`
	ServerAddress string `yaml:"server_address" json:"server_address" cbor:"server_address"`
	LoraDevice    string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud      int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
}

// VehicleConfig config for a Vehicle agent.
type VehicleConfig struct {
	VehicleID     string `yaml:"id" json:"id" cbor:"id"`
	LoraDevice    string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud      int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
	ArduinoDevice string `yaml:"arduino_device" json:"arduino_device" cbor:"arduino_device"`
	ArduinoBaud   int    `yaml:"arduino_baud" json:"arduino_baud" cbor:"arduino_baud"`
}

// ArduinoConfig defines a serial Arduino connection.
type ArduinoConfig struct {
	Device string `yaml:"device" json:"device" cbor:"device"`
	Baud   int    `yaml:"baud" json:"baud" cbor:"baud"`
}

// VirtualSerialConfig defines a pair of linked virtual serial endpoints.
type VirtualSerialConfig struct {
	Left  string `yaml:"left" json:"left" cbor:"left"`
	Right string `yaml:"right" json:"right" cbor:"right"`
}

// Validate checks the configuration for basic correctness.
// It returns an error describing the first problem found.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.Server.Address == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no server configured")
	}
	for i, gw := range c.Gateways {
		if gw.Address == "" {
			return fmt.Errorf("gateways[%d]: address is empty", i)
		}
		if gw.ServerAddress == "" {
			return fmt.Errorf("gateways[%d]: server_address is empty", i)
		}
	}
	// Validate vehicles
	for i, v := range c.Vehicles {
		if v.VehicleID == "" {
			return fmt.Errorf("vehicles[%d]: id is empty", i)
		}
	}
	return nil
}
