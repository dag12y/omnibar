package modules

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"runtime/debug"
	"sync"

	"omnibar/internal/config"
)

// ErrDuplicateModule is returned when a module name is registered twice.
var ErrDuplicateModule = errors.New("module already registered")

// Registry holds the statically registered modules. It is safe for
// concurrent use.
//
// Registration happens once during startup, but reads and capability
// checks may happen from multiple IPC connection goroutines.
type Registry struct {
	mu          sync.RWMutex
	lifecycleMu sync.Mutex

	order         []string
	entries       map[string]*entry
	moduleConfigs map[string]config.ModuleConfig
	permissions   config.Permissions
	logger        *slog.Logger
}

// entry is the registry's internal record for a registered module.
type entry struct {
	module   Module
	info     ModuleInfo
	declared map[string]struct{}
	state    State
}

// NewRegistry creates an empty registry using the module enablement and
// permission grants from cfg. A nil cfg uses configuration defaults.
//
// The configuration is snapshotted: changes require an Omnibar restart,
// matching the Phase 1 permission model.
func NewRegistry(cfg *config.Config, logger *slog.Logger) *Registry {
	if cfg == nil {
		cfg = config.DefaultConfig()
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}

	moduleConfigs := make(map[string]config.ModuleConfig, len(cfg.Modules))
	for name, mc := range cfg.Modules {
		moduleConfigs[name] = mc
	}

	permissions := make(config.Permissions, len(cfg.Permissions))
	for name, granted := range cfg.Permissions {
		permissions[name] = granted
	}

	return &Registry{
		entries:       make(map[string]*entry),
		moduleConfigs: moduleConfigs,
		permissions:   permissions,
		logger:        logger,
	}
}

// Register adds a module to the registry. It returns ErrDuplicateModule
// if the name is already registered.
//
// The module's declared capabilities are recorded. They do not grant
// access; see HasCapability.
func (r *Registry) Register(m Module) error {
	if m == nil {
		return errors.New("register module: module must not be nil")
	}

	info, err := readInfo(m)
	if err != nil {
		return fmt.Errorf("register module: %w", err)
	}
	if info.Name == "" {
		return errors.New("register module: name must not be empty")
	}

	capabilities, err := normalizeCapabilities(info.Capabilities)
	if err != nil {
		return fmt.Errorf("register module %q: %w", info.Name, err)
	}
	info.Capabilities = capabilities

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.entries[info.Name]; exists {
		return fmt.Errorf("%w: %q", ErrDuplicateModule, info.Name)
	}

	// An explicit modules.<name>.enabled setting overrides the module's
	// declared default. A missing setting leaves the default in place.
	if mc, configured := r.moduleConfigs[info.Name]; configured {
		info.Enabled = mc.Enabled
	}

	r.entries[info.Name] = &entry{
		module:   m,
		info:     info,
		declared: capabilitySet(capabilities),
		state:    StateRegistered,
	}
	r.order = append(r.order, info.Name)

	return nil
}

// Get returns the registered module implementation by name.
func (r *Registry) Get(name string) (Module, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[name]
	if !ok {
		return nil, false
	}
	return e.module, true
}

// Info returns a copy of a module's effective metadata by name. The
// Enabled field reflects configuration overrides.
func (r *Registry) Info(name string) (ModuleInfo, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[name]
	if !ok {
		return ModuleInfo{}, false
	}
	return cloneInfo(e.info), true
}

// List returns all registered modules in registration order.
func (r *Registry) List() []ModuleInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]ModuleInfo, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, cloneInfo(r.entries[name].info))
	}
	return out
}

// Names returns all registered module names in registration order.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}

// EnabledNames returns the names of enabled modules in registration order.
func (r *Registry) EnabledNames() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.order))
	for _, name := range r.order {
		if r.entries[name].info.Enabled {
			out = append(out, name)
		}
	}
	return out
}

// IsEnabled reports whether a registered module is enabled.
func (r *Registry) IsEnabled(name string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[name]
	return ok && e.info.Enabled
}

// State returns a module's current lifecycle state.
func (r *Registry) State(name string) (State, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[name]
	if !ok {
		return "", false
	}
	return e.state, true
}

// DeclaredCapabilities returns the capabilities a module declared in its
// ModuleInfo, in declaration order.
func (r *Registry) DeclaredCapabilities(name string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[name]
	if !ok {
		return []string{}
	}
	return append([]string(nil), e.info.Capabilities...)
}

// GrantedCapabilities returns the intersection of a module's declared
// capabilities and the permissions the user granted in configuration.
//
// A permission granted to a module that does not declare it is ignored.
func (r *Registry) GrantedCapabilities(name string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[name]
	if !ok {
		return []string{}
	}

	out := make([]string, 0, len(e.info.Capabilities))
	for _, capability := range e.info.Capabilities {
		if r.permissions.IsGranted(capability) {
			out = append(out, capability)
		}
	}
	return out
}

// HasCapability reports whether a module may use a capability.
//
// It returns true only when all of the following hold:
//   - the module is registered and enabled;
//   - the module declared the capability;
//   - the user granted the corresponding permission in config.toml;
//   - the module has not failed or been disposed.
//
// Any missing or invalid input fails closed.
func (r *Registry) HasCapability(moduleName, capability string) bool {
	if capability == "" {
		return false
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	e, ok := r.entries[moduleName]
	if !ok || !e.info.Enabled {
		return false
	}
	if e.state == StateFailed || e.state == StateDisposed {
		return false
	}
	if _, declared := e.declared[capability]; !declared {
		return false
	}
	return r.permissions.IsGranted(capability)
}

// Initialize runs the optional Initialize lifecycle operation for every
// enabled module that is still registered. Modules that do not implement
// Lifecycle simply move to StateInitialized.
//
// A failing or panicking module is marked StateFailed, logged, and does
// not prevent other modules from being initialized.
func (r *Registry) Initialize(ctx context.Context) error {
	return r.runLifecycle(ctx, "initialize",
		[]State{StateRegistered},
		"",
		StateInitialized,
		func(ctx context.Context, lc Lifecycle) error { return lc.Initialize(ctx) },
	)
}

// Start runs the optional Start lifecycle operation for every enabled,
// initialized module. Modules that do not implement Lifecycle simply
// move to StateRunning.
func (r *Registry) Start(ctx context.Context) error {
	return r.runLifecycle(ctx, "start",
		[]State{StateInitialized},
		StateStarted,
		StateRunning,
		func(ctx context.Context, lc Lifecycle) error { return lc.Start(ctx) },
	)
}

// Stop runs the optional Stop lifecycle operation for every enabled
// initialized or running module.
//
// Failed modules are not passed to Stop: Initialize and Start are
// required to clean up their own partial work. A module whose Stop fails
// is marked failed, and Dispose is still attempted afterwards.
func (r *Registry) Stop(ctx context.Context) error {
	return r.runLifecycle(ctx, "stop",
		[]State{StateInitialized, StateStarted, StateRunning},
		"",
		StateStopped,
		func(ctx context.Context, lc Lifecycle) error { return lc.Stop(ctx) },
	)
}

// Dispose runs the optional Dispose lifecycle operation for every
// enabled stopped or failed module. Failed modules are included so that
// failures during Initialize, Start, or Stop still receive a final
// cleanup opportunity.
//
// A Dispose failure leaves the module failed, allowing a later shutdown
// call to retry it. Repeated calls after a successful Dispose do nothing.
func (r *Registry) Dispose(ctx context.Context) error {
	return r.runLifecycle(ctx, "dispose",
		[]State{StateStopped, StateFailed},
		"",
		StateDisposed,
		func(ctx context.Context, lc Lifecycle) error { return lc.Dispose(ctx) },
	)
}

// runLifecycle applies one lifecycle operation to all enabled modules in
// the given starting states. Lifecycle operations are serialized while
// state reads and capability checks remain concurrent.
func (r *Registry) runLifecycle(
	ctx context.Context,
	operation string,
	fromStates []State,
	transient State,
	final State,
	call func(context.Context, Lifecycle) error,
) error {
	if ctx == nil {
		ctx = context.Background()
	}

	r.lifecycleMu.Lock()
	defer r.lifecycleMu.Unlock()

	r.mu.RLock()
	entries := make([]*entry, 0, len(r.order))
	for _, name := range r.order {
		entries = append(entries, r.entries[name])
	}
	r.mu.RUnlock()

	var errs []error
	for _, e := range entries {
		r.mu.RLock()
		enabled := e.info.Enabled
		state := e.state
		name := e.info.Name
		lc, hasLifecycle := e.module.(Lifecycle)
		r.mu.RUnlock()

		if !enabled || !stateIn(state, fromStates) {
			continue
		}

		if transient != "" {
			r.setEntryState(e, transient)
		}

		var err error
		if hasLifecycle {
			err = r.invoke(name, operation, func() error {
				return call(ctx, lc)
			})
		}

		if err != nil {
			r.setEntryState(e, StateFailed)
			r.logger.Error("module lifecycle failed",
				"module", name,
				"operation", operation,
				"error", err,
			)
			errs = append(errs, fmt.Errorf("module %q %s: %w", name, operation, err))
			continue
		}

		r.setEntryState(e, final)
	}

	return errors.Join(errs...)
}

// setEntryState updates an entry's lifecycle state.
func (r *Registry) setEntryState(e *entry, state State) {
	r.mu.Lock()
	e.state = state
	r.mu.Unlock()
}

// invoke runs fn at a module execution boundary and converts a panic
// into an error. recover only catches panics in this goroutine; it
// cannot catch process-level faults such as segfaults.
func (r *Registry) invoke(moduleName, operation string, fn func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			r.logger.Error("module panic recovered",
				"module", moduleName,
				"operation", operation,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			err = fmt.Errorf("panic: %v", recovered)
		}
	}()

	return fn()
}

// readInfo calls Module.Info at a registration boundary, converting a
// panic into an error.
func readInfo(m Module) (info ModuleInfo, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("Module.Info panicked: %v", recovered)
		}
	}()

	return m.Info(), nil
}

func normalizeCapabilities(capabilities []string) ([]string, error) {
	out := make([]string, 0, len(capabilities))
	seen := make(map[string]struct{}, len(capabilities))

	for _, capability := range capabilities {
		if capability == "" {
			return nil, errors.New("capability name must not be empty")
		}
		if _, duplicate := seen[capability]; duplicate {
			continue
		}
		seen[capability] = struct{}{}
		out = append(out, capability)
	}

	return out, nil
}

func capabilitySet(capabilities []string) map[string]struct{} {
	set := make(map[string]struct{}, len(capabilities))
	for _, capability := range capabilities {
		set[capability] = struct{}{}
	}
	return set
}

func cloneInfo(info ModuleInfo) ModuleInfo {
	info.Capabilities = append([]string(nil), info.Capabilities...)
	return info
}

func stateIn(state State, states []State) bool {
	for _, candidate := range states {
		if state == candidate {
			return true
		}
	}
	return false
}
