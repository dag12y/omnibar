package jsonrpc

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

func TestValidRequestStringID(t *testing.T) {
	d := NewDispatcher()
	d.Register("echo", func(params json.RawMessage) (json.RawMessage, *Error) {
		return params, nil
	})

	raw := `{"jsonrpc":"2.0","id":"msg-1","method":"echo","params":{"text":"hello"}}`
	out, err := d.Handle([]byte(raw))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if out == nil {
		t.Fatal("Handle() returned nil for a request")
	}

	var resp Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if resp.JSONRPC != "2.0" {
		t.Errorf("response jsonrpc = %q, want 2.0", resp.JSONRPC)
	}
	if string(resp.ID) != `"msg-1"` {
		t.Errorf("response id = %v (type %T), want msg-1", resp.ID, resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}
	if string(resp.Result) != `{"text":"hello"}` {
		t.Errorf("response result = %q, want {\"text\":\"hello\"}", resp.Result)
	}
}

func TestValidRequestNumericID(t *testing.T) {
	d := NewDispatcher()
	d.Register("add", func(params json.RawMessage) (json.RawMessage, *Error) {
		return []byte(`7`), nil
	})

	raw := `{"jsonrpc":"2.0","id":42,"method":"add"}`
	out, err := d.Handle([]byte(raw))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}

	var resp Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if string(resp.ID) != "42" {
		t.Errorf("response id = %v (type %T), want 42", resp.ID, resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}
	if !strings.HasPrefix(string(resp.Result), "7") {
		t.Errorf("response result = %q", resp.Result)
	}
}

func TestResponseHasNoMethodField(t *testing.T) {
	d := NewDispatcher()
	d.Register("noop", func(params json.RawMessage) (json.RawMessage, *Error) {
		return nil, nil
	})

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"noop"}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if strings.Contains(string(out), `"method"`) {
		t.Errorf("response must not contain a method field: %s", out)
	}
}

func TestResponseHasResultOrErrorNotBoth(t *testing.T) {
	d := NewDispatcher()
	d.Register("ok", func(params json.RawMessage) (json.RawMessage, *Error) {
		return []byte(`1`), nil
	})
	d.Register("boom", func(params json.RawMessage) (json.RawMessage, *Error) {
		return nil, NewError(CodeInternalError, "something broke", "")
	})

	okOut, _ := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"ok"}`))
	var okResp Response
	json.Unmarshal(okOut, &okResp)
	if okResp.Result == nil || okResp.Error != nil {
		t.Errorf("success response should have result and no error: %s", okOut)
	}

	errOut, _ := d.Handle([]byte(`{"jsonrpc":"2.0","id":2,"method":"boom"}`))
	var errResp Response
	json.Unmarshal(errOut, &errResp)
	if errResp.Error == nil || errResp.Result != nil {
		t.Errorf("error response should have error and no result: %s", errOut)
	}
}

func TestMissingJSONRPCVersion(t *testing.T) {
	d := NewDispatcher()
	out, err := d.Handle([]byte(`{"id":1,"method":"foo"}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if resp.Error == nil {
		t.Error("expected error response for missing jsonrpc")
	}
	if resp.Error.Code != CodeInvalidRequest {
		t.Errorf("error code = %d, want %d", resp.Error.Code, CodeInvalidRequest)
	}
	if string(resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
}

func TestInvalidJSONRPCVersion(t *testing.T) {
	d := NewDispatcher()
	out, err := d.Handle([]byte(`{"jsonrpc":"1.0","id":1,"method":"foo"}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	var resp Response
	json.Unmarshal(out, &resp)
	if resp.Error.Code != CodeInvalidRequest {
		t.Errorf("error code = %d, want %d", resp.Error.Code, CodeInvalidRequest)
	}
}

func TestMissingMethod(t *testing.T) {
	d := NewDispatcher()
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	var resp Response
	json.Unmarshal(out, &resp)
	if resp.Error.Code != CodeInvalidRequest {
		t.Errorf("error code = %d, want %d", resp.Error.Code, CodeInvalidRequest)
	}
}

func TestMalformedJSON(t *testing.T) {
	d := NewDispatcher()
	out, err := d.Handle([]byte(`{not valid json`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	var resp Response
	json.Unmarshal(out, &resp)
	if resp.Error.Code != CodeParseError {
		t.Errorf("error code = %d, want %d", resp.Error.Code, CodeParseError)
	}
	// id is null for parse errors.
	if string(resp.ID) != "null" {
		t.Errorf("response id = %v, want null", resp.ID)
	}
}

func TestUnknownMethod(t *testing.T) {
	d := NewDispatcher()
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"does.not.exist"}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	var resp Response
	json.Unmarshal(out, &resp)
	if resp.Error.Code != CodeMethodNotFound {
		t.Errorf("error code = %d, want %d", resp.Error.Code, CodeMethodNotFound)
	}
	if string(resp.ID) != "1" {
		t.Errorf("response id = %v, want 1", resp.ID)
	}
}

func TestNotificationNoResponse(t *testing.T) {
	d := NewDispatcher()
	d.Register("event", func(params json.RawMessage) (json.RawMessage, *Error) {
		return nil, nil
	})

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","method":"event","params":{"a":1}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if out != nil {
		t.Errorf("expected no response for notification, got %s", out)
	}
}

func TestExplicitNullID(t *testing.T) {
	d := NewDispatcher()
	d.Register("echo", func(params json.RawMessage) (json.RawMessage, *Error) {
		return params, nil
	})

	// An explicitly present id: null is a request, not a notification.
	// It must be answered with "id": null.
	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":null,"method":"echo","params":{"x":1}}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	if out == nil {
		t.Fatal("expected a response for explicit id: null")
	}

	var resp Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if string(resp.ID) != "null" {
		t.Errorf("response id = %v, want null", resp.ID)
	}
	if resp.Error != nil {
		t.Errorf("unexpected error: %+v", resp.Error)
	}
	if string(resp.Result) != `{"x":1}` {
		t.Errorf("response result = %q, want {\"x\":1}", resp.Result)
	}
}

func TestStandardErrorCodes(t *testing.T) {
	if CodeParseError != -32700 {
		t.Errorf("CodeParseError = %d, want -32700", CodeParseError)
	}
	if CodeInvalidRequest != -32600 {
		t.Errorf("CodeInvalidRequest = %d, want -32600", CodeInvalidRequest)
	}
	if CodeMethodNotFound != -32601 {
		t.Errorf("CodeMethodNotFound = %d, want -32601", CodeMethodNotFound)
	}
	if CodeInvalidParams != -32602 {
		t.Errorf("CodeInvalidParams = %d, want -32602", CodeInvalidParams)
	}
	if CodeInternalError != -32603 {
		t.Errorf("CodeInternalError = %d, want -32603", CodeInternalError)
	}
}

func TestCustomErrorCodeRange(t *testing.T) {
	if !IsCustomCode(-32000) {
		t.Error("-32000 should be a custom code")
	}
	if !IsCustomCode(-32099) {
		t.Error("-32099 should be a custom code")
	}
	if IsCustomCode(-32100) {
		t.Error("-32100 should not be a custom code")
	}
	if IsCustomCode(-32600) {
		t.Error("-32600 is standard, not custom")
	}
}

func TestCustomErrorInResponse(t *testing.T) {
	d := NewDispatcher()
	d.Register("fail", func(params json.RawMessage) (json.RawMessage, *Error) {
		return nil, NewError(-32001, "module-specific failure", "audio")
	})

	out, err := d.Handle([]byte(`{"jsonrpc":"2.0","id":9,"method":"fail"}`))
	if err != nil {
		t.Fatalf("Handle() returned error: %v", err)
	}
	var resp Response
	json.Unmarshal(out, &resp)
	if resp.Error.Code != -32001 {
		t.Errorf("error code = %d, want -32001", resp.Error.Code)
	}
	if resp.Error.Module != "audio" {
		t.Errorf("error module = %q, want audio", resp.Error.Module)
	}
	if string(resp.ID) != "9" {
		t.Errorf("response id = %v, want 9", resp.ID)
	}
}

func TestMultipleRequestsStatefulDispatcher(t *testing.T) {
	d := NewDispatcher()
	var calls int
	d.Register("count", func(params json.RawMessage) (json.RawMessage, *Error) {
		calls++
		return []byte(`{"n":` + strconv.Itoa(calls) + `}`), nil
	})

	for i := 1; i <= 3; i++ {
		id := strconv.Itoa(i)
		raw := `{"jsonrpc":"2.0","id":` + id + `,"method":"count"}`
		out, err := d.Handle([]byte(raw))
		if err != nil {
			t.Fatalf("request %d: Handle() returned error: %v", i, err)
		}
		var resp Response
		json.Unmarshal(out, &resp)
		if resp.Error != nil {
			t.Errorf("request %d: unexpected error %+v", i, resp.Error)
		}
	}
	if calls != 3 {
		t.Errorf("expected 3 handler calls, got %d", calls)
	}
}

type ctxKey struct{}

func TestHandleContextPassesContextToHandler(t *testing.T) {
	d := NewDispatcher()
	d.RegisterContext("whoami", func(ctx context.Context, params json.RawMessage) (json.RawMessage, *Error) {
		who, _ := ctx.Value(ctxKey{}).(string)
		out, _ := json.Marshal(who)
		return out, nil
	})

	ctx := context.WithValue(context.Background(), ctxKey{}, "session-7")
	out, err := d.HandleContext(ctx, []byte(`{"jsonrpc":"2.0","id":1,"method":"whoami"}`))
	if err != nil {
		t.Fatalf("HandleContext() returned error: %v", err)
	}
	var resp Response
	if err := json.Unmarshal(out, &resp); err != nil {
		t.Fatalf("response unmarshal: %v", err)
	}
	if string(resp.Result) != `"session-7"` {
		t.Errorf("result = %s, want the context value", resp.Result)
	}
}

func TestHandleUsesBackgroundContext(t *testing.T) {
	d := NewDispatcher()
	d.RegisterContext("probe", func(ctx context.Context, params json.RawMessage) (json.RawMessage, *Error) {
		if ctx == nil || ctx.Err() != nil {
			return nil, NewError(CodeInternalError, "bad context", "")
		}
		return []byte(`true`), nil
	})

	out, _ := d.Handle([]byte(`{"jsonrpc":"2.0","id":1,"method":"probe"}`))
	var resp Response
	_ = json.Unmarshal(out, &resp)
	if resp.Error != nil {
		t.Fatalf("Handle gave the handler a bad context: %+v", resp.Error)
	}
}

func TestContextHandlerReceivesNotifications(t *testing.T) {
	d := NewDispatcher()
	called := make(chan context.Context, 1)
	d.RegisterContext("note", func(ctx context.Context, params json.RawMessage) (json.RawMessage, *Error) {
		called <- ctx
		return nil, nil
	})

	ctx := context.WithValue(context.Background(), ctxKey{}, "n")
	out, err := d.HandleContext(ctx, []byte(`{"jsonrpc":"2.0","method":"note"}`))
	if err != nil || out != nil {
		t.Fatalf("notification produced out=%q err=%v, want neither", out, err)
	}
	if got := (<-called).Value(ctxKey{}); got != "n" {
		t.Errorf("notification handler context value = %v", got)
	}
}

func TestSessionErrorCodesAreInCustomRange(t *testing.T) {
	for name, code := range map[string]int{
		"CodePermissionDenied":  CodePermissionDenied,
		"CodeModuleUnavailable": CodeModuleUnavailable,
	} {
		if !IsCustomCode(code) {
			t.Errorf("%s = %d is outside the server-defined range", name, code)
		}
	}
	if CodePermissionDenied != -32001 || CodeModuleUnavailable != -32002 {
		t.Error("error codes must match ADR-0009 (-32001, -32002)")
	}
}
