// Package lockfile prevents multiple piper instances from running simultaneously
// and corrupting the shared store file. It uses flock(2) for atomic kernel-level
// locking — no TOCTOU race, and the kernel automatically releases the lock when
// the process exits (crash, kill -9, OOM, or normal shutdown).
package lockfile

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// ErrLocked is returned when another live instance holds the lock.
type ErrLocked struct {
	PID  int
	Port int
}

func (e *ErrLocked) Error() string {
	return fmt.Sprintf("另一個 piper 實例正在運行 (PID %d, port %d)", e.PID, e.Port)
}

// Lockfile manages a flock-based lock file.
type Lockfile struct {
	path string
	file *os.File
}

// New creates a Lockfile at the given path.
func New(path string) *Lockfile {
	return &Lockfile{path: path}
}

// Acquire attempts to take an exclusive flock on the lock file.
// If another process holds it, returns ErrLocked with its PID and port
// (read from the file for diagnostics). The lock is automatically released
// by the kernel when the process exits, regardless of how it dies.
func (l *Lockfile) Acquire(port int) error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return fmt.Errorf("open lockfile %s: %w", l.path, err)
	}

	// LOCK_EX: exclusive lock, LOCK_NB: fail immediately if held
	err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Lock is held — read PID+port for a helpful error message
		pid, oldPort := readOwner(f)
		f.Close()
		return &ErrLocked{PID: pid, Port: oldPort}
	}

	// Lock acquired — write our PID+port for diagnostics (human-readable)
	_ = f.Truncate(0)
	_, _ = f.Seek(0, 0)
	fmt.Fprintf(f, "%d %d", os.Getpid(), port)
	_ = f.Sync()

	// Keep fd open — closing it releases the flock
	l.file = f
	return nil
}

// Release explicitly closes the file descriptor, releasing the flock.
// Safe to call multiple times. Not strictly required — the kernel releases
// the lock on process exit — but good practice for clean shutdown.
func (l *Lockfile) Release() {
	if l.file != nil {
		l.file.Close()
		l.file = nil
	}
}

// readOwner reads PID and port from an existing lock file for diagnostics.
// Returns zeros if the content can't be parsed.
func readOwner(f *os.File) (pid, port int) {
	_, _ = f.Seek(0, 0)
	buf := make([]byte, 64)
	n, _ := f.Read(buf)
	if n == 0 {
		return 0, 0
	}
	parts := strings.Fields(strings.TrimSpace(string(buf[:n])))
	if len(parts) != 2 {
		return 0, 0
	}
	pid, _ = strconv.Atoi(parts[0])
	port, _ = strconv.Atoi(parts[1])
	return pid, port
}
