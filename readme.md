---
Đồ án tốt nghiệp: Triển khai LoraWan cho xe tự hành
---

# LoRaWAN Fog for Autonomous Vehicles — Deployment Document (Version 1)

- **Mục tiêu:**
  Tài liệu này mô tả một phương án triển khai mô hình **fog computing** cho mạng LoRaWAN
  dùng trong hệ thống truyền dữ liệu cho xe tự hành (đề tài tốt nghiệp đơn giản).
- Ứng dụng chính viết bằng **Go (Golang)**.

---

## 1. Tổng quan kiến trúc

Kiến trúc gồm các thành phần chính:

1. **Simulator (Sensor)**
   Mô phỏng các cảm biến trên xe (vị trí GPS, tốc độ, trạng thái cảm biến)
   Gửi payload theo định kỳ qua UDP tới Gateway địa phương.
2. **Gateway (Fog Gateway)**
   Nhận UDP từ sensor
   Chuyển tiếp (forward) dữ liệu tới Network Server
   Cũng có thể thực hiện một số xử lý biên (edge processing): lọc, nén, tiền xử lý để giảm băng thông.
3. **Network Server (Fog / Regional)**
   Nhận uplink từ Gateway (HTTP POST)
   Giải mã LoRaWAN (nếu có)
   Xác thực DevAddr / AppKey
   Định tuyến tới Application Server
   Có thể chạy tại rìa (fog) hoặc cloud tuỳ tầm
4. **Application Server**
   Lưu trữ dữ liệu cảm biến (SQLite / Postgres)
   Cung cấp API REST/WebSocket cho dashboard/clients
5. **Dashboard / Web UI**
   Hiển thị realtime dữ liệu
   Kết nối qua WebSocket tới Application Server
6. **Optional: Message Broker (MQTT)** (Nếu muốn tách khâu realtime và lưu trữ)

Mô hình fog cho xe tự hành có thể đặt Gateway + Network Server ở trạm gác/edge node gần khu vực hoạt động (ví dụ bến cảng, trạm dừng)
Còn Application Server có thể là cloud hoặc vùng local cluster.

---

## 2. Cấu trúc thư mục tiêu chuẩn Golang (gợi ý)

```
lorawan-fog/                 # root repo
├── cmd/                     # programs entrypoints
│   ├── sensor/              # sensor simulator binary (cmd/sensor/main.go)
│   ├── gateway/             # gateway binary (cmd/gateway/main.go)
│   └── appserver/           # application server binary (cmd/appserver/main.go)
├── internal/                # private application code (non-public)
│   ├── ns/                  # network-server logic
│   ├── gw/                  # gateway helpers
│   ├── sensor/              # sensor simulator logic
│   ├── db/                  # database layer (migrations, dao)
│   └── ws/                  # websocket hub
├── pkg/                     # public packages (if muốn tái sử dụng)
│   └── lorawan/             # utils: lora encoding/decoding
├── api/                     # openapi / swagger spec (yaml/json)
├── web/                     # frontend (if nhỏ, tĩnh) or reference assets
├── configs/                 # cấu hình môi trường (yaml/env.example)
├── deployments/             # docker, k8s manifests, terraform (sau này)
│   ├── docker/              # Dockerfile templates và compose.yaml
│   └── k8s/                 # k8s manifests (deployment, svc)
├── scripts/                 # build / helper scripts (build.sh, run_local.sh)
├── migrations/              # database migration files (if dùng postgres)
├── Makefile                 # helper commands
├── go.mod
└── README.md
```

**Lý do phân chia:**

- `cmd/` cho mỗi binary riêng dễ build, release.
- `internal/` chứa logic không export ra ngoài (bảo vệ boundary).
- `pkg/` cho code có thể tái sử dụng.
- `deployments/` để gom manifest và Dockerfile.

---

## 3. Ví dụ Dockerfile (cho binary Go)

`deployments/docker/Dockerfile.app`

```dockerfile
# Build stage
FROM golang:1.21-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/app ./cmd/appserver

# Final minimal image
FROM scratch
COPY --from=builder /out/app /app
# (Nếu cần CA certs, chuyển sang alpine:3.x hoặc add ca-certificates)
EXPOSE 8080
ENTRYPOINT ["/app"]
```

Gợi ý: cho môi trường phát triển, image dùng `golang:1.21-alpine` để debug; production có thể multi-stage với `scratch` hoặc `distroless`.

---

## 4. Ví dụ `compose.yaml` (local dev)

`deployments/docker/compose.yaml`

```yaml
version: "3.8"
services:
  sensor:
    build:
      context: ../../
      dockerfile: deployments/docker/Dockerfile.app
    command: ["/app", "--mode=sensor"]
    networks:
      - lorawan-net
    depends_on:
      - gateway

  gateway:
    build:
      context: ../../
      dockerfile: deployments/docker/Dockerfile.app
    command: ["/app", "--mode=gateway"]
    ports:
      - "1680:1680/udp" # nếu gateway lắng nghe UDP
    networks:
      - lorawan-net
    environment:
      - NS_ENDPOINT=http://network-server:10000/uplink

  network-server:
    build:
      context: ../../
      dockerfile: deployments/docker/Dockerfile.app
    command: ["/app", "--mode=ns"]
    ports:
      - "10000:10000"
    networks:
      - lorawan-net
    environment:
      - APP_SERVER=http://appserver:9999/sensor

  appserver:
    build:
      context: ../../
      dockerfile: deployments/docker/Dockerfile.app
    command: ["/app", "--mode=app"]
    ports:
      - "9999:9999"
    volumes:
      - ./data:/data
    networks:
      - lorawan-net

networks:
  lorawan-net:
    driver: bridge
```

> **Ghi chú:**
> Trong repo demo trước đây (nếu bạn đã có `sensor.go`, `gateway.go`, `network-server.go`, `app.go`)
> thì `--mode=` ở command có thể chọn run logic tương ứng (như một binary đa năng).
> Tuy nhiên với sản phẩm thực tế nên tách binary rõ ràng.

---

## 5. Biến môi trường (env) quan trọng

- `NS_ENDPOINT` — URL network server.
- `APP_SERVER` — URL application server.
- `DB_DSN` — connection string cho DB (sqlite file path or postgres DSN).
- `LOG_LEVEL` — debug/info/warn.
- `LORA_APP_KEY`, `LORA_NWK_KEY` — (nếu cần giải mã LoRaWAN payload).

Tạo file mẫu `configs/.env.example` và hướng dẫn `cp configs/.env.example .env`.

---

## 6. Database: SQLite vs Postgres

- **SQLite**: thuận tiện cho demo/simple thesis, không cần server, file local. Dùng khi scale thấp.
- **Postgres**: dùng khi cần concurrency, nhiều node, production. Bạn cần migrations (Flyway, golang-migrate).

Trong Version 1, khuyến nghị bắt đầu với **SQLite** để đơn giản.

---

## 7. Thông số mạng & cổng

- Sensor -> Gateway: UDP port (ví dụ 1680)
- Gateway -> Network Server: HTTP POST (ví dụ 10000)
- Network Server -> App Server: HTTP POST (ví dụ 9999)
- App Server -> Web UI: HTTP/WS (8080 hoặc 3000)

---

## 8. CI/CD (gợi ý sơ bộ)

- **CI**: GitHub Actions / GitLab CI
  - Steps: `go test ./...` → `golangci-lint` → `go vet` → build artifacts → build docker images in ephemeral runner

- **CD**: push images to registry (DockerHub / GitLab Container Registry) -> deploy via docker-compose on target host hoặc Kubernetes manifests.

Ví dụ step build & push:

```yaml
# .github/workflows/ci.yml (tóm tắt)
name: CI
on: [push]
jobs:
  build:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v3
      - name: Setup Go
        uses: actions/setup-go@v4
        with: { go-version: "1.21" }
      - run: go test ./...
      - run: golangci-lint run || true
      - run: docker build -t ghcr.io/yourorg/lorawan-fog:latest .
      - run: echo ${{ secrets.GHCR_TOKEN }} | docker login ghcr.io -u USER --password-stdin
      - run: docker push ghcr.io/yourorg/lorawan-fog:latest
```

---

## 9. Local development workflow

1. `cp configs/.env.example .env` và chỉnh thông số.
2. `make build` — build tất cả binary vào `./bin/`.
3. `docker-compose -f deployments/docker/compose.yaml up --build`
4. Kiểm tra logs: `docker-compose logs -f appserver`
5. Dùng `curl` hoặc trình duyệt đến `http://localhost:9999` và websocket `ws://localhost:9999/ws`.

```makefile
.PHONY: build clean up run
build:
 go build -o bin/sensor ./cmd/sensor
 go build -o bin/gateway ./cmd/gateway
 go build -o bin/appserver ./cmd/appserver

run-app:
 ./bin/appserver --config=configs/dev.yaml

clean:
 rm -rf bin/
```

---

## 10. Logging, Observability, và Debug

- Logging: dùng `zap` hoặc `logrus` (structured logs).
- Tracing: tích hợp OpenTelemetry (OTel) nếu muốn trace request xuyên component.
- Metrics: expose `/metrics` (Prometheus) từ mỗi service.
- Health checks: `/healthz` và `/readyz` endpoints.

---

## 11. Security / Key management

- **Không** commit keys (AppKey, NwkKey) vào git.
- Dùng `env` hoặc Vault/Secret Manager.
- HTTPS / TLS cho endpoints quan trọng.
- Access control cho API (token-based, JWT).

Nếu cần giải mã LoRaWAN, giữ `AppSKey`/`NwkSKey` an toàn.

---

## 12. Test plan (kiểm thử cơ bản)

1. **Unit tests**: mỗi package có test tương ứng `*_test.go`.
2. **Integration tests**: chạy `docker-compose` local với một sensor simulator gửi vài message và xác nhận app server nhận và lưu trữ.
3. **End-to-end**: sensor -> gateway -> ns -> app -> websocket client hiển thị realtime.

Ví dụ integration script: `scripts/integration_test.sh` sẽ:

- lên network (docker-compose)
- gửi 10 UDP packages tới gateway
- query appserver DB để kiểm tra records

---

## 13. Mapping tới codebase hiện có (nếu bạn đã có các file trước đó)

Nếu repository hiện tại đã có các file bạn từng cung cấp (ví dụ `sensor.go`, `gateway.go`, `network-server.go`, `app.go`):

- Di chuyển logic vào `internal/sensor`, `internal/gw`, `internal/ns`, `internal/app` tương ứng.
- Tạo `cmd/sensor/main.go` dùng package `internal/sensor`.
- Tách cấu hình (flag/env) ra `configs/`.

---

## 14. Kịch bản triển khai (sơ bộ)

**Môi trường demo (một host):** dùng Docker Compose.

**Môi trường prototype (fog + cloud):**

- Fog node: chạy `gateway` + `network-server` tại cạnh (VM/edge box). Dùng docker-compose hoặc systemd + containerd.
- Cloud: chạy `appserver` + DB + dashboard (k8s hoặc VM).

**Môi trường production nhỏ:**

- K8s cluster cho appserver, network-server. Gateways là lightweight container hoặc dịch vụ chạy trên các edge devices.

---

## 15. Checklist Version 1 (những việc cần làm để hoàn thiện V1)

- [ ] Thiết lập repo với cấu trúc ở mục 2.
- [ ] Chuyển các file hiện có (sensor.go, gateway.go, network-server.go, app.go) vào `internal/` & tạo `cmd/` entrypoints.
- [ ] Tạo `configs/.env.example` và `configs/dev.yaml`.
- [ ] Viết Dockerfile cho từng service (hoặc multi-mode binary) và `docker-compose.yml`.
- [ ] Implement basic REST endpoints và WebSocket hub.
- [ ] Lưu thử nghiệm dữ liệu vào SQLite.
- [ ] Viết script integration test gửi UDP và kiểm tra data flow.
- [ ] Viết README hướng dẫn chạy local.

---

## 16. Next steps / Phiên bản sau

- Version 2: chuyển SQLite -> Postgres, thêm migrations, deploy-in-k8s manifests.
- Thêm TLS/HTTPS và auth cho API.
- Tích hợp OTel + Prometheus + Grafana.
- Thử nghiệm thực tế với gateway hardware (nếu có) và LoRaWAN gateway stack.

---

## 17. Tài liệu tham khảo & Links (gợi ý)

- LoRaWAN specs (Truy cập khi cần)
- Go project layout: \[standard project layout]
- Docker docs / Docker Compose

---

## 18. Phụ lục: Ví dụ API endpoints (gợi ý)

- `POST /sensor` — nhận payload từ Network Server (JSON)
- `GET /sensors` — list recent sensor data
- `GET /sensors/{id}` — chi tiết
- `GET /ws` — websocket endpoint (realtime)
- `GET /metrics` — prometheus metrics
