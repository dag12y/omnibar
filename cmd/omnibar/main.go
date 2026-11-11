// Package main is the entry point for the Omnibar backend binary.
//
// It is intentionally thin: all testable startup logic lives in the
// internal/app package. main() starts the IPC server via app.Run, waits
// for shutdown signals (SIGTERM or SIGINT), and gracefully shuts down
// the server.
package main

import (
	"os"
	"os/signal"
	"syscall"

	"omnibar/internal/app"
)

func main() {
	backend, err := app.Run(os.Args[1:], os.Stdout, os.Stderr)
	if err != nil {
		os.Exit(1)
	}
	if backend == nil {
		// --help was shown; exit successfully.
		os.Exit(0)
	}

	// Wait for shutdown signal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)
	<-sigCh

	// Gracefully shut down the IPC server and modules.
	if err := backend.Shutdown(); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
