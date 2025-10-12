package parser

import (
	"fmt"
	"strconv"
)

// ParseNMEACoord converts NMEA ddmm.mmmm to decimal degrees.
func ParseNMEACoord(value string, dir string) (float64, error) {
	if len(value) < 4 {
		return 0, fmt.Errorf("invalid nmea coord")
	}
	var degPart, minPart string
	// latitude has 2 digit degrees vs lon 3 digits; detect by dir
	if dir == "N" || dir == "S" {
		degPart = value[:2]
		minPart = value[2:]
	} else {
		degPart = value[:3]
		minPart = value[3:]
	}
	deg, err := strconv.ParseFloat(degPart, 64)
	if err != nil {
		return 0, err
	}
	min, err := strconv.ParseFloat(minPart, 64)
	if err != nil {
		return 0, err
	}
	dec := deg + min/60.0
	if dir == "S" || dir == "W" {
		dec = -dec
	}
	return dec, nil
}

// ToNMEACoord converts NMEA decimal degrees to ddmm.mmmm.
func ToNMEACoord(dec float64, isLat bool) (string, string) {
	dir := "N"
	if !isLat {
		dir = "E"
	}
	if dec < 0 {
		dec = -dec
		if isLat {
			dir = "S"
		} else {
			dir = "W"
		}
	}
	deg := int(dec)
	min := (dec - float64(deg)) * 60
	if isLat {
		return fmt.Sprintf("%02d%06.3f", deg, min), dir
	}
	return fmt.Sprintf("%03d%06.3f", deg, min), dir
}
