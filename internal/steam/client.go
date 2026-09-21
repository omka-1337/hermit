package steam

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"hermit/internal/platform"
)

// The Steam client keeps its shortcuts in memory and writes them out when it
// exits, so a shortcut added behind its back is only kept if Steam is not
// running. These helpers are what the manager uses to tell, and to restart it.

// clientNames are the process names of the Steam client itself. Its helper
// processes (steamwebhelper and friends) are not the client.
var clientNames = []string{"steam", "steam.sh"}

// Running reports whether the Steam client is running for this user.
func Running() bool {
	return runningPID() > 0
}

func runningPID() int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return 0
	}
	uid := os.Getuid()
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pid := 0
		if _, err := fmt.Sscanf(entry.Name(), "%d", &pid); err != nil || pid <= 0 {
			continue
		}
		comm, err := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if err != nil {
			continue
		}
		name := strings.TrimSpace(string(comm))
		if !contains(clientNames, name) {
			continue
		}
		// Only this user's Steam matters; another user's is none of our business.
		if info, err := os.Stat(filepath.Join("/proc", entry.Name())); err == nil {
			if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Uid) != uid {
				continue
			}
		}
		return pid
	}
	return 0
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

// Shutdown asks Steam to exit and waits until it has, so that its shortcuts
// are written out before anything else touches them.
func Shutdown(wait time.Duration) error {
	if !Running() {
		return nil
	}
	if err := platform.Command("steam", "-shutdown").Run(); err != nil {
		return fmt.Errorf("asking Steam to exit: %w", err)
	}
	deadline := time.Now().Add(wait)
	for time.Now().Before(deadline) {
		if !Running() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("Steam is still running %s after being asked to exit", wait)
}

// Start launches the Steam client and returns without waiting for it.
func Start() error {
	cmd := platform.Command("steam")
	// Detach: Steam must outlive Hermit, and survive it being closed.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("starting Steam: %w", err)
	}
	return cmd.Process.Release()
}
