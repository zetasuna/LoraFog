package main

import (
	"bufio"
	"fmt"
	"log"
	"time"

	serial "go.bug.st/serial"
)

func main() {
	// Mở cổng serial (sửa đường dẫn tùy hệ thống)
	port, err := serial.Open("/dev/lora", &serial.Mode{
		BaudRate: 9600,
	})
	if err != nil {
		log.Fatalf("Không mở được cổng serial: %v", err)
	}
	defer port.Close()

	// Goroutine đọc dữ liệu nhận về
	go func() {
		reader := bufio.NewReader(port)
		for {
			line, err := reader.ReadBytes('\n')
			if err == nil && len(line) > 0 {
				fmt.Printf("[RX] %s", string(line))
			}
		}
	}()

	// Gửi dữ liệu định kỳ
	for i := 0; ; i++ {
		msg := fmt.Sprintf("Xin chao tu xe A lan %d\n", i)
		_, err := port.Write([]byte(msg))
		if err != nil {
			log.Println("Lỗi gửi:", err)
		} else {
			log.Printf("[TX] %s", msg)
		}
		time.Sleep(2 * time.Second)
	}
}
