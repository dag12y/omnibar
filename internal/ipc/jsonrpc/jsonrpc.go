// Package jsonrpc implements JSON-RPC 2.0 protocol handling for Omnibar.
//
// It provides request parsing, response construction, error codes, and a
// method dispatcher. It is transport-agnostic: it operates on raw JSON
// frames and does not depend on the socket server or framing layer. The
// caller is responsible for delimiting frames (e.g. newlines) and sending
// the returned bytes.
package jsonrpc

import (
	"context"
	"encoding/json"
	"sync"
)

// ProtocolVersion is the Omnibar IPC protocol version exchanged during
// handshake. It must match ADR-0003.
const ProtocolVersion = "omnibar.ipc/v1"

// Standard JSON-RPC 2.0 error codes (reserved for protocol errors).
const (
	CodeParseError     = -32700
	CodeInvalidRequest = -32600
	CodeMethodNotFound = -32601
	CodeInvalidParams  = -32602
	CodeInternalError  = -32603
)

// Custom server error range (inclusive). Server-defined errors must fall
// within -32000 to -32099. Numerically, -32099 is the lower bound.
const (
	CodeCustomMin = -32099
	CodeCustomMax = -32000
)

// Server-defined error codes (ADR-0009).
const (
	// CodePermissionDenied means a required capability is not granted.
	CodePermissionDenied = -32001
	// CodeModuleUnavailable means the module is disabled or not running.
	CodeModuleUnavailable = -32002
)

// IsStandardCode reports whether code is a standard JSON-RPC 2.0 error code.
func IsStandardCode(code int) bool {
	switch code {
	case CodeParseError, CodeInvalidRequest, CodeMethodNotFound, CodeInvalidParams, CodeInternalError:
		return true
	}
	return false
}

// IsCustomCode reports whether code is in the server-defined custom range.
func IsCustomCode(code int) bool {
	return code >= CodeCustomMin && code <= CodeCustomMax
}

// Request is a JSON-RPC 2.0 request object.
//
// ID is preserved as raw JSON so an absent id (notification) can be
// distinguished from an explicit id: null. String and numeric IDs are
// passed through unchanged without rounding through float64.
//
// A request without an id is a notification and expects no response.
type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Response is a JSON-RPC 2.0 response object. It contains either result or
// error (never both), never a method field, and preserves the request id as
// raw JSON.
type Response struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// Error is a JSON-RPC 2.0 error object. Omnibar adds an optional module
// field for routing errors to the responsible module.
type Error struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Module  string          `json:"module,omitempty"`
	Data    json.RawMessage `json:"data,omitempty"`
}

// NewError constructs a JSON-RPC error.
func NewError(code int, message, module string) *Error {
	return &Error{Code: code, Message: message, Module: module}
}

// MethodHandler handles a single JSON-RPC method. params is the raw params
// field (nil when absent). It returns a raw JSON result or a protocol error.
type MethodHandler func(params json.RawMessage) (json.RawMessage, *Error)

// ContextMethodHandler is a MethodHandler that also receives the request
// context. The IPC server puts the calling client session in the context,
// which methods such as module.subscribe need.
type ContextMethodHandler func(ctx context.Context, params json.RawMessage) (json.RawMessage, *Error)

// Dispatcher maps method names to handlers and dispatches JSON-RPC requests.
// It is safe for concurrent use by multiple connections.
type Dispatcher struct {
	mu      sync.RWMutex
	methods map[string]ContextMethodHandler
}

// NewDispatcher creates an empty dispatcher.
func NewDispatcher() *Dispatcher {
	return &Dispatcher{methods: make(map[string]ContextMethodHandler)}
}

// Register associates a method name with a handler. Registering the same
// method twice replaces the previous handler.
func (d *Dispatcher) Register(method string, handler MethodHandler) {
	d.RegisterContext(method, func(_ context.Context, params json.RawMessage) (json.RawMessage, *Error) {
		return handler(params)
	})
}

// RegisterContext is like Register for a handler that needs the request
// context.
func (d *Dispatcher) RegisterContext(method string, handler ContextMethodHandler) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.methods[method] = handler
}

// Handle processes a single JSON-RPC frame (raw JSON without a trailing
// newline). It returns a response frame (without a trailing newline) for
// requests, or (nil, nil) for notifications, which expect no response.
//
// Handle never returns a non-nil error: protocol-level problems (malformed
// JSON, invalid requests, unknown methods) are converted into JSON-RPC
// error responses so malformed messages never crash the caller.
func (d *Dispatcher) Handle(raw []byte) ([]byte, error) {
	return d.HandleContext(context.Background(), raw)
}

// HandleContext is like Handle but passes ctx to the method handler.
func (d *Dispatcher) HandleContext(ctx context.Context, raw []byte) ([]byte, error) {
	req, parseErr := parseRequest(raw)
	if parseErr != nil {
		// The id could not be determined; per spec it is null for parse
		// errors.
		return buildResponse(nil, nil, NewError(CodeParseError, parseErr.Error(), "")), nil
	}

	// Validate the JSON-RPC version.
	if req.JSONRPC != "2.0" {
		return buildResponse(req.ID, nil, NewError(CodeInvalidRequest, `jsonrpc must be "2.0"`, "")), nil
	}

	// A request without an id is a notification: dispatch but send no
	// response. An explicit id: null is not a notification and must be
	// answered with "id": null.
	if len(req.ID) == 0 {
		_, _ = d.dispatch(ctx, req.Method, req.Params)
		return nil, nil
	}

	// The method is required for requests.
	if req.Method == "" {
		return buildResponse(req.ID, nil, NewError(CodeInvalidRequest, "method is required", "")), nil
	}

	result, err := d.dispatch(ctx, req.Method, req.Params)
	if err != nil {
		return buildResponse(req.ID, nil, err), nil
	}
	return buildResponse(req.ID, result, nil), nil
}

// parseRequest decodes raw into a Request. It returns an error if the bytes
// are not valid JSON or are not a JSON object.
func parseRequest(raw []byte) (*Request, error) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return nil, err
	}
	return &req, nil
}

// dispatch looks up and invokes the handler for method.
func (d *Dispatcher) dispatch(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, *Error) {
	d.mu.RLock()
	handler, ok := d.methods[method]
	d.mu.RUnlock()
	if !ok {
		return nil, NewError(CodeMethodNotFound, "Method not found: "+method, "")
	}
	return handler(ctx, params)
}

// buildResponse marshals a JSON-RPC 2.0 response. The response preserves the
// request id and contains either result or error (never both, never method).
func buildResponse(id json.RawMessage, result json.RawMessage, err *Error) []byte {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
	}
	if err != nil {
		resp.Error = err
	} else {
		resp.Result = result
	}
	out, _ := json.Marshal(resp)
	return out
}
