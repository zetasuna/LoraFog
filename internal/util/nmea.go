// Package util provides NMEA coordinate conversion utilities for GPS data.
// It supports parsing from ddmm.mmmm format and conversion to decimal degrees.
package util

import (
	"fmt"
	"strconv"
)

// ParseNMEACoord converts NMEA ddmm.mmmm format to decimal degrees.
// For example, 2101.7102,N -> 21.0285033
func ParseNMEACoord(value string, dir string) (float64, error) {
	if len(value) < 4 {
		return 0, fmt.Errorf("invalid NMEA coord")
	}
	var degPart, minPart string
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

// ToNMEACoord converts decimal degrees to ddmm.mmmm string format.
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
