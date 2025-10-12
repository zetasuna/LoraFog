// Package parser converts CSV wire format to structured types and vice-versa.
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
)

// ControlToCSV converts a ControlMessage into CSV to send over LoRa.
// Format: VEHICLE_ID,PAYLOAD,MSGID
func ControlToCSV(ctl model.ControlMessage) string {
	// msgID := ctl.MsgID
	// if msgID == "" {
	// 	msgID = strconv.FormatInt(time.Now().UnixNano(), 10)
	// }
	// payload := strings.ReplaceAll(ctl.Payload, ",", ";")
	// return fmt.Sprintf("%s,%s,%s", ctl.VehicleID, payload, msgID)
	return fmt.Sprintf("%s,%.1f,%.2f,%.6f,%.6f,%.2f,%.2f,%.2f",
		ctl.VehicleID, ctl.Mode, ctl.Spd, ctl.Lat, ctl.Lon, ctl.Kp, ctl.Ki, ctl.Kd)
}

func ParseControlCSV(line string) (model.ControlMessage, error) {
	fields := strings.Split(strings.TrimSpace(line), ",")
	if len(fields) != 8 {
		return model.ControlMessage{}, fmt.Errorf("expected 8 fields, got %d", len(fields))
	}

	mode, err := strconv.ParseFloat(fields[1], 64)
	if err != nil {
		return model.ControlMessage{}, errors.New("invalid mode")
	}
	spd, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return model.ControlMessage{}, errors.New("invalid speed")
	}
	lat, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return model.ControlMessage{}, errors.New("invalid lat")
	}
	lon, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return model.ControlMessage{}, errors.New("invalid lon")
	}
	kp, err := strconv.ParseFloat(fields[5], 64)
	if err != nil {
		return model.ControlMessage{}, errors.New("invalid kp")
	}
	ki, err := strconv.ParseFloat(fields[6], 64)
	if err != nil {
		return model.ControlMessage{}, errors.New("invalid ki")
	}
	kd, err := strconv.ParseFloat(fields[7], 64)
	if err != nil {
		return model.ControlMessage{}, errors.New("invalid kd")
	}

	return model.ControlMessage{
		VehicleID: fields[0],
		Mode:      mode,
		Spd:       spd,
		Lat:       lat,
		Lon:       lon,
		Kp:        kp,
		Ki:        ki,
		Kd:        kd,
		// Timestamp: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
