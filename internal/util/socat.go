// Package util provides helpers for virtual serial management using socat.
package util

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"time"
)

// SocatManager manages socat processes used to create virtual serial pairs.
type SocatManager struct {
	mu      sync.Mutex
	cmds    []*exec.Cmd
	links   []string
	stopped bool
}

// NewSocatManager constructs an empty SocatManager.
func NewSocatManager() *SocatManager {
	return &SocatManager{}
}

// CreatePair starts a socat process to link two PTYs (left <-> right).
// It returns an error if socat cannot be started.
func (m *SocatManager) CreatePair(left, right string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.stopped {
		return fmt.Errorf("[Socat] Manager is stopped")
	}

	cmd := exec.CommandContext(context.Background(),
		"socat", "-d", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", left),
		fmt.Sprintf("pty,raw,echo=0,link=%s", right),
	)
	// Direct logs to program stderr/stdout for visibility.
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.Warn("[Socat] Failed to start socat", "left", left, "right", right, "error", err)
		return fmt.Errorf("start socat: %w", err)
	}

	slog.Info("[Socat] Started", "pid", cmd.Process.Pid, "left", left, "right", right)
	m.cmds = append(m.cmds, cmd)
	m.links = append(m.links, left, right)

	// Give socat some time to create the links
	timeout := time.After(500 * time.Millisecond)
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-timeout:
			return nil
		case <-ticker.C:
			if _, err := os.Stat(left); err == nil {
				return nil
			}
		}
	}
}

// CreateBus tạo một BUS broadcast giả lập LoRa
func (m *SocatManager) CreateBus(busPath string) error {
	cmd := exec.Command("socat", "-d", "-d",
		fmt.Sprintf("UNIX-LISTEN:%s,fork,reuseaddr", busPath),
		"PIPE",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start bus socat: %w", err)
	}

	slog.Info("[Socat] Bus started", "bus", busPath, "pid", cmd.Process.Pid)
	m.cmds = append(m.cmds, cmd)
	m.links = append(m.links, busPath)
	return nil
}

func (m *SocatManager) CreateHub(path string) (*LoRaHub, error) {
	hub := NewLoRaHub(path)
	if err := hub.Start(); err != nil {
		return nil, err
	}
	return hub, nil
}

// CreateConnector tạo một PTY ảo nối vào 1 BUS broadcast
func (m *SocatManager) CreateConnector(ptyPath, busPath string) error {
	cmd := exec.Command("socat",
		"-d", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", ptyPath),
		fmt.Sprintf("UNIX-CONNECT:%s", busPath),
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("connector failed: %w", err)
	}

	slog.Info("[Socat] Connector created", "pty", ptyPath, "bus", busPath)
	m.cmds = append(m.cmds, cmd)
	m.links = append(m.links, ptyPath)
	return nil
}

// Cleanup stops all started socat processes and removes links created.
func (m *SocatManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.stopped {
		return
	}
	m.stopped = true

	for _, cmd := range m.cmds {
		if cmd == nil || cmd.Process == nil {
			continue
		}
		p := cmd.Process
		slog.Info("[Socat] Killing socat process", "pid", p.Pid)
		_ = p.Signal(syscall.SIGTERM)
		// wait with timeout
		done := make(chan error, 1)
		go func(c *exec.Cmd) { done <- c.Wait() }(cmd)
		select {
		case <-time.After(500 * time.Millisecond):
			_ = p.Kill()
		case <-done:
		}
	}

	// Remove links if exist
	for _, path := range m.links {
		if _, err := os.Lstat(path); err == nil {
			if err := os.Remove(path); err != nil {
				slog.Warn("[Socat] Failed to remove socat link", "path", path, "error", err)
			} else {
				slog.Info("[Socat] Removed socat link", "path", path)
			}
		}
	}

	// clear slices
	m.cmds = nil
	m.links = nil
	slog.Info("[Socat] Cleanup complete")
}

// CleanupAll is a failsafe that attempts to kill any running socat globally.
func (m *SocatManager) CleanupAll() {
	// Best-effort: use pkill if available.
	_ = exec.Command("pkill", "-f", "socat").Run()
	slog.Info("[Socat] Global cleanup attempted")
}
