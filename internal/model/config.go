// Package model defines configuration and message structures used across LoraFog.
package model

// CHANGELOG (refactor v2):
// - Added JSON and CBOR tags to all serializable structs
// - Consolidated config structs and added Validate() for config validation
// - Clarified naming and godoc comments

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
	Addr     string            `yaml:"address" json:"address" cbor:"address"`
	AppAddr  string            `yaml:"app_address" json:"app_address" cbor:"app_address"`
	Gateways []GatewayRegistry `yaml:"gateway_registry" json:"gateway_registry" cbor:"gateway_registry"`
}

// GatewayRegistry is used to register gateways to the fog server on startup.
type GatewayRegistry struct {
	ID   string `yaml:"id" json:"id" cbor:"id"`
	Addr string `yaml:"address" json:"address" cbor:"address"`
	// Vehicles []string `yaml:"vehicles" json:"vehicles" cbor:"vehicles"`
}

// GatewayConfig config for a Gateway instance.
type GatewayConfig struct {
	ID         string `yaml:"id" json:"id" cbor:"id"`
	Addr       string `yaml:"address" json:"address" cbor:"address"`
	ServerAddr string `yaml:"server_address" json:"server_address" cbor:"server_address"`
	LoraDev    string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud   int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
}

// VehicleConfig config for a Vehicle agent.
type VehicleConfig struct {
	ID          string `yaml:"id" json:"id" cbor:"id"`
	LoraDev     string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud    int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
	ArduinoDev  string `yaml:"arduino_device" json:"arduino_device" cbor:"arduino_device"`
	ArduinoBaud int    `yaml:"arduino_baud" json:"arduino_baud" cbor:"arduino_baud"`
}

// ArduinoConfig defines a serial Arduino connection.
type ArduinoConfig struct {
	Dev  string `yaml:"device" json:"device" cbor:"device"`
	Baud int    `yaml:"baud" json:"baud" cbor:"baud"`
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
	if c.Server.Addr == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no server configured")
	}
	if c.Server.Addr == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no gateways configured")
	}
	if c.Server.Addr == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no vehicles configured")
	}
	ids := map[string]struct{}{}
	for i, gw := range c.Gateways {
		if gw.ID == "" {
			return fmt.Errorf("gateways[%d]: id is empty", i)
		}
		if gw.Addr == "" {
			return fmt.Errorf("gateways[%d]: address is empty", i)
		}
		if gw.ServerAddr == "" {
			return fmt.Errorf("gateways[%d]: server_address is empty", i)
		}
		if _, ok := ids[gw.ID]; ok {
			return fmt.Errorf("duplicate id in gateways: %s", gw.ID)
		}
		ids[gw.ID] = struct{}{}
	}
	// Validate vehicles
	for i, v := range c.Vehicles {
		if v.ID == "" {
			return fmt.Errorf("vehicles[%d]: id is empty", i)
		}
	}
	return nil
}
