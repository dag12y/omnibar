// Package logging initializes structured logging using Go's standard
// library log/slog.
//
// Logging is initialized independently and can be tested in isolation.
package logging

import (
	"io"
	"log/slog"
	"os"
)

// Init configures structured logging with the given level and output
// writer. Valid levels: debug, info, warn, warning, error.
// Any other value (including empty) defaults to info.
//
// The returned logger is also set as the default slog logger.
func Init(level string, w io.Writer) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn", "warning":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default: // "info" or anything unrecognized
		lvl = slog.LevelInfo
	}

	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: lvl,
	}))
	slog.SetDefault(logger)
	return logger
}

// LevelFromString converts a string level name to a slog.Level.
// Unrecognized names default to Info.
func LevelFromString(level string) slog.Level {
	switch level {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}

// Ensure os.Stderr is used as the default output when no writer is
// provided by the caller.
var DefaultOutput io.Writer = os.Stderr
