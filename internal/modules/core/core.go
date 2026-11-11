// Package core provides Omnibar's built-in core module.
//
// The core module is a static placeholder for the backend's own
// services. It owns no external resources in Phase 1; its purpose is to
// make the core visible through the module registry and IPC handshake.
package core

import "omnibar/internal/modules"

const (
	// Name is the module's unique registry name.
	Name = "core"

	// Version is the core module version reported in ModuleInfo.
	Version = "0.1.0"
)

// Module implements modules.Module for the Omnibar core.
type Module struct{}

var _ modules.Module = (*Module)(nil)

// New returns the built-in core module.
func New() *Module {
	return &Module{}
}

// Info returns the core module's metadata.
//
// The core module is enabled by default and declares no capabilities.
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{
		Name:        Name,
		Description: "Omnibar core services and module registry.",
		Version:     Version,
		Enabled:     true,
	}
}
