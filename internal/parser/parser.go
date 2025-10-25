// Package parser provides an abstraction layer for encoding and decoding data
// (telemetry and control messages) in multiple wire formats such as CSV and JSON.
package parser

import "LoraFog/internal/model"

// Parser defines a generic interface for encoding and decoding telemetry/control data.
// Different implementations support different wire formats such as CSV or JSON.
type Parser interface {
	// EncodeTelemetry converts a structured VehicleData into a wire string (CSV/JSON).
	EncodeTelemetry(model.VehicleData) (string, error)

	// DecodeTelemetry parses a raw string into a structured VehicleData.
	DecodeTelemetry(string) (model.VehicleData, error)

	// EncodeControl converts a ControlMessage into a wire string (CSV/JSON).
	EncodeControl(model.ControlMessage) (string, error)

	// DecodeControl parses a raw string into a structured ControlMessage.
	DecodeControl(string) (model.ControlMessage, error)
}
