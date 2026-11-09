package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
