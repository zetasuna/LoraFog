// Package parser implements the CSVParser which handles encoding and decoding
// of telemetry and control data using comma-separated values format.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"LoraFog/internal/model"
)

// CSVParser implements Parser interface using CSV format.
// Example telemetry CSV: VEHICLE_ID,LAT,LON,HEAD_CUR,HEAD_TAR,LEFT,RIGHT,PID
type CSVParser struct{}

// NewCSVParser creates a new CSV parser instance.
func NewCSVParser() *CSVParser { return &CSVParser{} }

// EncodeTelemetry converts VehicleData into CSV string.
func (p *CSVParser) EncodeTelemetry(v model.VehicleData) (string, error) {
	line := fmt.Sprintf("%s,%.6f,%.6f,%d,%d,%d,%d,%d",
		v.VehicleID, v.Latitude, v.Longitude, v.CurrentHead, v.TargetHead, v.LeftSpeed, v.RightSpeed, v.PID)
	return line, nil
}

// DecodeTelemetry parses a CSV telemetry line into VehicleData struct.
func (p *CSVParser) DecodeTelemetry(line string) (model.VehicleData, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) != 8 {
		return model.VehicleData{}, fmt.Errorf("expected 8 fields, got %d", len(fields))
	}

	latitude, _ := strconv.ParseFloat(fields[1], 64)
	longitude, _ := strconv.ParseFloat(fields[2], 64)
	currentHead, _ := strconv.ParseFloat(fields[3], 64)
	targetHead, _ := strconv.ParseFloat(fields[4], 64)
	leftSpeed, _ := strconv.ParseFloat(fields[5], 64)
	rightSpeed, _ := strconv.ParseFloat(fields[6], 64)
	pid, _ := strconv.ParseFloat(fields[7], 64)

	return model.VehicleData{
		VehicleID:   fields[0],
		Latitude:    latitude,
		Longitude:   longitude,
		CurrentHead: int(currentHead),
		TargetHead:  int(targetHead),
		LeftSpeed:   int(leftSpeed),
		RightSpeed:  int(rightSpeed),
		PID:         int(pid),
	}, nil
}

// EncodeControl converts a ControlData into CSV string.
func (p *CSVParser) EncodeControl(c model.ControlData) (string, error) {
	line := fmt.Sprintf("%s,%d,%d,%.6f,%.6f,%.6f,%.6f,%.6f",
		c.VehicleID, c.Mode, c.Speed, c.Latitude, c.Longitude, c.Kp, c.Ki, c.Kd)
	return line, nil
}

// DecodeControl parses a CSV control message into ControlData struct.
func (p *CSVParser) DecodeControl(line string) (model.ControlData, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) != 8 {
		return model.ControlData{}, fmt.Errorf("expected 8 fields, got %d", len(fields))
	}

	mode, _ := strconv.ParseFloat(fields[1], 64)
	speed, _ := strconv.ParseFloat(fields[2], 64)
	latitude, _ := strconv.ParseFloat(fields[3], 64)
	longitude, _ := strconv.ParseFloat(fields[4], 64)
	kp, _ := strconv.ParseFloat(fields[5], 64)
	ki, _ := strconv.ParseFloat(fields[6], 64)
	kd, _ := strconv.ParseFloat(fields[7], 64)

	return model.ControlData{
		VehicleID: fields[0],
		Mode:      int(mode),
		Speed:     int(speed),
		Latitude:  latitude,
		Longitude: longitude,
		Kp:        kp,
		Ki:        ki,
		Kd:        kd,
	}, nil
}
