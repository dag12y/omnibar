package ipc

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// A long-lived launched app or clipboard owner must not retain the backend's
// single-instance lease after the backend exits.
func TestInstanceLockNotInheritedByDesktopChildren(t *testing.T) {
	path := filepath.Join(t.TempDir(), "instance.lock")
	fd, err := acquireLock(path)
	if err != nil {
		t.Fatal(err)
	}
	child := exec.Command("sleep", "30")
	if err := child.Start(); err != nil {
		releaseLock(fd)
		t.Fatal(err)
	}
	defer func() { child.Process.Kill(); child.Wait() }()
	releaseLock(fd)
	next, err := acquireLock(path)
	if err != nil {
		t.Fatalf("desktop child retained instance lock after shutdown: %v", err)
	}
	releaseLock(next)
}
