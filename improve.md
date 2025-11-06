👉 Mình sẽ giải thích **từ gốc đến ngọn**, cực chi tiết, từng bước và có mô hình cụ thể cho **SX1278 single-channel**, gồm 4 chức năng chính bạn nêu:
1️⃣ **Tự động đăng ký thuyền**,
2️⃣ **Quản lý thuyền tại gateway**,
3️⃣ **Gửi telemetry**,
4️⃣ **Nhận control**,
tất cả **theo chiến lược TDMA + Guard Interval + Timestamp Sync + Delay Compensation**.

---

## 🧭 1. Tổng quan chiến lược

Mục tiêu:
Tạo **mạng LoRa đồng bộ thời gian** kiểu _quasi-TDMA_ hoạt động ổn định trên **1 tần số duy nhất**, **nhiều thuyền**, chỉ cần **1 module SX1278 ở gateway**.

### Các thành phần

- **Gateway (Master):**
  Duy trì “chu kỳ TDMA” (TDMA cycle), gửi SYNC, cấp slot, nhận telemetry, phát control.
- **Vehicle (Slave):**
  Đồng bộ theo gateway, gửi telemetry trong slot được cấp, đăng ký nếu chưa có slot.

### Cấu trúc chu kỳ TDMA (ví dụ 10s)

```
| SYNC | SLOT1 | GUARD | SLOT2 | GUARD | SLOT3 | GUARD | REGISTER WINDOW |
```

- **SYNC:** Gateway broadcast timestamp & slot assignment
- **SLOT:** Vehicle gửi telemetry
- **GUARD:** khoảng trống bù trễ & jitter
- **REGISTER WINDOW:** Thuyền mới được phép đăng ký
- **Timestamp Sync:** Vehicle dùng SYNC để chỉnh lệch thời gian
- **Delay Compensation:** Vehicle bù trừ thời gian trễ trung bình đo được giữa chu kỳ

---

## ⚙️ 2. Quy trình từng bước chi tiết

### 🧩 (A) Giai đoạn khởi động & tự động đăng ký

#### 1️⃣ Gateway khởi động

- Đặt `cycleID = 0`
- Khởi tạo danh sách slot:

  ```go
  slotTable := map[int]string{} // map[slotIndex]vehicleID
  ```

- Mở LoRa để phát/thu, bắt đầu gửi SYNC frame định kỳ (mỗi TDMA cycle).

#### 2️⃣ Thuyền khởi động

- Chưa có slot → ở chế độ **LISTENING**.
- Nghe gói **SYNC** từ gateway:

  ```
  S,<cycle_id>,<gateway_time>,<num_slots>,<slot_map>,<reg_window_ms>
  ```

- Nếu thấy mình chưa có trong `slot_map` → chờ đến `register_window` và gửi gói:

  ```
  R,<vehicle_id>,<lat>,<lon>
  ```

  trong khoảng [Tstart_reg, Tend_reg] (chọn ngẫu nhiên thời điểm để tránh va chạm).

#### 3️⃣ Gateway nhận REGISTER

- Nếu gói hợp lệ → cấp slot mới:

  ```go
  slotTable[nextFreeSlot] = vehicleID
  ```

- Gửi phản hồi:

  ```
  ASSIGN,<vehicle_id>,<slot_index>,<telemetry_interval_ms>
  ```

- Gửi cập nhật slot map trong SYNC tiếp theo.

#### 4️⃣ Vehicle nhận ASSIGN

- Lưu `slot_index` và `telemetry_interval_ms`
- Chuyển sang trạng thái **ACTIVE**.

---

### 🧩 (B) Đồng bộ thời gian (Timestamp Sync)

#### 1️⃣ Gateway gửi SYNC frame mỗi chu kỳ

```text
S,<cycle_id>,<server_time_ms>,<slot_duration_ms>,<guard_ms>,<slot_map_json>
```

Ví dụ:

```
S,42,100000,800,200,{"1":"boatA","2":"boatB"},1000
```

#### 2️⃣ Vehicle nhận SYNC

- Ghi nhận thời gian nhận SYNC (`t_local_recv`)
- So sánh với `server_time_ms`
- Tính **offset**:

  ```
  offset = t_local_recv - server_time_ms
  ```

- Cập nhật đồng hồ nội bộ:

  ```go
  v.localTimeCorrection = offset
  ```

→ Lần tới khi gửi telemetry, vehicle sẽ gửi **sớm hơn offset này** để bù trễ LoRa.

---

### 🧩 (C) Gửi Telemetry (uplink TDMA)

#### 1️⃣ Tính thời điểm gửi

Mỗi thuyền chỉ gửi trong slot của mình:

```
slotStart = syncTime + (slotIndex-1) * (slotDuration + guard)
```

Có bù trễ:

```
adjustedSlotStart = slotStart - delayCompensation - clockOffset
```

→ Giúp gói đến gateway **đúng lúc bắt đầu slot thực**.

#### 2️⃣ Vehicle gửi telemetry

```
T,<vehicle_id>,<lat>,<lon>,<head>,<speedL>,<speedR>
```

#### 3️⃣ Gateway nhận

- Đọc từng dòng LoRa
- Dựa vào `vehicle_id` xác định slot
- Gửi ACK:

  ```
  ACK,<vehicle_id>,<recv_time_ms>,<delay_ms>
  ```

  trong đó `delay_ms = recv_time_ms - expected_slot_time`

#### 4️⃣ Vehicle nhận ACK

- Cập nhật lại delay compensation:

  ```go
  v.delayCompensation = alpha*v.delayCompensation + (1-alpha)*delay_ms
  ```

  (EMA – exponential moving average để mượt).

---

### 🧩 (D) Nhận Control (downlink)

#### 1️⃣ FogServer gửi control đến Gateway qua HTTP `/api/control`

```json
{
  "boatId": "boatA",
  "speed": 500,
  "targetLat": 21.0286,
  "targetLon": 105.8048
}
```

#### 2️⃣ Gateway mã hóa theo LoRa format

```
C,<boatId>,<speed>,<lat>,<lon>,<kp>,<ki>,<kd>
```

#### 3️⃣ Gửi xuống vehicle:

- Gateway phát control **trong khoảng riêng sau tất cả slot uplink** (tránh đụng uplink).
- Vehicle đang lắng nghe LoRa trong giai đoạn “downlink window”.

#### 4️⃣ Vehicle nhận control

- Parse gói `C,boatId,...`
- Nếu `boatId == v.ID` → thực thi, gửi lại `ACK_C,<boatId>` để xác nhận.

---

## 🧠 3. Các cơ chế chống xung đột & ổn định

| Cơ chế                 | Mục đích                 | Cách hoạt động                              |
| ---------------------- | ------------------------ | ------------------------------------------- |
| **TDMA Slot**          | Tránh xung đột uplink    | Mỗi thuyền có slot riêng                    |
| **Guard Interval**     | Bù sai lệch              | Khoảng trống 200–300ms giữa slot            |
| **Timestamp Sync**     | Giữ đồng hồ đồng bộ      | Gateway gửi timestamp, thuyền điều chỉnh    |
| **Delay Compensation** | Bù delay vật lý LoRa     | Vehicle đo & trừ delay trung bình           |
| **ACK Feedback**       | Kiểm tra chính xác       | Gateway gửi delay đo được, Vehicle cập nhật |
| **Register Window**    | Giới hạn gói đăng ký     | Thuyền mới chỉ gửi trong vùng cho phép      |
| **Adaptive Slot**      | Tái phân bổ slot nếu cần | Gateway phát SYNC với slot map mới          |

---

## 🚤 4. Luồng hoạt động tổng thể minh họa

### Giai đoạn khởi động

```
[Gateway] → S,0,100000,slotMap={}
[Boat A]  → R,boatA,21.0,105.8
[Gateway] → ASSIGN,boatA,1
[Boat B]  → R,boatB,21.01,105.81
[Gateway] → ASSIGN,boatB,2
```

### Giai đoạn hoạt động ổn định

```
Cycle 42:
[Gateway] → S,42,120000,slots={1:boatA,2:boatB}
[Boat A]  → T,boatA,...  (at slot 1)
[Gateway] → ACK,boatA,delay_ms=950
[Boat B]  → T,boatB,...  (at slot 2)
[Gateway] → ACK,boatB,delay_ms=100
[Gateway] → C,boatB,... (send control)
```

---

## 📐 5. Sơ đồ thời gian (Timeline)

```
Time (s):   0      1      2      3      4      5     6     7     8     9     10
---------------------------------------------------------------------------------
Gateway:   [SYNC] [--- SLOT1 ---][G] [--- SLOT2 ---][G][--- SLOT3 ---][G][REG][G]
Boat1:             send telemetry ↑ wait ack ↑
Boat2:                     send telemetry ↑ wait ack ↑
Boat3:                             send telemetry ↑ wait ack ↑
BoatX(new):                                             send REGISTER ↑
```

(G = Guard interval)

---

## 🧮 6. Tham số khuyến nghị thực tế (với SX1278)

| Tham số            | Giá trị khuyên dùng | Giải thích                         |
| ------------------ | ------------------- | ---------------------------------- |
| SF                 | 9 hoặc 10           | Cân bằng giữa độ trễ & khoảng cách |
| BW                 | 125 kHz             | Chuẩn                              |
| Payload            | ≤ 24 bytes          | giữ ToA ngắn                       |
| Slot time          | ~400–800 ms         | đủ cho 1 telemetry                 |
| Guard interval     | 200 ms              | bù jitter, delay                   |
| TDMA cycle         | 10–12 s             | cho 5–8 thuyền ổn định             |
| Register window    | 1 s cuối chu kỳ     | đủ 1–2 gói đăng ký                 |
| ACK delay feedback | mỗi chu kỳ          | giúp tự hiệu chỉnh                 |

---

## ✅ 7. Ưu điểm của chiến lược này

| Tiêu chí                | Đạt được                                   |
| ----------------------- | ------------------------------------------ |
| Tự tổ chức mạng         | ✅ Có (auto-register)                      |
| Không va chạm           | ✅ Có (TDMA + guard)                       |
| Độ trễ thấp             | ✅ Có (bù delay)                           |
| Không cần GPS sync      | ✅ Có (timestamp sync)                     |
| Tự phục hồi             | ✅ Có (delay compensation + reassign slot) |
| Dễ mở rộng              | ✅ Có (gateway cập nhật slot map)          |
| Phù hợp SX1278 đơn kênh | ✅ Chính xác                               |

---

## 🔧 8. Mô hình thực thi trong code của bạn

| File         | Bổ sung mới                                                 |
| ------------ | ----------------------------------------------------------- |
| `vehicle.go` | Hàm `handleSync()`, `sendRegister()`, `adjustTiming()`      |
| `gateway.go` | Hàm `broadcastSync()`, `handleRegister()`, `measureDelay()` |
| `message.go` | Struct `SyncFrame`, `RegisterData`, `AssignData`, `AckData` |
| `config.yml` | Thêm `slot_duration_ms`, `guard_ms`, `cycle_ms`             |
