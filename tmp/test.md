# Yêu cầu

- Dựa vào thông tin code được cung cấp, viết lại hoàn chỉnh theo chuẩn oop có thể dễ dàng mở rộng và bảo trì,...
- Có đầy đủ comment rõ ràng bằng tiếng anh và chuẩn Golang
- Có thể sau này mở rộng có nhiều thiết bị hơn không chi có gps
- Sẽ hợp lý hơn nếu ta coi mỗi thành phần là 1 đối tượng: đối tượng thiết bị được kế thừa bởi gps, lora, ... đối tượng vehicle, đối tượng gateway
- Chuơng trình có khả năng chạy được với nhiều vehicle và gateway được cấu hình trong file config.yml
- Hiện tại chương trình ở tất cả các thành phần đêu đang lắng nghe thông tin đến dạng text, format csv, hay hỗ trợ các đối tượng cần thiết có thể lắng nghe hoặc csv hoắc json và có thể chọn loại bằng việc setup trong file config.yml

# Code

- internal/gps/read.go

```go
// Package gps provides utilities for GPS.
package gps

import (
 "LoraFog/internal/model"
 "LoraFog/internal/parser"
 "bufio"
 "fmt"
 "log"
 "strings"
 "time"

 serial "go.bug.st/serial"
)

// ReadStream continuously read GPS data from serial
func ReadStream(device string, baud int, out chan<- model.GPSData) error {
 port, err := serial.Open(device, &serial.Mode{BaudRate: baud})
 if err != nil {
  return fmt.Errorf("open serial failed: %w", err)
 }
 defer func() {
  if err := port.Close(); err != nil {
   log.Printf("warning: failed to close serial port: %v", err)
  }
 }()

 reader := bufio.NewReader(port)
 for {
  line, err := reader.ReadString('\n')
  if err != nil {
   continue
  }
  line = strings.TrimSpace(line)
  log.Printf("RAW GPS: %s", line)

  // Chỉ xử lý câu NMEA hợp lệ
  // if !strings.HasPrefix(line, "$GPGGA") && !strings.HasPrefix(line, "$GNRMC") {
  //  continue
  // }

  parts := strings.Split(line, ",")
  // if len(parts) < 6 {
  //  continue
  // }

  lat, err1 := parser.ParseNMEACoord(parts[2], parts[3])
  lon, err2 := parser.ParseNMEACoord(parts[4], parts[5])
  log.Printf("gps data: %.6f, %.6f", lat, lon)
  if err1 != nil || err2 != nil {
   continue
  }

  out <- model.GPSData{Lat: lat, Lon: lon}
 }
}

// ReadGPSFromDevice reads NMEA sentence from GPS serial (device param).
func ReadGPSFromDevice(device string, baud int, timeout time.Duration) (float64, float64, error) {
 // Use go.bug.st/serial directly for GPS reading
 port, err := serial.Open(device, &serial.Mode{BaudRate: baud})
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
    lat, err1 := parser.ParseNMEACoord(parts[2], parts[3])
    lon, err2 := parser.ParseNMEACoord(parts[4], parts[5])
    if err1 == nil && err2 == nil {
     return lat, lon, nil
    }
   }
  }
 }
 return 0, 0, fmt.Errorf("no gps fix")
}
```

- internal/lora/serial.go

```go
// Package lora provides a light wrapper over a serial port used for LoRa E32 modules.
// It reads/writes newline-delimited text lines.
package lora

import (
 "bufio"
 "errors"
 "time"

 serial "go.bug.st/serial"
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

- cmd/gateway/main.go

```go
// Gateway program:
// - Reads CSV from LoRa (/dev/serial0)
// - Parses CSV -> JSON and forwards to Fog /ingest (worker pool)
// - Exposes /command endpoint (JSON) to receive control from Fog; converts to CSV and sends via LoRa
// - Registers itself to Fog on startup (POST /register)
package main

import (
 "LoraFog/internal/lora"
 "LoraFog/internal/model"
 "LoraFog/internal/parser"
 "bytes"
 "context"
 "encoding/json"
 "flag"
 "fmt"
 "io"
 "log"
 "net/http"
 "os"
 "os/signal"
 "strings"
 "sync"
 "time"
)

func main() {
 serialDev := flag.String("lora", "/dev/serial0", "LoRa serial device")
 baud := flag.Int("baud", 9600, "serial baud")
 fogAddr := flag.String("fog", "http://192.168.2.245:3001", "Fog base URL")
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
 for i := range 4 {
  wg.Add(1)
  go func(id int) {
   defer wg.Done()
   for v := range forwardCh {
    // b, _ := json.Marshal(v)
    // resp, err := client.Post(*fogAddr+"/ingest", "application/json", bytes.NewReader(b))
    // // Convert struct -> CSV string
    csvLine := parser.VehicleToCSV(v)
    fmt.Printf("forward: %s\n", csvLine)
    resp, err := client.Post(*fogAddr+"/api/telemetry", "text/plain", strings.NewReader(csvLine))
    if err != nil {
     log.Printf("forward worker %d post err: %v", id, err)
     continue
    }
    _, _ = io.Copy(io.Discard, resp.Body)
    if cerr := resp.Body.Close(); cerr != nil {
     log.Printf("warning: forward worker %d close body: %v", id, cerr)
    }
   }
  }(i)
 }

 // send workers
 for i := range 2 {
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
  URL:       "http://127.0.0.1:10001", // gateway's own /command endpoint; in production set external reachable URL
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
  // var ctl model.ControlMessage
  // if err := json.NewDecoder(r.Body).Decode(&ctl); err != nil {
  //  http.Error(w, err.Error(), http.StatusBadRequest)
  //  return
  // }
  // convert to CSV: VEHICLE_ID,PAYLOAD,MSGID
  // csv := parser.ControlToCSV(ctl)
  body, err := io.ReadAll(r.Body)
  if err != nil {
   http.Error(w, "failed to read request body", http.StatusBadRequest)
   return
  }
  if cerr := r.Body.Close(); cerr != nil {
   log.Printf("warning: error closing request body: %v", cerr)
  }

  csv := strings.TrimSpace(string(body))
  if csv == "" {
   http.Error(w, "empty command", http.StatusBadRequest)
   return
  }
  log.Printf("Received command text: %s", csv)

  select {
  case sendCh <- csv:
   w.WriteHeader(http.StatusAccepted)
   if _, err := w.Write([]byte("enqueued")); err != nil {
    log.Printf("failed to write HTTP response: %v", err)
   }
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
 srv := &http.Server{Addr: ":10001"}
 go func() {
  log.Println("gateway http listening :10001")
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

- cmd/fog_server/main.go

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
 "LoraFog/internal/model"
 "LoraFog/internal/parser"
 "bytes"
 "encoding/json"
 "flag"
 "log"
 "net/http"
 "sync"

 "github.com/gorilla/websocket"
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
 addr := flag.String("addr", ":10000", "listen address")
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

  // Convert JSON -> CSV for unified wire format
  csvLine := parser.VehicleToCSV(vd)
  // log.Printf("ingested: %s at %s -> CSV: %s", vd.VehicleID, vd.Timestamp, csvLine)
  log.Printf("ingested: %s -> CSV: %s", vd.VehicleID, csvLine)
  // log.Printf("ingested: %s at %s", vd.VehicleID, vd.Timestamp)

  // broadcast to WS clients
  mu.Lock()
  for c := range clients {
   // _ = c.WriteJSON(vd)
   if err := c.WriteMessage(websocket.TextMessage, []byte(csvLine)); err != nil {
    log.Printf("ws write err: %v", err)
   }
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
    if err := c.Close(); err != nil {
     log.Printf("failed to close websocket: %v", err)
    }
   }()
   for {
    // var v any
    // if err := c.ReadJSON(&v); err != nil {
    //  break
    // }
    // Not expect messages from the client => detect close
    if _, _, err := c.ReadMessage(); err != nil {
     break
    }
   }
  }(conn)
 })

 // POST /control
 http.HandleFunc("/control", func(w http.ResponseWriter, r *http.Request) {
  var req model.ControlMessage
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
  ctl := model.ControlMessage{VehicleID: req.VehicleID, Mode: req.Mode, Spd: req.Spd, Lat: req.Lat, Lon: req.Lon, Kp: req.Kp, Ki: req.Ki, Kd: req.Kd}
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

- cmd/vehicle/main.go

```go
// Vehicle agent (realistic): reads GPS from serial, sends CSV telemetry via LoRa (/dev/serial0),
// listens for control CSV and replies with ACKs. Resources are properly closed and checked.
package main

import (
 "LoraFog/internal/gps"
 "LoraFog/internal/lora"
 "LoraFog/internal/model"
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
)

func main() {
 vehicleID := flag.String("id", "00001", "vehicle id")
 gpsDev := flag.String("gps", "/tmp/ttyV1", "gps serial device")
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

 // GPS Channel
 gpsCh := make(chan model.GPSData, 10)
 // Goroutine for reading GPS module
 go func() {
  if err := gps.ReadStream(*gpsDev, *gpsBaud, gpsCh); err != nil {
   log.Printf("gps stream error: %v", err)
  }
 }()

 for {
  select {
  case <-stop:
   log.Println("vehicle stopping")
   return
   // case <-ticker.C:
   //  lat, lon, err := gps.ReadGPSFromDevice(*gpsDev, *gpsBaud, 2*time.Second)
   //  if err != nil {
   //   log.Printf("gps read failed: %v; using fallback", err)
   //   // lat = 21.028511
   //   // lon = 105.804817
   //   lat = 21.0285 + (rand.Float64()-0.5)*0.01
   //   lon = 105.8048 + (rand.Float64()-0.5)*0.01
   //  }
  case data := <-gpsCh:
   headCur := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
   headTar := math.Mod(float64(time.Now().UnixNano()/1e6)/100.0, 360.0)
   // left/right speed from local sensors (not implemented) — placeholder
   left := 12.0
   right := 12.0
   pid := 1.0
   v := fmt.Sprintf("%s,%.6f,%.6f,%.2f,%.2f,%.2f,%.2f,%.1f", *vehicleID, data.Lat, data.Lon, headCur, headTar, left, right, pid)
   if err := l.WriteLine(v); err != nil {
    log.Printf("lora write err: %v", err)
   } else {
    log.Printf("sent telemetry: %s", v)
   }
  }
 }
}
```

- cmd/simulation/main.go

```go
// Telemetry simulator: writes CSV telemetry lines to the specified serial device.
// Use this for local testing when you don't have real vehicle hardware.
package main

import (
 "LoraFog/internal/parser"
 "flag"
 "fmt"
 "log"
 "math/rand"
 "os"
 "time"

 "go.bug.st/serial"
)

func main() {
 device := flag.String("device", "/tmp/ttyV0", "serial device to write telemetry into")
 baud := flag.Int("baud", 9600, "baud rate")
 flag.Parse()
 port, err := serial.Open(*device, &serial.Mode{BaudRate: *baud})
 if err != nil {
  fmt.Printf("failed to open serial port %s: %v\n", port, err)
  os.Exit(1)
 }
 defer func() {
  if err := port.Close(); err != nil {
   log.Printf("warning: failed to close serial port: %v", err)
  }
 }()

 fmt.Printf("GPS simulator started on %s (baud %d)\n", *device, *baud)

 for {
  // Giả lập vị trí Hà Nội (gần Hồ Gươm)
  lat := 21.0285 + (rand.Float64()-0.5)*0.001
  lon := 105.8048 + (rand.Float64()-0.5)*0.001
  latStr, latDir := parser.ToNMEACoord(lat, true)
  lonStr, lonDir := parser.ToNMEACoord(lon, false)
  timeUTC := time.Now().UTC().Format("150405.00")

  // Chuỗi NMEA $GPGGA đơn giản
  // nmea := fmt.Sprintf("$GPGGA,%.4f,N,%.4f,E,1,08,0.9,10.0,M,0.0,M,,*47\r\n",lat, lon)
  nmea := fmt.Sprintf("$GPGGA,%s,%s,%s,%s,%s,1,08,0.9,10.0,M,0.0,M,,*47\r\n",
   timeUTC, latStr, latDir, lonStr, lonDir)

  _, err = port.Write([]byte(nmea))
  if err != nil {
   fmt.Printf("write error: %v\n", err)
  } else {
   fmt.Printf("sent: %s", nmea)
  }

  time.Sleep(2 * time.Second)
 }
}
```

- internal/model/message.go

```go
// Package model defines shared message structures for LoraFog.
package model

// VehicleData represents telemetry parsed from CSV and forwarded as JSON.
type VehicleData struct {
 VehicleID string  `json:"vehicle_id"`
 Lat       float64 `json:"lat"`
 Lon       float64 `json:"lon"`
 HeadCur   float64 `json:"head_current"`
 HeadTar   float64 `json:"head_target"`
 LeftSpd   float64 `json:"left_speed"`
 RightSpd  float64 `json:"right_speed"`
 PID       float64 `json:"pid"`
}

// ControlMessage is used by Fog -> Gateway (JSON). Gateway converts to CSV for LoRa.
type ControlMessage struct {
 VehicleID string  `json:"vehicle_id"`
 Mode      float64 `json:"mode"`
 Spd       float64 `json:"speed"`
 Lat       float64 `json:"lat"`
 Lon       float64 `json:"lon"`
 Kp        float64 `json:"kp"`
 Ki        float64 `json:"ki"`
 Kd        float64 `json:"kd"`
}

// GPSData is used by GPS
type GPSData struct {
 Lat float64 `json:"lat"`
 Lon float64 `json:"lon"`
}

// AckMessage is a simple ack structure.
type AckMessage struct {
 MsgID string `json:"msg_id"`
 Ack   bool   `json:"ack"`
}

// GatewayRegistration is used when a gateway registers to Fog.
type GatewayRegistration struct {
 GatewayID string   `json:"gateway_id"`
 URL       string   `json:"url"`      // public URL where Fog can reach gateway (HTTP)
 Vehicles  []string `json:"vehicles"` // optional list of vehicle IDs this gateway serves
}
```

- internal/parser/vehicle.go

```go
// Package parser converts CSV wire format to structured types and vice-versa.
//
// CSV telemetry wire format (vehicle -> gateway):
//
// VEHICLE_ID,LAT,LON,HEAD,LEFT_SPEED,RIGHT_SPEED
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
```

- internal/parser/gps.go

```go
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
```

- internal/parser/control.go

```go
// Package parser converts CSV wire format to structured types and vice-versa.
//
// CSV control wire format (fog -> vehicle):
//
// VEHICLE_ID,PAYLOAD,MSGID
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
 //  msgID = strconv.FormatInt(time.Now().UnixNano(), 10)
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
```
