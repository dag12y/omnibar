package ipc

import (
	"context"
	"encoding/json"

	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/modules"
	"omnibar/internal/modules/systemmonitor"
)

type moduleStatusParams struct {
	Module string `json:"module"`
}

type moduleStatusResult struct {
	Name                 string   `json:"name"`
	Description          string   `json:"description,omitempty"`
	Version              string   `json:"version,omitempty"`
	Enabled              bool     `json:"enabled"`
	State                string   `json:"state"`
	Running              bool     `json:"running"`
	DeclaredCapabilities []string `json:"declared_capabilities"`
	GrantedCapabilities  []string `json:"granted_capabilities"`
}

// handleModuleGetStatus implements module.getStatus.
func handleModuleGetStatus(params json.RawMessage, registry *modules.Registry) (json.RawMessage, *jsonrpc.Error) {
	var p moduleStatusParams
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "Invalid module.getStatus params", "")
	}
	if p.Module == "" {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidParams, "module param is required", "")
	}
	if registry == nil {
		return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "Module registry is unavailable", p.Module)
	}

	info, ok := registry.Info(p.Module)
	if !ok {
		return nil, jsonrpc.NewError(jsonrpc.CodeInvalidRequest, "Module not found", p.Module)
	}
	state, _ := registry.State(p.Module)

	declared := registry.DeclaredCapabilities(p.Module)
	if declared == nil {
		declared = []string{}
	}
	granted := registry.GrantedCapabilities(p.Module)
	if granted == nil {
		granted = []string{}
	}

	result := moduleStatusResult{
		Name:                 info.Name,
		Description:          info.Description,
		Version:              info.Version,
		Enabled:              info.Enabled,
		State:                string(state),
		Running:              state == modules.StateRunning,
		DeclaredCapabilities: declared,
		GrantedCapabilities:  granted,
	}
	out, _ := json.Marshal(result)
	return out, nil
}

// RegisterModuleMethods registers the module-related JSON-RPC methods
// required by M5. The handshake provides the module list; this method
// exposes one module's status.
func RegisterModuleMethods(d *jsonrpc.Dispatcher, registry *modules.Registry) {
	d.Register("module.getStatus", func(params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		return handleModuleGetStatus(params, registry)
	})
}

// RegisterSystemMethods registers system.* JSON-RPC methods, beginning with
// system.getSnapshot (M10). It requires the system.info capability.
func RegisterSystemMethods(d *jsonrpc.Dispatcher, registry *modules.Registry, sm *systemmonitor.Module) {
	d.RegisterContext("system.getSnapshot", func(ctx context.Context, params json.RawMessage) (json.RawMessage, *jsonrpc.Error) {
		if !registry.HasCapability(systemmonitor.Name, systemmonitor.Capability) {
			return nil, jsonrpc.NewError(jsonrpc.CodePermissionDenied, "Permission denied: system.info", systemmonitor.Name)
		}
		snap := sm.Snapshot()
		if snap == nil {
			return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "Snapshot unavailable", systemmonitor.Name)
		}
		out, err := json.Marshal(snap)
		if err != nil {
			return nil, jsonrpc.NewError(jsonrpc.CodeInternalError, "marshal snapshot: "+err.Error(), systemmonitor.Name)
		}
		return out, nil
	})
}
