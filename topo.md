---
# 🚀 **LORA FLEET MANAGEMENT SYSTEM — FULL DESIGN SPEC (Final Revision)**
---

## 1️⃣ MỤC TIÊU HỆ THỐNG

Xây dựng một **hệ thống quản lý đội thuyền qua LoRa**, gồm:

- **Vehicle (thuyền):** gửi telemetry định kỳ, nhận lệnh control.
- **Gateway:** điều phối TDMA, xác thực thuyền, đồng bộ với Fog.
- **Fog/Server:** quản lý vùng, thiết bị, registry, và cấu hình.

Tất cả chạy trên **SX1278 đơn tần (single-channel)**.
Mục tiêu:

- Tránh xung đột, giảm ToA.
- Tự động đăng ký và đồng bộ thời gian.
- Bảo mật nhẹ, đủ an toàn cho IoT.
- Mở rộng vùng dễ dàng, vận hành ổn định lâu dài.

---

## 2️⃣ KIẾN TRÚC HỆ THỐNG

```
┌──────────────────────────────┐
│          FOG SERVER          │
│  - Registry & key manager     │
│  - Region management          │
│  - Web/API giám sát & điều khiển │
└────────────┬─────────────────┘
             │
         (HTTP / MQTT)
             │
┌──────────────────────────────┐
│           GATEWAY            │
│  - SX1278 LoRa radio         │
│  - TDMA master (SYNC, slot)  │
│  - Local Auto-Tuning         │
│  - Xác thực vehicle          │
│  - Gửi báo cáo & nhận config │
└────────────┬─────────────────┘
             │
        (LoRa link)
             │
┌──────────────────────────────┐
│         VEHICLE NODE         │
│  - Auto-register             │
│  - Telemetry uplink          │
│  - Control downlink          │
│  - AES-CTR + CMAC security   │
└──────────────────────────────┘
```

---

## 3️⃣ CƠ CHẾ TDMA & TRUYỀN NHẬN

### ⚙️ Cấu trúc TDMA Cycle

```
| SYNC | SLOT1 | GUARD | SLOT2 | GUARD | ... | REGISTER_WINDOW |
```

- **SYNC:** Gateway phát gói đồng bộ thời gian.
- **SLOTn:** mỗi thuyền gửi telemetry đúng slot của mình.
- **GUARD:** 200–300 ms để bù lệch thời gian LoRa.
- **REGISTER_WINDOW:** cuối chu kỳ dành cho thuyền mới đăng ký.

### 🕒 SYNC Frame

```
S,<cycle_id>,<gateway_time>,<slot_dur>,<guard>,<slot_map>
```

→ Vehicle tính lệch (`offset`) để đồng bộ thời gian phát.

### 🧠 Delay Compensation

- Gateway đo `delay_ms` thực tế giữa phát và nhận.
- Gửi `ACK,<boat_id>,delay_ms=85` → thuyền gửi sớm hơn tương ứng.

---

## 4️⃣ GATEWAY TỰ ĐIỀU CHỈNH TDMA (Local Auto-Tuning)

Gateway theo dõi chất lượng mạng và **tự tinh chỉnh thông số TDMA cục bộ**.

### ⚙️ Cơ chế hoạt động

- **Đo các chỉ số:** RSSI trung bình, delay, số gói lỗi, collision count.
- **Điều chỉnh động:**
  - Nếu delay > ngưỡng → tăng `guard_interval` (+20%).
  - Nếu nhiều collision → giảm số slot / tăng `slotDuration`.
  - Nếu nhiều slot trống → thu hẹp `cycle_time` để giảm latency.

- **Lưu cấu hình tạm thời:** áp dụng trong 3–5 chu kỳ tiếp theo.
- **Gửi báo cáo Fog:** Fog có thể ghi nhận lại để tối ưu vùng.

### 📦 Ví dụ

```
avg_delay = 140 ms  → guard_interval = 250 ms
collision_rate = 10% → slot_count giảm từ 8 → 6
```

→ giúp mạng duy trì ổn định mà không cần can thiệp thủ công.

---

## 5️⃣ ĐỊNH DẠNG GÓI TIN

### 🧱 Packet Layout

```
| PREAMBLE(1) | LEN(1) | TYPE(1) | SEQ(1) | NONCE(8) | CIPHERTEXT(N) | TAG(8) |
```

| Field      | Size | Description                                  |
| ---------- | ---- | -------------------------------------------- |
| PREAMBLE   | 1B   | 0xAA                                         |
| LEN        | 1B   | tổng độ dài                                  |
| TYPE       | 1B   | loại gói (telemetry, control, register, ack) |
| SEQ        | 1B   | sequence chống replay                        |
| NONCE      | 8B   | unique cho mỗi gói                           |
| CIPHERTEXT | N    | payload packed đã mã hoá                     |
| TAG        | 8B   | AES-CMAC truncated 8B                        |

---

### 🔐 Bảo mật

- **Encryption:** AES-CTR (PSK per-vehicle)
- **Authentication:** AES-CMAC 8 bytes
- **Replay protection:** SEQ + NONCE unique
- **Key:** 16 bytes (AES-128)

---

## 6️⃣ PAYLOAD PACKED (TỐI GIẢN)

**Telemetry payload (12 bytes):**

```
lat_i32   (4B) = lat × 1e7
lon_i32   (4B) = lon × 1e7
speed_u16 (2B) = speed × 100
heading_u16 (2B) = góc lái 0–359
```

→ Mã hoá bằng AES-CTR → ciphertext = 12 B.
→ Gói tổng cộng ≈ 32 – 36 B (ToA ≈ 50–70 ms @ SF9).

---

## 7️⃣ QUẢN LÝ & ĐỒNG BỘ DỮ LIỆU

### A. Registry trung tâm (Fog)

Fog lưu toàn bộ mapping:

```json
{
  "vehicles": {
    "boat7": { "gateway": "gw2", "slot": 4, "lastSeen": 1730900123 }
  },
  "gateways": {
    "gw2": {
      "region": "north",
      "slots": ["boat3", "boat7"],
      "config": { "slotDur": 800, "guard": 200 }
    }
  }
}
```

Gateway gửi báo cáo định kỳ:

```
POST /api/gw/report
{ "gw_id":"gw2","slotUsage":5,"avgDelay":80,"vehicles":[{"id":"boat7","rssi":-85}] }
```

Fog lưu lại, hiển thị trong dashboard giám sát vùng.

---

## 8️⃣ QUẢN LÝ THEO VÙNG (REGION MANAGEMENT)

- Mỗi **gateway phụ trách một vùng duy nhất**, không overlap.
- Vehicle chỉ tương tác với gateway thuộc vùng đó.
- Nếu di chuyển sang vùng khác, Fog cập nhật gateway mới.

```json
{
  "regions": {
    "north": ["gw1"],
    "south": ["gw2"]
  }
}
```

### Ưu điểm

✅ Không xung đột phủ sóng
✅ Dễ mở rộng / bảo trì
✅ Slot quản lý cục bộ, độc lập từng vùng

---

## 9️⃣ GATEWAY ĐĂNG NHẬP VÀ ĐỒNG BỘ VỚI FOG

### 🧩 A. Đăng ký gateway lần đầu

Gateway gửi:

```http
POST /api/fog/register-gateway
{
  "gw_id": "GW-21E305A1",
  "region": "north",
  "mac": "00:13:EF:8A:12:9B"
}
```

Fog ghi nhận → tạo entry mới và đánh dấu `"status": "approved"` hoặc `"pending"` cho admin duyệt.

### 🧩 B. Đăng nhập định kỳ

Gateway khởi động → gửi “login” đơn giản:

```http
POST /api/fog/login
{ "gw_id": "GW-21E305A1", "region": "north" }
```

Fog trả về cấu hình vùng hiện tại (slotDuration, guard, registerWindow...).

### 🧩 C. Gửi báo cáo định kỳ

```
POST /api/gw/report
{
  "gw_id":"GW-21E305A1",
  "region":"north",
  "slotUsage":6,
  "avgDelay":90,
  "collision":0
}
```

→ Không có xác thực phức tạp (vì hệ thống nội bộ).
→ Bảo mật chính vẫn ở tầng LoRa.

---

## 🔟 HỆ THỐNG BẢO MẬT (TẦNG LORA)

| Thành phần             | Cơ chế                  | Ghi chú           |
| ---------------------- | ----------------------- | ----------------- |
| **Vehicle ↔ Gateway** | AES-CTR + CMAC 8B       | PSK per-vehicle   |
| **Gateway ↔ Fog**     | HTTP đơn giản (no-TLS)  | chạy mạng nội bộ  |
| **Fog duyệt thiết bị** | approve/pending         | bảo vệ registry   |
| **Key management**     | Fog sinh – gateway sync | JSON cache cục bộ |

---

## 11️⃣ KEY & ID MANAGEMENT

### 🔹 PSK per-vehicle

- Sinh tại Fog khi approve thiết bị.
- Lưu trong:
  - Vehicle (EEPROM)
  - Gateway (`keys.json`)
  - Fog registry.

### 🔹 Vehicle ID

- Tự động từ **MAC/UID chip**:

  ```
  V-<last6bytes(MAC)>
  ```

  hoặc hash:

  ```
  V-<SHA1(MAC+seed)[:6]>
  ```

### 🔹 Fog Accept Flow

1. Vehicle gửi REGISTER (LoRa AES-CTR encrypted).
2. Gateway forward đến Fog.
3. Fog kiểm tra ID → approve/reject.
4. Nếu approved → cấp slot và PSK.

---

## 12️⃣ CẤU HÌNH KHUNG CHU KỲ (RECOMMENDED)

| Tham số            | Giá trị          |
| ------------------ | ---------------- |
| SF                 | 9–10             |
| BW                 | 125 kHz          |
| Slot Duration      | 800 ms           |
| Guard Interval     | 200 ms           |
| Cycle              | 10 s             |
| Register Window    | 1 s              |
| Payload            | 12 bytes         |
| ACK delay feedback | Có               |
| Key length         | 128 bit AES      |
| Region             | 1 gateway / vùng |

---

## 13️⃣ TỐI ƯU TOÀN HỆ THỐNG

| Cấp           | Cơ chế tối ưu                                       |
| ------------- | --------------------------------------------------- |
| **Vehicle**   | Delay compensation, send early theo offset          |
| **Gateway**   | Auto-tuning guard/slot, reclaim slot rảnh           |
| **Fog**       | Quản lý vùng, approve thiết bị, lưu thống kê        |
| **Toàn mạng** | Không overlap vùng, giảm collision, giữ ToA < 70 ms |

---

## ✅ 14️⃣ ƯU ĐIỂM TỔNG THỂ

| Tính năng        | Đạt được                    |
| ---------------- | --------------------------- |
| TDMA ổn định     | ✅ Không xung đột           |
| Guard interval   | ✅ Bù trễ vật lý            |
| Auto-tuning      | ✅ Gateway tự tối ưu        |
| Auto-register    | ✅ Không cần cấu hình tay   |
| AES-CTR + CMAC   | ✅ Bảo mật nhẹ              |
| PSK per-device   | ✅ Chống giả mạo            |
| Region isolation | ✅ Không overlap            |
| Fog registry     | ✅ Duyệt thiết bị tập trung |
| Simple HTTP      | ✅ Dễ vận hành thực địa     |
| Expandable       | ✅ Mở rộng dễ dàng          |

---

## 🔚 15️⃣ TỔNG KẾT CUỐI

> 🔸 **Vehicle–Gateway:**
> TDMA + Guard + DelayComp + AES-CTR/CMAC.
>
> 🔸 **Gateway:**
> Đăng nhập Fog, nhận config, **tự auto-tune TDMA** theo chất lượng link.
>
> 🔸 **Fog:**
> Duyệt thiết bị, cấp PSK, quản lý vùng và cấu hình.
>
> 🔸 **Bảo mật:**
> Tập trung ở tầng LoRa, tránh phức tạp ở HTTP.
>
> 👉 Kết quả: hệ thống **nhẹ, ổn định, tự điều chỉnh, dễ triển khai thực tế** với SX1278 và mạng nội bộ.
