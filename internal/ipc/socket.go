// Package ipc implements the Unix domain socket server for Omnibar.
//
// Go owns and listens on the socket at $XDG_RUNTIME_DIR/omnibar/omnibar.sock.
// The QML UI connects later using Quickshell.Io.Socket.
//
// The server provides transport and framing only: newline-delimited
// messages with a 1 MiB maximum frame size. No JSON-RPC or application
// logic is implemented here.
package ipc

import (
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"syscall"
)

// SocketPath resolves the Unix domain socket path.
//
// It reads XDG_RUNTIME_DIR and resolves
// <XDG_RUNTIME_DIR>/omnibar/omnibar.sock. XDG_RUNTIME_DIR is mandatory;
// an unset or empty value returns an error. There is no fallback to
// /tmp or any other location.
func SocketPath() (string, error) {
	xdgRuntimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if xdgRuntimeDir == "" {
		return "", errors.New("XDG_RUNTIME_DIR is not set")
	}
	return filepath.Join(xdgRuntimeDir, "omnibar", "omnibar.sock"), nil
}

// lockFileName is the name of the file-lock used for single-instance
// safety. It lives inside the private Omnibar runtime directory and is
// kept after shutdown to avoid inode races from lock-file deletion.
const lockFileName = "omnibar.lock"

// EnsureRuntimeDir creates the parent directory of the socket with mode
// 0700. If the directory already exists, it verifies that it is a
// directory and that group/others have no permissions. It does not
// modify an existing directory, even if its permissions are unsafe;
// instead it returns an error.
func EnsureRuntimeDir(socketPath string) error {
	dir := filepath.Dir(socketPath)

	info, err := os.Stat(dir)
	if err != nil {
		if !os.IsNotExist(err) {
			return fmt.Errorf("stat runtime directory %q: %w", dir, err)
		}
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return fmt.Errorf("create runtime directory %q: %w", dir, err)
		}
		info, err = os.Stat(dir)
		if err != nil {
			return fmt.Errorf("stat runtime directory %q: %w", dir, err)
		}
	}

	if !info.IsDir() {
		return fmt.Errorf("%q is not a directory", dir)
	}

	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("runtime directory %q has unsafe permissions %o (expected 0700)", dir, info.Mode().Perm())
	}

	return nil
}

// IsStaleSocket conservatively determines whether path is a stale Unix
// socket entry with no live listener.
//
// It uses os.Lstat() so symlinks are never followed. Only an actual Unix
// socket file mode entry is eligible for stale recovery. A successful
// connection means a live server is listening (not stale). Connection
// refused means the socket is stale. Any other error (permission denied,
// timeout, etc.) is treated as NOT stale — the caller must not remove
// the entry.
//
// Path disappearance is handled safely: if Lstat reports the path no
// longer exists, it is treated as not stale (concurrent removal).
func IsStaleSocket(path string) (bool, error) {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}

	// Only actual Unix socket entries are eligible for stale recovery.
	// Symlinks, regular files, directories, and other entry types are
	// never removed.
	if info.Mode()&os.ModeSocket == 0 {
		return false, nil
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		// Connection refused means no listener — the socket is stale.
		if errors.Is(err, syscall.ECONNREFUSED) {
			return true, nil
		}
		// Permission errors, timeouts, and other failures are not
		// treated as stale. Leave the entry untouched.
		return false, nil
	}
	conn.Close()
	return false, nil
}

// RemoveStaleSocket removes path only if it is confirmed to be a stale
// Unix socket. It follows the same conservative rules as IsStaleSocket:
// symlinks, regular files, directories, and other entries are never
// removed. This function must only be called within the serialized
// startup sequence protected by the file lock.
func RemoveStaleSocket(path string) error {
	stale, err := IsStaleSocket(path)
	if err != nil {
		return err
	}
	if !stale {
		return nil
	}
	return os.Remove(path)
}

// acquireLock opens (or creates) the lock file in the runtime directory
// and acquires an exclusive, non-blocking flock. If another process
// holds the lock, it returns an already-running error. The caller must
// call releaseLock on the returned fd when done.
// O_CLOEXEC is set atomically: desktop children must never retain this lease.
func acquireLock(lockPath string) (int, error) {
	fd, err := syscall.Open(lockPath, os.O_RDWR|os.O_CREATE|syscall.O_CLOEXEC, 0o600)
	if err != nil {
		return -1, fmt.Errorf("open lock file %q: %w", lockPath, err)
	}

	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		syscall.Close(fd)
		if err == syscall.EWOULDBLOCK {
			return -1, errors.New("another instance is already running")
		}
		return -1, fmt.Errorf("acquire lock on %q: %w", lockPath, err)
	}

	return fd, nil
}

// releaseLock closes the lock file descriptor. The lock file itself is
// intentionally not removed, because deleting lock files can introduce
// inode races.
func releaseLock(fd int) {
	if fd >= 0 {
		syscall.Close(fd)
	}
}
