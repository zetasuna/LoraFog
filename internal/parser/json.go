// Package parser implements the JSONParser which encodes and decodes telemetry
// and control data in JSON format.
package parser

import (
	"encoding/json"

	"LoraFog/internal/model"
)

// JSONParser implements Parser interface using JSON serialization.
type JSONParser struct{}

// NewJSONParser creates a new JSON parser.
func NewJSONParser() *JSONParser { return &JSONParser{} }

// EncodeTelemetry encodes VehicleData into JSON string.
func (p *JSONParser) EncodeTelemetry(v model.VehicleData) (string, error) {
	b, err := json.Marshal(v)
	return string(b), err
}

// DecodeTelemetry decodes JSON string into VehicleData.
func (p *JSONParser) DecodeTelemetry(s string) (model.VehicleData, error) {
	var v model.VehicleData
	err := json.Unmarshal([]byte(s), &v)
	return v, err
}

// EncodeControl encodes ControlData into JSON string.
func (p *JSONParser) EncodeControl(c model.ControlData) (string, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

// DecodeControl decodes JSON string into ControlData.
func (p *JSONParser) DecodeControl(s string) (model.ControlData, error) {
	var c model.ControlData
	err := json.Unmarshal([]byte(s), &c)
	return c, err
}
