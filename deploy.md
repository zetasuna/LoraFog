---
Title: Deploy
---

## Project

```bash
sudo dnf install -y socat
cd /path/to/LoraFog
go mod tidy
make run
```

---

## Arduino

```bash
pip install -U platformio
cd arduino
pio pkg install
pio run -t compiledb
sudo $(which pio) run -t upload
```

---

## Tạo một tên thiết bị serial cố định trên Pi

```txt
/dev/arduino
```

… luôn trỏ đúng đến board Arduino của **dù reboot hay đổi cổng USB**.

---

### 🧩 1. Cắm Arduino vào Raspberry Pi

Chạy lệnh để xem thiết bị xuất hiện là gì:

```bash
ls /dev/tty*
```

Thông thường sẽ thấy:

```txt
/dev/ttyUSB0
```

hoặc

```txt
/dev/ttyACM0
```

---

### 🧩 2. Xem thông tin phần cứng của thiết bị

Chạy:

```bash
udevadm info -a -n /dev/ttyUSB0 | grep -E "idVendor|idProduct|serial"
```

Kết quả ví dụ (Arduino Uno):

```txt
ATTRS{idVendor}=="2341"
ATTRS{idProduct}=="0043"
ATTRS{serial}=="754393033383518011E0"
```

---

### 🧩 3. Tạo file udev rule cố định

Tạo file mới:

```bash
sudo nano /etc/udev/rules.d/99-arduino.rules
```

Thêm nội dung sau (chỉnh lại theo giá trị của bạn):

```bash
SUBSYSTEM=="tty", ATTRS{idVendor}=="2341", ATTRS{idProduct}=="0043", ATTRS{serial}=="754393033383518011E0", SYMLINK+="arduino"
```

👉 Dòng này sẽ tạo alias `/dev/arduino` cho đúng thiết bị Arduino đó.

---

### 🧩 4. Reload udev và kiểm tra

Chạy:

```bash
sudo udevadm control --reload-rules
sudo udevadm trigger
```

Rồi rút và cắm lại Arduino.
Kiểm tra:

```bash
ls -l /dev/arduino
```

Kết quả:

```txt
lrwxrwxrwx 1 root root 7 Oct 24 02:30 /dev/arduino -> ttyUSB0
```

✅ Nghĩa là `/dev/arduino` giờ luôn trỏ đến cổng đúng, dù có đổi sang USB khác.

---

### 🧩 5. Cho phép user `pi` truy cập thiết bị

```bash
sudo usermod -aG dialout $USER
```

Sau đó **logout/login lại**, hoặc reboot.
