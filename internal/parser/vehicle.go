// Package parser converts CSV wire format to structured types and vice-versa.
//
// CSV telemetry wire format (vehicle -> gateway):
//
//	VEHICLE_ID,LAT,LON,HEAD,LEFT_SPEED,RIGHT_SPEED
package parser

import (
	"LoraFog/internal/model"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// VehicleToCSV converts a VehicleData struct into CSV format to send over LoRa.
// Format: VEHICLE_ID,LAT,LON,HEAD,LEFT_SPEED,RIGHT_SPEED
func VehicleToCSV(v model.VehicleData) string {
	return fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f,%.2f,%.1f",
		v.VehicleID, v.Lat, v.Lon, v.HeadCur, v.HeadTar, v.LeftSpd, v.RightSpd, v.PID)
}

// ParseTelemetryCSV parses a CSV telemetry line into model.VehicleData.
// Returns error on invalid format.
func ParseTelemetryCSV(line string) (model.VehicleData, error) {
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
	headCur, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid head_current")
	}
	headTar, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid head_target")
	}
	left, err := strconv.ParseFloat(fields[5], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid left_speed")
	}
	right, err := strconv.ParseFloat(fields[6], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid right_speed")
	}
	pid, err := strconv.ParseFloat(fields[7], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid pid")
	}

	return model.VehicleData{
		VehicleID: fields[0],
		Lat:       lat,
		Lon:       lon,
		HeadCur:   headCur,
		HeadTar:   headTar,
		LeftSpd:   left,
		RightSpd:  right,
		PID:       pid,
		// Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
