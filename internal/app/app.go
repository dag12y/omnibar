// Package app implements the Omnibar backend application core.
//
// It is intentionally kept separate from cmd/omnibar so that startup
// behavior can be tested without invoking the binary.
package app

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"omnibar/internal/config"
	"omnibar/internal/logging"
)

// Run executes the Omnibar backend with the provided command-line
// arguments (excluding the program name). It returns nil on success
// and a non-nil error on failure. The caller is responsible for
// translating the error into an exit code.
//
// Startup sequence:
//   1. Parse command-line arguments (--help, --config)
//   2. Handle --help
//   3. Resolve configuration path ($XDG_CONFIG_HOME/omnibar/config.toml,
//      or --config override)
//   4. Load configuration (defaults if file missing)
//   5. Initialize structured logging (log/slog)
//   6. Validate configuration
//   7. Return error if configuration is invalid
func Run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("omnibar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	help := fs.Bool("help", false, "Show help and exit")
	configPath := fs.String("config", "", "Path to configuration file")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}

	if *help {
		printUsage(stdout)
		return nil
	}

	// Resolve configuration path.
	path := *configPath
	if path == "" {
		var err error
		path, err = config.ConfigPath()
		if err != nil {
			return fmt.Errorf("resolve config path: %w", err)
		}
	}

	// Load configuration (defaults if file missing).
	cfg, err := config.Load(path)
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	// Initialize logging before any further output.
	logger := logging.Init(cfg.Core.LogLevel, stderr)

	// Validate configuration.
	if err := cfg.Validate(); err != nil {
		logger.Error("configuration validation failed", "error", err)
		return fmt.Errorf("validate configuration: %w", err)
	}

	logger.Info("omnibar backend starting",
		"config", path,
		"log_level", cfg.Core.LogLevel,
		"modules", len(cfg.Modules),
		"permissions", len(cfg.Permissions),
	)

	return nil
}

// isHelpRequest reports whether args contains a help flag. It is
// exported so tests can exercise it without parsing full argument
// vectors.
func isHelpRequest(args []string) bool {
	for _, a := range args {
		if a == "-help" || a == "--help" || a == "-h" || a == "--h" {
			return true
		}
	}
	return false
}

// argsWithoutProgram removes a leading program name if present, so the
// function behaves consistently whether or not os.Args[0] is included.
func argsWithoutProgram(args []string) []string {
	if len(args) > 0 && strings.ContainsRune(args[0], os.PathSeparator) {
		return args[1:]
	}
	return args
}

// printUsage writes the command-line usage to the given writer.
func printUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: omnibar [flags]\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -help     Show this help and exit\n")
	fmt.Fprintf(w, "  -config   Path to configuration file\n")
}
