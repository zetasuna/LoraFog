// Package util provides helpers for virtual serial management using socat.
package util

import (
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
)

// SocatManager manages lifecycle of socat-created virtual serial pairs.
type SocatManager struct {
	mu     sync.Mutex
	cmds   []*exec.Cmd
	links  []string
	closed bool
}

// NewSocatManager initializes an empty manager.
func NewSocatManager() *SocatManager {
	return &SocatManager{}
}

// CreatePair starts a socat process that links two PTYs (bidirectional).
func (m *SocatManager) CreatePair(left, right string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	cmd := exec.Command(
		"socat", "-d", "-d",
		fmt.Sprintf("pty,raw,echo=0,link=%s", left),
		fmt.Sprintf("pty,raw,echo=0,link=%s", right),
	)
	cmd.Stdout = log.Writer()
	cmd.Stderr = log.Writer()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start socat: %w", err)
	}

	log.Printf("[virt-serial] started socat (pid=%d): %s <-> %s", cmd.Process.Pid, left, right)

	m.cmds = append(m.cmds, cmd)
	m.links = append(m.links, left, right)
	return nil
}

// Cleanup stops all socat processes and removes created links.
func (m *SocatManager) Cleanup() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	m.closed = true

	for _, cmd := range m.cmds {
		if cmd.Process != nil {
			log.Printf("[virt-serial] killing socat pid=%d", cmd.Process.Pid)
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	}

	for _, path := range m.links {
		if _, err := os.Lstat(path); err == nil {
			_ = os.Remove(path)
			log.Printf("[virt-serial] removed link: %s", path)
		}
	}

	log.Printf("[virt-serial] cleanup complete (%d pairs)", len(m.links)/2)
}

// CleanupAll forcibly kills all socat processes (failsafe).
func (m *SocatManager) CleanupAll() {
	_ = exec.Command("pkill", "-f", "socat").Run()
	// matches, _ := filepath.Glob("/tmp/tty*")
	// for _, m := range matches {
	// 	info, err := os.Lstat(m)
	// 	if err == nil && info.Mode()&os.ModeSymlink != 0 {
	// 		_ = os.Remove(m)
	// 	}
	// }
	log.Println("[virt-serial] global cleanup done")
}
