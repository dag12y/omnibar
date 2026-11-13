// Package ipc implements the Unix domain socket server for Omnibar.
//
// The handshake lets a client (the Quickshell QML UI) verify that it is
// communicating with Omnibar and learn the supported protocol version and
// registered modules.
package ipc

import (
	"encoding/json"

	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
)

// ServerVersion is the backend version reported during handshake.
const ServerVersion = "0.3.0"

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
func handleClientHandshake(params json.RawMessage, registry *modules.Registry) (json.RawMessage, *jsonrpc.Error) {
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

	names := []string{}
	if registry != nil {
		names = registry.Names()
		if names == nil {
			names = []string{}
		}
	}

	result := handshakeResult{
		Protocol: jsonrpc.ProtocolVersion,
		Version:  ServerVersion,
		Modules:  names,
	}
	out, _ := json.Marshal(result)
	return out, nil
}

// RegisterHandshake registers the client.handshake method on the
// dispatcher. The handshake reports every statically registered module,
// including modules that are currently disabled; clients can query
// module.getStatus for enablement and lifecycle state.
func RegisterHandshake(d *jsonrpc.Dispatcher, registry *modules.Registry) {
	d.Register("client.handshake", func(params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		return handleClientHandshake(params, registry)
	})
}
