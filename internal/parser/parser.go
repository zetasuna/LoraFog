// Package parser converts CSV wire format to structured types and vice-versa.
//
// CSV telemetry wire format (vehicle -> gateway):
//
//	VEHICLE_ID,LAT,LON,HEAD,LEFT_SPEED,RIGHT_SPEED
//
// CSV control wire format (fog -> vehicle):
//
//	VEHICLE_ID,PAYLOAD,MSGID
package parser

import (
	"LoraFog/internal/model"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ParseTelemetryCSV parses a CSV telemetry line into model.VehicleData.
// Returns error on invalid format.
func ParseTelemetryCSV(line string) (model.VehicleData, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) != 6 {
		return model.VehicleData{}, fmt.Errorf("expected 6 fields, got %d", len(fields))
	}

	lat, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid lat")
	}
	lon, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid lon")
	}
	head, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid head")
	}
	left, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid left_speed")
	}
	right, err := strconv.ParseFloat(fields[5], 64)
	if err != nil {
		return model.VehicleData{}, errors.New("invalid right_speed")
	}

	return model.VehicleData{
		VehicleID: fields[0],
		Lat:       lat,
		Lon:       lon,
		Head:      head,
		LeftSpd:   left,
		RightSpd:  right,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// ControlToCSV converts a ControlMessage into CSV to send over LoRa.
// Format: VEHICLE_ID,PAYLOAD,MSGID
func ControlToCSV(ctl model.ControlMessage) string {
	msgID := ctl.MsgID
	if msgID == "" {
		msgID = strconv.FormatInt(time.Now().UnixNano(), 10)
	}
	payload := strings.ReplaceAll(ctl.Payload, ",", ";")
	return fmt.Sprintf("%s,%s,%s", ctl.VehicleID, payload, msgID)
}

// VehicleToCSV converts a VehicleData struct into CSV format to send over LoRa.
// Format: VEHICLE_ID,LAT,LON,HEAD,LEFT_SPEED,RIGHT_SPEED
func VehicleToCSV(v model.VehicleData) string {
	return fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f",
		v.VehicleID, v.Lat, v.Lon, v.Head, v.LeftSpd, v.RightSpd)
}
