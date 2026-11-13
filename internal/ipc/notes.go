package ipc

import (
	"context"
	"encoding/json"
	"errors"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/notes"
	"time"
)

const noteConflictCode = -32004

func RegisterNoteMethods(d *jsonrpc.Dispatcher, r *modules.Registry, n *notes.Module) {
	for _, method := range []string{"notes.getSnapshot", "notes.get", "notes.create", "notes.update", "notes.delete", "notes.copy"} {
		d.RegisterContext(method, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
			if !r.IsEnabled(notes.Name) {
				return nil, jsonrpc.NewError(jsonrpc.CodeModuleUnavailable, "Notes is disabled", notes.Name)
			}
			if !r.HasCapability(notes.Name, notes.Capability) {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Enable notes.manage in your Omnibar config and restart the backend", notes.Name)
			}
			copyAllowed := r.HasCapability(notes.Name, notes.CopyCapability)
			if method == "notes.copy" && !copyAllowed {
				return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Copy requires clipboard.write", notes.Name)
			}
			var p struct {
				ID       string  `json:"id"`
				Query    string  `json:"query"`
				Title    *string `json:"title"`
				Body     *string `json:"body"`
				Pinned   *bool   `json:"pinned"`
				Revision uint64  `json:"revision"`
			}
			if len(raw) == 0 {
				raw = json.RawMessage(`{}`)
			}
			if json.Unmarshal(raw, &p) != nil || len(p.Query) > 256 {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid note parameters", notes.Name)
			}
			if method != "notes.getSnapshot" && method != "notes.create" && !notes.ValidID(p.ID) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid note ID", notes.Name)
			}
			if (method == "notes.update" || method == "notes.delete" || method == "notes.copy") && !notes.ValidRevision(p.Revision) {
				return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Expected note revision is required", notes.Name)
			}
			if method == "notes.create" || method == "notes.update" {
				if p.Title == nil || p.Body == nil || !notes.ValidTitle(*p.Title) || !notes.ValidBody(*p.Body) || (method == "notes.update" && p.Pinned == nil) {
					return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Title must be 1–128 bytes; note text must be at most 32 KiB", notes.Name)
				}
			}
			ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			var out notes.Result
			var err error
			switch method {
			case "notes.getSnapshot":
				out.Snapshot = n.Snapshot(p.Query)
			case "notes.get":
				out, err = n.Get(p.ID, p.Query)
			case "notes.create":
				out, err = n.Create(*p.Title, *p.Body, p.Query)
			case "notes.update":
				out, err = n.Update(notes.Note{ID: p.ID, Title: *p.Title, Body: *p.Body, Pinned: *p.Pinned, Revision: p.Revision}, p.Query)
			case "notes.delete":
				out, err = n.Delete(p.ID, p.Revision, p.Query)
			case "notes.copy":
				out, err = n.Copy(ctx, p.ID, p.Revision, p.Query)
			}
			if err != nil {
				code := jsonrpc.CodeInternalError
				if errors.Is(err, notes.ErrConflict) {
					code = noteConflictCode
				}
				return nil, jsonrpc.NewError(code, err.Error(), notes.Name)
			}
			out.CanCopy = copyAllowed
			encoded, _ := json.Marshal(out)
			return encoded, nil
		})
	}
}
