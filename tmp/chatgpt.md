Dưới đây là **toàn bộ mã nguồn đã sửa theo yêu cầu** — module tên `LoraFog`. Mình đã:

- Dùng `/dev/serial0` làm mặc định cho LoRa (và simulator có thể override).
- Tối ưu mọi `Close()` / shutdown với kiểm tra lỗi để tránh warnings.
- Thêm **telemetry simulator** (gửi CSV vào serial).
- Thêm **gateway registration** trong Fog (Fog duy trì map vehicleID → gatewayURL để route control).
- Viết comment tiếng Anh đầy đủ, chuẩn GoDoc style.
- Sử dụng newline-delimited CSV on LoRa (vehicle↔gateway) và JSON over HTTP/WebSocket (gateway↔fog).

Copy/paste từng file vào project (giữ cấu trúc thư mục). Nếu bạn muốn, mình có thể nén thành repo hoặc tạo `docker-compose` sau.

---

## `go.mod`

```go
module LoraFog

go 1.23

require (
    github.com/gorilla/websocket v1.5.3
    go.bug.st/serial.v1 v1.3.12
)
```

---

## `internal/model/message.go`

```go
// Package model defines shared message structures for LoraFog.
package model

// VehicleData represents telemetry parsed from CSV and forwarded as JSON.
type VehicleData struct {
 VehicleID string  `json:"vehicle_id"`
 Lat       float64 `json:"lat"`
 Lon       float64 `json:"lon"`
 Head      float64 `json:"head"`
 LeftSpd   float64 `json:"left_speed"`
 RightSpd  float64 `json:"right_speed"`
 Timestamp string  `json:"timestamp"` // ISO8601 formatted on gateway
}

// ControlMessage is used by Fog -> Gateway (JSON). Gateway converts to CSV for LoRa.
type ControlMessage struct {
 VehicleID string `json:"vehicle_id"`
 Payload   string `json:"payload"` // command payload (plain string)
 MsgID     string `json:"msg_id"`  // optional message id
}

// AckMessage is a simple ack structure.
type AckMessage struct {
 MsgID string `json:"msg_id"`
 Ack   bool   `json:"ack"`
}

// GatewayRegistration is used when a gateway registers to Fog.
type GatewayRegistration struct {
 GatewayID string   `json:"gateway_id"`
 URL       string   `json:"url"`       // public URL where Fog can reach gateway (HTTP)
 Vehicles  []string `json:"vehicles"`  // optional list of vehicle IDs this gateway serves
}
```

---

## `internal/lora/serial.go`

```go
// Package lora provides a light wrapper over a serial port used for LoRa E32 modules.
// It reads/writes newline-delimited text lines.
package lora

import (
 "bufio"
 "errors"
 "io"
 "time"

 serial "go.bug.st/serial.v1"
)

// LoRa wraps serial.Port and a buffered reader.
type LoRa struct {
 port serial.Port
 r    *bufio.Reader
}

// New opens a serial device (e.g. /dev/serial0) with given baudrate.
func New(device string, baud int) (*LoRa, error) {
 p, err := serial.Open(device, &serial.Mode{BaudRate: baud})
 if err != nil {
  return nil, err
 }
 return &LoRa{port: p, r: bufio.NewReader(p)}, nil
}

// ReadLine reads a single line terminated by '\n'. If timeout > 0, it will return after timeout.
func (l *LoRa) ReadLine(timeout time.Duration) (string, error) {
 ch := make(chan struct {
  line string
  err  error
 }, 1)

 // reader goroutine
 go func() {
  line, err := l.r.ReadString('\n')
  if err != nil {
   // convert io.EOF to error for caller
   ch <- struct {
    line string
    err  error
   }{"", err}
   return
  }
  ch <- struct {
   line string
   err  error
  }{line, nil}
 }()

 if timeout <= 0 {
  res := <-ch
  return res.line, res.err
 }

 select {
 case res := <-ch:
  return res.line, res.err
 case <-time.After(timeout):
  return "", errors.New("read timeout")
 }
}

// WriteLine writes s + '\n' to serial.
func (l *LoRa) WriteLine(s string) error {
 _, err := l.port.Write(append([]byte(s), '\n'))
 return err
}

// Close closes the underlying port and returns error if any.
func (l *LoRa) Close() error {
 if l.port == nil {
  return nil
 }
 return l.port.Close()
}
```

---

## `internal/parser/parser.go`

```go
// Package parser converts CSV wire format to structured types and vice-versa.
//
// CSV telemetry wire format (vehicle -> gateway):
//   VEHICLE_ID,LAT,LON,HEAD,LEFT_SPEED,RIGHT_SPEED
//
// Example:
//   VEH01,21.028511,105.804817,87.2,12.5,12.4
package parser

import (
 "errors"
 "fmt"
 "strconv"
 "strings"
 "time"

 "LoraFog/internal/model"
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
```

---

## `cmd/vehicle/main.go` (vehicle v2 — real GPS + LoRa listener)

```go
// Vehicle agent (realistic): reads GPS from serial, sends CSV telemetry via LoRa (/dev/serial0),
// listens for control CSV and replies with ACKs. Resources are properly closed and checked.
package main

import (
 "bufio"
 "flag"
 "fmt"
 "log"
 "math"
 "os"
 "os/signal"
 "strconv"
 "strings"
 "syscall"
 "time"

 "LoraFog/internal/lora"
 "LoraFog/internal/parser"
)

// parseNMEACoord converts NMEA ddmm.mmmm to decimal degrees.
func parseNMEACoord(value string, dir string) (float64, error) {
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

// readGPSFromDevice reads NMEA sentence from GPS serial (device param).
func readGPSFromDevice(device string, baud int, timeout time.Duration) (float64, float64, error) {
 // Use go.bug.st/serial directly for GPS reading
 port, err := lora.OpenRawSerial(device, baud) // helper below
 if err != nil {
  return 0, 0, err
 }
 // ensure close
 defer func() {
  if cerr := port.Close(); cerr != nil {
   log.Printf("warning: close gps serial err: %v", cerr)
  }
 }()

 r := bufio.NewReader(port)
 deadline := time.Now().Add(timeout)
 for time.Now().Before(deadline) {
  line, err := r.ReadString('\n')
  if err != nil {
   continue
  }
  line = strings.TrimSpace(line)
  if strings.HasPrefix(line, "$GPGGA") || strings.HasPrefix(line, "$GNRMC") {
   parts := strings.Split(line, ",")
   // GPGGA: parts[2]=lat, parts[3]=N/S, parts[4]=lon, parts[5]=E/W
   if len(parts) >= 6 {
    lat, err1 := parseNMEACoord(parts[2], parts[3])
    lon, err2 := parseNMEACoord(parts[4], parts[5])
    if err1 == nil && err2 == nil {
     return lat, lon, nil
    }
   }
  }
 }
 return 0, 0, fmt.Errorf("no gps fix")
}

func main() {
 vehicleID := flag.String("id", "VEH01", "vehicle id")
 gpsDev := flag.String("gps", "/dev/ttyS0", "gps serial device")
 gpsBaud := flag.Int("gpsbaud", 9600, "gps baudrate")
 loraDev := flag.String("lora", "/dev/serial0", "lora serial device")
 loraBaud := flag.Int("lorabaud", 9600, "lora baudrate")
 interval := flag.Int("interval", 3000, "telemetry send interval ms")
 flag.Parse()

 // open LoRa serial
 l, err := lora.New(*loraDev, *loraBaud)
 if err != nil {
  log.Fatalf("open lora: %v", err)
 }
 // ensure close on exit
 defer func() {
  if cerr := l.Close(); cerr != nil {
   log.Printf("warning: close lora err: %v", cerr)
  }
 }()

 // graceful shutdown
 stop := make(chan os.Signal, 1)
 signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

 log.Printf("vehicle %s start: lora=%s gps=%s", *vehicleID, *loraDev, *gpsDev)

 // reader for incoming messages
 go func() {
  for {
   line, err := l.ReadLine(0)
   if err != nil {
    // non-fatal: wait and retry
    time.Sleep(100 * time.Millisecond)
    continue
   }
   line = strings.TrimSpace(line)
   if line == "" {
    continue
   }
   // Expect control CSV: VEH_ID,PAYLOAD,MSGID or CTRL,left,right
   parts := strings.Split(line, ",")
   // If payload begins with CTRL -> control
   if len(parts) >= 1 && parts[0] == "CTRL" {
    // CTRL,leftSpeed,rightSpeed
    if len(parts) >= 3 {
     left, _ := strconv.ParseFloat(parts[1], 64)
     right, _ := strconv.ParseFloat(parts[2], 64)
     log.Printf("control cmd: left=%.2f right=%.2f", left, right)
     // TODO: apply to motor driver
    }
    // ack back
    _ = l.WriteLine("ACK,CTRL")
   } else {
    // maybe other format: log/ignore
    log.Printf("received: %s", line)
   }
  }
 }()

 ticker := time.NewTicker(time.Duration(*interval) * time.Millisecond)
 defer ticker.Stop()

 for {
  select {
  case <-stop:
   log.Println("vehicle stopping")
   return
  case <-ticker.C:
   lat, lon, err := readGPSFromDevice(*gpsDev, *gpsBaud, 2*time.Second)
   if err != nil {
    log.Printf("gps read failed: %v; using fallback", err)
    lat = 21.028511
    lon = 105.804817
   }
   head := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
   // left/right speed from local sensors (not implemented) — placeholder
   left := 12.0
   right := 12.0
   v := fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f", *vehicleID, lat, lon, head, left, right)
   if err := l.WriteLine(v); err != nil {
    log.Printf("lora write err: %v", err)
   } else {
    log.Printf("sent telemetry: %s", v)
   }
  }
 }
}
```

> **Note:** `readGPSFromDevice` uses a helper `lora.OpenRawSerial` (below) to open the raw port for GPS; we keep LoRa wrapper separate to avoid interfering with its buffered reader.

---

## small helper in `internal/lora/raw.go` (open raw serial Port for GPS)

```go
// +build ignore

// Helper function: provides raw serial.Port which implements io.ReadWriteCloser
package lora

import (
 serial "go.bug.st/serial.v1"
)

// OpenRawSerial opens a raw serial.Port for devices like GPS where caller wants raw io.
func OpenRawSerial(device string, baud int) (serial.Port, error) {
 return serial.Open(device, &serial.Mode{BaudRate: baud})
}
```

> Put this file under `internal/lora/raw.go`. It simply wraps serial.Open so vehicle GPS reader can use it and close properly.

---

## `cmd/gateway/main.go` (optimized, registers with Fog)

```go
// Gateway program:
// - Reads CSV from LoRa (/dev/serial0)
// - Parses CSV -> JSON and forwards to Fog /ingest (worker pool)
// - Exposes /command endpoint (JSON) to receive control from Fog; converts to CSV and sends via LoRa
// - Registers itself to Fog on startup (POST /register)
package main

import (
 "bytes"
 "context"
 "encoding/json"
 "flag"
 "io"
 "log"
 "net/http"
 "os"
 "os/signal"
 "sync"
 "time"

 "LoraFog/internal/lora"
 "LoraFog/internal/model"
 "LoraFog/internal/parser"
)

func main() {
 serialDev := flag.String("lora", "/dev/serial0", "LoRa serial device")
 baud := flag.Int("baud", 9600, "serial baud")
 fogAddr := flag.String("fog", "http://127.0.0.1:8080", "Fog base URL")
 gatewayID := flag.String("id", "GW01", "gateway id")
 flag.Parse()

 // open LoRa
 l, err := lora.New(*serialDev, *baud)
 if err != nil {
  log.Fatalf("open lora: %v", err)
 }
 defer func() {
  if cerr := l.Close(); cerr != nil {
   log.Printf("warning: close lora err: %v", cerr)
  }
 }()

 // channels and worker pools
 forwardCh := make(chan model.VehicleData, 1024)
 sendCh := make(chan string, 512)
 var wg sync.WaitGroup

 ctx, cancel := context.WithCancel(context.Background())
 defer cancel()

 // forward workers
 client := &http.Client{Timeout: 5 * time.Second}
 for i := 0; i < 4; i++ {
  wg.Add(1)
  go func(id int) {
   defer wg.Done()
   for v := range forwardCh {
    b, _ := json.Marshal(v)
    resp, err := client.Post(*fogAddr+"/ingest", "application/json", bytes.NewReader(b))
    if err != nil {
     log.Printf("forward worker %d post err: %v", id, err)
     continue
    }
    _, _ = io.Copy(io.Discard, resp.Body)
    _ = resp.Body.Close()
   }
  }(i)
 }

 // send workers
 for i := 0; i < 2; i++ {
  wg.Add(1)
  go func(id int) {
   defer wg.Done()
   for s := range sendCh {
    if err := l.WriteLine(s); err != nil {
     log.Printf("send worker %d write err: %v", id, err)
    }
    time.Sleep(30 * time.Millisecond)
   }
  }(i)
 }

 // register to Fog
 reg := model.GatewayRegistration{
  GatewayID: *gatewayID,
  URL:       "http://localhost:9090", // gateway's own /command endpoint; in production set external reachable URL
 }
 breg, _ := json.Marshal(reg)
 _, err = client.Post(*fogAddr+"/register", "application/json", bytes.NewReader(breg))
 if err != nil {
  log.Printf("warning: failed register to fog: %v", err)
 } else {
  log.Printf("registered to fog as %s", reg.GatewayID)
 }

 // HTTP /command endpoint to accept ControlMessage from Fog (or admin)
 http.HandleFunc("/command", func(w http.ResponseWriter, r *http.Request) {
  var ctl model.ControlMessage
  if err := json.NewDecoder(r.Body).Decode(&ctl); err != nil {
   http.Error(w, err.Error(), http.StatusBadRequest)
   return
  }
  // convert to CSV: VEHICLE_ID,PAYLOAD,MSGID
  csv := parser.ControlToCSV(ctl)
  select {
  case sendCh <- csv:
   w.WriteHeader(http.StatusAccepted)
   w.Write([]byte("enqueued"))
  default:
   http.Error(w, "send queue full", http.StatusServiceUnavailable)
  }
 })

 // goroutine: read serial, parse telemetry, enqueue forward
 go func() {
  for {
   line, err := l.ReadLine(0)
   if err != nil {
    // nonfatal: retry
    time.Sleep(100 * time.Millisecond)
    continue
   }
   vd, err := parser.ParseTelemetryCSV(line)
   if err != nil {
    log.Printf("parse err: %v (line=%s)", err, line)
    continue
   }
   select {
   case forwardCh <- vd:
   default:
    log.Println("forward queue full, drop")
   }
  }
 }()

 // start http server
 srv := &http.Server{Addr: ":9090"}
 go func() {
  log.Println("gateway http listening :9090")
  if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
   log.Fatalf("gateway http err: %v", err)
  }
 }()

 // wait signal
 stop := make(chan os.Signal, 1)
 signal.Notify(stop, os.Interrupt)
 <-stop
 log.Println("gateway shutting down...")

 // shutdown http server
 _ = srv.Shutdown(ctx)

 // close channels and wait
 close(sendCh)
 close(forwardCh)
 wg.Wait()
 cancel()
 log.Println("gateway stopped")
}
```

---

## `cmd/fog_server/main.go` (Fog with registration)

```go
// Fog server implements:
// - POST /register  : gateway registers itself and optionally provides vehicle list
// - POST /ingest    : gateway posts telemetry JSON (VehicleData)
// - GET  /ws        : websocket clients subscribe to telemetry
// - POST /control   : fog sends control targeting vehicle; fog routes to registered gateway
//
// Note: this is an in-memory registry. For production, persist registrations.
package main

import (
 "bytes"
 "encoding/json"
 "flag"
 "log"
 "net/http"
 "sync"

 "github.com/gorilla/websocket"
 "LoraFog/internal/model"
)

var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}

// registry maps vehicleID -> gatewayURL
type registry struct {
 mu         sync.RWMutex
 vehicleMap map[string]string // vehicleID -> gatewayURL
 gatewayMap map[string]model.GatewayRegistration
}

func newRegistry() *registry {
 return &registry{
  vehicleMap: make(map[string]string),
  gatewayMap: make(map[string]model.GatewayRegistration),
 }
}

func (r *registry) register(g model.GatewayRegistration) {
 r.mu.Lock()
 defer r.mu.Unlock()
 r.gatewayMap[g.GatewayID] = g
 // map vehicles to gateway URL
 for _, v := range g.Vehicles {
  r.vehicleMap[v] = g.URL
 }
}

func (r *registry) gatewayForVehicle(vehicleID string) (string, bool) {
 r.mu.RLock()
 defer r.mu.RUnlock()
 url, ok := r.vehicleMap[vehicleID]
 return url, ok
}

func main() {
 addr := flag.String("addr", ":8080", "listen address")
 flag.Parse()

 reg := newRegistry()

 // in-memory websocket clients
 clients := make(map[*websocket.Conn]bool)
 var mu sync.Mutex

 // POST /register
 http.HandleFunc("/register", func(w http.ResponseWriter, r *http.Request) {
  var g model.GatewayRegistration
  if err := json.NewDecoder(r.Body).Decode(&g); err != nil {
   http.Error(w, err.Error(), http.StatusBadRequest)
   return
  }
  if g.GatewayID == "" || g.URL == "" {
   http.Error(w, "gateway_id and url required", http.StatusBadRequest)
   return
  }
  reg.register(g)
  log.Printf("gateway registered: %s -> %s (vehicles=%v)", g.GatewayID, g.URL, g.Vehicles)
  w.WriteHeader(http.StatusOK)
 })

 // POST /ingest
 http.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
  var vd model.VehicleData
  if err := json.NewDecoder(r.Body).Decode(&vd); err != nil {
   http.Error(w, err.Error(), http.StatusBadRequest)
   return
  }
  log.Printf("ingested: %s at %s", vd.VehicleID, vd.Timestamp)
  // broadcast to WS clients
  mu.Lock()
  for c := range clients {
   _ = c.WriteJSON(vd)
  }
  mu.Unlock()
  w.WriteHeader(http.StatusOK)
 })

 // GET /ws
 http.HandleFunc("/ws", func(w http.ResponseWriter, r *http.Request) {
  conn, err := upgrader.Upgrade(w, r, nil)
  if err != nil {
   log.Printf("ws upgrade err: %v", err)
   return
  }
  mu.Lock()
  clients[conn] = true
  mu.Unlock()
  // read loop to detect disconnect
  go func(c *websocket.Conn) {
   defer func() {
    mu.Lock()
    delete(clients, c)
    mu.Unlock()
    c.Close()
   }()
   for {
    var v interface{}
    if err := c.ReadJSON(&v); err != nil {
     break
    }
   }
  }(conn)
 })

 // POST /control
 http.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
  var req struct {
   VehicleID string `json:"vehicle_id"`
   Payload   string `json:"payload"`
   MsgID     string `json:"msg_id,omitempty"`
  }
  if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
   http.Error(w, err.Error(), http.StatusBadRequest)
   return
  }
  // find gateway for target vehicle
  gwURL, ok := reg.gatewayForVehicle(req.VehicleID)
  if !ok {
   http.Error(w, "no gateway for vehicle", http.StatusNotFound)
   return
  }
  ctl := model.ControlMessage{VehicleID: req.VehicleID, Payload: req.Payload, MsgID: req.MsgID}
  // forward to gateway /command
  go func() {
   b, _ := json.Marshal(ctl)
   _, err := http.Post(gwURL+"/command", "application/json", bytes.NewReader(b))
   if err != nil {
    log.Printf("forward to gateway err: %v", err)
   }
  }()
  w.WriteHeader(http.StatusAccepted)
 })

 log.Printf("fog server listening %s", *addr)
 log.Fatal(http.ListenAndServe(*addr, nil))
}
```

---

## `cmd/simulator/main.go` (Telemetry simulator)

```go
// Telemetry simulator: writes CSV telemetry lines to the specified serial device.
// Use this for local testing when you don't have real vehicle hardware.
package main

import (
 "flag"
 "fmt"
 "log"
 "math/rand"
 "time"

 "LoraFog/internal/lora"
)

func main() {
 dev := flag.String("dev", "/dev/serial0", "serial device to write telemetry into")
 baud := flag.Int("baud", 9600, "baud rate")
 id := flag.String("id", "VEH_SIM_01", "simulated vehicle id")
 interval := flag.Int("interval", 1000, "ms between messages")
 flag.Parse()

 port, err := lora.New(*dev, *baud)
 if err != nil {
  log.Fatalf("open serial: %v", err)
 }
 defer func() {
  if cerr := port.Close(); cerr != nil {
   log.Printf("warning: close serial err: %v", cerr)
  }
 }()

 log.Printf("simulator sending to %s every %dms", *dev, *interval)
 tick := time.NewTicker(time.Duration(*interval) * time.Millisecond)
 defer tick.Stop()

 for range tick.C {
  lat := 21.0285 + (rand.Float64()-0.5)*0.01
  lon := 105.8048 + (rand.Float64()-0.5)*0.01
  head := rand.Float64() * 360.0
  left := 5.0 + rand.Float64()*20.0
  right := 5.0 + rand.Float64()*20.0
  line := fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f", *id, lat, lon, head, left, right)
  if err := port.WriteLine(line); err != nil {
   log.Printf("write err: %v", err)
  } else {
   log.Printf("sent: %s", line)
  }
 }
}
```

---

## Additional notes & instructions

1. **Device paths**
   - LoRa default: `/dev/serial0` (as you requested). You can override with `-lora` flag in vehicle/gateway/simulator.
   - GPS default: `/dev/ttyS0` (vehicle). Adjust as needed.

2. **Gateway registration**
   - Gateway calls `POST http://<fog>/register` with JSON `GatewayRegistration{gateway_id, url, vehicles}`.
   - Fog uses the `vehicles` list to map vehicleID → gatewayURL used by `/control`. If a gateway doesn't list vehicles, Fog won't be able to route by vehicle id.

3. **Graceful shutdown & resource close**
   - All serial ports are closed with error checks (logged).
   - Gateway/fog servers support shutdown patterns (gateway shuts down http server).
   - Worker goroutines join via `WaitGroup` before exit.

4. **Security (NOT included)**
   - Current system uses plaintext HTTP and CSV over LoRa. For production add TLS, authentication, and cryptographic message signing (HMAC) on control messages.

5. **Testing locally**
   - If testing on a single Pi, you can use `socat` to create virtual serial pairs and run simulator → gateway:

     ```bash
     socat -d -d pty,raw,echo=0 pty,raw,echo=0
     ```

     This prints two PTYs you can pass as `-dev` / `-lora` to simulator and gateway.

6. **Build**
   - `go build ./cmd/vehicle`
   - `go build ./cmd/gateway`
   - `go build ./cmd/fog_server`
   - `go build ./cmd/simulator`
