// Package integration contains end-to-end tests for the Omnibar backend.
//
// The tests build and spawn the real cmd/omnibar binary, then talk to it
// over a Unix domain socket in an isolated XDG_RUNTIME_DIR. This exercises
// the same process, socket, framing, and JSON-RPC boundaries used by the
// Quickshell UI.
package integration
