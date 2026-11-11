package integration

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"

	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"omnibar/internal/ipc/jsonrpc"
)

const (
	// startupTimeout bounds how long a spawned backend may take to create
	// its socket.
	startupTimeout = 10 * time.Second

	// responseTimeout bounds a normal request/response round trip.
	responseTimeout = 3 * time.Second

	// noResponseWindow is how long we wait to prove that a notification
	// produced no response.
	noResponseWindow = 250 * time.Millisecond

	// shutdownTimeout bounds graceful process shutdown.
	shutdownTimeout = 10 * time.Second
)

// backendBinary is built once by TestMain and reused by every test.
var backendBinary string

// TestMain builds the real backend binary from cmd/omnibar. Building in
// TestMain keeps the tests at the process boundary: the same binary, socket,
// framing, and dispatcher code paths that the UI uses.
func TestMain(m *testing.M) {
	root, err := repoRoot()
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: locate repository root: %v\n", err)
		os.Exit(1)
	}

	dir, err := os.MkdirTemp("", "omnibar-integration-bin-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "integration: create build directory: %v\n", err)
		os.Exit(1)
	}

	backendBinary = filepath.Join(dir, "omnibar")
	build := exec.Command("go", "build", "-o", backendBinary, "./cmd/omnibar")
	build.Dir = root
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "integration: build backend: %v\n%s", err, out)
		os.RemoveAll(dir)
		os.Exit(1)
	}

	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

// repoRoot returns the module root by walking up from the working directory
// (`go test` runs in the package directory) to the first go.mod. It does not
// use runtime.Caller because -trimpath, which package builds use, makes that
// path relative.
func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found in any parent of the working directory")
		}
		dir = parent
	}
}

// testEnv is an isolated environment for one backend instance. runtimeDir
// and configDir are separate from the developer's real session so tests never
// touch the live Omnibar socket or configuration.
type testEnv struct {
	runtimeDir string
	configDir  string
	configPath string
	socketPath string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	base := t.TempDir()
	env := &testEnv{
		runtimeDir: filepath.Join(base, "run"),
		configDir:  filepath.Join(base, "config"),
		configPath: filepath.Join(base, "config", "config.toml"),
	}
	if err := os.MkdirAll(env.runtimeDir, 0o700); err != nil {
		t.Fatalf("create runtime dir: %v", err)
	}
	if err := os.MkdirAll(env.configDir, 0o700); err != nil {
		t.Fatalf("create config dir: %v", err)
	}
	env.socketPath = filepath.Join(env.runtimeDir, "omnibar", "omnibar.sock")
	return env
}

// writeConfig writes config.toml. Calling it more than once replaces the
// file; restart tests intentionally reuse the same path.
func (e *testEnv) writeConfig(t *testing.T, content string) {
	t.Helper()
	if err := os.WriteFile(e.configPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// backendEnv returns the parent environment with XDG paths replaced so the
// spawned backend cannot see the developer's real runtime or config.
func (e *testEnv) backendEnv() []string {
	out := make([]string, 0, len(os.Environ())+2)
	for _, kv := range os.Environ() {
		if strings.HasPrefix(kv, "XDG_RUNTIME_DIR=") ||
			strings.HasPrefix(kv, "XDG_CONFIG_HOME=") ||
			strings.HasPrefix(kv, "XDG_STATE_HOME=") {
			continue
		}
		out = append(out, kv)
	}
	return append(out,
		"XDG_RUNTIME_DIR="+e.runtimeDir,
		"XDG_CONFIG_HOME="+e.configDir,
	)
}

// backendProcess is a spawned backend with output captured to a file.
type backendProcess struct {
	cmd     *exec.Cmd
	logPath string
	done    chan struct{}

	mu      sync.Mutex
	waitErr error
}

// startBackend spawns the built backend and waits until its socket accepts
// connections. A cleanup hook kills the process if a test fails early.
func startBackend(t *testing.T, env *testEnv) *backendProcess {
	t.Helper()

	logPath := filepath.Join(t.TempDir(), "backend.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		t.Fatalf("create backend log: %v", err)
	}

	cmd := exec.Command(backendBinary, "-config", env.configPath)
	cmd.Env = env.backendEnv()
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		logFile.Close()
		t.Fatalf("start backend: %v", err)
	}
	logFile.Close()

	p := &backendProcess{
		cmd:     cmd,
		logPath: logPath,
		done:    make(chan struct{}),
	}
	go func() {
		err := cmd.Wait()
		p.mu.Lock()
		p.waitErr = err
		p.mu.Unlock()
		close(p.done)
	}()

	t.Cleanup(func() {
		if !p.exited() {
			_ = p.cmd.Process.Kill()
			<-p.done
		}
	})

	waitForSocket(t, env.socketPath, p)
	return p
}

func (p *backendProcess) exited() bool {
	select {
	case <-p.done:
		return true
	default:
		return false
	}
}

func (p *backendProcess) wait(timeout time.Duration) error {
	select {
	case <-p.done:
		p.mu.Lock()
		defer p.mu.Unlock()
		return p.waitErr
	case <-time.After(timeout):
		return fmt.Errorf("process did not exit within %s", timeout)
	}
}

func (p *backendProcess) exitCode() int {
	<-p.done
	if p.cmd.ProcessState == nil {
		return -1
	}
	return p.cmd.ProcessState.ExitCode()
}

func (p *backendProcess) output() string {
	data, err := os.ReadFile(p.logPath)
	if err != nil {
		return fmt.Sprintf("<could not read backend log: %v>", err)
	}
	return string(data)
}

// stopGracefully sends SIGTERM and requires a zero exit status.
func (p *backendProcess) stopGracefully(t *testing.T) {
	t.Helper()

	if err := p.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM: %v", err)
	}
	if err := p.wait(shutdownTimeout); err != nil {
		_ = p.cmd.Process.Kill()
		<-p.done
		t.Fatalf("%v\n%s", err, p.output())
	}
	if code := p.exitCode(); code != 0 {
		t.Fatalf("backend exited with code %d, want 0\n%s", code, p.output())
	}
}

// killHard simulates a crash. The socket file is intentionally left behind.
func (p *backendProcess) killHard(t *testing.T) {
	t.Helper()

	if err := p.cmd.Process.Kill(); err != nil {
		t.Fatalf("kill backend: %v", err)
	}
	<-p.done
}

func waitForSocket(t *testing.T, socketPath string, p *backendProcess) {
	t.Helper()

	deadline := time.Now().Add(startupTimeout)
	for time.Now().Before(deadline) {
		if p.exited() {
			p.mu.Lock()
			err := p.waitErr
			p.mu.Unlock()
			t.Fatalf("backend exited before its socket was ready: %v\n%s", err, p.output())
		}

		conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if err == nil {
			conn.Close()
			return
		}
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatalf("socket %s did not become available within %s\n%s", socketPath, startupTimeout, p.output())
}

// rpcClient is a minimal JSON-RPC 2.0 client over the real Unix socket. It
// deliberately uses only the wire format, the same way the QML UI does.
type rpcClient struct {
	t      *testing.T
	conn   net.Conn
	reader *bufio.Reader
	nextID int
}

func newRPCClient(t *testing.T, conn net.Conn) *rpcClient {
	t.Helper()

	c := &rpcClient{
		t:      t,
		conn:   conn,
		reader: bufio.NewReader(conn),
	}
	t.Cleanup(func() { conn.Close() })
	return c
}

func dial(t *testing.T, socketPath string) *rpcClient {
	t.Helper()

	conn, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if err != nil {
		t.Fatalf("dial %s: %v", socketPath, err)
	}
	return newRPCClient(t, conn)
}

// dialWithRetry models the UI's reconnect behavior: retry with a short
// backoff until the restarted backend accepts connections.
func dialWithRetry(t *testing.T, socketPath string, timeout time.Duration) *rpcClient {
	t.Helper()

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("unix", socketPath, 100*time.Millisecond)
		if err == nil {
			return newRPCClient(t, conn)
		}
		lastErr = err
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("could not connect to %s within %s: %v", socketPath, timeout, lastErr)
	return nil
}

type requestFrame struct {
	JSONRPC string `json:"jsonrpc"`
	ID      int    `json:"id"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

type notificationFrame struct {
	JSONRPC string `json:"jsonrpc"`
	Method  string `json:"method"`
	Params  any    `json:"params,omitempty"`
}

func (c *rpcClient) writeRaw(frame string) {
	c.t.Helper()

	if !strings.HasSuffix(frame, "\n") {
		frame += "\n"
	}
	if err := c.conn.SetWriteDeadline(time.Now().Add(responseTimeout)); err != nil {
		c.t.Fatalf("set write deadline: %v", err)
	}
	if _, err := io.WriteString(c.conn, frame); err != nil {
		c.t.Fatalf("write frame: %v", err)
	}
	if err := c.conn.SetWriteDeadline(time.Time{}); err != nil {
		c.t.Fatalf("clear write deadline: %v", err)
	}
}

func (c *rpcClient) writeJSON(v any) {
	c.t.Helper()

	data, err := json.Marshal(v)
	if err != nil {
		c.t.Fatalf("marshal request: %v", err)
	}
	c.writeRaw(string(data))
}

// request sends a request and verifies the JSON-RPC response envelope:
// matching id, jsonrpc 2.0, and no method field.
func (c *rpcClient) request(method string, params any) (jsonrpc.Response, map[string]json.RawMessage) {
	c.t.Helper()

	c.nextID++
	id := c.nextID
	c.writeJSON(requestFrame{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	})

	resp, raw := c.readFrame()
	if resp.JSONRPC != "2.0" {
		c.t.Errorf("response jsonrpc = %q, want %q", resp.JSONRPC, "2.0")
	}
	if _, hasMethod := raw["method"]; hasMethod {
		c.t.Errorf("response must not contain a method field: %s", rawJSON(raw))
	}

	var gotID int
	if err := json.Unmarshal(resp.ID, &gotID); err != nil {
		c.t.Fatalf("unmarshal response id %s: %v", resp.ID, err)
	}
	if gotID != id {
		c.t.Errorf("response id = %d, want %d", gotID, id)
	}

	return resp, raw
}

// notify sends a JSON-RPC notification (no id) and expects no response.
func (c *rpcClient) notify(method string, params any) {
	c.t.Helper()

	c.writeJSON(notificationFrame{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	})
}

func (c *rpcClient) readFrame() (jsonrpc.Response, map[string]json.RawMessage) {
	c.t.Helper()

	if err := c.conn.SetReadDeadline(time.Now().Add(responseTimeout)); err != nil {
		c.t.Fatalf("set read deadline: %v", err)
	}
	line, err := c.reader.ReadBytes('\n')
	if err != nil {
		c.t.Fatalf("read response: %v", err)
	}
	if err := c.conn.SetReadDeadline(time.Time{}); err != nil {
		c.t.Fatalf("clear read deadline: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(line, &resp); err != nil {
		c.t.Fatalf("unmarshal response %q: %v", strings.TrimSpace(string(line)), err)
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(line, &raw); err != nil {
		c.t.Fatalf("unmarshal response object %q: %v", strings.TrimSpace(string(line)), err)
	}
	return resp, raw
}

// expectNoResponse proves the server sent nothing within window. It is used
// for JSON-RPC notifications.
func (c *rpcClient) expectNoResponse(window time.Duration) {
	c.t.Helper()

	if err := c.conn.SetReadDeadline(time.Now().Add(window)); err != nil {
		c.t.Fatalf("set read deadline: %v", err)
	}
	_, err := c.reader.ReadByte()
	if clearErr := c.conn.SetReadDeadline(time.Time{}); clearErr != nil {
		c.t.Fatalf("clear read deadline: %v", clearErr)
	}

	if err == nil {
		c.t.Fatal("expected no response, but the server sent data")
	}
	var netErr net.Error
	if !errors.As(err, &netErr) || !netErr.Timeout() {
		c.t.Fatalf("expected a read timeout proving no response, got: %v", err)
	}
}

// expectClosed waits for the server to close the connection.
func (c *rpcClient) expectClosed(window time.Duration) {
	c.t.Helper()

	if err := c.conn.SetReadDeadline(time.Now().Add(window)); err != nil {
		c.t.Fatalf("set read deadline: %v", err)
	}
	_, err := c.reader.ReadByte()
	if clearErr := c.conn.SetReadDeadline(time.Time{}); clearErr != nil {
		c.t.Fatalf("clear read deadline: %v", clearErr)
	}

	if err == nil {
		c.t.Fatal("expected the connection to be closed, but data was received")
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		c.t.Fatalf("connection was still open after %s", window)
	}
}

// handshake performs the M4 client.handshake and returns its result.
func handshake(t *testing.T, c *rpcClient) struct {
	Protocol string   `json:"protocol"`
	Version  string   `json:"version"`
	Modules  []string `json:"modules"`
} {
	t.Helper()

	resp, _ := c.request("client.handshake", map[string]string{
		"protocol":  "omnibar.ipc/v1",
		"client_id": "integration-test",
	})
	if resp.Error != nil {
		t.Fatalf("handshake failed: code=%d message=%q", resp.Error.Code, resp.Error.Message)
	}

	var result struct {
		Protocol string   `json:"protocol"`
		Version  string   `json:"version"`
		Modules  []string `json:"modules"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal handshake result: %v", err)
	}
	if result.Protocol != "omnibar.ipc/v1" {
		t.Fatalf("handshake protocol = %q, want %q", result.Protocol, "omnibar.ipc/v1")
	}
	if result.Version != "0.1.0" {
		t.Fatalf("handshake version = %q, want %q", result.Version, "0.1.0")
	}
	return result
}

func rawJSON(raw map[string]json.RawMessage) string {
	data, err := json.Marshal(raw)
	if err != nil {
		return fmt.Sprintf("<marshal: %v>", err)
	}
	return string(data)
}
