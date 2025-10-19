---
Đồ án tốt nghiệp: Triển khai LoraWan cho xe tự hành
---

# 📡 LoraFog – Modular LoRaWAN Fog Computing System (Golang OOP)

> A modular, object-oriented Golang system
> for distributed telemetry collection and control over LoRa networks.
> The system supports vehicles, gateways, and fog servers
> (all orchestrated from a single configuration file).

---

## 📑 Table of Contents

1. [Overview](#overview)
2. [Architecture](#architecture)
3. [Features](#features)
4. [Project Structure](#project-structure)
5. [Configuration](#configuration)
6. [Component Details](#component-details)
   - [System](#system)
   - [Fog Server](#fog-server)
   - [Gateway](#gateway)
   - [Vehicle](#vehicle)

7. [Parser Abstraction](#parser-abstraction)
8. [Device Abstraction](#device-abstraction)
9. [Build & Run](#build--run)
10. [Development Guidelines](#development-guidelines)
11. [Extensibility](#extensibility)

---

## Overview

**LoraFog** is a modular Golang framework
designed for **fog computing over LoRa networks**,
typically used in remote telemetry and control systems
such as:

- Vessel monitoring (offshore boats)
- Smart agriculture
- Distributed IoT data collection

The system integrates three main actors:

1. **Vehicles** — collect GPS and telemetry data.
2. **Gateways** — receive telemetry from vehicles via LoRa,
   convert formats, and forward to the fog.
3. **Fog Server** — central node that aggregates telemetry,
   serves websocket clients, and dispatches control messages.

Everything is configured and launched **from a single entry point (`main.go`)**
using a YAML configuration file.

---

## Architecture

```txt
           +------------------------+
           |      Fog Server        |
           |------------------------|
           |  /ingest (telemetry)   |
           |  /control (commands)   |
           |  /ws (websocket)       |
           +-----------^------------+
                       |
                JSON / CSV over HTTP
                       |
           +-----------v------------+
           |        Gateway         |
           |------------------------|
           | Receives from Vehicle  |
           | via LoRa (CSV/JSON)    |
           | Converts and sends to  |
           | Fog (configurable)     |
           +-----------^------------+
                       |
                  LoRa Serial Link
                       |
           +-----------v------------+
           |        Vehicle         |
           |------------------------|
           | GPS + Sensors          |
           | Send telemetry (CSV)   |
           | Receive controls       |
           +------------------------+
```

---

## Features

✅ **Single process orchestration** (Fog, Gateways, Vehicles from one config)
✅ **Object-Oriented design** for extensibility
✅ **Dynamic format conversion** (CSV ↔ JSON) per gateway
✅ **Unified lifecycle management (Start/Stop)**
✅ **Serial communication abstraction** (`Device` interface)
✅ **Custom parser system** for future protocols
✅ **WebSocket broadcasting** from FogServer
✅ **Graceful shutdown** and error-safe I/O
✅ **Full configuration via YAML**

---

## Project Structure

```txt
LoraFog/
├── cmd/
│   └── lora_fog/
│       └── main.go              # Single entry point
├── configs/
│   └── config.yml               # System configuration
├── internal/
│   ├── core/                    # Runtime components
│   │   ├── system.go
│   │   ├── gateway.go
│   │   ├── vehicle.go
│   │   └── fog_server.go
│   ├── device/                  # Device abstraction (LoRa, Serial)
│   │   ├── device.go
│   │   └── serial_device.go
│   ├── gps/                     # GPS reader (NMEA)
│   │   └── gps.go
│   ├── parser/                  # Format parser implementations
│   │   ├── parser.go
│   │   ├── csv_parser.go
│   │   ├── json_parser.go
│   │   └── nmea.go
│   ├── model/                   # Shared data models
│   │   ├── config.go
│   │   └── message.go
│   └── util/                    # Utilities
│       └── logger.go
└── go.mod
```

---

## Configuration

```yaml
# config.yml
global:
  wire_format: "csv" # default format (csv/json)
  fog_addr: ":10000" # fog server listen address

gateways:
  - id: "GW01"
    lora_device: "/dev/ttyUSB0"
    lora_baud: 9600
    wire_in: "csv" # format received from vehicle
    wire_out: "json" # format sent to fog
    fog_url: "http://127.0.0.1:10000"
    vehicles: ["V01"]

vehicles:
  - id: "V01"
    lora_device: "/dev/ttyS1"
    lora_baud: 9600
    gps_device: "/tmp/ttyGPS0"
    gps_baud: 9600
    telemetry_interval_ms: 2000
    wire_format: "csv"
```

---

## Component Details

### System

- Loads and validates configuration.
- Initializes all parsers and components.
- Starts FogServer, then all Gateways and Vehicles.
- Handles graceful shutdown (SIGINT / SIGTERM).

### Fog Server

- HTTP server that exposes:
  - `/register`: register gateways
  - `/ingest`: receive telemetry
  - `/control`: send control messages
  - `/ws`: broadcast telemetry to WebSocket clients

- In-memory registry maps `vehicleID → gatewayURL`.

### Gateway

- Communicates with multiple vehicles via LoRa.
- Converts data format using:
  - `wire_in`: for incoming data (e.g. CSV)
  - `wire_out`: for outgoing data (e.g. JSON)

- Forwards telemetry to Fog and handles `/command` HTTP endpoint.

### Vehicle

- Reads GPS data from serial (NMEA).
- Generates telemetry at fixed intervals.
- Sends data to gateway via LoRa.
- Listens for control messages (CSV or JSON).

---

## Parser Abstraction

| Interface    | Description                                                 |
| ------------ | ----------------------------------------------------------- |
| `Parser`     | Abstracts encoding/decoding for telemetry and control data. |
| `CSVParser`  | Implements CSV-based encoding/decoding.                     |
| `JSONParser` | Implements JSON-based encoding/decoding.                    |

All parsers implement:

```go
EncodeTelemetry(v model.VehicleData) (string, error)
DecodeTelemetry(s string) (model.VehicleData, error)
EncodeControl(c model.ControlMessage) (string, error)
DecodeControl(s string) (model.ControlMessage, error)
```

> 💡 New formats (e.g., protobuf, CBOR) can be added simply
> by creating a new struct implementing `Parser`.

---

## Device Abstraction

| Interface      | Description                                        |
| -------------- | -------------------------------------------------- |
| `Device`       | Abstracts communication medium (LoRa, Serial).     |
| `SerialDevice` | Uses `go.bug.st/serial` to perform I/O operations. |

Interface:

```go
type Device interface {
    ReadLine(timeout time.Duration) (string, error)
    WriteLine(s string) error
    Close() error
}
```

---

## Build & Run

### Install dependencies

```bash
go mod tidy
```

### Run the system

```bash
go run ./cmd/lora_fog
```

### Observe logs

You will see:

```txt
[Logger] Initialized
[Fog] Listening on :10000
[Gateway GW01] Started
[Vehicle V01] Sending telemetry...
```

---

## Development Guidelines

### Code Style

- Follow Go standard formatting (`go fmt`).
- Comments must follow GoDoc conventions.
- Return early on errors (`if err != nil { return err }`).
- Always check return values from `Close()` and `io.Copy()`.

### Linting

Use `golangci-lint`:

```bash
golangci-lint run ./...
```

Recommended `.golangci.yml`:

```yaml
linters:
  enable:
    - errcheck
    - govet
    - staticcheck
    - revive
    - gosimple
    - unused
run:
  timeout: 3m
```

---

## Extensibility

The system is designed to be extended easily:

- Add new **parser formats** (e.g. `ProtobufParser`).
- Implement **new device types** (e.g. `BLEDevice`, `MQTTDevice`).
- Extend `Vehicle` for autonomous control logic.
- Use persistent database in `FogServer` (SQLite, PostgreSQL, etc).

Example: to add a new parser:

```go
type ProtobufParser struct {}
func (p *ProtobufParser) EncodeTelemetry(v model.VehicleData) (string, error) { ... }
```

Then register it in `System.initParsers()`.

---

## Author

**Nguyễn Đức Nam**
Researcher / Developer – IoT, Edge & Fog Systems
🚀 Built with Golang, passion, and minimalism.

---

### Example Screenshot
