package ipc

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

// newTestLogger returns a slog logger writing to a buffer for tests.
func newTestLogger() (*slog.Logger, *bytes.Buffer) {
	buf := &bytes.Buffer{}
	return slog.New(slog.NewTextHandler(buf, nil)), buf
}

// tempSocketPath creates a temp directory, sets XDG_RUNTIME_DIR to it,
// and returns the resolved socket path. The caller must restore the
// original XDG_RUNTIME_DIR after the test.
func tempSocketPath(t *testing.T) (string, func()) {
	t.Helper()
	dir := t.TempDir()
	old := os.Getenv("XDG_RUNTIME_DIR")
	os.Setenv("XDG_RUNTIME_DIR", dir)
	path := filepath.Join(dir, "omnibar", "omnibar.sock")
	return path, func() {
		if old == "" {
			os.Unsetenv("XDG_RUNTIME_DIR")
		} else {
			os.Setenv("XDG_RUNTIME_DIR", old)
		}
	}
}

func TestSocketPathMissingXDG(t *testing.T) {
	old := os.Getenv("XDG_RUNTIME_DIR")
	os.Unsetenv("XDG_RUNTIME_DIR")
	defer func() {
		if old == "" {
			os.Unsetenv("XDG_RUNTIME_DIR")
		} else {
			os.Setenv("XDG_RUNTIME_DIR", old)
		}
	}()

	_, err := SocketPath()
	if err == nil {
		t.Fatal("expected error when XDG_RUNTIME_DIR is unset")
	}
	if !strings.Contains(err.Error(), "XDG_RUNTIME_DIR is not set") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSocketPathValid(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	actual, err := SocketPath()
	if err != nil {
		t.Fatalf("SocketPath() returned error: %v", err)
	}
	expected := filepath.Join(filepath.Dir(path), "omnibar.sock")
	if actual != expected {
		t.Errorf("SocketPath() = %q, want %q", actual, expected)
	}
	if !strings.HasSuffix(actual, "omnibar"+string(os.PathSeparator)+"omnibar.sock") {
		t.Errorf("unexpected socket path: %s", actual)
	}
}

func TestEnsureRuntimeDirCreates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "omnibar", "omnibar.sock")

	if err := EnsureRuntimeDir(path); err != nil {
		t.Fatalf("EnsureRuntimeDir() returned error: %v", err)
	}

	info, err := os.Stat(filepath.Dir(path))
	if err != nil {
		t.Fatalf("stat dir: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory to exist")
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("directory permissions = %o, want 0700", info.Mode().Perm())
	}
}

func TestEnsureRuntimeDirUnsafeExisting(t *testing.T) {
	dir := t.TempDir()
	runtimeDir := filepath.Join(dir, "omnibar")
	if err := os.MkdirAll(runtimeDir, 0o777); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(runtimeDir, "omnibar.sock")

	err := EnsureRuntimeDir(path)
	if err == nil {
		t.Fatal("expected error for unsafe directory permissions")
	}
	if !strings.Contains(err.Error(), "unsafe permissions") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestServerStartAndShutdown(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)

	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Verify socket file exists with 0600.
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("socket file does not exist: %v", err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Errorf("socket permissions = %o, want 0600", info.Mode().Perm())
	}

	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}

	// Socket file should be removed.
	if _, err := os.Stat(path); err == nil {
		t.Error("socket file still exists after shutdown")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected error stat-ing socket: %v", err)
	}
}

func TestServerShutdownDoesNotLogClientTeardownError(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, logs := newTestLogger()
	handled := make(chan struct{}, 1)
	s := New(path, logger, func([]byte) ([]byte, error) {
		select {
		case handled <- struct{}{}:
		default:
		}
		return nil, nil
	})
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write frame: %v", err)
	}
	select {
	case <-handled:
	case <-time.After(time.Second):
		t.Fatal("handler was not called")
	}

	// The handler loop is now blocked reading. Shutdown closes the
	// connection; that expected teardown must not be logged as an error.
	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}
	if strings.Contains(logs.String(), "read frame error") {
		t.Errorf("shutdown logged a client teardown error:\n%s", logs.String())
	}
}

func TestServerSecondInstanceRejected(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger1, _ := newTestLogger()
	s1 := New(path, logger1, nil)
	if err := s1.Start(); err != nil {
		t.Fatalf("first Start() returned error: %v", err)
	}
	defer s1.Shutdown()

	logger2, _ := newTestLogger()
	s2 := New(path, logger2, nil)
	err := s2.Start()
	if err == nil {
		t.Fatal("expected error for second server instance")
	}
	if !strings.Contains(err.Error(), "another instance is already running") {
		t.Errorf("expected already-running error, got: %v", err)
	}
	s2.Shutdown() // should be safe to call on unstarted server
}

func TestServerConcurrentStartup(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger1, _ := newTestLogger()
	logger2, _ := newTestLogger()

	s1 := New(path, logger1, nil)
	s2 := New(path, logger2, nil)

	errCh := make(chan error, 2)
	go func() { errCh <- s1.Start() }()
	go func() { errCh <- s2.Start() }()

	err1 := <-errCh
	err2 := <-errCh

	// Exactly one should succeed due to the exclusive lock.
	successCount := 0
	var successServer *Server
	if err1 == nil {
		successCount++
		successServer = s1
		defer s1.Shutdown()
	} else {
		defer s2.Shutdown()
	}
	if err2 == nil {
		successCount++
		successServer = s2
		defer s2.Shutdown()
	} else {
		defer s1.Shutdown()
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one successful startup, got %d (err1=%v, err2=%v)", successCount, err1, err2)
	}
	_ = successServer
}

func TestServerStaleSocketRecovery(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Create a stale socket file (no listener).
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Create a real Unix socket then close it without listening.
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln.Close() // leaves a stale socket file

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() should recover from stale socket: %v", err)
	}
	defer s.Shutdown()

	// Verify we can connect.
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()
}

func TestServerSymlinkNotRemoved(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Create a symlink at the socket path pointing elsewhere.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(filepath.Dir(path), "target.sock")
	if err := os.Symlink(target, path); err != nil {
		t.Fatalf("symlink: %v", err)
	}

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	err := s.Start()
	if err == nil {
		t.Fatal("expected error when symlink exists at socket path")
	}
	s.Shutdown()

	// Symlink should still exist and not have been followed or removed.
	if _, err := os.Lstat(path); err != nil {
		t.Errorf("symlink should not be removed: %v", err)
	}
}

func TestServerRegularFileNotDeleted(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Create a regular file at the socket path.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("not a socket"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	err := s.Start()
	if err == nil {
		t.Fatal("expected error when regular file exists at socket path")
	}
	s.Shutdown()

	// Regular file should still exist.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("regular file should not be deleted: %v", err)
	}
}

func TestServerReplacementEntryNotRemoved(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Replace the socket with a regular file while the server is running.
	s.listener.Close()
	os.Remove(path)
	if err := os.WriteFile(path, []byte("replacement"), 0o644); err != nil {
		t.Fatalf("write replacement: %v", err)
	}

	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}

	// The replacement file should still exist; shutdown should not
	// remove it because ownership verification fails.
	if _, err := os.Stat(path); err != nil {
		t.Errorf("replacement file should not be removed: %v", err)
	}
}

func TestServerFailedStartupNoSocketLeftBehind(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Pre-create a regular file at the socket path so startup fails.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	err := s.Start()
	if err == nil {
		t.Fatal("expected startup failure when regular file blocks socket path")
	}

	// The lock was released; no socket should have been created.
	info, err := os.Lstat(path)
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}
	if err == nil {
		// The path should still be a regular file, not a socket.
		if info.Mode()&os.ModeSocket != 0 {
			t.Error("a socket should not have been created on failed startup")
		}
	}
}

func TestServerShutdownUnstartedSafe(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)

	// Shutdown on an unstarted server must not close stdin or any
	// other unrelated descriptor, and must not return an error.
	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() on unstarted server returned error: %v", err)
	}

	// Verify stdin (fd 0) is still open. fcntl(F_GETFD) checks
	// descriptor validity without blocking waiting for data.
	if _, _, errno := syscall.Syscall(syscall.SYS_FCNTL, 0, uintptr(syscall.F_GETFD), 0); errno != 0 {
		t.Fatalf("stdin (fd 0) was closed by Shutdown() on an unstarted server: %v", errno)
	}
}

func TestServerStartAfterShutdown(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger1, _ := newTestLogger()
	s1 := New(path, logger1, nil)
	if err := s1.Start(); err != nil {
		t.Fatalf("first Start() returned error: %v", err)
	}
	if err := s1.Shutdown(); err != nil {
		t.Fatalf("first Shutdown() returned error: %v", err)
	}

	// A second server should be able to start after the first shuts down
	// and releases the lock.
	logger2, _ := newTestLogger()
	s2 := New(path, logger2, nil)
	if err := s2.Start(); err != nil {
		t.Fatalf("second Start() after shutdown returned error: %v", err)
	}
	defer s2.Shutdown()
}

func TestServerStartAfterFailedStartup(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Block the socket path with a regular file so the first startup fails.
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte("blocker"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	logger1, _ := newTestLogger()
	s1 := New(path, logger1, nil)
	if err := s1.Start(); err == nil {
		t.Fatal("expected startup failure when regular file blocks socket path")
	}
	s1.Shutdown()

	// Remove the blocker; a second startup should now succeed.
	if err := os.Remove(path); err != nil {
		t.Fatalf("remove blocker: %v", err)
	}

	logger2, _ := newTestLogger()
	s2 := New(path, logger2, nil)
	if err := s2.Start(); err != nil {
		t.Fatalf("second Start() after failure returned error: %v", err)
	}
	defer s2.Shutdown()
}

func TestServerStaleRecoveryCompetition(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Pre-create a stale socket (no listener).
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	ln.Close() // leaves a stale socket file

	logger1, _ := newTestLogger()
	logger2, _ := newTestLogger()
	s1 := New(path, logger1, nil)
	s2 := New(path, logger2, nil)

	errCh := make(chan error, 2)
	go func() { errCh <- s1.Start() }()
	go func() { errCh <- s2.Start() }()

	err1 := <-errCh
	err2 := <-errCh

	// Exactly one should win the lock and recover the stale socket.
	successCount := 0
	if err1 == nil {
		successCount++
		defer s1.Shutdown()
	}
	if err2 == nil {
		successCount++
		defer s2.Shutdown()
	}
	if successCount != 1 {
		t.Fatalf("expected exactly one successful startup, got %d (err1=%v, err2=%v)", successCount, err1, err2)
	}

	// The winning server's socket should be live and connectable.
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial to winning server socket: %v", err)
	}
	conn.Close()
}

func TestLockHeldUntilSocketCleanup(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// While the server is running, the lock should be held: a second
	// server cannot start.
	logger2, _ := newTestLogger()
	s2 := New(path, logger2, nil)
	err := s2.Start()
	if err == nil {
		t.Fatal("expected error: lock should be held while server is running")
	}

	// After shutdown, the lock should be released and the socket removed.
	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}

	// Socket should be gone.
	if _, err := os.Lstat(path); err == nil {
		t.Error("socket file should have been removed after shutdown")
	} else if !os.IsNotExist(err) {
		t.Fatalf("unexpected stat error: %v", err)
	}

	// A new server should now be able to start.
	logger3, _ := newTestLogger()
	s3 := New(path, logger3, nil)
	if err := s3.Start(); err != nil {
		t.Fatalf("server start after shutdown returned error: %v", err)
	}
	defer s3.Shutdown()
}

func TestServerMultipleClients(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Echo handler for testing.
	handler := func(msg []byte) ([]byte, error) {
		return append([]byte("echo:"), msg...), nil
	}
	logger, _ := newTestLogger()
	s := New(path, logger, handler)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer s.Shutdown()

	// Connect two clients.
	conn1, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("client1 dial: %v", err)
	}
	conn2, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("client2 dial: %v", err)
	}
	defer conn1.Close()
	defer conn2.Close()

	// Send from client1, receive echo.
	if _, err := conn1.Write([]byte("ping1\n")); err != nil {
		t.Fatalf("client1 write: %v", err)
	}
	buf := make([]byte, 1024)
	n, err := conn1.Read(buf)
	if err != nil {
		t.Fatalf("client1 read: %v", err)
	}
	if string(buf[:n]) != "echo:ping1\n" {
		t.Errorf("client1 got %q, want %q", buf[:n], "echo:ping1\n")
	}

	// Send from client2, receive echo.
	if _, err := conn2.Write([]byte("ping2\n")); err != nil {
		t.Fatalf("client2 write: %v", err)
	}
	n, err = conn2.Read(buf)
	if err != nil {
		t.Fatalf("client2 read: %v", err)
	}
	if string(buf[:n]) != "echo:ping2\n" {
		t.Errorf("client2 got %q, want %q", buf[:n], "echo:ping2\n")
	}
}

func TestServerClientDisconnectDoesNotStopServer(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer s.Shutdown()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close() // client disconnects

	// Server should still accept new connections.
	conn2, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("second dial after client disconnect: %v", err)
	}
	conn2.Close()
}

func TestServerMultipleMessagesOneConnection(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	handler := func(msg []byte) ([]byte, error) {
		return append([]byte("r:"), msg...), nil
	}
	logger, _ := newTestLogger()
	s := New(path, logger, handler)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer s.Shutdown()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Send multiple messages.
	msgs := []string{"one\n", "two\n", "three\n"}
	for _, m := range msgs {
		if _, err := conn.Write([]byte(m)); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	// Read multiple replies and verify their contents.
	reader := bufio.NewReaderSize(conn, 64*1024)
	expected := []string{"r:one", "r:two", "r:three"}
	for i, exp := range expected {
		line, err := reader.ReadString('\n')
		if err != nil {
			t.Fatalf("read %d: %v", i, err)
		}
		if strings.TrimRight(line, "\n") != exp {
			t.Errorf("reply %d = %q, want %q", i, strings.TrimRight(line, "\n"), exp)
		}
	}
}

func TestServerShutdownClosesConnections(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}

	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}

	// Connection should be closed.
	buf := make([]byte, 1)
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected connection to be closed after shutdown")
	}
}

func TestServerNoGoroutineLeak(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	// Connect and disconnect a client.
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	conn.Close()

	// Give goroutines time to clean up.
	time.Sleep(50 * time.Millisecond)

	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}
}

func TestReadFrameWithConn(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Echo handler so the server sends data back.
	handler := func(msg []byte) ([]byte, error) {
		return msg, nil
	}
	logger, _ := newTestLogger()
	s := New(path, logger, handler)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer s.Shutdown()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	// Write a frame.
	msg := []byte("test frame\n")
	if _, err := conn.Write(msg); err != nil {
		t.Fatalf("write: %v", err)
	}

	// Read the echo back using ReadFrame with a persistent bufio.Reader.
	reader := bufio.NewReaderSize(conn, 64*1024)
	frame, err := ReadFrame(reader)
	if err != nil {
		t.Fatalf("ReadFrame() returned error: %v", err)
	}
	if !bytes.Equal(frame, []byte("test frame")) {
		t.Errorf("ReadFrame() = %q, want %q", frame, "test frame")
	}
}

// waitForState waits until the server reaches want, failing the test if
// it does not happen within a generous timeout.
func waitForState(t *testing.T, s *Server, want lifecycleState) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		s.mu.Lock()
		got := s.state
		s.mu.Unlock()
		if got == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("server state = %v, want %v", got, want)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestServerLifecycleStateTransitions(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)

	// A new server begins in the new/unstarted state.
	s.mu.Lock()
	if s.state != stateNew {
		s.mu.Unlock()
		t.Fatalf("new server state = %v, want stateNew", s.state)
	}
	s.mu.Unlock()

	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}

	s.mu.Lock()
	if s.state != stateRunning {
		s.mu.Unlock()
		t.Fatalf("state after Start() = %v, want stateRunning", s.state)
	}
	s.mu.Unlock()

	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}

	s.mu.Lock()
	if s.state != stateStopped {
		s.mu.Unlock()
		t.Fatalf("state after Shutdown() = %v, want stateStopped", s.state)
	}
	s.mu.Unlock()
}

func TestServerSecondStartFails(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("first Start() returned error: %v", err)
	}
	defer s.Shutdown()

	err := s.Start()
	if err == nil {
		t.Fatal("second Start() on the same instance returned nil")
	}
	if !strings.Contains(err.Error(), "already started") {
		t.Errorf("second Start() error = %v, want already-started error", err)
	}
}

func TestServerShutdownBeforeStartSafeAndIdempotent(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)

	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() before Start() returned error: %v", err)
	}
	if err := s.Shutdown(); err != nil {
		t.Fatalf("repeated Shutdown() returned error: %v", err)
	}

	s.mu.Lock()
	state := s.state
	s.mu.Unlock()
	if state != stateStopped {
		t.Errorf("state after Shutdown() before Start() = %v, want stateStopped", state)
	}
}

func TestServerStartAfterShutdownFails(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if err := s.Shutdown(); err != nil {
		t.Fatalf("Shutdown() returned error: %v", err)
	}

	err := s.Start()
	if err == nil {
		t.Fatal("Start() after Shutdown() returned nil")
	}
	if !strings.Contains(err.Error(), "already stopped") {
		t.Errorf("Start() after Shutdown() error = %v, want already-stopped error", err)
	}
}

func TestServerRepeatedShutdownSafe(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger, _ := newTestLogger()
	s := New(path, logger, nil)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	for i := 0; i < 3; i++ {
		if err := s.Shutdown(); err != nil {
			t.Fatalf("Shutdown() call %d returned error: %v", i+1, err)
		}
	}
}

func TestServerNewInstanceAfterShutdown(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	logger1, _ := newTestLogger()
	s1 := New(path, logger1, nil)
	if err := s1.Start(); err != nil {
		t.Fatalf("first Start() returned error: %v", err)
	}
	if err := s1.Shutdown(); err != nil {
		t.Fatalf("first Shutdown() returned error: %v", err)
	}

	logger2, _ := newTestLogger()
	s2 := New(path, logger2, nil)
	if err := s2.Start(); err != nil {
		t.Fatalf("Start() on a new instance after shutdown returned error: %v", err)
	}
	defer s2.Shutdown()
}

// fakeListener is a test-controlled net.Listener. It lets a test deliver
// a connection to the accept loop at a precise point during shutdown.
type fakeListener struct {
	acceptCh    chan net.Conn
	unblockCh   chan struct{}
	unblockOnce sync.Once
}

func newFakeListener() *fakeListener {
	return &fakeListener{
		acceptCh:  make(chan net.Conn),
		unblockCh: make(chan struct{}),
	}
}

func (f *fakeListener) Accept() (net.Conn, error) {
	select {
	case conn := <-f.acceptCh:
		return conn, nil
	case <-f.unblockCh:
		return nil, net.ErrClosed
	}
}

// Close is a no-op: the test controls Accept unblocking via unblock so a
// connection can be delivered after Shutdown has marked the server
// stopped.
func (f *fakeListener) Close() error { return nil }

func (f *fakeListener) unblock() {
	f.unblockOnce.Do(func() { close(f.unblockCh) })
}

func (f *fakeListener) Addr() net.Addr { return fakeAddr{} }

type fakeAddr struct{}

func (fakeAddr) Network() string { return "unix" }
func (fakeAddr) String() string  { return "fake" }

// TestServerShutdownAcceptRace deterministically exercises a connection
// delivered to Accept while Shutdown is in progress. The connection must
// be closed without being registered or starting a handler, and Shutdown
// must complete without leaking a handler.
func TestServerShutdownAcceptRace(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	handlerStarted := make(chan struct{}, 1)
	handler := func([]byte) ([]byte, error) {
		handlerStarted <- struct{}{}
		return nil, nil
	}

	logger, _ := newTestLogger()
	s := New(path, logger, handler)

	fake := newFakeListener()
	serverConn, clientConn := net.Pipe()
	defer clientConn.Close()
	defer serverConn.Close()

	// Install the test listener and start the real accept loop.
	s.mu.Lock()
	s.state = stateRunning
	s.listener = fake
	s.mu.Unlock()

	s.wg.Add(1)
	go s.acceptLoop()

	shutdownErr := make(chan error, 1)
	go func() {
		shutdownErr <- s.Shutdown()
	}()

	// Wait until Shutdown has marked the server stopped. Any connection
	// accepted from this point on must be closed without a handler.
	waitForState(t, s, stateStopped)

	// Deliver a connection to Accept while Shutdown is in progress.
	fake.acceptCh <- serverConn

	// If the connection were incorrectly registered, the handler would
	// read this frame and signal.
	_ = clientConn.SetWriteDeadline(time.Now().Add(5 * time.Second))
	_, _ = clientConn.Write([]byte("ping\n"))

	// The connection must be closed by the accept loop.
	_ = clientConn.SetReadDeadline(time.Now().Add(5 * time.Second))
	if _, err := clientConn.Read(make([]byte, 1)); err == nil {
		t.Fatal("connection accepted during shutdown was not closed")
	}

	select {
	case <-handlerStarted:
		t.Fatal("handler started for a connection accepted during shutdown")
	default:
	}

	// Let the accept loop observe listener closure and exit so Shutdown
	// can finish waiting.
	fake.unblock()

	select {
	case err := <-shutdownErr:
		if err != nil {
			t.Fatalf("Shutdown() returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Shutdown() did not complete; a handler leaked")
	}

	s.mu.Lock()
	remaining := len(s.conns)
	s.mu.Unlock()
	if remaining != 0 {
		t.Errorf("connection accepted during shutdown was registered: %d remaining", remaining)
	}
}

func TestServerJSONRPCMultipleRequestsOneConnection(t *testing.T) {
	path, restore := tempSocketPath(t)
	defer restore()

	// Echo handler: returns params unchanged.
	d := jsonrpc.NewDispatcher()
	d.Register("echo", func(params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		return params, nil
	})
	// Register handshake so the server speaks the full protocol.
	RegisterHandshake(d, modules.NewRegistry(nil, nil))

	logger, _ := newTestLogger()
	s := New(path, logger, d.Handle)
	if err := s.Start(); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	defer s.Shutdown()

	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	writer := bufio.NewWriterSize(conn, 64*1024)
	reader := bufio.NewReaderSize(conn, 64*1024)

	send := func(id int, method string, params json.RawMessage) {
		req := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
			"method":  method,
		}
		if params != nil {
			req["params"] = params
		}
		data, _ := json.Marshal(req)
		data = append(data, '\n')
		if _, err := writer.Write(data); err != nil {
			t.Fatalf("write: %v", err)
		}
		if err := writer.Flush(); err != nil {
			t.Fatalf("flush: %v", err)
		}
	}

	// Send multiple sequential requests over the same connection.
	// Request 1: handshake.
	send(1, "client.handshake", []byte(`{"protocol":"omnibar.ipc/v1","client_id":"ui-1"}`))
	resp1 := readResponse(t, reader)
	if string(resp1.ID) != "1" || resp1.Error != nil {
		t.Fatalf("handshake response = %+v", resp1)
	}

	// Request 2: echo with string result.
	send(2, "echo", []byte(`"hello"`))
	resp2 := readResponse(t, reader)
	if string(resp2.ID) != "2" || resp2.Error != nil || string(resp2.Result) != `"hello"` {
		t.Fatalf("echo response = %+v", resp2)
	}

	// Request 3: unknown method.
	send(3, "no.such.method", nil)
	resp3 := readResponse(t, reader)
	if string(resp3.ID) != "3" || resp3.Error == nil || resp3.Error.Code != jsonrpc.CodeMethodNotFound {
		t.Fatalf("unknown method response = %+v", resp3)
	}

	// Request 4: missing jsonrpc version (invalid request).
	raw4 := `{"id":4,"method":"echo"}` + "\n"
	if _, err := writer.Write([]byte(raw4)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	resp4 := readResponse(t, reader)
	if string(resp4.ID) != "4" || resp4.Error == nil || resp4.Error.Code != jsonrpc.CodeInvalidRequest {
		t.Fatalf("invalid request response = %+v", resp4)
	}

	// Notification: no response expected.
	if _, err := writer.Write([]byte(`{"jsonrpc":"2.0","method":"echo","params":{}}\n`)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writer.Flush(); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// No frame should arrive for the notification.
	conn.SetReadDeadline(nowPlus(100 * time.Millisecond))
	_, err = reader.ReadByte()
	if err == nil {
		t.Error("expected no response for notification")
	}
}

// readResponse reads one JSON-RPC response frame from reader.
func readResponse(t *testing.T, r *bufio.Reader) jsonrpc.Response {
	t.Helper()
	frame, err := ReadFrame(r)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	var resp jsonrpc.Response
	if err := json.Unmarshal(frame, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	return resp
}

// nowPlus returns the current time plus d.
func nowPlus(d time.Duration) time.Time {
	return time.Now().Add(d)
}
