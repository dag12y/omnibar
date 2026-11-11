// Package ipc implements the Unix domain socket server for Omnibar.
//
// The handshake lets a client (the Quickshell QML UI) verify that it is
// communicating with Omnibar and learn the supported protocol version and
// available modules.
package ipc

import (
	"encoding/json"

	"omnibar/internal/ipc/jsonrpc"
)

// ServerVersion is the backend version reported during handshake.
const ServerVersion = "0.1.0"

type handshakeParams struct {
	Protocol string `json:"protocol"`
	ClientID string `json:"client_id,omitempty"`
}

type handshakeResult struct {
	Protocol string   `json:"protocol"`
	Version  string   `json:"version"`
	Modules  []string `json:"modules"`
}

// handleClientHandshake implements the client.handshake method (UI ->
// backend). It validates the protocol version and returns the server's
// handshake result.
func handleClientHandshake(params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
	var p handshakeParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid handshake params", "")
	}
	if p.Protocol != jsonrpc.ProtocolVersion {
		return nil, jsonrpc.NewError(
			jsonrpc.CodeInvalidRequest,
			"Protocol version mismatch: expected "+jsonrpc.ProtocolVersion+", got "+p.Protocol,
			"",
		)
	}
	result := handshakeResult{
		Protocol: jsonrpc.ProtocolVersion,
		Version:  ServerVersion,
		Modules:  []string{},
	}
	out, _ := json.Marshal(result)
	return out, nil
}

// RegisterHandshake registers the client.handshake method on the
// dispatcher. A module registry is introduced in M5, so the module list is
// empty until then.
func RegisterHandshake(d *jsonrpc.Dispatcher) {
	d.Register("client.handshake", handleClientHandshake)
}
