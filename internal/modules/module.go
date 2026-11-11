// Package modules implements Omnibar's central module registry.
//
// Built-in Go modules register statically at compile time. There is no
// dynamic loading of Go packages, and the registry does not manage
// dependencies between modules or supervise/restart them.
package modules

import "context"

// ModuleInfo describes a statically registered module.
//
// Capabilities are what the module declares it needs. They are not
// grants: a capability is usable only when the user has also granted the
// corresponding permission in config.toml under [permissions].
//
// Enabled is the module's default enabled state. An explicit
// modules.<name>.enabled entry in the configuration overrides it.
type ModuleInfo struct {
	Name         string   `json:"name"`
	Description  string   `json:"description,omitempty"`
	Version      string   `json:"version,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Enabled      bool     `json:"enabled"`
}

// Module is the minimal interface implemented by a built-in module.
type Module interface {
	Info() ModuleInfo
}

// Lifecycle is an optional interface for modules that acquire resources
// during initialization or start.
//
// Modules that do not implement Lifecycle can still be registered and
// reported; lifecycle transitions are then metadata-only.
//
// Cleanup contract:
//
//   - Initialize and Start must release any resources they partially
//     acquired before returning a non-nil error. The registry does not
//     call Stop when Initialize or Start fails because the operation did
//     not complete successfully.
//   - Stop releases resources acquired by a successful Initialize and
//     Start. If Stop returns an error, the registry marks the module
//     failed but still attempts Dispose during shutdown.
//   - Dispose is the final cleanup hook. It must be safe to call for a
//     module in the stopped or failed state, including after a failed
//     Stop, and it must be safe to retry after an earlier Dispose error.
//     A successful Dispose moves the module to StateDisposed; a failed
//     Dispose leaves it StateFailed so a later shutdown can retry.
//
// Cleanup callbacks run at the same serialized lifecycle boundary as the
// forward operations, and panics are recovered and logged.
type Lifecycle interface {
	Initialize(context.Context) error
	Start(context.Context) error
	Stop(context.Context) error
	Dispose(context.Context) error
}

// State is a module's lifecycle state.
type State string

const (
	// StateRegistered is the state immediately after registration.
	StateRegistered State = "registered"
	// StateInitialized means the module has read configuration and
	// allocated resources.
	StateInitialized State = "initialized"
	// StateStarted is a transient state while Start is executing.
	StateStarted State = "started"
	// StateRunning means the module started successfully.
	StateRunning State = "running"
	// StateStopped means the module has released its running resources.
	StateStopped State = "stopped"
	// StateDisposed means the module has completed final cleanup.
	StateDisposed State = "disposed"
	// StateFailed means a lifecycle operation returned an error or
	// panicked. Failed modules are not restarted automatically.
	StateFailed State = "failed"
)
