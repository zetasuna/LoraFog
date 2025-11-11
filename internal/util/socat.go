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
	mutex    sync.Mutex
	commands []*exec.Cmd
	links    []string
	stopped  bool
}

// NewSocatManager constructs an empty SocatManager.
func NewSocatManager() *SocatManager {
	return &SocatManager{}
}

// CreatePair starts a socat process to link two PTYs (left <-> right).
// It returns an error if socat cannot be started.
func (sm *SocatManager) CreatePair(left, right string) error {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()

	if sm.stopped {
		err := fmt.Errorf("Socat manager is stopped")
		slog.Warn(
			"create pair failed: manager is stopped",
			"left", left, "right", right,
		)
		return err
	}

	command := exec.CommandContext(context.Background(),
		"socat", "-d", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", left),
		fmt.Sprintf("pty,raw,echo=0,link=%s", right),
	)
	// Direct logs to program stderr/stdout for visibility.
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	if err := command.Start(); err != nil {
		slog.Warn(
			"failed to start socat",
			"component", "socat", "left", left, "right", right, "error", err,
		)
		return fmt.Errorf("start socat: %w", err)
	}

	slog.Info(
		"socat started",
		"component", "socat", "pid", command.Process.Pid, "left", left, "right", right,
	)
	sm.commands = append(sm.commands, command)
	sm.links = append(sm.links, left, right)

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

// Cleanup stops all started socat processes and removes links created.
func (sm *SocatManager) Cleanup() {
	sm.mutex.Lock()
	defer sm.mutex.Unlock()
	if sm.stopped {
		return
	}
	sm.stopped = true

	for _, command := range sm.commands {
		if command == nil || command.Process == nil {
			continue
		}
		p := command.Process
		slog.Info("killing socat process", "component", "socat", "pid", p.Pid)
		_ = p.Signal(syscall.SIGTERM)
		// wait with timeout
		done := make(chan error, 1)
		go func(c *exec.Cmd) { done <- c.Wait() }(command)
		select {
		case <-time.After(500 * time.Millisecond):
			_ = p.Kill()
		case <-done:
		}
	}

	// Remove links if exist
	for _, path := range sm.links {
		if _, err := os.Lstat(path); err == nil {
			if err := os.Remove(path); err != nil {
				slog.Warn("failed to remove socat link", "component", "socat", "path", path, "error", err)
			} else {
				slog.Info("removed socat link", "component", "socat", "path", path)
			}
		}
	}

	// clear slices
	sm.commands = nil
	sm.links = nil
	slog.Info("socat cleanup complete", "component", "socat")
}

// CleanupAll is a failsafe that attempts to kill any running socat globally.
func (sm *SocatManager) CleanupAll() {
	// Best-effort: use pkill if available.
	_ = exec.Command("pkill", "-f", "socat").Run()
	slog.Info("socat global cleanup attempted", "component", "socat")
}
