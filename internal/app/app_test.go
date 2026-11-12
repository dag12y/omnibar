package app

import (
	"bufio"
	"bytes"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"omnibar/internal/ipc"
	"omnibar/internal/ipc/jsonrpc"
)

func TestRunHelp(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	server, err := Run([]string{"-help"}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run(-help) returned error: %v", err)
	}
	if server != nil {
		t.Fatal("expected nil server for --help")
	}

	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("expected usage output, got: %q", out.String())
	}
	if errOut.Len() != 0 {
		t.Errorf("expected no stderr output, got: %q", errOut.String())
	}
}

func TestRunDoubleDashHelp(t *testing.T) {
	var out bytes.Buffer
	server, err := Run([]string{"--help"}, &out, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("Run(--help) returned error: %v", err)
	}
	if server != nil {
		t.Fatal("expected nil server for --help")
	}
	if !strings.Contains(out.String(), "Usage:") {
		t.Errorf("expected usage output, got: %q", out.String())
	}
}

func TestRunVersion(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	server, err := Run([]string{"-version"}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run(-version) returned error: %v", err)
	}
	if server != nil {
		t.Fatal("expected nil server for -version")
	}

	want := "omnibar " + ipc.ServerVersion + "\n"
	if out.String() != want {
		t.Errorf("version output = %q, want %q", out.String(), want)
	}
	if errOut.Len() != 0 {
		t.Errorf("expected no stderr output, got: %q", errOut.String())
	}
}

func TestRunNoArgs(t *testing.T) {
	var out bytes.Buffer
	var errOut bytes.Buffer

	server, err := Run([]string{}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run() with no args returned error: %v", err)
	}
	if server == nil {
		t.Fatal("expected non-nil server")
	}
	defer server.Shutdown()

	// Startup message is emitted via slog to stderr.
	if !strings.Contains(errOut.String(), "starting") {
		t.Errorf("expected startup message in stderr, got: %q", errOut.String())
	}
}

func TestRunParseError(t *testing.T) {
	// flag.Parse returns an error for unknown flags that look like
	// flags (start with - but aren't recognized). Usage goes to stderr.
	var out bytes.Buffer
	var errOut bytes.Buffer

	_, err := Run([]string{"-unknown-flag"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for unknown flag")
	}
	if out.Len() != 0 {
		t.Errorf("expected no stdout output, got: %q", out.String())
	}
	if errOut.Len() == 0 {
		t.Error("expected usage output on stderr for parse error")
	}
}

func TestRunInvalidConfig(t *testing.T) {
	// Create a temporary directory with a malformed config file.
	dir := t.TempDir()
	malformed := filepath.Join(dir, "config.toml")
	if err := os.WriteFile(malformed, []byte("= invalid toml ="), 0o644); err != nil {
		t.Fatalf("write malformed config: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer

	_, err := Run([]string{"-config", malformed}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for malformed config")
	}
	if !strings.Contains(err.Error(), "parse config file") {
		t.Errorf("expected parse error in result, got: %v", err)
	}
}

func TestRunValidConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
config_version = 1
[core]
  log_level = "debug"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer

	server, err := Run([]string{"-config", path}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run() with valid config returned error: %v", err)
	}
	if server == nil {
		t.Fatal("expected non-nil server")
	}
	defer server.Shutdown()

	if !strings.Contains(errOut.String(), "starting") {
		t.Errorf("expected startup message in stderr, got: %q", errOut.String())
	}
}

func TestRunInvalidLogLevel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	content := `
config_version = 1
[core]
  log_level = "not-a-level"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out bytes.Buffer
	var errOut bytes.Buffer

	_, err := Run([]string{"-config", path}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error for invalid log level")
	}
	if !strings.Contains(err.Error(), "validate configuration") {
		t.Errorf("expected validation error, got: %v", err)
	}
}

func TestIsHelpRequest(t *testing.T) {
	cases := []struct {
		args []string
		want bool
	}{
		{[]string{"-help"}, true},
		{[]string{"--help"}, true},
		{[]string{"-h"}, true},
		{[]string{"--h"}, true},
		{[]string{}, false},
		{[]string{"foo"}, false},
		{[]string{"-help", "foo"}, true},
	}

	for _, c := range cases {
		if got := isHelpRequest(c.args); got != c.want {
			t.Errorf("isHelpRequest(%v) = %v, want %v", c.args, got, c.want)
		}
	}
}

func TestArgsWithoutProgram(t *testing.T) {
	// No path separator: args returned unchanged.
	args := argsWithoutProgram([]string{"-help", "foo"})
	if len(args) != 2 || args[0] != "-help" || args[1] != "foo" {
		t.Errorf("argsWithoutProgram(%v) = %v, expected unchanged", []string{"-help", "foo"}, args)
	}

	// With path separator: leading program name removed.
	args = argsWithoutProgram([]string{"/usr/bin/omnibar", "-help"})
	if len(args) != 1 || args[0] != "-help" {
		t.Errorf("argsWithoutProgram(%v) = %v, expected [%q]", []string{"/usr/bin/omnibar", "-help"}, args, "-help")
	}
}

func TestRunHandshakeIncludesCoreModule(t *testing.T) {
	runtimeDir := t.TempDir()
	oldRuntime := os.Getenv("XDG_RUNTIME_DIR")
	if err := os.Setenv("XDG_RUNTIME_DIR", runtimeDir); err != nil {
		t.Fatalf("set XDG_RUNTIME_DIR: %v", err)
	}
	defer func() {
		if oldRuntime == "" {
			os.Unsetenv("XDG_RUNTIME_DIR")
		} else {
			os.Setenv("XDG_RUNTIME_DIR", oldRuntime)
		}
	}()

	var out bytes.Buffer
	var errOut bytes.Buffer

	backend, err := Run([]string{"-config", filepath.Join(t.TempDir(), "missing.toml")}, &out, &errOut)
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if backend == nil {
		t.Fatal("expected non-nil backend")
	}
	defer backend.Shutdown()

	socketPath := filepath.Join(runtimeDir, "omnibar", "omnibar.sock")
	conn, err := net.Dial("unix", socketPath)
	if err != nil {
		t.Fatalf("dial socket: %v", err)
	}
	defer conn.Close()

	reader := bufio.NewReader(conn)
	if _, err := conn.Write([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":{"protocol":"omnibar.ipc/v1"}}` + "\n")); err != nil {
		t.Fatalf("write handshake: %v", err)
	}

	line, err := reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read handshake response: %v", err)
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(line, &resp); err != nil {
		t.Fatalf("unmarshal handshake response: %v", err)
	}
	if resp.Error != nil {
		t.Fatalf("handshake error: %+v", resp.Error)
	}

	var result struct {
		Modules []string `json:"modules"`
	}
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("unmarshal handshake result: %v", err)
	}
	if len(result.Modules) != 3 || result.Modules[0] != "core" || result.Modules[1] != "system_monitor" || result.Modules[2] != "keep_awake" {
		t.Errorf("handshake modules = %v, want [core system_monitor keep_awake]", result.Modules)
	}

	if _, err := conn.Write([]byte(`{"jsonrpc":"2.0","id":2,"method":"module.getStatus","params":{"module":"core"}}` + "\n")); err != nil {
		t.Fatalf("write status request: %v", err)
	}
	line, err = reader.ReadBytes('\n')
	if err != nil {
		t.Fatalf("read status response: %v", err)
	}

	var statusResp jsonrpc.Response
	if err := json.Unmarshal(line, &statusResp); err != nil {
		t.Fatalf("unmarshal status response: %v", err)
	}
	if statusResp.Error != nil {
		t.Fatalf("status error: %+v", statusResp.Error)
	}

	var status struct {
		Name    string `json:"name"`
		Enabled bool   `json:"enabled"`
		Running bool   `json:"running"`
	}
	if err := json.Unmarshal(statusResp.Result, &status); err != nil {
		t.Fatalf("unmarshal status result: %v", err)
	}
	if status.Name != "core" || !status.Enabled || !status.Running {
		t.Errorf("core status = %+v, want enabled and running", status)
	}
}
