// Package main is the entry point for the Omnibar backend binary.
//
// It is intentionally thin: all testable startup logic lives in the
// internal/app package. main() only parses os.Args, invokes app.Run,
// and translates the result into an exit code.
package main

import (
	"os"

	"omnibar/internal/app"
)

func main() {
	if err := app.Run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}
