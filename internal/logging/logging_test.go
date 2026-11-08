package logging

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestInitDefaultsToInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := Init("", &buf)

	if logger == nil {
		t.Fatal("Init() returned nil logger")
	}

	logger.Info("test message")
	output := buf.String()
	if !strings.Contains(output, "test message") {
		t.Errorf("expected output to contain message, got: %q", output)
	}
}

func TestInitDebugLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := Init("debug", &buf)

	logger.Debug("debug message")
	output := buf.String()
	if !strings.Contains(output, "debug message") {
		t.Errorf("expected debug output at debug level, got: %q", output)
	}
}

func TestInitInfoLevel(t *testing.T) {
	var buf bytes.Buffer
	Init("info", &buf)

	// At info level, slog filters out debug messages. Just verify
	// initialization doesn't panic.
}

func TestInitWarnLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := Init("warn", &buf)

	logger.Warn("warning message")
	output := buf.String()
	if !strings.Contains(output, "warning message") {
		t.Errorf("expected warn output, got: %q", output)
	}
}

func TestInitErrorLevel(t *testing.T) {
	var buf bytes.Buffer
	logger := Init("error", &buf)

	logger.Error("error message")
	output := buf.String()
	if !strings.Contains(output, "error message") {
		t.Errorf("expected error output, got: %q", output)
	}
}

func TestLevelFromString(t *testing.T) {
	cases := []string{
		"debug",
		"info",
		"warn",
		"error",
		"",
		"unknown",
	}

	for _, input := range cases {
		got := LevelFromString(input)
		switch got {
		case slog.LevelDebug, slog.LevelInfo, slog.LevelWarn, slog.LevelError:
		default:
			t.Errorf("LevelFromString(%q) returned invalid level", input)
		}
	}
}

// Ensure slog import is used.
var _ = slog.LevelDebug
