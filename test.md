
- /internal/model/config.go
```go
// Package model defines configuration structures used across LoraFog.
package model

// Config defines all system components loaded from YAML/JSON.
type Config struct {
	Server         ServerConfig          `yaml:"server" json:"server"`
	Gateways       []GatewayConfig       `yaml:"gateways" json:"gateways"`
	Vehicles       []VehicleConfig       `yaml:"vehicles" json:"vehicles"`
	Arduinos       []ArduinoConfig       `yaml:"arduinos" json:"arduinos"`
	VirtualSerials []VirtualSerialConfig `yaml:"virtual_serials" json:"virtual_serials"`
}

// ServerConfig configures the fog server and its exposed addresses.
type ServerConfig struct {
	Addr     string            `yaml:"address" json:"address"`
	AppAddr  string            `yaml:"app_address" json:"app_address"`
	Gateways []GatewayRegistry `yaml:"gateway_registry" json:"gateway_registry"`
}

// GatewayRegistry defines gateway entries for the fog registry.
type GatewayRegistry struct {
	ID   string `yaml:"id" json:"id"`
	Addr string `yaml:"address" json:"address"`
}

// GatewayConfig describes one gateway instance.
type GatewayConfig struct {
	ID         string `yaml:"id" json:"id"`
	Addr       string `yaml:"address" json:"address"`
	ServerAddr string `yaml:"server_address" json:"server_address"`
	LoraDev    string `yaml:"lora_device" json:"lora_device"`
	LoraBaud   int    `yaml:"lora_baud" json:"lora_baud"`
}

// VehicleConfig describes one LoRa vehicle.
type VehicleConfig struct {
	ID          string `yaml:"id" json:"id"`
	LoraDev     string `yaml:"lora_device" json:"lora_device"`
	LoraBaud    int    `yaml:"lora_baud" json:"lora_baud"`
	ArduinoDev  string `yaml:"arduino_device" json:"arduino_device"`
	ArduinoBaud int    `yaml:"arduino_baud" json:"arduino_baud"`
}

// ArduinoConfig defines a physical Arduino connected to serial port.
type ArduinoConfig struct {
	Dev  string `yaml:"device" json:"device"`
	Baud int    `yaml:"baud" json:"baud"`
}

// VirtualSerialConfig defines a linked virtual serial pair.
type VirtualSerialConfig struct {
	Left  string `yaml:"left" json:"left"`
	Right string `yaml:"right" json:"right"`
}

```

- /internal/model/packet.go
```go
// Package model implements the LoRa frame format and AES-CTR/CMAC encoding.
package model

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
)

// Frame constants.
const (
	PreambleByte byte = 0xAA
	TagLen            = 8
	NonceLen          = 8
	HeaderLen         = 1 + 1 + 1 + 1 + NonceLen
)

// PacketType defines LoRa packet categories.
type PacketType byte

const (
	TypeTelemetry PacketType = 't'
	TypeControl   PacketType = 'c'
	TypeHello     PacketType = 'h'
	TypeBeacon    PacketType = 'b'
	TypeAuth      PacketType = 'a'
	TypeAck       PacketType = 'k'
)

// TelemetryPacked is the compact 12-byte telemetry layout.
type TelemetryPacked struct {
	LatI32     int32
	LonI32     int32
	SpeedU16   uint16
	HeadingU16 uint16
}

// BuildTelemetryPacked packs telemetry into 12 bytes.
func BuildTelemetryPacked(lat, lon int32, speed, heading uint16) []byte {
	b := make([]byte, 12)
	binary.BigEndian.PutUint32(b[0:4], uint32(lat))
	binary.BigEndian.PutUint32(b[4:8], uint32(lon))
	binary.BigEndian.PutUint16(b[8:10], speed)
	binary.BigEndian.PutUint16(b[10:12], heading)
	return b
}

// ParseTelemetryPacked unpacks 12-byte telemetry into struct.
func ParseTelemetryPacked(b []byte) (TelemetryPacked, error) {
	if len(b) != 12 {
		return TelemetryPacked{}, fmt.Errorf("invalid telemetry length %d", len(b))
	}
	return TelemetryPacked{
		LatI32:     int32(binary.BigEndian.Uint32(b[0:4])),
		LonI32:     int32(binary.BigEndian.Uint32(b[4:8])),
		SpeedU16:   binary.BigEndian.Uint16(b[8:10]),
		HeadingU16: binary.BigEndian.Uint16(b[10:12]),
	}, nil
}

// BuildPlainFrame builds a plaintext LoRa frame (no encryption).
func BuildPlainFrame(typ PacketType, seq byte, nonce []byte, payload []byte) ([]byte, error) {
	if len(nonce) != NonceLen {
		return nil, errors.New("invalid nonce length")
	}
	totalLen := HeaderLen + len(payload) + TagLen
	if totalLen > 255 {
		return nil, errors.New("frame too long")
	}

	buf := &bytes.Buffer{}
	buf.WriteByte(PreambleByte)
	buf.WriteByte(byte(totalLen))
	buf.WriteByte(byte(typ))
	buf.WriteByte(seq)
	buf.Write(nonce)
	buf.Write(payload)
	buf.Write(make([]byte, TagLen))
	return buf.Bytes(), nil
}

// BuildSecureFrame encrypts and authenticates payload using AES-CTR + CMAC.
func BuildSecureFrame(typ PacketType, seq byte, nonce []byte, payload []byte, key []byte) ([]byte, error) {
	if len(key) != 16 {
		return nil, errors.New("key must be 16 bytes")
	}
	if len(nonce) != NonceLen {
		return nil, errors.New("invalid nonce length")
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	// Encrypt with AES-CTR
	iv := make([]byte, aes.BlockSize)
	copy(iv, nonce)
	iv[8] = seq
	ctr := cipher.NewCTR(block, iv)
	ct := make([]byte, len(payload))
	ctr.XORKeyStream(ct, payload)

	// Compute CMAC tag
	macData := append([]byte{byte(typ), seq}, append(nonce, ct...)...)
	tag := aesCmac(block, macData)[:TagLen]

	totalLen := HeaderLen + len(ct) + len(tag)
	if totalLen > 255 {
		return nil, errors.New("frame too long")
	}

	buf := &bytes.Buffer{}
	buf.WriteByte(PreambleByte)
	buf.WriteByte(byte(totalLen))
	buf.WriteByte(byte(typ))
	buf.WriteByte(seq)
	buf.Write(nonce)
	buf.Write(ct)
	buf.Write(tag)
	return buf.Bytes(), nil
}

// ParseFrame parses and verifies a LoRa frame.
func ParseFrame(frame []byte, key []byte) (PacketType, byte, []byte, []byte, error) {
	if len(frame) < HeaderLen+TagLen {
		return 0, 0, nil, nil, errors.New("frame too short")
	}
	if frame[0] != PreambleByte {
		return 0, 0, nil, nil, errors.New("invalid preamble")
	}

	length := int(frame[1])
	if length > len(frame) {
		return 0, 0, nil, nil, errors.New("declared length exceeds buffer")
	}
	frame = frame[:length]

	typ := PacketType(frame[2])
	seq := frame[3]
	nonce := frame[4 : 4+NonceLen]
	data := frame[4+NonceLen:]
	ct := data[:len(data)-TagLen]
	tag := data[len(data)-TagLen:]

	// no key = plain mode
	if key == nil {
		return typ, seq, nonce, ct, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return 0, 0, nil, nil, err
	}

	macData := append([]byte{byte(typ), seq}, append(nonce, ct...)...)
	expected := aesCmac(block, macData)[:TagLen]
	if !bytes.Equal(tag, expected) {
		return 0, 0, nil, nil, errors.New("cmac mismatch")
	}

	iv := make([]byte, aes.BlockSize)
	copy(iv, nonce)
	iv[8] = seq
	ctr := cipher.NewCTR(block, iv)
	pt := make([]byte, len(ct))
	ctr.XORKeyStream(pt, ct)
	return typ, seq, nonce, pt, nil
}

// aesCmac implements RFC4493 AES-CMAC (returns 16-byte tag).
func aesCmac(block cipher.Block, msg []byte) []byte {
	const n = aes.BlockSize
	zero := make([]byte, n)
	L := make([]byte, n)
	block.Encrypt(L, zero)
	k1 := subkey(L)
	k2 := subkey(k1)

	m := len(msg)
	num := (m + n - 1) / n
	if num == 0 {
		num = 1
	}
	X := make([]byte, n)
	Y := make([]byte, n)
	for i := range num - 1 {
		for j := range n {
			Y[j] = X[j] ^ msg[i*n+j]
		}
		block.Encrypt(X, Y)
	}
	last := make([]byte, n)
	if m > 0 && m%n == 0 {
		copy(last, msg[m-n:])
		for i := range n {
			last[i] ^= k1[i]
		}
	} else {
		start := (num - 1) * n
		copy(last, msg[start:])
		last[m%n] = 0x80
		for i := range n {
			last[i] ^= k2[i]
		}
	}
	for i := range n {
		Y[i] = X[i] ^ last[i]
	}
	T := make([]byte, n)
	block.Encrypt(T, Y)
	return T
}

// subkey generates next CMAC subkey.
func subkey(k []byte) []byte {
	const n = aes.BlockSize
	rb := byte(0x87)
	out := make([]byte, n)
	carry := byte(0)
	for i := n - 1; i >= 0; i-- {
		b := k[i]
		out[i] = (b << 1) | carry
		carry = (b >> 7) & 1
	}
	if k[0]>>7 == 1 {
		out[n-1] ^= rb
	}
	return out
}

// DecodeKeyHex decodes a 16-byte AES key from hex string.
func DecodeKeyHex(h string) ([]byte, error) {
	b, err := hex.DecodeString(h)
	if err != nil {
		return nil, err
	}
	if len(b) != 16 {
		return nil, fmt.Errorf("invalid key length %d", len(b))
	}
	return b, nil
}

```

- /internal/model/message.go
```go
// Package model defines the data structures
package model

// TelemetryData represents telemetry information reported by a vehicle.
// It is the common structure shared between vehicles, gateways and fog.
type TelemetryData struct {
	VehicleID   string  `json:"boatId"`
	Latitude    float64 `json:"lat"`
	Longitude   float64 `json:"lon"`
	CurrentHead uint16  `json:"head"`
	TargetHead  uint16  `json:"targetHead"`
	LeftSpeed   uint16  `json:"leftSpeed"`
	RightSpeed  uint16  `json:"rightSpeed"`
}

// ControlData represents a control command sent from Fog to a vehicle.
// It can be encoded either as JSON or CSV depending on gateway configuration.
type ControlData struct {
	VehicleID string  `json:"boatId"`
	Speed     int     `json:"speed"`
	Latitude  float64 `json:"targetLat"`
	Longitude float64 `json:"targetLon"`
	Kp        float64 `json:"kp"`
	Ki        float64 `json:"ki"`
	Kd        float64 `json:"kd"`
}

// ArduinoData represents telemetry data collected by arduino
type ArduinoData struct {
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	LeftSpeed   int     `json:"left_speed"`
	RightSpeed  int     `json:"right_speed"`
	CurrentHead int     `json:"current_head"`
	TargetHead  int     `json:"target_head"`
}

// ArduinoControl represents telemetry data collected by arduino
type ArduinoControl struct {
	CruiseSpeed int     `json:"cruise_speed"`
	Latitude    float64 `json:"latitude"`
	Longitude   float64 `json:"longitude"`
	Kp          float64 `json:"kp"`
	Ki          float64 `json:"ki"`
	Kd          float64 `json:"kd"`
}

```

- /internal/util/logger.go
```go
// Package util provides small utilities used by the system.
package util

// CHANGELOG (refactor v2):
// - Centralized slog setup for structured logging
// - Exported SetupLogger for callers to initialize global log behavior

import (
	"log/slog"
	"os"
)

// SetupLogger configures the global slog logger.
// Call once in main prior to starting System.
func SetupLogger() {
	// Use default handler (console) but include time and source.
	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		AddSource: true,
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

// CHANGELOG (refactor v2):
// - Improved process tracking and safe cleanup
// - Added context-aware start with error handling
// - Standardized naming and structured logging

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
		_ = sys.socatManager.CreatePair(vs.Left, vs.Right)
	}
	time.Sleep(300 * time.Millisecond)

	if cfg.Server.Addr != "" {
		sys.server = NewServer(cfg.Server.Addr, cfg.Server.AppAddr)
	}
	for _, g := range cfg.Gateways {
		sys.gateways = append(sys.gateways, NewGateway(g.ID, g.LoraDev, g.LoraBaud, g.Addr, g.ServerAddr))
	}
	for _, v := range cfg.Vehicles {
		sys.vehicles = append(sys.vehicles, NewVehicle(v.ID, v.LoraDev, v.LoraBaud, v.ArduinoDev, v.ArduinoBaud))
	}
	for _, a := range cfg.Arduinos {
		sys.arduinos = append(sys.arduinos, device.NewArduino(a.Dev, a.Baud))
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

// Shutdown gracefully stops all components.
func (s *System) Shutdown() {
	slog.Info("system shutting down")
	if s.cancel != nil {
		s.cancel()
	}
	for _, gw := range s.gateways {
		gw.Shutdown()
	}
	for _, vh := range s.vehicles {
		vh.Shutdown()
	}
	for _, ino := range s.arduinos {
		_ = ino.Close()
	}
	if s.server != nil {
		_ = s.server.Shutdown()
	}
	if s.socatManager != nil {
		s.socatManager.Cleanup()
	}
	s.wg.Wait()
	slog.Info("shutdown complete")
}

```

- /internal/core/vehicle.go
```go
// Package core implements Vehicle node logic.
package core

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
)

// Vehicle represents one LoRa boat node.
type Vehicle struct {
	ID         string
	lora       *device.Lora
	arduino    *device.Arduino
	sessionKey []byte

	cancel context.CancelFunc
}

// NewVehicle creates a new Vehicle instance.
func NewVehicle(id, dev string, baud int, arDev string, arBaud int) *Vehicle {
	v := &Vehicle{
		ID:   id,
		lora: device.NewLora(dev, baud),
	}
	if arDev != "" {
		v.arduino = device.NewArduino(arDev, arBaud)
	}
	return v
}

// Start begins LoRa and Arduino telemetry loop.
func (v *Vehicle) Start(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	v.cancel = cancel

	go v.listenLoop(ctx)
	if v.arduino != nil {
		go v.telemetryLoop(ctx)
	}
	return nil
}

// Shutdown stops the vehicle operations.
func (v *Vehicle) Shutdown() {
	if v.cancel != nil {
		v.cancel()
	}
	if v.lora != nil {
		_ = v.lora.Close()
	}
	if v.arduino != nil {
		_ = v.arduino.Close()
	}
	slog.Info("vehicle stopped", "id", v.ID)
}

// listenLoop handles incoming LoRa frames (beacon/auth/control).
func (v *Vehicle) listenLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// ReadFrames returns zero or more complete frames within timeout
		frames, err := v.lora.ReadFrames(10 * time.Second)
		if err != nil {
			// timeout or read error — continue listening
			continue
		}
		if len(frames) == 0 {
			// nothing read this cycle
			continue
		}

		// Try parse with current session key (if any). ParseFrame returns plaintext payload if key ok,
		// otherwise error. For plain frames, pass nil key.
		// Process each complete frame extracted by ReadFrames()
		for _, frame := range frames {
			var (
				typ     model.PacketType
				payload []byte
			)

			// If we have a session key, try decrypt/verify first.
			if v.sessionKey != nil {
				typ, _, _, payload, err = model.ParseFrame(frame, v.sessionKey)
				if err != nil {
					// try plain fallback (no key)
					typ, _, _, payload, err = model.ParseFrame(frame, nil)
					if err != nil {
						// unable to parse even as plain — skip this frame
						slog.Warn("failed to parse frame (secure and plain)", "vehicle", v.ID, "err", err)
						continue
					}
				}
			} else {
				// no key -> parse as plain
				typ, _, _, payload, err = model.ParseFrame(frame, nil)
				if err != nil {
					slog.Warn("failed to parse plain frame", "vehicle", v.ID, "err", err)
					continue
				}
			}
			switch typ {
			case model.TypeBeacon:
				v.handleBeacon()
			case model.TypeAuth:
				v.handleAuth(payload)
			case model.TypeControl:
				v.handleControl(payload)
			default:
				// ignore other types
				slog.Debug("ignoring unknown packet type", "vehicle", v.ID, "type", typ)
			}
		}
	}
}

// telemetryLoop reads Arduino data and sends telemetry frames.
func (v *Vehicle) telemetryLoop(ctx context.Context) {
	ch := make(chan model.ArduinoData, 4)
	stop, err := v.arduino.Read(ch)
	if err != nil {
		slog.Warn("arduino read failed", "vehicle", v.ID, "err", err)
		return
	}
	defer stop()

	for {
		select {
		case <-ctx.Done():
			return
		case d, ok := <-ch:
			if !ok {
				return
			}
			v.sendTelemetry(d)
		}
	}
}

// sendTelemetry packs and transmits telemetry data.
func (v *Vehicle) sendTelemetry(d model.ArduinoData) {
	lat := int32(d.Latitude * 1e7)
	lon := int32(d.Longitude * 1e7)
	speed := uint16(d.LeftSpeed / 10) // conversion example; adjust per your mapping
	head := uint16(d.CurrentHead % 360)

	payload := model.BuildTelemetryPacked(lat, lon, speed, head)

	nonce := make([]byte, model.NonceLen)
	copy(nonce, []byte(time.Now().Format("15040506"))[:model.NonceLen])

	var frame []byte
	var err error
	if v.sessionKey != nil {
		frame, err = model.BuildSecureFrame(model.TypeTelemetry, 0, nonce, payload, v.sessionKey)
	} else {
		frame, err = model.BuildPlainFrame(model.TypeTelemetry, 0, nonce, payload)
	}
	if err != nil {
		slog.Warn("build telemetry frame failed", "vehicle", v.ID, "err", err)
		return
	}
	if err := v.lora.WriteBytes(frame); err != nil {
		slog.Warn("failed to write telemetry frame", "vehicle", v.ID, "err", err)
		return
	}
	slog.Debug("telemetry sent", "vehicle", v.ID)
}

// handleBeacon sends a HELLO frame to gateway when a beacon is seen.
func (v *Vehicle) handleBeacon() {
	msg := map[string]any{"type": "hello", "vehicle_id": v.ID}
	b, _ := json.Marshal(msg)
	nonce := make([]byte, model.NonceLen)
	copy(nonce, []byte(time.Now().Format("15040506"))[:model.NonceLen])
	frame, _ := model.BuildPlainFrame(model.TypeHello, 0, nonce, b)
	_ = v.lora.WriteBytes(frame)
	slog.Info("HELLO sent", "vehicle", v.ID)
}

// handleAuth processes an auth frame payload (expects JSON with key_hex and ttl).
func (v *Vehicle) handleAuth(p []byte) {
	var am map[string]any
	if err := json.Unmarshal(p, &am); err != nil {
		slog.Warn("invalid auth payload", "vehicle", v.ID, "err", err)
		return
	}
	if kh, ok := am["key_hex"].(string); ok && kh != "" {
		kb, err := hex.DecodeString(kh)
		if err != nil {
			slog.Warn("invalid key hex", "vehicle", v.ID, "err", err)
		} else if len(kb) == 16 {
			v.sessionKey = kb
			slog.Info("session key applied", "vehicle", v.ID)
		}
	}
	if ttlf, ok := am["ttl"].(float64); ok {
		_ = ttlf // we could store expiry if needed
	}
}

// handleControl forwards downlink control to Arduino or logs it.
func (v *Vehicle) handleControl(p []byte) {
	// Control payload may be JSON or raw bytes; try JSON decode for helpful log.
	var ctrl map[string]any
	if err := json.Unmarshal(p, &ctrl); err == nil {
		slog.Info("control received", "vehicle", v.ID, "cmd", ctrl)
	} else {
		slog.Info("control received (raw)", "vehicle", v.ID)
	}
	if v.arduino != nil {
		// forward raw bytes as string (Arduino expects line-based). Adapt as necessary.
		_ = v.arduino.Write(string(p))
	}
}

// ApplyKeyHex sets AES key for secure transmission.
func (v *Vehicle) ApplyKeyHex(h string) error {
	k, err := hex.DecodeString(h)
	if err != nil {
		return err
	}
	if len(k) != 16 {
		return fmt.Errorf("invalid key length %d", len(k))
	}
	v.sessionKey = k
	slog.Info("applied session key", "vehicle", v.ID)
	return nil
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
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"LoraFog/internal/model"

	"github.com/gorilla/websocket"
)

// Server is the lightweight Fog backend managing registry and telemetry.
type Server struct {
	Addr    string
	AppAddr string

	server *http.Server

	vehicleRegistry sync.Map // vehicleID -> gatewayURL

	clients   map[*websocket.Conn]bool
	clientMu  sync.Mutex
	sessionMu sync.Mutex
	sessions  map[string]*Session
}

// Session stores temporary authentication for a vehicle.
type Session struct {
	VehicleID string
	GatewayID string
	CreatedAt time.Time
	TTL       time.Duration
}

// NewServer creates a new Fog HTTP server.
func NewServer(addr, appAddr string) *Server {
	return &Server{
		Addr:     addr,
		AppAddr:  appAddr,
		clients:  make(map[*websocket.Conn]bool),
		sessions: make(map[string]*Session),
	}
}

// Start runs the HTTP server and session sweeper.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/telemetry", s.handleTelemetry)
	mux.HandleFunc("/api/register", s.handleRegister)
	mux.HandleFunc("/api/control", s.handleControl)
	mux.HandleFunc("/api/gw/report", s.handleGatewayReport)
	mux.HandleFunc("/ws", s.handleWebSocket)

	addr := strings.TrimPrefix(strings.TrimPrefix(s.Addr, "http://"), "https://")
	s.server = &http.Server{Addr: addr, Handler: mux}

	go s.sweeper(ctx)
	slog.Info("Fog server started", "addr", addr)
	if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// Shutdown stops the HTTP server.
func (s *Server) Shutdown() error {
	if s.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	slog.Info("Stopping Fog server")
	return s.server.Shutdown(ctx)
}

// handleTelemetry accepts uplink telemetry and relays to app/websocket.
func (s *Server) handleTelemetry(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close telemetry request body",
					"component", "server", "error", err)
			}
		}
	}()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	var telemetry model.TelemetryData
	if err := json.Unmarshal(body, &telemetry); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}

	out, _ := json.Marshal(telemetry)
	s.broadcast(string(out))

	// Forward telemetry to App Server if configured
	if s.AppAddr != "" {
		go func() {
			resp, err := http.Post(s.AppAddr+"/api/telemetry",
				"application/json", bytes.NewReader(out))
			if err != nil {
				slog.Warn("failed to forward telemetry",
					"component", "fog", "app", s.AppAddr, "error", err)
				return
			}
			if resp != nil && resp.Body != nil {
				_ = resp.Body.Close()
			}
		}()
	}
	w.WriteHeader(http.StatusOK)
}

// handleRegister registers a vehicle and returns session info.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close register request body",
					"component", "server", "error", err)
			}
		}
	}()

	var req struct {
		GatewayID string `json:"gateway_id"`
		VehicleID string `json:"vehicle_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	s.sessionMu.Lock()
	s.sessions[req.VehicleID] = &Session{
		VehicleID: req.VehicleID,
		GatewayID: req.GatewayID,
		CreatedAt: time.Now(),
		TTL:       5 * time.Minute,
	}
	s.sessionMu.Unlock()

	resp := map[string]any{"vehicle_id": req.VehicleID, "key_hex": "", "ttl": 300}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
	slog.Info("registered vehicle", "vehicle", req.VehicleID, "gateway", req.GatewayID)
}

// handleControl forwards control messages to the target gateway.
func (s *Server) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close control request body",
					"component", "server", "error", err)
			}
		}
	}()

	var ctrl map[string]any
	if err := json.NewDecoder(r.Body).Decode(&ctrl); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	vehicleID, _ := ctrl["vehicle_id"].(string)
	val, ok := s.vehicleRegistry.Load(vehicleID)
	if !ok {
		http.Error(w, "gateway not found", http.StatusNotFound)
		return
	}
	gatewayURL := val.(string)
	payload, _ := json.Marshal(ctrl)
	go func() {
		resp, err := http.Post(gatewayURL+"/command",
			"application/json", bytes.NewReader(payload))
		if err != nil {
			slog.Warn("failed to send control to gateway",
				"component", "fog", "gateway", gatewayURL, "error", err)
			return
		}
		if resp != nil && resp.Body != nil {
			_ = resp.Body.Close()
		}
		slog.Info("control forwarded",
			"component", "fog", "vehicle", vehicleID, "gateway", gatewayURL)
	}()
	w.WriteHeader(http.StatusAccepted)
}

// handleWebSocket provides live telemetry streaming to dashboard clients.
func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	s.clientMu.Lock()
	s.clients[conn] = true
	s.clientMu.Unlock()

	go func() {
		defer func() {
			s.clientMu.Lock()
			delete(s.clients, conn)
			s.clientMu.Unlock()
			if err := conn.Close(); err != nil {
				slog.Warn("failed to close websocket", "error", err)
			}
		}()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
}

// handleGatewayReport logs periodic gateway reports.
func (s *Server) handleGatewayReport(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close report request body",
					"component", "server", "error", err)
			}
		}
	}()

	var rep struct {
		GWID      string `json:"gw_id"`
		Region    string `json:"region"`
		SlotUsage int    `json:"slotUsage"`
		AvgDelay  int    `json:"avgDelay"`
		Collision int    `json:"collision"`
	}
	if err := json.NewDecoder(r.Body).Decode(&rep); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	slog.Info("gateway report", "gw", rep.GWID, "usage", rep.SlotUsage, "delay", rep.AvgDelay, "coll", rep.Collision)
	w.WriteHeader(http.StatusOK)
}

// sweeper cleans expired sessions.
func (s *Server) sweeper(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			now := time.Now()
			s.sessionMu.Lock()
			for k, v := range s.sessions {
				if now.Sub(v.CreatedAt) > v.TTL {
					delete(s.sessions, k)
					slog.Info("session expired", "vehicle", k)
				}
			}
			s.sessionMu.Unlock()
		}
	}
}

// broadcast sends message to all connected WebSocket clients.
func (s *Server) broadcast(msg string) {
	s.clientMu.Lock()
	defer s.clientMu.Unlock()
	for conn := range s.clients {
		if err := conn.WriteMessage(websocket.TextMessage, []byte(msg)); err != nil {
			slog.Warn("websocket send failed",
				"component", "fog", "error", err)
		}
	}
}

```

- /internal/core/gateway.go
```go
// Package core implements Gateway: TDMA master, LoRa relay, auto-tuning.
package core

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"sync"
	"time"

	"LoraFog/internal/device"
	"LoraFog/internal/model"
)

// Gateway manages LoRa radio, TDMA timing, and reports to Fog.
type Gateway struct {
	ID         string
	Addr       string
	ServerAddr string
	lora       *device.Lora

	keyMu sync.Mutex
	keys  map[string][]byte

	slotDur   time.Duration
	guardMs   int
	slotCount int

	stopCtx    context.Context
	stopCancel context.CancelFunc
	wg         sync.WaitGroup
	httpSrv    *http.Server
}

// NewGateway creates a gateway instance.
func NewGateway(id, dev string, baud int, addr, srv string) *Gateway {
	return &Gateway{
		ID:         id,
		Addr:       addr,
		ServerAddr: srv,
		lora:       device.NewLora(dev, baud),
		keys:       make(map[string][]byte),
		slotDur:    800 * time.Millisecond,
		guardMs:    200,
		slotCount:  8,
	}
}

// Start runs LoRa uplink/downlink and beacon broadcasting.
func (g *Gateway) Start(ctx context.Context) error {
	g.stopCtx, g.stopCancel = context.WithCancel(ctx)

	g.wg.Add(1)
	go g.beaconLoop()
	g.wg.Add(1)
	go g.uplinkLoop()
	g.wg.Add(1)
	go g.reportLoop()

	mux := http.NewServeMux()
	mux.HandleFunc("/command", g.handleControl)
	g.httpSrv = &http.Server{Addr: g.Addr, Handler: mux}

	g.wg.Add(1)
	go func() {
		defer g.wg.Done()
		if err := g.httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("gateway http error", "err", err)
		}
	}()
	return nil
}

// Shutdown stops the gateway and closes LoRa interface.
func (g *Gateway) Shutdown() {
	if g.stopCancel != nil {
		g.stopCancel()
	}

	if g.lora != nil {
		if err := g.lora.Close(); err != nil {
			slog.Warn("failed to close device",
				"component", "gateway", "id", g.ID, "error", err)
		}
	}

	if g.httpSrv != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := g.httpSrv.Shutdown(ctx); err != nil {
			slog.Warn("gateway HTTP server shutdown error",
				"component", "gateway", "id", g.ID, "error", err)
		}
	}

	g.wg.Wait()
	slog.Info("gateway stopped", "id", g.ID)
}

// beaconLoop periodically sends TDMA sync beacons.
func (g *Gateway) beaconLoop() {
	defer g.wg.Done()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-g.stopCtx.Done():
			return
		case <-t.C:
			beacon := map[string]any{
				"type": "beacon", "gw": g.ID,
				"slot":  g.slotDur.Milliseconds(),
				"guard": g.guardMs,
			}
			b, _ := json.Marshal(beacon)
			frame, _ := model.BuildPlainFrame(model.TypeBeacon, 0, make([]byte, model.NonceLen), b)
			_ = g.lora.WriteBytes(frame)
			slog.Debug("beacon sent", "gw", g.ID)
		}
	}
}

// uplinkLoop reads frames from LoRa and forwards telemetry.
func (g *Gateway) uplinkLoop() {
	defer g.wg.Done()
	for {
		select {
		case <-g.stopCtx.Done():
			return
		default:
		}

		// ReadFrames returns zero or more complete frames within timeout.
		frames, err := g.lora.ReadFrames(5 * time.Second)
		if err != nil {
			continue
		}
		if len(frames) == 0 {
			// nothing read this cycle
			continue
		}

		// Try secure parse using known keys
		g.keyMu.Lock()
		keys := make(map[string][]byte, len(g.keys))
		maps.Copy(keys, g.keys)
		g.keyMu.Unlock()

		// Process each extracted frame
		for _, frame := range frames {
			handled := false

			// 1) Try to decrypt/verify with each known key (fast path for secure telemetry)
			for vid, key := range keys {
				typ, _, _, payload, err := model.ParseFrame(frame, key)
				if err != nil {
					// decryption/verification failed with this key, try next
					continue
				}
				// parsed OK with this key
				if typ == model.TypeTelemetry {
					go g.postTelemetry(vid, payload)
				}
				handled = true
				break // don't try other keys for this frame
			}
			if handled {
				continue // next frame
			}

			// 2) Fallback: try plain (no key) parse
			typ, _, _, payload, err := model.ParseFrame(frame, nil)
			if err != nil {
				// cannot parse frame even as plain -> drop and continue
				slog.Warn("unable to parse frame", "gw", g.ID, "err", err)
				continue
			}
			if typ == model.TypeTelemetry {
				// For plain telemetry we don't know vehicle id: post as generic telemetry.
				// If your plain payload contains vehicle id, modify postTelemetry to accept it.
				go g.postTelemetry("", payload)
			} else {
				// other plain types (hello/beacon/auth) may be ignored here or handled if needed
				slog.Debug("received non-telemetry plain frame", "gw", g.ID, "type", typ)
			}
		}
	}
}

// postTelemetry forwards telemetry to Fog server.
func (g *Gateway) postTelemetry(vid string, data []byte) {
	t, err := model.ParseTelemetryPacked(data)
	if err != nil {
		slog.Warn("failed to decode telemetry", "gw", g.ID, "err", err)
		return
	}

	telemetry := model.TelemetryData{
		VehicleID:  vid,
		Latitude:   float64(t.LatI32) / 1e7,
		Longitude:  float64(t.LonI32) / 1e7,
		LeftSpeed:  t.SpeedU16,
		RightSpeed: t.HeadingU16,
	}

	b, _ := json.Marshal(telemetry)
	url := "http://" + g.ServerAddr + "/api/telemetry"
	resp, err := http.Post(url, "application/json", bytes.NewReader(b))
	if err != nil {
		slog.Warn("failed to send report",
			"component", "gateway", "id", g.ID, "error", err)
		return
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

// handleControl handles HTTP control from Fog -> Vehicle.
func (g *Gateway) handleControl(w http.ResponseWriter, r *http.Request) {
	defer func() {
		if r.Body != nil {
			if err := r.Body.Close(); err != nil {
				slog.Warn("failed to close control request body",
					"component", "gateway", "id", g.ID, "error", err)
			}
		}
	}()
	var ctrl map[string]any
	if err := json.NewDecoder(r.Body).Decode(&ctrl); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	vid, _ := ctrl["vehicle_id"].(string)
	if vid == "" {
		http.Error(w, "vehicle_id missing", http.StatusBadRequest)
		return
	}
	b, _ := json.Marshal(ctrl)
	nonce := make([]byte, model.NonceLen)
	g.keyMu.Lock()
	key := g.keys[vid]
	g.keyMu.Unlock()

	var frame []byte
	var err error
	if len(key) == 16 {
		frame, err = model.BuildSecureFrame(model.TypeControl, 0, nonce, b, key)
	} else {
		frame, err = model.BuildPlainFrame(model.TypeControl, 0, nonce, b)
	}
	if err == nil {
		_ = g.lora.WriteBytes(frame)
	}
	w.WriteHeader(http.StatusAccepted)
}

// reportLoop posts periodic stats and performs simple auto-tuning.
func (g *Gateway) reportLoop() {
	defer g.wg.Done()
	t := time.NewTicker(20 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-g.stopCtx.Done():
			return
		case <-t.C:
			body := map[string]any{
				"gw_id": g.ID, "slotUsage": g.slotCount, "avgDelay": 80, "collision": 0,
			}
			payload, _ := json.Marshal(body)
			go func() {
				resp, err := http.Post("http://"+g.ServerAddr+"/api/gw/report", "application/json", bytes.NewReader(payload))
				if err != nil {
					slog.Warn("failed to send report",
						"component", "gateway", "id", g.ID, "error", err)
					return
				}
				if resp != nil && resp.Body != nil {
					_ = resp.Body.Close()
				}
			}()

			// simple tuning logic
			if g.guardMs < 1000 {
				g.guardMs += 20
			}
		}
	}
}

// RegisterKey stores a 16-byte key for a vehicle.
func (g *Gateway) RegisterKey(vid string, hexKey string) error {
	k, err := hex.DecodeString(hexKey)
	if err != nil {
		return err
	}
	if len(k) != 16 {
		return fmt.Errorf("invalid key length")
	}
	g.keyMu.Lock()
	g.keys[vid] = k
	g.keyMu.Unlock()
	return nil
}

```

- /internal/device/serial.go
```go
// Package device implements a simple wrapper for serial communication.
// It provides non-blocking read/write methods with optional timeout.
package device

// CHANGELOG (refactor v2):
// - Safe read/write with timeout
// - Added context support via external control
// - Structured logging (slog)
// - Safe Close() checks and standardized naming

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

// ReadBytes reads exactly n bytes from the serial port.
// It blocks until all bytes are received or an error occurs.
func (s *Serial) ReadBytes(n int) ([]byte, error) {
	if s.Port == nil {
		return nil, errors.New("serial port not initialized")
	}
	buf := make([]byte, n)
	total := 0
	for total < n {
		readCount, err := io.ReadFull(s.reader, buf[total:])
		total += readCount
		if err != nil {
			return nil, err
		}
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
// Package device implements ArduinoDevice for reading and writing telemetry
// over serial, as well as simulation support.
package device

// CHANGELOG (refactor v2):
// - Context-based lifecycle and safe shutdown
// - Replaced stop channel with function-returned closure
// - Structured logging (slog)
// - Safe Close() with nil checks
// - Added telemetry simulation helper

import (
	"fmt"
	"log/slog"
	"math/rand"
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

// StartSimulation generates synthetic telemetry data periodically for testing.
func (a *Arduino) StartSimulation(stop <-chan struct{}) error {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	slog.Info("starting Arduino simulation",
		"component", "arduino", "device", a.Device)

	for {
		select {
		case <-stop:
			slog.Info("stopping Arduino simulation",
				"component", "arduino", "device", a.Device)
			return nil
		case <-ticker.C:
			data := model.ArduinoData{
				Latitude:    21.027 + rand.Float64()*0.001,
				Longitude:   105.835 + rand.Float64()*0.001,
				CurrentHead: rand.Intn(361),
				TargetHead:  rand.Intn(361),
				LeftSpeed:   1000 + rand.Intn(1000),
				RightSpeed:  1000 + rand.Intn(1000),
			}
			line := fmt.Sprintf("%f,%f,%d,%d,%d,%d",
				data.Latitude, data.Longitude, data.CurrentHead,
				data.TargetHead, data.LeftSpeed, data.RightSpeed)

			if err := a.Write(line); err != nil {
				slog.Warn("failed to write simulated telemetry",
					"component", "arduino", "device", a.Device, "error", err)
			}
		}
	}
}

// Close closes the Arduino serial port safely.
func (a *Arduino) Close() error {
	if a.serial == nil {
		return nil
	}
	return a.serial.Close()
}

```

- /internal/device/lora.go
```go
// Package device implements LoraDevice, a binary (CBOR) serial communication handler.
// It is used for LoRa links between Vehicle and Gateway.
package device

// CHANGELOG (refactor v2):
// - Added new binary-based LoraDevice (no simulation, unlike Arduino)
// - Uses Serial.ReadBytes() and WriteBytes() for CBOR payloads
// - Safe close and structured logging (slog)

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"time"
)

// Lora manages binary (CBOR) communication over a serial LoRa interface.
type Lora struct {
	Path   string
	Baud   int
	serial *Serial
}

// NewLora creates a new Lora.
func NewLora(path string, baud int) *Lora {
	s, err := NewSerial(path, baud)
	if err != nil {
		slog.Warn("failed to connect Lora device",
			"component", "lora", "device", path, "error", err)
	}
	return &Lora{
		Path:   path,
		Baud:   baud,
		serial: s,
	}
}

// ReadFrames continuously parses complete frames from serial input.
func (l *Lora) ReadFrames(timeout time.Duration) ([][]byte, error) {
	var frames [][]byte
	buf := make([]byte, 0, 512)
	deadline := time.Now().Add(timeout)

	for time.Now().After(deadline) {
		chunk, _ := l.serial.ReadBytes(16) // read chunk (non-blocking)
		if len(chunk) == 0 {
			time.Sleep(5 * time.Millisecond)
			continue
		}
		buf = append(buf, chunk...)

		for len(buf) < 2 {
			if buf[0] != 0xAA {
				// discard noise until preamble
				buf = buf[1:]
				continue
			}
			length := int(buf[1])
			if len(buf) < length {
				break // wait for more data
			}
			frame := buf[:length]
			frames = append(frames, frame)
			buf = buf[length:] // move to next potential frame
		}
	}
	return frames, nil
}

// WriteFrame sends a binary packed message frame
func (l *Lora) WriteFrame(payload []byte) error {
	if l.serial == nil {
		return fmt.Errorf("lora serial not initialized")
	}
	length := uint16(len(payload))
	frame := make([]byte, 2+len(payload))
	binary.BigEndian.PutUint16(frame[0:2], length)
	copy(frame[2:], payload)
	return l.serial.WriteBytes(frame)
}

// WriteBytes writes a full frame to the serial device.
func (l *Lora) WriteBytes(b []byte) error {
	if l.serial == nil {
		return fmt.Errorf("lora serial not initialized")
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
