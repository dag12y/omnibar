package ipc

import (
	"encoding/json"
	"testing"

	"omnibar/internal/ipc/jsonrpc"
)

func TestHandshakeSuccess(t *testing.T) {
	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d)

	params := `{"protocol":"omnibar.ipc/v1","client_id":"omnibar-ui-1"}`
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":` + params + `}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if out == nil {
		t.Fatal("expected a handshake response")
	}

	var resp jsonrpc.Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if string(resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}

	var result map[string]json.RawMessage
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		t.Fatalf("result unmarshal: %v", err)
	}
	if string(result["protocol"]) != `"omnibar.ipc/v1"` {
		t.Errorf("result protocol = %s, want omnibar.ipc/v1", result["protocol"])
	}
	if string(result["version"]) != `"0.1.0"` {
		t.Errorf("result version = %s, want 0.1.0", result["version"])
	}
}

func TestHandshakeVersionMismatch(t *testing.T) {
	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d)

	params := `{"protocol":"omnibar.ipc/v2","client_id":"omnibar-ui-1"}`
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake","params":` + params + `}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	json.Unmarshal(out, &resp)
	if resp.Error == nil {
		t.Fatal("expected error response for version mismatch")
	}
	if resp.Error.Code != jsonrpc.CodeInvalidRequest {
		t.Errorf("error code = %d, want %d (-32600)", resp.Error.Code, jsonrpc.CodeInvalidRequest)
	}
	if string(resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
}

func TestHandshakeMissingParams(t *testing.T) {
	d := jsonrpc.NewDispatcher()
	RegisterHandshake(d)

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"client.handshake"}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp jsonrpc.Response
	json.Unmarshal(out, &resp)
	if resp.Error == nil {
		t.Fatal("expected error response for missing params")
	}
	if resp.Error.Code != jsonrpc.CodeInvalidParams {
		t.Errorf("error code = %d, want %d (-32602)", resp.Error.Code, jsonrpc.CodeInvalidParams)
	}
}
