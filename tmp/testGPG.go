package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"go.bug.st/serial"
	"go.bug.st/serial/enumerator"
)

func main() {
	portName := flag.String("port", "", "Serial port (e.g. /dev/ttyUSB0 or COM3)")
	baud := flag.Int("baud", 9600, "Baud rate (default 9600)")
	flag.Parse()

	if *portName == "" {
		fmt.Println("📡 Danh sách cổng serial khả dụng:")
		listPorts()
		fmt.Println("\n⚠️  Dùng cờ -port để chỉ định cổng, ví dụ:")
		fmt.Println("   go run main.go -port /dev/ttyUSB0 -baud 9600")
		return
	}

	mode := &serial.Mode{
		BaudRate: *baud,
		Parity:   serial.NoParity,
		DataBits: 8,
		StopBits: serial.OneStopBit,
	}

	port, err := serial.Open(*portName, mode)
	if err != nil {
		log.Fatalf("❌ Không mở được cổng %s: %v", *portName, err)
	}
	defer port.Close()

	fmt.Printf("✅ Đã mở %s @ %d baud\n", *portName, *baud)
	fmt.Println("📡 Đang đọc dữ liệu GPS raw (Ctrl+C để thoát)...\n")

	reader := bufio.NewReader(port)

	// Bắt tín hiệu Ctrl+C
	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, os.Interrupt, syscall.SIGTERM)

	for {
		select {
		case <-sigs:
			fmt.Println("\n👋 Kết thúc chương trình.")
			return
		default:
			line, err := reader.ReadString('\n')
			if err != nil {
				if strings.Contains(err.Error(), "EOF") {
					continue
				}
				log.Printf("Lỗi đọc serial: %v", err)
				continue
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			fmt.Println(line)
		}
	}
}

// listPorts in ra danh sách các cổng serial phát hiện được
func listPorts() {
	ports, err := enumerator.GetDetailedPortsList()
	if err != nil {
		fmt.Println("Không thể liệt kê cổng serial:", err)
		return
	}
	if len(ports) == 0 {
		fmt.Println("Không tìm thấy cổng serial nào.")
		return
	}
	for _, port := range ports {
		fmt.Printf("- %s", port.Name)
		if port.IsUSB {
			fmt.Printf(" (USB VID:PID=%s:%s, Serial=%s)\n", port.VID, port.PID, port.SerialNumber)
		} else {
			fmt.Println()
		}
	}
}
