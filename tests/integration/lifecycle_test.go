package integration

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGracefulShutdownRemovesSocket(t *testing.T) {
	env := newTestEnv(t)
	p := startBackend(t, env)

	c := dial(t, env.socketPath)
	handshake(t, c)

	p.stopGracefully(t)

	if _, err := os.Lstat(env.socketPath); !os.IsNotExist(err) {
		t.Fatalf("socket still exists after graceful shutdown: %v", err)
	}
	c.expectClosed(2 * time.Second)

	// The lock file is intentionally retained to avoid inode races.
	lockPath := filepath.Join(filepath.Dir(env.socketPath), "omnibar.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Errorf("lock file should remain after shutdown: %v", err)
	}

	if log := p.output(); !strings.Contains(log, "ipc server shutdown") {
		t.Errorf("backend log does not contain shutdown message:\n%s", log)
	}
	if log := p.output(); strings.Contains(log, "level=ERROR") {
		t.Errorf("graceful shutdown should not log errors:\n%s", log)
	}
}

func TestRestartReconnectsAndRehandshakes(t *testing.T) {
	env := newTestEnv(t)

	p1 := startBackend(t, env)
	c1 := dial(t, env.socketPath)
	handshake(t, c1)
	p1.stopGracefully(t)
	c1.expectClosed(2 * time.Second)

	// Start a replacement backend on the same runtime directory and model
	// the UI's reconnect loop: connect, handshake again, then query state.
	p2 := startBackend(t, env)
	c2 := dialWithRetry(t, env.socketPath, 5*time.Second)
	handshake(t, c2)

	resp, _ := c2.request("module.getStatus", map[string]string{"module": "core"})
	if resp.Error != nil {
		t.Fatalf("module.getStatus after restart failed: %+v", resp.Error)
	}

	p2.stopGracefully(t)
}

func TestCrashRestartRecoversStaleSocket(t *testing.T) {
	env := newTestEnv(t)

	p1 := startBackend(t, env)
	c1 := dial(t, env.socketPath)
	handshake(t, c1)

	// SIGKILL leaves the socket file behind: the next startup must detect
	// it as stale, remove it, and bind successfully.
	p1.killHard(t)
	if code := p1.exitCode(); code == 0 {
		t.Fatal("killed backend exited with code 0")
	}
	c1.expectClosed(2 * time.Second)

	if _, err := os.Lstat(env.socketPath); err != nil {
		t.Fatalf("expected the crashed backend to leave a stale socket: %v", err)
	}

	p2 := startBackend(t, env)
	c2 := dial(t, env.socketPath)
	handshake(t, c2)

	resp, _ := c2.request("module.getStatus", map[string]string{"module": "core"})
	if resp.Error != nil {
		t.Fatalf("module.getStatus after crash restart failed: %+v", resp.Error)
	}

	p2.stopGracefully(t)
}

func TestSecondInstanceRejected(t *testing.T) {
	env := newTestEnv(t)

	p1 := startBackend(t, env)
	c := dial(t, env.socketPath)
	handshake(t, c)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, backendBinary, "-config", env.configPath)
	cmd.Env = env.backendEnv()
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output

	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("second backend did not exit: %v\n%s", ctx.Err(), output.String())
	}
	if err == nil {
		t.Fatalf("second backend exited successfully, want single-instance failure\n%s", output.String())
	}
	if !strings.Contains(output.String(), "another instance is already running") {
		t.Errorf("second backend log does not mention the single-instance lock:\n%s", output.String())
	}

	// The first instance must remain fully usable.
	resp, _ := c.request("module.getStatus", map[string]string{"module": "core"})
	if resp.Error != nil {
		t.Fatalf("first backend was disturbed by the second instance: %+v", resp.Error)
	}

	p1.stopGracefully(t)
}
