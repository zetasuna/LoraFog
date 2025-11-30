
- /arduino/src/main.cpp
```go
// FINAL

#include <Arduino.h>
#include <QMC5883LCompass.h>
#include <Servo.h>
#include <SoftwareSerial.h>
#include <TinyGPSPlus.h>
#include <stdint.h>

// === CONFIG ===
#define GPS_TX_PIN 3
#define GPS_RX_PIN 4
#define RIGHT_ESC_PIN 2
#define LEFT_ESC_PIN 5
#define SERIAL_BAUD 9600
#define GPS_BAUD 9600
#define MIN_PPM 1000
#define MAX_PPM 2000
#define DEFAULT_LATITUDE 0.0
#define DEFAULT_LONGITUDE 0.0
#define DEFAULT_HEADING 0
#define UPDATE_INTERVAL 1000
#define DISTANCE_STOP 2.0 // 2m

// === STRUCT ===
struct TelemetryData {
  float latitude;
  float longitude;
  int16_t left_motor_speed;  // ppm
  int16_t right_motor_speed; // ppm
  int16_t current_heading;   // degrees
  int16_t desired_heading;   // degrees
};

struct ControlData {
  int16_t cruise_speed; // ppm
  float latitude;
  float longitude;
  float kp;
  float ki;
  float kd;
};

// === GLOBALS ===
QMC5883LCompass compass;
Servo right_esc;
Servo left_esc;
SoftwareSerial gpsSerial(GPS_TX_PIN, GPS_RX_PIN);
TinyGPSPlus gps;

struct TelemetryData t_data;
struct ControlData c_data;

bool has_target = false;
int16_t left_motor_speed = MIN_PPM;
int16_t right_motor_speed = MIN_PPM;
float latitude = DEFAULT_LATITUDE;
float longitude = DEFAULT_LONGITUDE;
int16_t current_heading = DEFAULT_HEADING;
int16_t desired_heading = DEFAULT_HEADING;
unsigned long last_update = 0;

// === FUNCTION DECLARATIONS ===
void autoControl();
void stopBoat();
bool isAtTarget(float current_latitude, float current_longitude,
                float target_latitude, float target_longitude);
void updateTelemetry();
void sendTelemetry();
void processControlMessage(const String &line);
int16_t getHeading();
int16_t calculateBearing(float current_latitude, float current_longitude,
                         float target_latitude, float target_longitude);
int16_t calculateTurnAngle(int16_t current, int16_t desired);
int16_t calculateLeftSpeed(int16_t error, int16_t turn_direction);
int16_t calculateRightSpeed(int16_t error, int16_t turn_direction);

// === SETUP ===
void setup() {
  Serial.begin(SERIAL_BAUD);
  gpsSerial.begin(GPS_BAUD);
  compass.init();

  right_esc.attach(RIGHT_ESC_PIN);
  left_esc.attach(LEFT_ESC_PIN);
  right_esc.writeMicroseconds(MIN_PPM);
  left_esc.writeMicroseconds(MIN_PPM);
  delay(2000);

  t_data.latitude = DEFAULT_LATITUDE;
  t_data.longitude = DEFAULT_LONGITUDE;
  t_data.left_motor_speed = MIN_PPM;
  t_data.right_motor_speed = MIN_PPM;
  t_data.current_heading = DEFAULT_HEADING;
  t_data.desired_heading = DEFAULT_HEADING;

  stopBoat();
}

// === LOOP ===
void loop() {
  unsigned long now = millis();
  if (now - last_update >= UPDATE_INTERVAL) {
    autoControl();
    updateTelemetry();
    sendTelemetry();
    last_update = now;
  }

  if (gpsSerial.available()) {
    if (gps.encode(gpsSerial.read())) {
      latitude = gps.location.isValid() ? gps.location.lat() : 0.0;
      longitude = gps.location.isValid() ? gps.location.lng() : 0.0;
      if (has_target && gps.location.isValid()) {
        desired_heading = calculateBearing(latitude, longitude, c_data.latitude,
                                           c_data.longitude);
      }
    }
  }

  if (Serial.available()) {
    String control = Serial.readStringUntil('\n');
    control.trim();
    if (control.length() > 0) {
      processControlMessage(control);
      has_target = true;
    }
  }
}

// === CONTROL BOAT ===
void autoControl() {
  if (has_target &&
      isAtTarget(latitude, longitude, c_data.latitude, c_data.longitude)) {
    has_target = false;
    stopBoat();
    return;
  }
  current_heading = getHeading();
  int16_t turn_angle = calculateTurnAngle(current_heading, desired_heading);
  int16_t turn_direction = (turn_angle > 180) ? -1 : 1;
  int16_t error = (turn_angle > 180) ? (turn_angle - 360) : turn_angle;
  error = constrain(error, -90, 90);

  left_motor_speed = calculateLeftSpeed(error, turn_direction);
  right_motor_speed = calculateRightSpeed(error, turn_direction);

  left_motor_speed = constrain(left_motor_speed, MIN_PPM, MAX_PPM);
  right_motor_speed = constrain(right_motor_speed, MIN_PPM, MAX_PPM);

  left_esc.writeMicroseconds(left_motor_speed);
  right_esc.writeMicroseconds(right_motor_speed);
}

void stopBoat() {
  left_motor_speed = MIN_PPM;
  right_motor_speed = MIN_PPM;
  left_esc.writeMicroseconds(left_motor_speed);
  right_esc.writeMicroseconds(right_motor_speed);
}

// === CHECK ARRIVAL ===
bool isAtTarget(float current_latitude, float current_longitude,
                float target_latitude, float target_longitude) {
  const float EARTH_RADIUS = 6371000.0; // m
  float delta_latitude = radians(target_latitude - current_latitude);
  float delta_longitude = radians(target_longitude - current_longitude);
  float a = sin(delta_latitude / 2) * sin(delta_latitude / 2) +
            cos(radians(current_latitude)) * cos(radians(target_latitude)) *
                sin(delta_longitude / 2) * sin(delta_longitude / 2);
  float c = 2 * atan2(sqrt(a), sqrt(1 - a));
  float distance = EARTH_RADIUS * c;
  return distance < DISTANCE_STOP; // đã đến nếu cách mục tiêu < 2m
}

// === TELEMETRY ===
void updateTelemetry() {
  t_data.latitude = latitude;
  t_data.longitude = longitude;
  t_data.left_motor_speed = left_motor_speed;
  t_data.right_motor_speed = right_motor_speed;
  t_data.current_heading = current_heading;
  t_data.desired_heading = desired_heading;
}

void sendTelemetry() {
  Serial.print(t_data.latitude, 6);
  Serial.print(",");
  Serial.print(t_data.longitude, 6);
  Serial.print(",");
  Serial.print(t_data.left_motor_speed);
  Serial.print(",");
  Serial.print(t_data.right_motor_speed);
  Serial.print(",");
  Serial.print(t_data.current_heading);
  Serial.print(",");
  Serial.println(t_data.desired_heading);
}

// === CONTROL ===
void processControlMessage(const String &line) {
  int16_t last_index = 0;
  int16_t token_index = 0;
  String tokens[6];
  for (int16_t i = 0; i < 6; i++)
    tokens[i] = "";

  while (token_index < 6) {
    int comma_index = line.indexOf(',', last_index);
    if (comma_index == -1) {
      tokens[token_index++] = line.substring(last_index);
      break;
    }
    tokens[token_index++] = line.substring(last_index, comma_index);
    last_index = comma_index + 1;
  }

  c_data.cruise_speed = tokens[0].toInt();
  c_data.latitude = tokens[1].toFloat();
  c_data.longitude = tokens[2].toFloat();
  c_data.kp = tokens[3].toFloat();
  c_data.ki = tokens[4].toFloat();
  c_data.kd = tokens[5].toFloat();
  // Serial.print(c_data.kp, 6);
  // Serial.print(",");
  // Serial.print(c_data.ki, 6);
  // Serial.print(",");
  // Serial.print(c_data.kd, 6);
  // Serial.print(",");
  // Serial.print(c_data.cruise_speed);
  // Serial.print(",");
  // Serial.println(c_data.desired_heading);
}

// === UTILS ===
int16_t getHeading() {
  compass.read();
  int16_t azimuth = compass.getAzimuth();
  if (azimuth < 0)
    azimuth += 360;
  return azimuth;
}

int16_t calculateBearing(float current_latitude, float current_longitude,
                         float target_latitude, float target_longitude) {
  float delta_longitude = radians(target_longitude - current_longitude);
  float y = sin(delta_longitude) * cos(radians(target_latitude));
  float x = cos(radians(current_latitude)) * sin(radians(target_latitude)) -
            sin(radians(current_latitude)) * cos(radians(target_latitude)) *
                cos(delta_longitude);
  float bearing = atan2(y, x) * 180 / PI;
  bearing = fmodf((bearing + 360), 360);
  return int16_t(round(bearing));
}

int16_t calculateTurnAngle(int16_t current, int16_t desired) {
  return (desired - current + 360) % 360;
}

int16_t calculateLeftSpeed(int16_t error, int16_t turn_direction) {
  int16_t left_speed =
      c_data.cruise_speed + (c_data.kp * error * turn_direction);
  left_speed = constrain(left_speed, MIN_PPM, MAX_PPM);
  return left_speed;
}

int16_t calculateRightSpeed(int16_t error, int16_t turn_direction) {
  int16_t right_speed =
      c_data.cruise_speed - (c_data.kp * error * turn_direction);
  right_speed = constrain(right_speed, MIN_PPM, MAX_PPM);
  return right_speed;
}

```

- /cmd/lora_fog/main.go
```go
// Command lora runs the LoraFog system orchestrator.
//
// CHANGELOG (refactor v2):
// - Unified startup with structured logging and graceful shutdown
// - Integrated util.SetupLogger()
// - Uses context cancellation on SIGINT/SIGTERM
// - Validates YAML configuration before launch
// - Safe close sequence for System
// - Clean exit codes and consistent logs

package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"LoraFog/internal/core"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

func main() {
	// --- Parse CLI flags ---
	cfgPath := flag.String("c", "configs/config.yml", "Path to YAML configuration file")
	flag.Parse()

	// --- Setup global logger ---
	util.SetupLogger()
	slog.Info("starting LoraFog runtime", "component", "main", "config", *cfgPath)

	// --- Validate config early ---
	cfg, err := loadAndValidateConfig(*cfgPath)
	if err != nil {
		slog.Error("invalid configuration", "component", "main", "error", err)
		os.Exit(1)
	}
	_ = cfg // not used directly (System loads internally)

	// --- Create system instance ---
	system, err := core.NewSystem(*cfgPath)
	if err != nil {
		slog.Error("failed to initialize system", "component", "main", "error", err)
		os.Exit(1)
	}

	// --- Setup signal handling for graceful shutdown ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// --- Start all components ---
	if err := system.Start(ctx); err != nil {
		slog.Error("system start failed", "component", "main", "error", err)
		os.Exit(1)
	}

	// --- Wait for termination signal ---
	slog.Info("system running (press Ctrl+C to exit)", "component", "main")
	<-ctx.Done()

	// --- Graceful shutdown ---
	slog.Info("shutting down system...", "component", "main")
	system.Stop()

	// Allow time for async cleanup/logs
	time.Sleep(500 * time.Millisecond)
	slog.Info("LoraFog terminated successfully", "component", "main")
}

// loadAndValidateConfig loads the YAML file and validates it.
func loadAndValidateConfig(path string) (*model.Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	if err := cfg.Validate(); err != nil {
		return nil, errors.New("configuration invalid: " + err.Error())
	}
	return &cfg, nil
}

```

- /internal/model/config.go
```go
// Package model defines configuration and message structures used across LoraFog.
package model

import (
	"errors"
	"fmt"
)

// Config is the root configuration structure for the system.
type Config struct {
	Server         ServerConfig          `yaml:"server" json:"server" cbor:"server"`
	Gateways       []GatewayConfig       `yaml:"gateways" json:"gateways" cbor:"gateways"`
	Vehicles       []VehicleConfig       `yaml:"vehicles" json:"vehicles" cbor:"vehicles"`
	Arduinos       []ArduinoConfig       `yaml:"arduinos" json:"arduinos" cbor:"arduinos"`
	VirtualSerials []VirtualSerialConfig `yaml:"virtual_serials" json:"virtual_serials" cbor:"virtual_serials"`
}

// ServerConfig configures the fog server and app forwarder.
type ServerConfig struct {
	Address    string `yaml:"address" json:"address" cbor:"address"`
	AppAddress string `yaml:"app_address" json:"app_address" cbor:"app_address"`
}

// GatewayConfig config for a Gateway instance.
type GatewayConfig struct {
	Address       string `yaml:"address" json:"address" cbor:"address"`
	ServerAddress string `yaml:"server_address" json:"server_address" cbor:"server_address"`
	LoraDevice    string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud      int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
}

// VehicleConfig config for a Vehicle agent.
type VehicleConfig struct {
	VehicleID     string `yaml:"id" json:"id" cbor:"id"`
	LoraDevice    string `yaml:"lora_device" json:"lora_device" cbor:"lora_device"`
	LoraBaud      int    `yaml:"lora_baud" json:"lora_baud" cbor:"lora_baud"`
	ArduinoDevice string `yaml:"arduino_device" json:"arduino_device" cbor:"arduino_device"`
	ArduinoBaud   int    `yaml:"arduino_baud" json:"arduino_baud" cbor:"arduino_baud"`
}

// ArduinoConfig defines a serial Arduino connection.
type ArduinoConfig struct {
	Device string `yaml:"device" json:"device" cbor:"device"`
	Baud   int    `yaml:"baud" json:"baud" cbor:"baud"`
}

// VirtualSerialConfig defines a pair of linked virtual serial endpoints.
type VirtualSerialConfig struct {
	Left  string `yaml:"left" json:"left" cbor:"left"`
	Right string `yaml:"right" json:"right" cbor:"right"`
}

// Validate checks the configuration for basic correctness.
// It returns an error describing the first problem found.
func (c *Config) Validate() error {
	if c == nil {
		return errors.New("config is nil")
	}
	if c.Server.Address == "" && len(c.Gateways) == 0 && len(c.Vehicles) == 0 {
		return errors.New("no server configured")
	}
	for i, gw := range c.Gateways {
		if gw.Address == "" {
			return fmt.Errorf("gateways[%d]: address is empty", i)
		}
		if gw.ServerAddress == "" {
			return fmt.Errorf("gateways[%d]: server_address is empty", i)
		}
	}
	// Validate vehicles
	for i, v := range c.Vehicles {
		if v.VehicleID == "" {
			return fmt.Errorf("vehicles[%d]: id is empty", i)
		}
	}
	return nil
}

```

- /internal/model/message.go
```go
// Package model provides message definitions exchanged across components.
package model

const (
	PacketTelemetry string = "t"
	PacketControl   string = "c"
	PacketBeacon    string = "b"
	PacketHello     string = "h"
)

// BeaconMessage broadcast bởi Gateway đầu mỗi chu kỳ TDMA.
type BeaconMessage struct {
	Type             string `json:"type" cbor:"type"` // "beacon"
	GatewayAddress   string `json:"gateway_address" cbor:"gateway_address"`
	CycleStart       int64  `json:"cycle_start" cbor:"cycle_start"`
	NextCycleStart   int64  `json:"next_cycle_start" cbor:"next_cycle_start"`
	CycleDurationMs  int64  `json:"cycle_dur_ms" cbor:"cycle_dur_ms"` // Tổng thời gian chu kỳ
	SlotDurationMs   int64  `json:"slot_dur_ms" cbor:"slot_dur_ms"`   // Thời gian 1 slot
	GuardTimeMs      int64  `json:"guard_ms" cbor:"guard_ms"`         // Thời gian nghỉ giữa các slot
	RegisterWindowMs int64  `json:"reg_win_ms" cbor:"reg_win_ms"`     // Thời gian cuối cho đăng ký
	// Map: VehicleID -> SlotIndex. Vehicle dựa vào đây để biết mình được gửi ở slot nào.
	SlotMap map[string]int `json:"slot_map" cbor:"slot_map"`
}

// HelloMessage gửi bởi Vehicle trong vùng Register Window để đăng ký.
type HelloMessage struct {
	Type      string `json:"type" cbor:"type"` // "hello"
	VehicleID string `json:"vehicle" cbor:"vehicle"`
}

// RegisterRequest là cấu trúc Server nhận từ Gateway
type RegisterRequest struct {
	GatewayAddress string `json:"gateway_address"`
	VehicleID      string `json:"vehicle_id"`
}

// RegisterResponse trả về slot cho Gateway (khi register thành công)
type RegisterResponse struct {
	Slot           int    `json:"slot"`
	GatewayAddress string `json:"gateway_address"`
}

// ArduinoData giả lập dữ liệu thô từ Arduino/Cảm biến.
type ArduinoData struct {
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	CurrentHead int64   `json:"current_head" cbor:"current_head"`
	TargetHead  int64   `json:"target_head" cbor:"target_head"`
	LeftSpeed   int64   `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int64   `json:"right_speed" cbor:"right_speed"`
}

// VehicleData là gói tin Telemetry gửi từ Vehicle lên Gateway.
type VehicleData struct {
	Type        string  `json:"type" cbor:"type"` // "telemetry"
	VehicleID   string  `json:"vehicle_id" cbor:"vehicle_id"`
	Latitude    float64 `json:"latitude" cbor:"latitude"`
	Longitude   float64 `json:"longitude" cbor:"longitude"`
	CurrentHead int64   `json:"current_head" cbor:"current_head"`
	TargetHead  int64   `json:"target_head" cbor:"target_head"`
	LeftSpeed   int64   `json:"left_speed" cbor:"left_speed"`
	RightSpeed  int64   `json:"right_speed" cbor:"right_speed"`
}

type VehicleApp struct {
	VehicleID   string  `json:"boatId" cbor:"vehicle_id"`
	Latitude    float64 `json:"lat" cbor:"latitude"`
	Longitude   float64 `json:"lon" cbor:"longitude"`
	CurrentHead int64   `json:"head" cbor:"current_head"`
	TargetHead  int64   `json:"targetHead" cbor:"target_head"`
	LeftSpeed   int64   `json:"leftSpeed" cbor:"left_speed"`
	RightSpeed  int64   `json:"rightSpeed" cbor:"right_speed"`
}

// ControlData là gói tin điều khiển gửi từ Gateway xuống Vehicle.
type ControlData struct {
	Type      string  `json:"type" cbor:"type"` // "control"
	VehicleID string  `json:"vehicle_id" cbor:"vehicle_id"`
	Speed     int     `json:"speed" cbor:"speed"`
	Latitude  float64 `json:"latitude" cbor:"latitude"`
	Longitude float64 `json:"longitude" cbor:"longitude"`
	Kp        float64 `json:"kp" cbor:"kp"`
	Ki        float64 `json:"ki" cbor:"ki"`
	Kd        float64 `json:"kd" cbor:"kd"`
}

type ControlApp struct {
	VehicleID string  `json:"boatId" cbor:"vehicle_id"`
	Speed     int     `json:"speed" cbor:"speed"`
	Latitude  float64 `json:"targetLat" cbor:"latitude"`
	Longitude float64 `json:"targetLon" cbor:"longitude"`
	Kp        float64 `json:"kp" cbor:"kp"`
	Ki        float64 `json:"ki" cbor:"ki"`
	Kd        float64 `json:"kd" cbor:"kd"`
}

```

- /internal/util/logger.go
```go
// Package util provides small utilities used by the system.
package util

import (
	"log/slog"
	"os"
)

// SetupLogger configures the global slog logger.
// Call once in main prior to starting System.
func SetupLogger() {
	// Use default handler (console) but include time and source.
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: false,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)
	slog.Info("logger initialized", "component", "util")
}

```

- /internal/util/socat.go
```go
// Package util provides helpers for virtual serial management using socat.
package util

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// SocatManager manages socat processes used to create virtual serial pairs.
type SocatManager struct {
	mu      sync.Mutex
	cmds    []*exec.Cmd
	links   []string
	stopped bool
}

// NewSocatManager constructs an empty SocatManager.
func NewSocatManager() *SocatManager {
	return &SocatManager{}
}

// CreatePair starts a socat process to link two PTYs (left <-> right).
// It returns an error if socat cannot be started.
func (m *SocatManager) CreatePair(left, right string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return fmt.Errorf("socat manager is stopped")
	}

	cmd := exec.CommandContext(context.Background(),
		"socat", "-d", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", left),
		fmt.Sprintf("pty,raw,echo=0,link=%s", right),
	)
	// Direct logs to program stderr/stdout for visibility.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.Warn("failed to start socat", "component", "socat", "left", left, "right", right, "error", err)
		return fmt.Errorf("start socat: %w", err)
	}

	slog.Info("socat started", "component", "socat", "pid", cmd.Process.Pid, "left", left, "right", right)
	m.cmds = append(m.cmds, cmd)
	m.links = append(m.links, left, right)

	// Give socat some time to create the links
	timeout := time.After(500 * time.Millisecond)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return nil
		case <-ticker.C:
			if _, err := os.Stat(left); err == nil {
				return nil
			}
		}
	}
}

// Cleanup stops all started socat processes and removes links created.
func (m *SocatManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.stopped = true

	for _, cmd := range m.cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		p := cmd.Process
		slog.Info("killing socat process", "component", "socat", "pid", p.Pid)
		_ = p.Signal(syscall.SIGTERM)
		// wait with timeout
		done := make(chan error, 1)
		go func(c *exec.Cmd) { done <- c.Wait() }(cmd)
		select {
		case <-time.After(500 * time.Millisecond):
			_ = p.Kill()
		case <-done:
		}
	}

	// Remove links if exist
	for _, path := range m.links {
		if _, err := os.Lstat(path); err == nil {
			if err := os.Remove(path); err != nil {
				slog.Warn("failed to remove socat link", "component", "socat", "path", path, "error", err)
			} else {
				slog.Info("removed socat link", "component", "socat", "path", path)
			}
		}
	}

	// clear slices
	m.cmds = nil
	m.links = nil
	slog.Info("socat cleanup complete", "component", "socat")
}

// CleanupAll is a failsafe that attempts to kill any running socat globally.
func (m *SocatManager) CleanupAll() {
	// Best-effort: use pkill if available.
	_ = exec.Command("pkill", "-f", "socat").Run()
	slog.Info("socat global cleanup attempted", "component", "socat")
}

```

- /internal/core/system.go
```go
// Package core orchestrates system startup and shutdown.
package core

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"LoraFog/internal/database"
	"LoraFog/internal/device"
	"LoraFog/internal/model"
	"LoraFog/internal/util"

	"gopkg.in/yaml.v3"
)

// System manages Fog server, gateways, and vehicles.
type System struct {
	configPath   string
	config       *model.Config
	server       *Server
	gateways     []*Gateway
	vehicles     []*Vehicle
	arduinos     []*device.Arduino
	serverDB     *database.ServerDB
	socatManager *util.SocatManager

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewSystem constructs a System from YAML configuration.
func NewSystem(path string) (*System, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg model.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}

	sys := &System{
		configPath:   path,
		config:       &cfg,
		socatManager: util.NewSocatManager(),
	}

	for _, vs := range cfg.VirtualSerials {
		if err := sys.socatManager.CreatePair(
			vs.Left,
			vs.Right,
		); err != nil {
			slog.Error("Failed to create socat pair", "left", vs.Left, "right", vs.Right, "error", err)
		}
	}
	time.Sleep(300 * time.Millisecond)

	// 1. Mở kết nối DB ở đây (trong main)
	dsn := "admin:admin@tcp(localhost:3306)/boat_db"
	sys.serverDB, err = database.NewServerDB(dsn, 10, 5)
	if err != nil {
		slog.Error("Database established fail", "error", err)
	} else {
		slog.Info("Database established success")
	}
	if cfg.Server.Address != "" {
		sys.server = NewServer(
			cfg.Server.Address,
			cfg.Server.AppAddress,
			sys.serverDB,
		)
	}
	for _, g := range cfg.Gateways {
		sys.gateways = append(sys.gateways, NewGateway(
			g.Address,
			g.ServerAddress,
			g.LoraDevice,
			g.LoraBaud,
		))
	}
	for _, v := range cfg.Vehicles {
		sys.vehicles = append(sys.vehicles, NewVehicle(
			v.VehicleID,
			v.LoraDevice,
			v.LoraBaud,
			v.ArduinoDevice,
			v.ArduinoBaud,
		))
	}
	for _, a := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos, device.NewArduino(a.Device, a.Baud))
	}

	return sys, nil
}

// Start launches all system components.
func (s *System) Start(ctx context.Context) error {
	if s.server == nil && len(s.gateways) == 0 && len(s.vehicles) == 0 {
		return fmt.Errorf("no active components")
	}
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	if s.server != nil {
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			_ = s.server.Start(ctx)
		}()
	}
	for _, gw := range s.gateways {
		_ = gw.Start(ctx)
	}
	for _, vh := range s.vehicles {
		_ = vh.Start(ctx)
	}
	for _, ino := range s.arduinos {
		s.wg.Add(1)
		go func(ino *device.Arduino) {
			defer s.wg.Done()
			stop := make(chan struct{})
			go func() {
				<-ctx.Done()
				close(stop)
			}()
			_ = ino.StartSimulation(stop)
		}(ino)
	}
	return nil
}

// Stop gracefully stops all components.
func (s *System) Stop() {
	slog.Info("System is stopping...")
	if s.cancel != nil {
		s.cancel()
	}
	for _, gw := range s.gateways {
		gw.Stop()
	}
	for _, vh := range s.vehicles {
		vh.Stop()
	}
	for _, ino := range s.arduinos {
		_ = ino.Close()
	}
	// if s.server != nil {
	// 	_ = s.server.Stop()
	// }
	if s.serverDB != nil {
		_ = s.serverDB.Close()
	}
	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}
	s.wg.Wait()
	slog.Info("Shutdown complete")
}

```

- /internal/core/vehicle.go
```go
// Package core implements the Vehicle agent
package core

import (
	"context"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"

	"github.com/fxamacker/cbor/v2"
)

// VehicleState định nghĩa trạng thái của xe
type VehicleState int

const (
	StateIdle    VehicleState = 0 // Chờ Beacon, chưa có slot
	StateJoining VehicleState = 1 // Đã gửi Hello, chờ Beacon tiếp theo để confirm slot
	StateSending VehicleState = 2 // Đã có slot, gửi Telemetry định kỳ
)

// Vehicle là đại diện cho thiết bị thuyền/xe
type Vehicle struct {
	ID      string
	lora    *device.Lora
	arduino *device.Arduino // Có thể nil nếu dùng dữ liệu giả lập

	state          VehicleState
	currentGateway string
	assignedSlot   int

	// Cache dữ liệu telemetry mới nhất từ Arduino
	mu            sync.Mutex
	lastTelemetry model.ArduinoData

	// estimated one way delay (ms) for timing compensation
	estOWDms int64
	owdMu    sync.Mutex

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewVehicle tạo một Vehicle mới
func NewVehicle(
	id string,
	loraDev string, loraBaud int,
	arduinoDev string, arduinoBaud int,
) *Vehicle {
	lora := device.NewLora(loraDev, loraBaud)
	v := &Vehicle{
		ID:           id,
		lora:         lora,
		state:        StateIdle,
		assignedSlot: -1,
		estOWDms:     150, // default initial estimate (ms)
	}
	if arduinoDev != "" {
		v.arduino = device.NewArduino(arduinoDev, arduinoBaud)
	}
	return v
}

func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	slog.Info("Vehicle started", "id", v.ID)

	// 1. Goroutine đọc Arduino liên tục để update lastTelem
	if v.arduino != nil {
		v.wg.Add(1)
		go v.arduinoLoop(ctx)
	}

	// 2. Goroutine chính: LoRa Loop (State Machine)
	v.wg.Add(1)
	go v.loraLoop(ctx)

	return nil
}

func (v *Vehicle) Stop() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.lora != nil {
		if err := v.lora.Close(); err != nil {
			slog.Warn("Failed to close LoRa device", "error", err)
		}
	}
	if v.arduino != nil {
		if err := v.arduino.Close(); err != nil {
			slog.Warn("Failed to close Arduino device", "error", err)
		}
	}
	v.wg.Wait()
	slog.Info("Vehicle stopped", "id", v.ID)
}

// arduinoLoop đọc dữ liệu từ Arduino/Simulator
func (v *Vehicle) arduinoLoop(ctx context.Context) {
	defer v.wg.Done()
	dataCh := make(chan model.ArduinoData, 5)
	stop, err := v.arduino.Read(dataCh)
	if err != nil {
		slog.Warn("Arduino.Read returned error", "err", err)
		return
	}
	defer stop()

	// Khởi tạo data giả nếu arduino nil
	if v.arduino == nil {
		v.mu.Lock()
		v.lastTelemetry = model.ArduinoData{
			Latitude:    21.027 + rand.Float64()*0.001,
			Longitude:   105.835 + rand.Float64()*0.001,
			CurrentHead: rand.Int63n(361),
			TargetHead:  rand.Int63n(361),
			LeftSpeed:   1000 + rand.Int63n(1000),
			RightSpeed:  1000 + rand.Int63n(1000),
		}
		v.mu.Unlock()
	}

	for {
		select {
		case <-ctx.Done():
			return
		case data, ok := <-dataCh:
			if !ok {
				return
			}
			v.mu.Lock()
			v.lastTelemetry = data
			v.mu.Unlock()
			slog.Info("Vehicle received arduino data",
				"vehicle", v.ID,
				"lat", data.Latitude,
				"lon", data.Longitude,
			)
		}
	}
}

// loraLoop là State Machine chính của Vehicle
func (v *Vehicle) loraLoop(ctx context.Context) {
	defer v.wg.Done()

	// Sử dụng giá trị mặc định cho timeout chờ beacon (ví dụ: 5 giây)
	beaconTimeout := 5 * time.Second
	var lastBeaconTime time.Time

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Lắng nghe Beacon
			frame, err := v.lora.Read(beaconTimeout)
			if err != nil {
				// Nếu timeout hoặc lỗi sau khi đã từng có session -> Reset về IDLE
				if err == device.ErrLoraTimeout {
					// if we had a previous beacon and too long passed -> reset
					if !lastBeaconTime.IsZero() && time.Since(lastBeaconTime) > 2*beaconTimeout {
						if v.state != StateIdle {
							slog.Warn("Lost beacon connection, resetting to IDLE", "id", v.ID)
						}
						v.state = StateIdle
						v.currentGateway = ""
						v.assignedSlot = -1
					}
					continue
				}
				// Nếu đã Idle thì cứ tiếp tục lắng nghe
				slog.Error("Lora read error", "err", err)
				time.Sleep(100 * time.Millisecond)
				continue
			}

			// Phân loại gói tin
			var generic map[string]any
			if err := cbor.Unmarshal(frame, &generic); err != nil {
				slog.Warn("Received unknown or corrupted CBOR packet", "err", err)
				continue
			}

			msgType, ok := generic["type"].(string)
			if !ok {
				slog.Warn("Packet type missing")
				continue
			}

			switch msgType {
			case model.PacketBeacon:
				var beacon model.BeaconMessage
				if err := cbor.Unmarshal(frame, &beacon); err != nil {
					// Có thể là packet Control hoặc nhiễu
					slog.Debug("Received non-beacon frame or corrupted beacon", "err", err)
					continue
				}
				slog.Info("Received beacon", "vehicle", v.ID)
				// record last beacon time and timestamp (ms)
				lastBeaconTime = time.Now()
				// Xử lý logic dựa trên State và Beacon nhận được
				v.handleBeacon(ctx, beacon)
			case model.PacketControl:
				var control model.ControlData
				if err := cbor.Unmarshal(frame, &control); err != nil {
					// Có thể là packet Control hoặc nhiễu
					slog.Debug("Received non-control frame or corrupted control", "err", err)
					continue
				}
				slog.Info("Received control", "vehicle", v.ID)
				arduinoControl := fmt.Sprintf("%d,%.6f,%.6f,%.6f,%.6f,%.6f",
					control.Speed,
					control.Latitude,
					control.Longitude,
					control.Kp,
					control.Ki,
					control.Kd,
				)
				// Forward control data to Arduino
				if err := v.arduino.Write(arduinoControl); err != nil {
					slog.Error("Failed to forward control to Arduino", "vehicle", v.ID, "error", err)
				} else {
					slog.Info("Forwarded control to Arduino", "vehicle", v.ID)
				}
			default:
				slog.Debug("Received unhandled message type", "type", msgType)
			}
		}
	}
}

func (v *Vehicle) handleBeacon(ctx context.Context, b model.BeaconMessage) {
	// Logic Roaming: Nếu gateway ID khác với hiện tại và đã có slot
	if v.state == StateSending && v.currentGateway != "" && v.currentGateway != b.GatewayAddress {
		slog.Info("Roaming detected, preparing to switch", "old", v.currentGateway, "new", b.GatewayAddress)
		v.state = StateIdle // Reset về Idle để đăng ký lại với Gateway mới
		v.assignedSlot = -1
		// gửi lại hello ngay lập tức
		hello := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(hello)
		if err := v.lora.Write(payload); err != nil {
			slog.Warn("Failed to send hello during roaming", "err", err)
		} else {
			slog.Info("Sent HELLO after roaming", "vehicle", v.ID)
			v.state = StateJoining
		}
		v.currentGateway = b.GatewayAddress
		return
	}
	// Luôn cập nhật Gateway ID mới nhất
	v.currentGateway = b.GatewayAddress

	switch v.state {
	case StateIdle:
		slog.Info("Received Beacon (Idle), preparing to send Hello", "gw", b.GatewayAddress)

		// use sleepUntilRegisterWindow to compensate timing
		if ok := v.sleepUntilRegisterWindow(ctx, b); !ok {
			return
		}

		// Re-check: maybe beacon or another goroutine assigned slot
		if v.assignedSlot != -1 {
			// already have slot -> go to sending
			v.state = StateSending
			slog.Info("Already assigned slot while waiting, state -> SENDING", "vehicle", v.ID, "slot", v.assignedSlot)
			return
		}

		// Gửi Hello
		msg := model.HelloMessage{
			Type:      model.PacketHello,
			VehicleID: v.ID,
		}
		payload, _ := cbor.Marshal(msg)
		if err := v.lora.Write(payload); err != nil {
			slog.Warn("Failed to write Hello", "err", err)
		} else {
			v.state = StateJoining
			slog.Info("Sent Hello, state -> JOINING", "id", v.ID)
		}

	case StateJoining:
		// Trạng thái CHUẨN BỊ GỬI: Kiểm tra xem trong Beacon mới có Slot cho mình chưa
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot
			v.state = StateSending
			slog.Info("Joined successfully state -> SENDING", "vehicle", v.ID, "slot", slot)
			// Sau khi nhận beacon (T0), thực hiện TDMA gửi ngay trong chu kỳ này
			go v.performTDMA(ctx, b)
		} else {
			// Chưa thấy tên mình, gói Hello có thể bị mất. Gửi lại Hello ở cuối chu kỳ này
			slog.Warn("Waiting for slot assignment...", "id", v.ID)

			// use sleepUntilRegisterWindow for retry
			if ok := v.sleepUntilRegisterWindow(ctx, b); !ok {
				return
			}

			// Re-check prior to sending
			if v.assignedSlot != -1 {
				slog.Info("Slot assigned during wait, skip resend", "vehicle", v.ID, "slot", v.assignedSlot)
				v.state = StateSending
				// start TDMA for this cycle if possible
				go v.performTDMA(ctx, b)
				return
			}

			msg := model.HelloMessage{Type: model.PacketHello, VehicleID: v.ID}
			payload, _ := cbor.Marshal(msg)
			if err := v.lora.Write(payload); err != nil {
				slog.Warn("Failed to write Hello (retry)", "err", err)
			} else {
				slog.Info("Resent Hello (JOINING)", "vehicle", v.ID)
			}
			// State vẫn là Joining
		}

	case StateSending:
		// Trạng thái GỬI: Kiểm tra lại SlotMap xem còn được cấp phép không
		if slot, ok := b.SlotMap[v.ID]; ok {
			v.assignedSlot = slot // Cập nhật slot nếu Gateway thay đổi
			go v.performTDMA(ctx, b)
		} else {
			slog.Warn("Lost slot allocation, returning to IDLE", "id", v.ID)
			v.state = StateIdle
			v.assignedSlot = -1
		}
	}
}

// performTDMA now accepts ctx and uses sleepUntilSlot to schedule transmission
func (v *Vehicle) performTDMA(ctx context.Context, b model.BeaconMessage) {
	if v.assignedSlot < 1 {
		slog.Warn("Invalid slot, skipping TDMA", "vehicle", v.ID, "slot", v.assignedSlot)
		return
	}

	// compute 0-based index
	slotIndex := v.assignedSlot - 1

	// Wait until the slot (potentially current or next cycle)
	if ok := v.sleepUntilSlot(ctx, b, slotIndex); !ok {
		// skip if context canceled or timing not possible
		return
	}

	// Re-check assigned slot hasn't changed
	if v.assignedSlot-1 != slotIndex {
		slog.Info("Assigned slot changed before transmit, skipping", "vehicle", v.ID, "slotIndex", slotIndex, "currentSlot", v.assignedSlot)
		return
	}

	// Lấy dữ liệu mới nhất
	v.mu.Lock()
	data := v.lastTelemetry
	v.mu.Unlock()

	// Đóng gói
	pkt := model.VehicleData{
		Type:        model.PacketTelemetry,
		VehicleID:   v.ID,
		Latitude:    data.Latitude,
		Longitude:   data.Longitude,
		CurrentHead: data.CurrentHead,
		TargetHead:  data.TargetHead,
		LeftSpeed:   data.LeftSpeed,
		RightSpeed:  data.RightSpeed,
	}
	payload, _ := cbor.Marshal(pkt)

	// Gửi và đo RTT to update estOWD
	start := time.Now()
	if err := v.lora.Write(payload); err != nil {
		slog.Warn("Failed to send telemetry", "err", err)
		return
	}
	// // we don't have ack mechanism here; as approximation, measure local write duration
	// elapsed := time.Since(start)
	// // update estOWD as EWMA of previous and elapsed/2
	// v.owdMu.Lock()
	// prev := v.estOWDms
	// meas := int64(elapsed.Milliseconds() / 2)
	// alpha := 0.7
	// v.estOWDms = int64(math.Round(alpha*float64(prev) + (1.0-alpha)*float64(meas)))
	// v.owdMu.Unlock()

	// we cannot measure true OWD without ACK; use conservative EWMA & bounds
	elapsed := time.Since(start)
	meas := int64(elapsed.Milliseconds() / 2)
	if meas < 10 {
		meas = 10
	}
	if meas > 2000 {
		meas = 2000
	}
	// alpha smaller to reduce oscillation
	alpha := 0.3
	v.owdMu.Lock()
	prev := v.estOWDms
	v.estOWDms = int64(math.Round(alpha*float64(meas) + (1.0-alpha)*float64(prev)))
	v.owdMu.Unlock()

	slog.Debug("Sent Telemetry (TDMA)",
		"vehicle", v.ID,
		"slot", v.assignedSlot,
		"lat", data.Latitude,
		"lon", data.Longitude,
		"estOWDms", v.estOWDms,
	)
}

// sleepUntilRegisterWindow sleeps until the appropriate time to send HELLO
// It uses NextCycleStart and regStartOffset logic and compensates for estimated one-way delay
func (v *Vehicle) sleepUntilRegisterWindow(ctx context.Context, b model.BeaconMessage) bool {
	cycleMs := int64(b.CycleDurationMs)
	regWindowMs := int64(b.RegisterWindowMs)
	regStartOffset := cycleMs - regWindowMs

	// compute register window start for the next cycle
	regStartMs := b.NextCycleStart + regStartOffset

	// estimate one-way delay
	v.owdMu.Lock()
	estOWD := v.estOWDms
	v.owdMu.Unlock()

	// safety margin and minimal prep time
	const safetyMs = int64(60)
	const minPrepMs = int64(40)

	nowMs := time.Now().UnixNano() / int64(time.Millisecond)

	// target time to start transmitting so arrival inside window
	targetTxMs := regStartMs - estOWD - safetyMs
	sleepMs := targetTxMs - nowMs

	if sleepMs <= 0 {
		// maybe still possible to send (small backoff) if we are within window
		regEndMs := regStartMs + regWindowMs
		arrivalIfNow := nowMs + estOWD
		// if arrival before end and we have time to prepare -> do short random backoff
		if arrivalIfNow+minPrepMs < regEndMs {
			backoff := rand.Int63n(int64(math.Min(float64(regWindowMs/4), 200)))
			select {
			case <-ctx.Done():
				return false
			case <-time.After(time.Duration(backoff) * time.Millisecond):
				return true
			}
		}
		// else skip to next cycle
		nextRegStart := regStartMs + cycleMs
		sleepMs = nextRegStart - nowMs
	}

	if sleepMs > 0 {
		timer := time.NewTimer(time.Duration(sleepMs) * time.Millisecond)
		// defer timer.Stop()
		defer func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			// IMPORTANT: re-check state/assignedSlot/last received beacon timestamp
			if v.state == StateJoining && v.assignedSlot != -1 {
				// slot already assigned while sleeping -> do NOT resend
				return true
			}
			return true
		}
	}
	return true
}

// sleepUntilSlot sleeps until the correct slot start (supports current or next cycle)
func (v *Vehicle) sleepUntilSlot(ctx context.Context, b model.BeaconMessage, slotIndex int) bool {
	// slotIndex assumed 0-based
	slotDurMs := int64(b.SlotDurationMs)
	guardMs := int64(b.GuardTimeMs)
	oneSlotMs := slotDurMs + guardMs
	// cycleMs := int64(b.CycleDurationMs)

	// compute candidate starts: current cycle and next cycle
	candidates := []int64{b.CycleStart, b.NextCycleStart}

	v.owdMu.Lock()
	estOWD := v.estOWDms
	v.owdMu.Unlock()

	const safetyMs = int64(40)
	const minPrepMs = int64(20)

	nowMs := time.Now().UnixNano() / int64(time.Millisecond)

	for _, base := range candidates {
		slotStart := base + guardMs + int64(slotIndex)*oneSlotMs
		// target transmit so arrival at slotStart
		targetTxMs := slotStart - estOWD - safetyMs
		sleepMs := targetTxMs - nowMs
		if sleepMs <= 0 {
			// maybe still possible if arrival before slot end
			slotEnd := slotStart + slotDurMs
			arrivalIfNow := nowMs + estOWD
			if arrivalIfNow+minPrepMs < slotEnd {
				// tiny random backoff to avoid collisions
				backoff := rand.Int63n(50)
				select {
				case <-ctx.Done():
					return false
				case <-time.After(time.Duration(backoff) * time.Millisecond):
					return true
				}
			}
			// else try next candidate
			continue
		}
		// wait until targetTxMs
		timer := time.NewTimer(time.Duration(sleepMs) * time.Millisecond)
		// defer timer.Stop()
		defer func() {
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
		}()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			// IMPORTANT: re-check state/assignedSlot/last received beacon timestamp
			if v.state == StateJoining && v.assignedSlot != -1 {
				// slot already assigned while sleeping -> do NOT resend
				return false
			}
			return true
		}
	}
	// if nothing matched, wait a short time then return false to let caller skip
	select {
	case <-ctx.Done():
		return false
	case <-time.After(100 * time.Millisecond):
		return false
	}
}

```

- /internal/core/server.go
```go
// Package core implements the Fog server — registry, WebSocket, telemetry & control APIs.
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"LoraFog/internal/database"
	"LoraFog/internal/model"
	// "github.com/gorilla/websocket"
)

// var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// Session đại diện cho một kết nối Vehicle đang hoạt động
type Session struct {
	VehicleID      string
	GatewayAddress string
	CreatedAt      time.Time
	TTL            time.Duration // Thời gian sống tối đa không nhận Telemetry
	Slot           int           // Slot TDMA được cấp phát
}

// Server là lõi quản lý của hệ thống Fog
type Server struct {
	Address    string
	AppAddress string
	database   *database.ServerDB // Giả lập Database
	httpClient *http.Client
	// httpServer *http.Server
	// mutexWS    sync.Mutex
	// clients    map[*websocket.Conn]bool

	mutex        sync.Mutex
	sessions     map[string]*Session     // Map: VehicleID -> Session
	gatewaySlots map[string]map[int]bool // Map: GwID -> (SlotIndex -> Used)
	// Map này giúp Server quản lý và cấp phát slot cho từng Gateway
}

// NewServer tạo một Server mới
func NewServer(
	address string,
	appAddress string,
	serverDB *database.ServerDB,
) *Server {
	return &Server{
		Address:      address,
		AppAddress:   appAddress,
		database:     serverDB,
		httpClient:   &http.Client{Timeout: 5 * time.Second},
		sessions:     make(map[string]*Session),
		gatewaySlots: make(map[string]map[int]bool),
	}
}

// Start khởi động Server và các tiến trình
func (s *Server) Start(ctx context.Context) error {
	// Khởi động Goroutine quét Session (TTL)
	go s.sweeper(ctx)

	// Khởi động HTTP Server
	mux := http.NewServeMux()
	mux.HandleFunc("/register", s.handleRegister)
	mux.HandleFunc("/telemetry", s.handleTelemetry)
	mux.HandleFunc("/command", s.handleControl)
	// mux.HandleFunc("/ws", s.handleWS)

	server := &http.Server{Addr: s.Address, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("Server HTTP failed to start", "error", err)
		}
	}()
	slog.Info("Server started", "address", s.Address)

	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = server.Shutdown(ctxShutdown)
	slog.Info("Server stopped")
	return nil
}

// handleRegister: Xử lý yêu cầu đăng ký mới từ Gateway (Hello Packet)
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var req model.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}

	slog.Info(
		"Server received register",
		"gateway", req.GatewayAddress,
		"vehicle", req.VehicleID,
	)

	if req.VehicleID == "" || req.GatewayAddress == "" {
		http.Error(w,
			"vehicle and gateway required",
			http.StatusBadRequest,
		)
		return
	}

	// 1. Logic Roaming/Trùng Session
	var oldGw string
	var oldSlot int
	s.mutex.Lock()
	if oldSes, ok := s.sessions[req.VehicleID]; ok {
		oldGw = oldSes.GatewayAddress
		oldSlot = oldSes.Slot

		delete(s.sessions, req.VehicleID)
		s.releaseSlot(oldGw, oldSlot)
	}
	s.mutex.Unlock()

	// 2. Nếu có session cũ → unregister DB + push update
	if oldGw != "" {
		// Update DB to clear gateway for vehicle (safeguard)
		_ = s.database.UpdateVehicleGateway(
			context.Background(),
			req.VehicleID,
			"",
		)
		s.pushSlotUpdateToGateway(oldGw)
		slog.Info(
			"Roaming: cleared old session",
			"vehicle", req.VehicleID,
			"old_gw", oldGw,
		)
	}

	// 2. Cấp Slot mới tại Gateway mới
	s.mutex.Lock()
	newSlot := s.assignNewSlot(req.GatewayAddress)
	if newSlot == -1 {
		s.mutex.Unlock()
		http.Error(w, "No slot available", http.StatusServiceUnavailable)
		return
	}

	// 3. Tạo Session mới
	newSession := &Session{
		VehicleID:      req.VehicleID,
		GatewayAddress: req.GatewayAddress,
		CreatedAt:      time.Now(),
		TTL:            60 * time.Second, // Mặc định 60s TTL
		Slot:           newSlot,
	}
	s.sessions[req.VehicleID] = newSession
	s.mutex.Unlock()
	slog.Info("New Session created",
		"vehicle", req.VehicleID,
		"gw", req.GatewayAddress,
		"slot", newSlot,
	)

	// 4. Cập nhật DB (Boat -> GatewayID)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()
	if err := s.database.UpdateVehicleGateway(
		ctx, req.VehicleID, req.GatewayAddress,
	); err != nil {
		slog.Error(
			"DB update failed during register",
			"vehicle", req.VehicleID, "error", err,
		)
		// try to rollback session
		s.mutex.Lock()
		delete(s.sessions, req.VehicleID)
		s.releaseSlot(req.GatewayAddress, newSlot)
		s.mutex.Unlock()
		http.Error(w, "DB error", http.StatusInternalServerError)
		return
	}

	// 5. Quan trọng: Push danh sách Slot mới xuống Gateway
	go s.pushSlotUpdateToGateway(req.GatewayAddress)

	// w.WriteHeader(http.StatusOK)
	// Respond with slot details
	resp := model.RegisterResponse{
		Slot:           newSlot,
		GatewayAddress: req.GatewayAddress,
	}
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	if err := enc.Encode(&resp); err != nil {
		slog.Error("failed to write register response", "err", err)
	}
}

// handleTelemetry: Xử lý dữ liệu Telemetry và Reset TTL
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var telem model.VehicleData
	if err := json.NewDecoder(r.Body).Decode(&telem); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	slog.Info(
		"Server received telemetry",
		"vehicle", telem.VehicleID,
		"lat", telem.Latitude,
		"lon", telem.Longitude,
	)

	s.mutex.Lock()
	if ses, ok := s.sessions[telem.VehicleID]; ok {
		ses.CreatedAt = time.Now() // Reset TTL
		slog.Debug("Telemetry received, TTL reset", "vehicle", telem.VehicleID)
		// forward to app server (if configured) - safe call with timeout
		if s.AppAddress != "" {
			go func(t model.VehicleData) {
				ctx, cancel := context.WithTimeout(
					context.Background(),
					3*time.Second,
				)
				defer cancel()
				vehicleApp := model.VehicleApp{
					VehicleID:   t.VehicleID,
					Latitude:    t.Latitude,
					Longitude:   t.Longitude,
					CurrentHead: t.CurrentHead,
					TargetHead:  t.TargetHead,
					LeftSpeed:   t.LeftSpeed,
					RightSpeed:  t.RightSpeed,
				}
				body, _ := json.Marshal(vehicleApp)
				req, err := http.NewRequestWithContext(ctx,
					"POST",
					"http://"+s.AppAddress+"/api/telemetry",
					bytes.NewReader(body),
				)
				if err != nil {
					slog.Warn("failed build forward request", "err", err)
					return
				}
				req.Header.Set("Content-Type", "application/json")
				resp, err := s.httpClient.Do(req)
				if err != nil {
					slog.Warn("forward telemetry failed", "err", err)
					return
				}
				_ = resp.Body.Close()
				// s.broadcast(string(body))
				// slog.Info("broadcast to websocket")
			}(telem)
		}
	} else {
		// session unknown: log and drop
		slog.Warn("Telemetry received for unknown session", "vehicle", telem.VehicleID)
	}
	s.mutex.Unlock()

	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()

	var controlApp model.ControlApp
	if err := json.NewDecoder(r.Body).Decode(&controlApp); err != nil {
		http.Error(w, "Invalid request format", http.StatusBadRequest)
		return
	}
	slog.Info("Server received control")

	if controlApp.VehicleID == "" {
		http.Error(w,
			"vehicle id required",
			http.StatusBadRequest,
		)
		return
	}

	gatewayID, _ := s.database.GetVehicleGateway(context.Background(), controlApp.VehicleID)
	if gatewayID == "" {
		slog.Error("Control cannot reach because vehicle not belong to any gateway")
		return
	}

	controlData := model.ControlData{
		Type:      model.PacketControl,
		VehicleID: controlApp.VehicleID,
		Speed:     controlApp.Speed,
		Latitude:  controlApp.Latitude,
		Longitude: controlApp.Longitude,
		Kp:        controlApp.Kp,
		Ki:        controlApp.Ki,
		Kd:        controlApp.Kd,
	}
	go func(c model.ControlData) {
		ctx, cancel := context.WithTimeout(
			context.Background(),
			3*time.Second,
		)
		defer cancel()
		body, _ := json.Marshal(c)
		req, err := http.NewRequestWithContext(ctx,
			"POST",
			"http://"+gatewayID+"/control",
			bytes.NewReader(body),
		)
		if err != nil {
			slog.Warn("failed build forward request", "err", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := s.httpClient.Do(req)
		if err != nil {
			slog.Warn("forward control failed", "err", err)
			return
		}
		_ = resp.Body.Close()
	}(controlData)
}

// sweeper: Quét các Session đã hết hạn (TTL Expired)
func (s *Server) sweeper(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			s.mutex.Lock()
			now := time.Now()
			dirtyGateways := make(map[string]bool)

			for vID, ses := range s.sessions {
				if now.Sub(ses.CreatedAt) > ses.TTL {
					slog.Info(
						"Session expired (TTL)",
						"vehicle", vID, "gw", ses.GatewayAddress,
					)

					// 1. Cập nhật DB -> NULL/Empty
					// Update DB -> NULL/Empty
					go func(vehicle string) {
						_ = s.database.UpdateVehicleGateway(context.Background(), vehicle, "")
					}(vID)

					// 2. Release Slot và đánh dấu Gateway cần cập nhật
					s.releaseSlot(ses.GatewayAddress, ses.Slot)
					dirtyGateways[ses.GatewayAddress] = true

					// 3. Xóa session
					delete(s.sessions, vID)
				}
			}
			s.mutex.Unlock()

			// 4. Báo cho các Gateway có thay đổi session để cập nhật Beacon
			for gwID := range dirtyGateways {
				go s.pushSlotUpdateToGateway(gwID)
			}
		case <-ctx.Done():
			return
		}
	}
}

// assignNewSlot: Tìm và cấp phát slot mới cho Gateway (trong lock)
func (s *Server) assignNewSlot(gwAddr string) int {
	if _, ok := s.gatewaySlots[gwAddr]; !ok {
		s.gatewaySlots[gwAddr] = make(map[int]bool)
	}

	// Bắt đầu tìm từ slot 1, tối đa 20 slot
	for i := 1; i <= 20; i++ {
		if !s.gatewaySlots[gwAddr][i] {
			s.gatewaySlots[gwAddr][i] = true
			return i
		}
	}
	slog.Error("Max slots reached", "gateway", gwAddr)
	return -1 // Không thể cấp slot
}

// releaseSlot: Giải phóng slot (trong lock)
func (s *Server) releaseSlot(gwID string, slot int) {
	if slots, ok := s.gatewaySlots[gwID]; ok {
		delete(slots, slot)
	}
}

// pushSlotUpdateToGateway: Gửi danh sách Slot Map đầy đủ xuống Gateway
func (s *Server) pushSlotUpdateToGateway(gwAddr string) {
	s.mutex.Lock()
	// Tạo SlotMap chỉ chứa các vehicle thuộc Gateway này
	slotMap := make(map[string]int)
	for _, ses := range s.sessions {
		if ses.GatewayAddress == gwAddr {
			slotMap[ses.VehicleID] = ses.Slot
		}
	}
	s.mutex.Unlock()

	// Gửi POST xuống Gateway /update_beacon
	body, _ := json.Marshal(slotMap)
	ctx, cancel := context.WithTimeout(
		context.Background(),
		3*time.Second,
	)
	defer cancel()

	// Lưu ý: Giả định gwAddr là địa chỉ HTTP (ví dụ: localhost:8081)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+gwAddr+"/update_beacon",
		bytes.NewReader(body),
	)
	if err != nil {
		slog.Error(
			"Failed to build request for gateway",
			"gw", gwAddr, "err", err,
		)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"Failed to push slot update to Gateway",
			"gw_addr", gwAddr, "error", err,
		)
		return
	}
	// Ensure body closed
	_ = resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		slog.Warn("Gateway rejected slot update", "gw_addr", gwAddr, "status", resp.Status)
	} else {
		slog.Info("Successfully pushed slot map to Gateway", "gw_addr", gwAddr, "count", len(slotMap))
	}
}

// // handleWS upgrades HTTP to websocket and registers the client for broadcasts.
// func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
// 	conn, err := upgrader.Upgrade(w, r, nil)
// 	if err != nil {
// 		return
// 	}
// 	s.mutexWS.Lock()
// 	s.clients[conn] = true
// 	s.mutexWS.Unlock()
//
// 	go func() {
// 		defer func() {
// 			s.mutexWS.Lock()
// 			delete(s.clients, conn)
// 			s.mutexWS.Unlock()
// 			if err := conn.Close(); err != nil {
// 				slog.Error("Failed to close websocket", "error", err)
// 			}
// 		}()
// 		for {
// 			if _, _, err := conn.ReadMessage(); err != nil {
// 				break
// 			}
// 		}
// 	}()
// }
//
// // broadcast sends a message to all connected websocket clients.
// func (s *Server) broadcast(msg string) {
// 	s.mutexWS.Lock()
// 	defer s.mutexWS.Unlock()
// 	for c := range s.clients {
// 		_ = c.WriteMessage(websocket.TextMessage, []byte(msg))
// 	}
// }

```

- /internal/core/gateway.go
```go
// Package core defines the Gateway component responsible for
// bridging LoRa-connected vehicles with the FogServer
// using CBOR (for LoRa) and JSON (for HTTP).
package core

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"

	"github.com/fxamacker/cbor/v2"
)

const (
	// Thông số TDMA cố định
	CycleDuration    = 10000 * time.Millisecond
	SlotDurationMs   = int64(2000)
	GuardTimeMs      = int64(500)
	RegisterWindowMs = int64(5000)
)

// Gateway là đại diện cho thiết bị Gateway LoRaWAN
type Gateway struct {
	Address       string // Địa chỉ HTTP/TCP của Gateway (dùng làm ID)
	ServerAddress string // Địa chỉ HTTP của Fog Server
	lora          *device.Lora
	httpClient    *http.Client

	slotMutex    sync.Mutex
	currentSlots map[string]int // Map VehicleID -> SlotIndex. Cập nhật từ Server.

	cancel context.CancelFunc
	wg     sync.WaitGroup
}

// NewGateway tạo một Gateway mới
func NewGateway(
	address, serverAddress string,
	loraDev string, loraBaud int,
) *Gateway {
	return &Gateway{
		Address:       address,
		ServerAddress: serverAddress,
		lora:          device.NewLora(loraDev, loraBaud),
		httpClient:    &http.Client{Timeout: 5 * time.Second},
		currentSlots:  make(map[string]int),
	}
}

// Start khởi động các tiến trình của Gateway
func (g *Gateway) Start(ctx context.Context) error {
	// slog.Warn(">>> ENTER Gateway.Start() <<<")
	ctx, cancel := context.WithCancel(ctx)
	g.cancel = cancel

	// 1. Khởi động HTTP Server để nhận lệnh từ Server (Control & Update Beacon)
	// slog.Warn(">>> BEFORE startHTTPServer <<<")
	g.wg.Add(1)
	go g.startHTTPServer(ctx)

	// 2. Khởi động vòng lặp phát Beacon
	// slog.Warn(">>> BEFORE beaconLoop <<<")
	g.wg.Add(1)
	go g.beaconLoop(ctx)

	// 3. Khởi động vòng lặp lắng nghe Uplink (Hello & Telemetry)
	// slog.Warn(">>> BEFORE uplinkLoop <<<")
	g.wg.Add(1)
	go g.uplinkLoop(ctx)

	slog.Info("Gateway started", "address", g.Address)
	return nil
}

// Stop gracefully stops the gateway and closes resources.
func (g *Gateway) Stop() {
	slog.Info("Gateway is stopping", "address", g.Address)

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn("Failed to close LoRa device", "error", err)
		}
	}

	if g.cancel != nil {
		g.cancel()
	}

	g.wg.Wait()
	slog.Info("Gateway stopped", "address", g.Address)
}

// startHTTPServer khởi động server HTTP nội bộ để nhận lệnh từ Fog Server
func (g *Gateway) startHTTPServer(ctx context.Context) {
	defer g.wg.Done()
	mux := http.NewServeMux()

	// Endpoint nhận danh sách Slot Map mới từ Server
	mux.HandleFunc("/update_beacon", g.handleBeaconUpdate)
	mux.HandleFunc("/control", g.handleControl)

	server := &http.Server{Addr: g.Address, Handler: mux}

	go func() {
		if err := server.ListenAndServe(); err != http.ErrServerClosed {
			slog.Error("Gateway HTTP server error", "addr", g.Address, "error", err)
		}
	}()

	<-ctx.Done()
	ctxShutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(ctxShutdown); err != nil {
		slog.Error("Gateway HTTP server shutdown failed", "addr", g.Address, "error", err)
	} else {
		slog.Info("Gateway HTTP server shutdown clean", "addr", g.Address)
	}
}

// handleBeaconUpdate: Nhận Slot Map mới từ Server (khi có đăng ký/hết hạn session)
func (g *Gateway) handleBeaconUpdate(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var newMap map[string]int
	if err := json.NewDecoder(r.Body).Decode(&newMap); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}

	g.slotMutex.Lock()
	g.currentSlots = newMap
	g.slotMutex.Unlock()
	slog.Info("Beacon slots updated by Server",
		"gateway", g.Address,
		"count", len(newMap),
	)

	w.WriteHeader(http.StatusOK)
}

// handleControl
func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() { _ = r.Body.Close() }()
	var controlJSON model.ControlData
	if err := json.NewDecoder(r.Body).Decode(&controlJSON); err != nil {
		http.Error(w, "Invalid JSON format", http.StatusBadRequest)
		return
	}
	controlCBOR, err := cbor.Marshal(controlJSON)
	if err != nil {
		slog.Error("Failed to marshal beacon", "error", err)
		return
	}
	if err := g.lora.Write(controlCBOR); err != nil {
		slog.Error("Failed to write beacon to LoRa", "err", err)
	} else {
		slog.Info("Gateway send control", "gateway", g.Address)
	}
}

// beaconLoop: Phát Beacon TDMA định kỳ
func (g *Gateway) beaconLoop(ctx context.Context) {
	defer g.wg.Done()
	// Giả lập chu kỳ TDMA 2 giây
	ticker := time.NewTicker(CycleDuration)

	for {
		select {
		case <-ctx.Done():
			// slog.Info("CTX DEAD INSIDE BEACON LOOP")
			ticker.Stop()
			return
		case t := <-ticker.C:
			g.slotMutex.Lock()
			slots := make(map[string]int)
			// Copy map để đảm bảo Thread-safe khi broadcast
			maps.Copy(slots, g.currentSlots)
			g.slotMutex.Unlock()

			cycleStart := t.UnixNano() / int64(time.Millisecond)
			cycleDuration := int64(CycleDuration / time.Millisecond)
			nextCycleStart := cycleStart + cycleDuration
			beacon := model.BeaconMessage{
				Type:             model.PacketBeacon,
				GatewayAddress:   g.Address,
				CycleStart:       cycleStart,
				NextCycleStart:   nextCycleStart,
				CycleDurationMs:  cycleDuration,
				SlotDurationMs:   SlotDurationMs,
				GuardTimeMs:      GuardTimeMs,
				RegisterWindowMs: RegisterWindowMs,
				SlotMap:          slots,
			}

			payload, err := cbor.Marshal(beacon)
			if err != nil {
				slog.Error("Failed to marshal beacon", "error", err)
				continue
			}

			// SYNC (tại thời điểm này)
			if err := g.lora.Write(payload); err != nil {
				slog.Error("Failed to write beacon to LoRa", "err", err)
			} else {
				slog.Info(
					"Beacon Broadcast",
					"gateway", g.Address, "slots", len(slots),
				)
			}
		}
	}
}

// uplinkLoop: Lắng nghe gói Hello (Register) và Telemetry từ Vehicle
func (g *Gateway) uplinkLoop(ctx context.Context) {
	defer g.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		// ReadFrameWithTimeout (50ms để không bị block lâu)
		frame, err := g.lora.Read(50 * time.Millisecond)
		if err != nil {
			if err == device.ErrLoraTimeout {
				continue // Tiếp tục vòng lặp
			}
			slog.Error("Lora read error", "error", err)
			time.Sleep(100 * time.Millisecond)
			continue
		}

		// Phân loại gói tin
		var generic map[string]any
		if err := cbor.Unmarshal(frame, &generic); err != nil {
			slog.Warn("Received unknown or corrupted CBOR packet", "err", err)
			continue
		}

		msgType, ok := generic["type"].(string)
		if !ok {
			slog.Warn("Packet type missing")
			continue
		}

		switch msgType {
		case model.PacketHello:
			var hello model.HelloMessage
			if err := cbor.Unmarshal(frame, &hello); err == nil {
				slog.Info("Received Hello (Register)", "vehicle_id", hello.VehicleID)
				g.postRegisterToServer(hello.VehicleID)
			}
		case model.PacketTelemetry:
			var telemetry model.VehicleData
			if err := cbor.Unmarshal(frame, &telemetry); err == nil {
				// Lấy lock để đọc currentSlots
				g.slotMutex.Lock()
				_, exists := g.currentSlots[telemetry.VehicleID]
				g.slotMutex.Unlock()

				if !exists {
					// Vehicle không có slot → bỏ qua telemetry
					slog.Warn("Telemetry ignored: vehicle has no active slot",
						"vehicle_id", telemetry.VehicleID)
					continue
				}

				// Vehicle hợp lệ → gửi lên Server
				slog.Debug("Received Telemetry", "vehicle_id", telemetry.VehicleID)
				g.postTelemetryToServer(telemetry)
			}
		default:
			slog.Debug("Received unhandled message type", "type", msgType)
		}
	}
}

// postRegisterToServer: Gửi yêu cầu đăng ký lên Server
func (g *Gateway) postRegisterToServer(vehicleID string) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	register := model.RegisterRequest{
		GatewayAddress: g.Address,
		VehicleID:      vehicleID,
	}
	payload, _ := json.Marshal(register)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+g.ServerAddress+"/register",
		bytes.NewReader(payload),
	)
	if err != nil {
		slog.Error("Failed to build register request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"Failed to POST register to Server",
			"server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() { _ = resp.Body.Close() }()
	slog.Info(
		"Gateway send registry",
		"gateway", g.Address,
		"vehicle", vehicleID,
	)

	if resp.StatusCode != http.StatusOK {
		slog.Warn("Server rejected registration", "status", resp.Status)
		return
	}

	// Optional: parse response (slot/gateway)
	var registerResponse model.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&registerResponse); err != nil && err != http.ErrBodyReadAfterClose {
		// decode error is non-fatal but log
		slog.Warn("Failed to parse register response", "err", err)
	} else {
		slog.Info(
			"Register accepted by server",
			"vehicle", vehicleID,
			"slot", registerResponse.Slot,
			"gateway", registerResponse.GatewayAddress,
		)
	}
}

// postTelemetryToServer: Gửi dữ liệu Telemetry lên Server
func (g *Gateway) postTelemetryToServer(data model.VehicleData) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	// Gửi VehicleData lên Server để reset TTL session
	body, _ := json.Marshal(data)
	req, err := http.NewRequestWithContext(ctx,
		"POST",
		"http://"+g.ServerAddress+"/telemetry",
		bytes.NewReader(body),
	)
	if err != nil {
		slog.Error("Failed to build telemetry request", "err", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := g.httpClient.Do(req)
	if err != nil {
		slog.Error(
			"Failed to POST telemetry to Server",
			"server", g.ServerAddress, "error", err,
		)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	slog.Info(
		"Gateway send registry",
		"gateway", g.Address,
		"vehicle", data.VehicleID,
		"lat", data.Latitude,
		"lon", data.Longitude,
	)
}

```

- /internal/device/lora.go
```go
// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

import (
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"time"
)

// ErrLoraTimeout được dùng khi không nhận được frame trong thời gian quy định
var ErrLoraTimeout = errors.New("lora read timeout")

// Lora manages binary (CBOR) communication over a serial LoRa interface.
type Lora struct {
	Path   string
	Baud   int
	serial *Serial
}

// NewLora creates a new Lora.
func NewLora(path string, baud int) *Lora {
	serial, err := NewSerial(path, baud)
	if err != nil {
		slog.Warn("failed to connect Lora device",
			"component", "lora", "device", path, "error", err)
	}
	return &Lora{
		Path:   path,
		Baud:   baud,
		serial: serial,
	}
}

// Read tries to read a CBOR frame with timeout.
// Timeout=0 means blocking indefinitely.
// (Logic goroutine/select đã được loại bỏ)
func (l *Lora) Read(timeout time.Duration) ([]byte, error) {
	if l.serial == nil {
		return nil, fmt.Errorf("lora serial not initialized")
	}

	// 1. Đọc 2 bytes header (độ dài), sử dụng timeout
	// (Chú ý: Đã thay thế io.ReadFull trên reader bằng l.serial.ReadBytes)
	header, err := l.serial.ReadBytes(2, timeout)
	if err != nil {
		return nil, err
	}

	length := binary.BigEndian.Uint16(header)
	if length == 0 || length > 512 { // Thêm check giới hạn kích thước
		return nil, fmt.Errorf("invalid frame length %d", length)
	}

	// 2. Đọc payload. Sử dụng timeout=0 (blocking) vì đã đọc được header,
	// ta muốn đợi đủ payload.
	payload, err := l.serial.ReadBytes(int(length), 0)
	if err != nil {
		return nil, fmt.Errorf("failed to read frame payload: %w", err)
	}
	return payload, nil
}

func (l *Lora) Write(b []byte) error {
	if l.serial == nil {
		return fmt.Errorf("lora serial not initialized")
	}
	// prefix with 2-byte big endian length
	if len(b) == 0 || len(b) > 0xFFFF {
		return fmt.Errorf("invalid payload size %d", len(b))
	}
	header := make([]byte, 2)
	binary.BigEndian.PutUint16(header, uint16(len(b)))

	// write header then payload
	if err := l.serial.WriteBytes(header); err != nil {
		return err
	}
	return l.serial.WriteBytes(b)
}

// Close safely closes the LoRa serial device.
func (l *Lora) Close() error {
	if l.serial == nil {
		return nil
	}
	return l.serial.Close()
}

```

- /internal/device/serial.go
```go
// Package device implements a simple wrapper for serial communication.
package device

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"
	"time"

	"go.bug.st/serial"
)

var ErrSerialTimeout = errors.New("serial read timeout")

// Serial represents a simple serial port connection.
type Serial struct {
	Port     serial.Port
	Path     string
	BaudRate int
	mu       sync.Mutex
	reader   *bufio.Reader
}

// NewSerial opens a serial port with the given path and baud rate.
func NewSerial(path string, baud int) (*Serial, error) {
	mode := &serial.Mode{BaudRate: baud}
	port, err := serial.Open(path, mode)
	if err != nil {
		return nil, fmt.Errorf("open serial port %s: %w", path, err)
	}
	s := &Serial{
		Port:     port,
		Path:     path,
		BaudRate: baud,
		reader:   bufio.NewReader(port),
	}
	slog.Info("serial port opened",
		"component", "serial", "path", path, "baud", baud)
	return s, nil
}

// ReadLine reads a line of data with an optional timeout (in milliseconds).
func (s *Serial) ReadLine(timeoutMs int) (string, error) {
	if s.Port == nil {
		return "", errors.New("serial port not initialized")
	}

	if timeoutMs > 0 {
		if err := s.Port.SetReadTimeout(time.Duration(timeoutMs) * time.Millisecond); err != nil {
			slog.Warn("failed to set read timeout",
				"component", "serial", "path", s.Path, "error", err)
		}
	}

	line, err := s.reader.ReadString('\n')
	if err != nil {
		return "", err
	}
	return line, nil
}

// WriteLine writes a single line (with newline terminator) to the serial port.
func (s *Serial) WriteLine(data string) error {
	if s.Port == nil {
		return errors.New("serial port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, err := s.Port.Write([]byte(data + "\n")); err != nil {
		slog.Warn("failed to write to serial",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	return nil
}

// ReadBytes reads exactly n bytes from the serial port with a timeout.
// Timeout=0 means blocking indefinitely.
func (s *Serial) ReadBytes(n int, timeout time.Duration) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("serial port not initialized")
	}
	buf := make([]byte, n)

	// s.mu.Lock()
	// defer s.mu.Unlock()

	// 1. Thiết lập timeout cho Port trước khi đọc
	if err := s.Port.SetReadTimeout(timeout); err != nil {
		slog.Warn("failed to set read timeout", "component", "serial", "path", s.Path, "error", err)
	}

	// 2. Đọc từ bufio.Reader
	readCount, err := io.ReadFull(s.reader, buf)

	// 3. Reset timeout về blocking (0) sau khi đọc xong
	_ = s.Port.SetReadTimeout(0)

	if err != nil {
		// Chuẩn hóa lỗi Timeout
		if errors.Is(err, ErrSerialTimeout) || errors.Is(err, io.EOF) {
			return nil, ErrSerialTimeout
		}
		if errors.Is(err, io.ErrUnexpectedEOF) && readCount > 0 {
			return nil, fmt.Errorf("read incomplete: %w", err)
		}
		return nil, err
	}

	if readCount != n {
		return nil, fmt.Errorf("read incomplete: expected %d, got %d", n, readCount)
	}
	return buf, nil
}

// WriteBytes writes raw binary data to the serial port without newline.
func (s *Serial) WriteBytes(b []byte) error {
	if s.Port == nil {
		return errors.New("serial port not initialized")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.Port.Write(b); err != nil {
		slog.Warn("failed to write bytes to serial",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	return nil
}

// Close closes the serial port safely.
func (s *Serial) Close() error {
	if s.Port == nil {
		return nil
	}
	if err := s.Port.Close(); err != nil {
		slog.Warn("failed to close serial port",
			"component", "serial", "path", s.Path, "error", err)
		return err
	}
	slog.Info("serial port closed", "component", "serial", "path", s.Path)
	return nil
}

```

- /internal/device/arduino.go
```go
// Package device implements ArduinoDevice for
// reading and writing telemetry over serial
// as well as simulation support.
package device

import (
	"fmt"
	"log/slog"
	"math"
	"time"

	"LoraFog/internal/model"
)

// Arduino represents a serial connection to an Arduino controller.
type Arduino struct {
	Device string
	Baud   int
	serial *Serial
}

// NewArduino creates and connects to an Arduino serial device.
func NewArduino(dev string, baud int) *Arduino {
	s, err := NewSerial(dev, baud)
	if err != nil {
		slog.Warn("failed to connect Arduino",
			"component", "arduino", "device", dev, "error", err)
	}
	return &Arduino{
		Device: dev,
		Baud:   baud,
		serial: s,
	}
}

// Read starts reading Arduino telemetry in a background goroutine and pushes it into dataCh.
// It returns a stop function that can be called to terminate the loop safely.
func (a *Arduino) Read(dataCh chan<- model.ArduinoData) (func(), error) {
	if a.serial == nil {
		return nil, fmt.Errorf("arduino serial not initialized")
	}

	stop := make(chan struct{})
	go func() {
		defer close(dataCh)
		for {
			select {
			case <-stop:
				slog.Info("stopping Arduino read loop",
					"component", "arduino", "device", a.Device)
				return
			default:
			}

			line, err := a.serial.ReadLine(0)
			if err != nil {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			var d model.ArduinoData
			if _, err := fmt.Sscanf(line, "%f,%f,%d,%d,%d,%d",
				&d.Latitude, &d.Longitude, &d.CurrentHead,
				&d.TargetHead, &d.LeftSpeed, &d.RightSpeed); err != nil {
				continue
			}
			select {
			case dataCh <- d:
			default:
			}
		}
	}()
	return func() { close(stop) }, nil
}

// Write sends a single line to the Arduino serial interface.
func (a *Arduino) Write(line string) error {
	if a.serial == nil {
		return fmt.Errorf("arduino serial not initialized")
	}
	return a.serial.WriteLine(line)
}

// Close closes the Arduino serial port safely.
func (a *Arduino) Close() error {
	if a.serial == nil {
		return nil
	}
	return a.serial.Close()
}

// StartSimulation generates synthetic telemetry data periodically for testing.
func (a *Arduino) StartSimulation(stop <-chan struct{}) error {
	// --- Simulation State ---
	var (
		latNow  = 21.027000
		lonNow  = 105.835000
		headNow = 0.0 // độ
	)

	// --- Server Command State ---
	var (
		baseSpeed = 1000.0
		targetLat = 21.027100
		targetLon = 105.835600
		Kp        = 0.0
		Ki        = 0.0
		Kd        = 0.0
	)

	var (
		integral float64 = 0
		lastErr  float64 = 0
	)

	readCh := make(chan string, 10)

	// Goroutine để đọc lệnh CSV server gửi xuống
	go func() {
		for {
			line, err := a.serial.ReadLine(0)
			if err != nil {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			readCh <- line
		}
	}()

	slog.Info("starting Arduino simulation",
		"component", "arduino", "device", a.Device)

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-stop:
			slog.Info("stopping Arduino simulation",
				"component", "arduino", "device", a.Device)
			return nil
		case line := <-readCh:
			// EXPECT: speed,lat,lon,Kp,Ki,Kd
			if _, err := fmt.Sscanf(line, "%f,%f,%f,%f,%f,%f",
				&baseSpeed, &targetLat, &targetLon, &Kp, &Ki, &Kd,
			); err == nil {
				slog.Info("Simulation: received new target",
					"speed", baseSpeed, "lat", targetLat, "lon", targetLon)
			}
		case <-ticker.C:
			// data := model.ArduinoData{
			// 	Latitude:    21.027 + rand.Float64()*0.001,
			// 	Longitude:   105.835 + rand.Float64()*0.001,
			// 	CurrentHead: rand.Int63n(361),
			// 	TargetHead:  rand.Int63n(361),
			// 	LeftSpeed:   1000 + rand.Int63n(1000),
			// 	RightSpeed:  1000 + rand.Int63n(1000),
			// }
			// line := fmt.Sprintf("%f,%f,%d,%d,%d,%d",
			// 	data.Latitude, data.Longitude, data.CurrentHead,
			// 	data.TargetHead, data.LeftSpeed, data.RightSpeed)
			// --- 1. Tính hướng cần đến ---
			targetHead := bearing(latNow, lonNow, targetLat, targetLon)

			// --- 2. PID Steering ---
			err := normalizeAngle(targetHead - headNow)
			integral += err
			derivative := err - lastErr
			lastErr = err

			turn := Kp*err + Ki*integral + Kd*derivative

			// --- 3. Tính motor ---
			left := baseSpeed + turn
			right := baseSpeed - turn

			left = clamp(left, 1000, 2000)
			right = clamp(right, 1000, 2000)

			// --- 4. Mô phỏng tàu di chuyển một chút ---
			headNow = normalizeAngle(headNow + turn*0.1)
			latNow += (math.Cos(deg2rad(headNow)) * 0.00001)
			lonNow += (math.Sin(deg2rad(headNow)) * 0.00001)

			// --- 5. Gửi dữ liệu như Arduino thật ---
			line := fmt.Sprintf("%.6f,%.6f,%d,%d,%d,%d",
				latNow, lonNow,
				int(headNow),
				int(targetHead),
				int(left),
				int(right),
			)

			if err := a.Write(line); err != nil {
				slog.Warn("failed to write simulated telemetry",
					"component", "arduino", "device", a.Device, "error", err)
			}
		}
	}
}

func bearing(lat1, lon1, lat2, lon2 float64) float64 {
	rad1 := deg2rad(lat1)
	rad2 := deg2rad(lat2)
	delta := deg2rad(lon2 - lon1)

	y := math.Sin(delta) * math.Cos(rad2)
	x := math.Cos(rad1)*math.Sin(rad2) -
		math.Sin(rad1)*math.Cos(rad2)*math.Cos(delta)

	ans := math.Atan2(y, x)
	return normalizeAngle(rad2deg(ans))
}

func deg2rad(d float64) float64 { return d * math.Pi / 180 }
func rad2deg(r float64) float64 { return r * 180 / math.Pi }

func normalizeAngle(a float64) float64 {
	a = math.Mod(a+360, 360)
	if a < 0 {
		a += 360
	}
	return a
}

func clamp(v, min, max float64) float64 {
	if v < min {
		return min
	}
	if v > max {
		return max
	}
	return v
}

```

- /internal/database/mysql.go
```go
// Package database provides MySQL-backed storage for vehicle/gateway mapping.
package database

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

type ServerDB struct {
	db *sql.DB
}

// NewServerDB opens a MySQL connection using DSN (user:pass@tcp(host:port)/dbname)
func NewServerDB(dsn string, maxOpen, maxIdle int) (*ServerDB, error) {
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(5 * time.Minute)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, err
	}
	slog.Info("MySQL connected", "dsn", dsn)
	return &ServerDB{db: db}, nil
}

// Close closes underlying DB connection.
func (s *ServerDB) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// UpdateVehicleGateway sets gateway_id for a boat (vehicle).
// If gatewayID == "" -> sets NULL. Uses context with timeout.
func (s *ServerDB) UpdateVehicleGateway(ctx context.Context, vehicleID, gatewayID string) error {
	if s == nil || s.db == nil {
		return errors.New("db not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	var q string
	if gatewayID == "" {
		q = `UPDATE boats SET gateway_id = NULL WHERE boatId = ?`
		_, err := s.db.ExecContext(ctx, q, vehicleID)
		if err != nil {
			slog.Error("UpdateVehicleGateway failed", "vehicle", vehicleID, "error", err)
			return err
		}
		slog.Info("DB: Vehicle unregistered", "vehicle", vehicleID)
		return nil
	}

	q = `UPDATE boats SET gateway_id = ? WHERE boatId = ?`
	_, err := s.db.ExecContext(ctx, q, gatewayID, vehicleID)
	if err != nil {
		slog.Error("UpdateVehicleGateway failed", "vehicle", vehicleID, "gateway", gatewayID, "error", err)
		return err
	}
	slog.Info("DB: Vehicle registered", "vehicle", vehicleID, "gateway", gatewayID)
	return nil
}

// GetVehicleGateway returns gatewayId for given vehicle boatId. Returns "" if none.
func (s *ServerDB) GetVehicleGateway(ctx context.Context, vehicleID string) (string, error) {
	if s == nil || s.db == nil {
		return "", errors.New("db not initialized")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	var gw sql.NullString
	q := `SELECT gateway_id FROM boats WHERE boatId = ? LIMIT 1`
	err := s.db.QueryRowContext(ctx, q, vehicleID).Scan(&gw)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", nil
		}
		return "", err
	}
	if gw.Valid {
		return gw.String, nil
	}
	return "", nil
}

// ClearVehicleGateway sets gateway_id to NULL for vehicle.
// UPDATED v2
func (s *ServerDB) ClearVehicleGateway(ctx context.Context, vehicleID string) error {
	return s.UpdateVehicleGateway(ctx, vehicleID, "")
}

```
