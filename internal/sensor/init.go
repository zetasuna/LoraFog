// Package sensor: Initialize Sensor and Generate Random Data
package sensor

import (
	"math"
	"math/rand"
	"time"
)

type SensorData struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Value     float64   `json:"value"`
	Unit      string    `json:"unit"`
	Timestamp time.Time `json:"timestamp"`
}

// Sensor đại diện cho 1 cảm biến cụ thể
type Sensor struct {
	ID       string
	TypeID   uint
	Interval uint
}

func NewSensor(id string, typeID uint, interval uint) *Sensor {
	return &Sensor{
		ID:       id,
		TypeID:   typeID,
		Interval: interval,
	}
}

// GenerateData create random data for sensor
func (s *Sensor) GenerateData() SensorData {
	var typeName string
	var value float64
	var unit string

	switch s.TypeID {
	case 1:
		typeName = "temperature"
		value = math.Round((25+rand.Float64()*5)*100) / 100
		unit = "°C"
	case 2:
		typeName = "pressure"
		value = math.Round((100+rand.Float64()*20)*100) / 100
		unit = "Pa"
	case 3:
		typeName = "flow"
		value = math.Round((rand.Float64()*100)*100) / 100
		unit = "L/s"
	case 4:
		typeName = "water_level"
		value = math.Round((2.5+rand.Float64()*1.5)*100) / 100
		unit = "m"
	default:
		typeName = "N/A"
		value = 0
		unit = "N/A"
	}

	return SensorData{
		ID:        s.ID,
		Type:      typeName,
		Value:     value,
		Unit:      unit,
		Timestamp: time.Now(),
	}
}
