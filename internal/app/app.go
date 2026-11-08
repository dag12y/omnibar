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
)

// Run executes the Omnibar backend with the provided command-line
// arguments (excluding the program name). It returns nil on success
// and a non-nil error on failure. The caller is responsible for
// translating the error into an exit code.
func Run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("omnibar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	help := fs.Bool("help", false, "Show help and exit")
	if err := fs.Parse(args); err != nil {
		return fmt.Errorf("parse arguments: %w", err)
	}

	if *help {
		printUsage(stdout)
		return nil
	}

	// Phase 1 scaffold: no configuration, logging, socket, or module
	// functionality is implemented yet. Those are added in later
	// milestones.
	fmt.Fprintln(stdout, "omnibar backend starting")
	fmt.Fprintln(stdout, "no modules enabled; run with --help for usage")
	return nil
}

// printUsage writes the command-line usage to the given writer.
func printUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: omnibar [flags]\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -help   Show this help and exit\n")
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
