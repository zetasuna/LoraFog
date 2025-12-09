---
title: Mô hình hệ thống
---

## CƠ CHẾ TDMA & TRUYỀN NHẬN
``## CƠ CHẾ TDMA & TRUYỀN NHẬN

- vehicle sẽ có 3 trạng thái chờ (trống thông tin slot, gateway_id quản lý,...), chuẩn bị gửi và gửi (slot gửi, gateway_id quản lý,...), ở trạng thái chờ sẽ lắng nghe beacon từ gateway nếu nhận được beacon (chứa slot gửi, gateway_id gửi,...) sẽ gửi gói tin hello và chuyển sang trạng thái chuẩn bị gửi (chờ gói tin beacon tiếp theo để update thông tin slot, gateway_id quản lý ,...), sau khi nhận gói tin beacon tiếp theo sẽ chuyển sang trạng thái gửi, gửi telemetry theo beacon mà gateway gửi (update status slot, gateway_id quản lý,...), sau 1 khoảng thời gian không nhận được beacon sẽ về trạng thái chờ để lắng nghe beacon. Nếu đang còn thông tin cũ mà nhận được beacon từ gateway khác với gateway_id đang có thì cập nhật lại thông tin và gửi gói tin hello 

- khi nhận được beacon, vehicle sẽ gửi lại hello chứa vehicleID cho gateway để gửi register cho server, server tạo sesion vehicle và gateway có ttl, tính toán slot cho tdma truyền nhận và post thông tin update beacon cho gateway để gateway cập nhật beacon broadcast, khi nhận được telemetry sẽ reset ttl này, sau 1 khoảng ttl mà không nhận được tin telemetry từ vehicle thì xóa session đó và gatewayID quản lý vehicle trong database (đặt giá trị rỗng hoặc null). Nếu 1 vehicle đã có 1 session (được quản lý bởi 1 gateway) mà lại có 1 gói tin đăng ký mới của vehicle đó đến server thì xóa thông tin session cũ, cập nhật lại thông tin cho gateway cũ, tạo session mới, update db và post thông tin cho gateway mới của vehicle 

```
| SYNC | SLOT1 | GUARD | SLOT2 | GUARD | ... | REGISTER_WINDOW |
```

- **SYNC:** Gateway phát gói đồng bộ thời gian.
- **SLOTn:** mỗi thuyền gửi telemetry đúng slot của mình.
- **GUARD:** 200–300 ms để bù lệch thời gian LoRa.
- **REGISTER_WINDOW:** cuối chu kỳ dành cho thuyền mới đăng ký.


## 🎯 Mục tiêu của mô hình

> Một nhóm xe tự hành (autonomous vehicles) **trao đổi dữ liệu với nhau
> bằng LoRa (V2V)**, đồng thời vẫn kết nối được với **Fog server** gần
> (ở cạnh đường hoặc trên cloud cục bộ) để chia sẻ, xử lý,
> và lưu trữ dữ liệu — tức là một mô hình **Fog Computing thực thụ**.

---

## 🧠 Tổng quan 3 lớp trong mô hình

```
╔══════════════════════════════════════════════════════╗
║                      CLOUD LAYER                     ║
║  - Phân tích dữ liệu lớn, huấn luyện AI, OTA update  ║
║  - Lưu trữ log dài hạn, thống kê toàn hệ thống       ║
╚══════════════════════════════════════════════════════╝
               ↑             ↑
               │             │ (WAN/Internet, 4G/5G)
               │
╔══════════════════════════════════════════════════════╗
║                      FOG LAYER                       ║
║  - Fog / Edge Server (RSU hoặc mini data center)     ║
║  - Thu thập dữ liệu từ nhiều xe                      ║
║  - Xử lý, cảnh báo cục bộ, phân phối bản đồ nhỏ      ║
║  - Có thể là 1 Gateway LoRa đa kênh                  ║
╚══════════════════════════════════════════════════════╝
               ↑
               │ (LoRa)
               │
╔══════════════════════════════════════════════════════╗
║                   VEHICLE LAYER                      ║
║  - Mỗi xe có:                                        ║
║     + Module LoRa SX1278 (giao tiếp V2V)             ║
║     + Bộ xử lý (Raspberry Pi / Jetson / NUC)         ║
║     + GNSS, cảm biến tốc độ, IMU, camera,…           ║
║     + Agent phần mềm: gửi/nhận gói LoRa, mã hóa dữ liệu║
║  - Một xe có thể đóng vai trò "local gateway"        ║
╚══════════════════════════════════════════════════════╝
```

---

## 🏗️ Giải thích từng thành phần

### 1️⃣ **Các xe (Vehicle Node)**

Mỗi xe là một **nút (node)** trong mạng LoRa.

- **Chức năng chính:**
  - Gửi beacon vị trí, vận tốc, hướng.
  - Nhận cảnh báo từ xe khác.
  - Nếu có thể, gửi dữ liệu cảm biến lên Fog Server
    (qua Wi-Fi/4G hoặc qua LoRa Gateway gần nhất).

- **Thành phần:**
  - MCU hoặc SBC (Raspberry Pi, Jetson Nano…)
  - Module LoRa (SX1278, E32-915T30D,…)
  - GNSS, cảm biến IMU, camera
  - Bộ xử lý phần mềm (Python, Go, C++) để:
    - Thu thập dữ liệu
    - Mã hóa + ký gói tin
    - Gửi qua LoRa
    - Nhận và giải mã gói tin

---

### 2️⃣ **Gateway (Fog Gateway / RSU)**

Fog Gateway (hoặc còn gọi là RSU – _Road Side Unit_)
là **thành phần trung gian giữa các xe và hệ thống Fog/Cloud**.

- **Có thể là:**
  - Một thiết bị cố định dọc đường (RSU)
  - Hoặc một xe được chọn làm **“leader vehicle”**,
    đóng vai trò gateway tạm thời cho nhóm xe.

- **Nhiệm vụ chính:**
  - Thu dữ liệu LoRa từ nhiều xe.
  - Gom, xử lý sơ bộ (filter, merge).
  - Gửi dữ liệu tổng hợp lên **Fog Server / Cloud** qua 4G, Ethernet hoặc Wi-Fi.
  - Có thể truyền lại các cảnh báo khu vực về cho xe trong vùng.

- **Phần cứng đề xuất:**
  - Máy nhúng có công suất mạnh hơn SBC
  - Module LoRa multi-channel (SX1302/SX1308)
  - Kết nối uplink (Ethernet, LTE, hoặc Wi-Fi)

---

### 3️⃣ **Fog Server (Edge Server)**

Đây là **máy chủ xử lý cục bộ gần vùng giao thông**.

- **Đặt ở:**
  - Trung tâm điều phối trong khu công nghiệp, bãi thử xe, khuôn viên…
  - Hoặc ngay tại trạm RSU (mini PC/mini data center)

- **Chức năng:**
  - Nhận dữ liệu từ gateway.
  - Chạy mô hình phân tích (AI/ML inference)
    để nhận diện nguy cơ, phân tích luồng giao thông.
  - Ra lệnh cảnh báo hoặc điều phối (gửi lại cho xe).
  - Lưu trữ tạm thời (1–3 ngày) trước khi đồng bộ lên Cloud.

- **Cấu hình ví dụ:**
  - CPU: 4–8 core, RAM 8–16 GB.
  - Kết nối mạng ổn định.
  - Chạy các container xử lý (Docker Compose / Kubernetes local).
  - Database: InfluxDB / MongoDB / PostgreSQL.

---

### 4️⃣ **Cloud Server (Tùy chọn)**

Không bắt buộc trong thử nghiệm nhỏ,
nhưng trong mô hình chuẩn thực tế **Fog–Cloud**,
nó là tầng cuối.

- **Nhiệm vụ:**
  - Lưu trữ dài hạn.
  - Phân tích dữ liệu lớn (training AI, route optimization).
  - Cung cấp OTA update cho xe và gateway.
  - Giám sát toàn hệ thống (monitor, dashboard Grafana/ELK).

---

## ⚙️ Mối quan hệ giữa các thành phần

| Liên kết       | Giao thức       | Mục đích                  | Tần suất                  |
| -------------- | --------------- | ------------------------- | ------------------------- |
| Xe ↔ Xe       | LoRa P2P        | Trao đổi beacon, cảnh báo | 1 Hz hoặc event           |
| Xe ↔ Gateway  | LoRa hoặc Wi-Fi | Gửi dữ liệu cảm biến      | 0.2–1 Hz                  |
| Gateway ↔ Fog | MQTT/HTTP/GRPC  | Upload dữ liệu tổng hợp   | 1–10 s                    |
| Fog ↔ Cloud   | HTTPS/MQTT      | Gửi dữ liệu, nhận update  | vài phút hoặc khi có mạng |

---

## 📡 Mô hình vật lý tổng thể (sơ đồ)

```
     ┌───────────────────────────────────────────────┐
     │                  CLOUD SERVER                 │
     │        (AI, Database, OTA, Analytics)         │
     └───────────────────────────────────────────────┘
                             │
                             │  (Internet / 4G/5G)
                             │
     ┌───────────────────────────────────────────────┐
     │                FOG SERVER / RSU               │
     │ (Edge processing, local decision, MQTT broker)│
     └───────────────────────┬───────────────────────┘
                             │  (LoRaWAN uplink/downlink)
                             │
                    ┌────────┴────────┐
                    │                 │
             ┌─────────────┐   ┌─────────────┐
             │  VEHICLE 1  │   │  VEHICLE 2  │
             │ (LoRa Node) │   │ (LoRa Node) │
             └─────────────┘   └─────────────┘
                    ↕                 ↕
                LoRa P2P  ←──────→  LoRa P2P
```
