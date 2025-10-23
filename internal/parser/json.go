// Package parser implements the JSONParser which encodes and decodes telemetry
// and control data in JSON format.
package parser

import (
	"LoraFog/internal/model"
	"encoding/json"
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

// EncodeControl encodes ControlMessage into JSON string.
func (p *JSONParser) EncodeControl(c model.ControlMessage) (string, error) {
	b, err := json.Marshal(c)
	return string(b), err
}

// DecodeControl decodes JSON string into ControlMessage.
func (p *JSONParser) DecodeControl(s string) (model.ControlMessage, error) {
	var c model.ControlMessage
	err := json.Unmarshal([]byte(s), &c)
	return c, err
}
