// Package parser implements the CSVParser which handles encoding and decoding
// of telemetry and control data using comma-separated values format.
package parser

import (
	"LoraFog/internal/model"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// CSVParser implements Parser interface using CSV format.
// Example telemetry CSV: VEHICLE_ID,LAT,LON,HEAD_CUR,HEAD_TAR,LEFT,RIGHT,PID
type CSVParser struct{}

// NewCSVParser creates a new CSV parser instance.
func NewCSVParser() *CSVParser { return &CSVParser{} }

// EncodeTelemetry converts VehicleData into CSV string.
func (p *CSVParser) EncodeTelemetry(v model.VehicleData) (string, error) {
	line := fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f,%.2f,%.1f",
		v.VehicleID, v.Lat, v.Lon, v.HeadCur, v.HeadTar, v.LeftSpd, v.RightSpd, v.PID)
	return line, nil
}

// DecodeTelemetry parses a CSV telemetry line into VehicleData struct.
func (p *CSVParser) DecodeTelemetry(line string) (model.VehicleData, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) != 8 {
		return model.VehicleData{}, fmt.Errorf("expected 8 fields, got %d", len(fields))
	}

	lat, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid lat")
	}
	lon, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid lon")
	}
	headCur, _ := strconv.ParseFloat(fields[3], 64)
	headTar, _ := strconv.ParseFloat(fields[4], 64)
	left, _ := strconv.ParseFloat(fields[5], 64)
	right, _ := strconv.ParseFloat(fields[6], 64)
	pid, _ := strconv.ParseFloat(fields[7], 64)

	return model.VehicleData{
		VehicleID: fields[0],
		Lat:       lat,
		Lon:       lon,
		HeadCur:   headCur,
		HeadTar:   headTar,
		LeftSpd:   left,
		RightSpd:  right,
		PID:       pid,
	}, nil
}

// EncodeControl converts a ControlMessage into CSV string.
func (p *CSVParser) EncodeControl(c model.ControlMessage) (string, error) {
	line := fmt.Sprintf("%s,%.1f,%.2f,%.6f,%.6f,%.2f,%.2f,%.2f",
		c.VehicleID, c.Mode, c.Spd, c.Lat, c.Lon, c.Kp, c.Ki, c.Kd)
	return line, nil
}

// DecodeControl parses a CSV control message into ControlMessage struct.
func (p *CSVParser) DecodeControl(line string) (model.ControlMessage, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) != 8 {
		return model.ControlMessage{}, fmt.Errorf("expected 8 fields, got %d", len(fields))
	}

	mode, _ := strconv.ParseFloat(fields[1], 64)
	spd, _ := strconv.ParseFloat(fields[2], 64)
	lat, _ := strconv.ParseFloat(fields[3], 64)
	lon, _ := strconv.ParseFloat(fields[4], 64)
	kp, _ := strconv.ParseFloat(fields[5], 64)
	ki, _ := strconv.ParseFloat(fields[6], 64)
	kd, _ := strconv.ParseFloat(fields[7], 64)

	return model.ControlMessage{
		VehicleID: fields[0],
		Mode:      mode,
		Spd:       spd,
		Lat:       lat,
		Lon:       lon,
		Kp:        kp,
		Ki:        ki,
		Kd:        kd,
	}, nil
}
