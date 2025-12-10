package util

import (
	"io"
	"log/slog"
	"net"
	"os"
	"sync"
)

type LoRaHub struct {
	path   string
	conns  map[net.Conn]bool
	mu     sync.Mutex
	closed bool
}

func NewLoRaHub(path string) *LoRaHub {
	return &LoRaHub{
		path:  path,
		conns: make(map[net.Conn]bool),
	}
}

func (h *LoRaHub) Start() error {
	// Remove old socket
	_ = os.Remove(h.path)

	ln, err := net.Listen("unix", h.path)
	if err != nil {
		return err
	}

	slog.Info("[LoRaHub] Listening", "socket", h.path)

	// Accept loop
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				if h.closed {
					return // hub shutting down
				}
				slog.Error("[LoRaHub] Accept failed", "err", err)
				continue
			}

			slog.Info("[LoRaHub] New connection")

			h.mu.Lock()
			h.conns[conn] = true
			h.mu.Unlock()

			// Start reading
			go h.handle(conn)
		}
	}()

	return nil
}

func (h *LoRaHub) handle(conn net.Conn) {
	buf := make([]byte, 1024)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				slog.Warn("[LoRaHub] Read error", "err", err)
			}
			h.mu.Lock()
			delete(h.conns, conn)
			h.mu.Unlock()
			_ = conn.Close()
			return
		}

		h.broadcast(conn, buf[:n])
	}
}

func (h *LoRaHub) broadcast(src net.Conn, data []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()

	for c := range h.conns {
		if c == src {
			continue
		}

		if _, err := c.Write(data); err != nil {
			slog.Warn("[LoRaHub] Write failed, removing conn", "err", err)
			_ = c.Close()
			delete(h.conns, c)
		}
	}
}

func (h *LoRaHub) Close() error {
	h.mu.Lock()
	h.closed = true
	defer h.mu.Unlock()

	slog.Info("[LoRaHub] Closing")

	for c := range h.conns {
		_ = c.Close()
		delete(h.conns, c)
	}

	if err := os.Remove(h.path); err != nil {
		slog.Warn("[LoRaHub] Failed to remove socket", "path", h.path, "err", err)
	}

	return nil
}
