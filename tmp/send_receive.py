import serial
import threading
import time

# --- Cấu hình UART ---
ser = serial.Serial("/dev/lora", 9600, timeout=0.5)  # timeout ngắn để không block


# --- Hàm gửi dữ liệu ---
def sender():
    while True:
        msg = "Hello from Fedora\n"
        ser.write(msg.encode())
        ser.flush()
        print(f"Sent: {msg.strip()}")
        time.sleep(4)  # gửi 2s/lần


# --- Hàm nhận dữ liệu ---
def receiver():
    while True:
        if ser.in_waiting:
            data = ser.readline().decode("utf-8", errors="ignore").strip()
            if data:
                print(f"Received: {data}")


# --- Khởi tạo thread ---
t_send = threading.Thread(target=sender, daemon=True)
t_recv = threading.Thread(target=receiver, daemon=True)

t_send.start()
t_recv.start()

# --- Giữ main thread chạy ---
while True:
    time.sleep(1)
