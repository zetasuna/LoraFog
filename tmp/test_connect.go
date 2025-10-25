package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.bug.st/serial"
)

func main() {
	portName := flag.String("port", "/dev/serial0", "serial port (e.g. /dev/ttyUSB0)")
	baud := flag.Int("baud", 9600, "baud rate")
	id := flag.String("id", "module1", "id string to include in automatic message")
	period := flag.Duration("period", 4*time.Second, "periodic send interval (0 = disable)")
	flag.Parse()

	mode := &serial.Mode{
		BaudRate: *baud,
	}

	port, err := serial.Open(*portName, mode)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Không mở được cổng %s: %v\n", *portName, err)
		os.Exit(1)
	}
	defer port.Close()
	fmt.Printf("Mở cổng %s @ %d baud\n", *portName, *baud)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// handle Ctrl+C
	sigc := make(chan os.Signal, 1)
	signal.Notify(sigc, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigc
		fmt.Println("\nĐã nhận signal, đóng...")
		cancel()
	}()

	// goroutine đọc từ serial
	go readLoop(ctx, port)

	// goroutine gửi tự động (nếu period > 0)
	if *period > 0 {
		go autoSendLoop(ctx, port, *id, *period)
	}

	// vòng chính: đọc stdin để gửi thủ công
	stdin := bufio.NewScanner(os.Stdin)
	fmt.Println("Gõ vào để gửi (Enter). Gõ ':quit' để thoát.")
	for stdin.Scan() {
		line := stdin.Text()
		if strings.TrimSpace(line) == "" {
			continue
		}
		if line == ":quit" || line == ":q" {
			cancel()
			break
		}
		if err := sendLine(port, line); err != nil {
			fmt.Fprintf(os.Stderr, "Gửi lỗi: %v\n", err)
		} else {
			fmt.Printf("[TX] %s\n", line)
		}
		// nếu context bị huỷ thì break
		select {
		case <-ctx.Done():
			break
		default:
		}
	}

	// đợi một chút để goroutine hoàn tất
	time.Sleep(200 * time.Millisecond)
	fmt.Println("Thoát.")
}

// readLoop: đọc liên tục, mỗi khi gặp newline in ra dòng nhận được.
func readLoop(ctx context.Context, port serial.Port) {
	r := bufio.NewReader(port)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// đọc đến newline hoặc timeout (Open port không set timeout => blocking)
			// Một số driver có Read timeout, tùy hệ thống.
			line, err := r.ReadString('\n')
			if err != nil {
				// nếu bị huỷ, thoát
				select {
				case <-ctx.Done():
					return
				default:
					// in lỗi rồi thử tiếp (non-fatal)
					// fmt.Fprintf(os.Stderr, "Read error: %v\n", err)
					time.Sleep(50 * time.Millisecond)
					continue
				}
			}
			// trim CR/LF
			line = strings.TrimRight(line, "\r\n")
			if line != "" {
				fmt.Printf("[RX] %s\n", line)
			}
		}
	}
}

// autoSendLoop: gửi "hello from <id>" định kỳ
func autoSendLoop(ctx context.Context, port serial.Port, id string, period time.Duration) {
	t := time.NewTicker(period)
	defer t.Stop()
	i := 1
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			msg := fmt.Sprintf("hello from %s #%d", id, i)
			if err := sendLine(port, msg); err != nil {
				fmt.Fprintf(os.Stderr, "Auto send error: %v\n", err)
			} else {
				fmt.Printf("[TX auto] %s\n", msg)
			}
			i++
		}
	}
}

// sendLine: gửi chuỗi kèm newline (một số module cần newline)
func sendLine(port serial.Port, s string) error {
	// append newline để bên nhận dễ parse (tùy cấu hình module)
	out := []byte(s + "\n")
	n, err := port.Write(out)
	if err != nil {
		return err
	}
	if n != len(out) {
		return fmt.Errorf("viết chưa đầy: %d/%d", n, len(out))
	}
	return nil
}

