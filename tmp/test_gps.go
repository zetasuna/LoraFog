package main

import (
	"bufio"
	"fmt"
	"log"
	"time"

	"go.bug.st/serial"
)

func main() {
	// Mở cổng serial
	mode := &serial.Mode{
		BaudRate: 9600, // phải trùng với tốc độ Serial.begin() trong Arduino
	}

	port, err := serial.Open("/dev/arduino", mode)
	if err != nil {
		log.Fatalf("Không thể mở cổng serial: %v", err)
	}
	defer port.Close()

	fmt.Println("✅ Đã kết nối với /dev/ttyUSB0")
	fmt.Println("⏳ Đang chờ dữ liệu từ thiết bị...")

	reader := bufio.NewReader(port)

	for {
		line, err := reader.ReadString('\n') // đọc đến ký tự xuống dòng
		if err != nil {
			log.Printf("Lỗi khi đọc: %v", err)
			time.Sleep(time.Second)
			continue
		}
		// line = strings.TrimSpace(line)
		fmt.Printf("📨 Nhận được: %q\n", line)
	}
}
