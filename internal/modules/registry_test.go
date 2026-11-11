package modules

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"omnibar/internal/config"
)

// staticModule is a minimal Module implementation for tests.
type staticModule struct {
	info ModuleInfo
}

func (m *staticModule) Info() ModuleInfo {
	return m.info
}

// lifecycleModule records lifecycle calls and can simulate failures.
type lifecycleModule struct {
	info            ModuleInfo
	initErr         error
	startErr        error
	stopErr         error
	disposeErr      error
	disposeFailures int
	panicInit       bool

	mu    sync.Mutex
	calls []string
}

func (m *lifecycleModule) Info() ModuleInfo {
	return m.info
}

func (m *lifecycleModule) Initialize(context.Context) error {
	m.record("initialize")
	if m.panicInit {
		panic("initialize panic")
	}
	return m.initErr
}

func (m *lifecycleModule) Start(context.Context) error {
	m.record("start")
	return m.startErr
}

func (m *lifecycleModule) Stop(context.Context) error {
	m.record("stop")
	return m.stopErr
}

func (m *lifecycleModule) Dispose(context.Context) error {
	m.mu.Lock()
	m.calls = append(m.calls, "dispose")
	failures := m.disposeFailures
	if failures > 0 {
		m.disposeFailures--
	}
	m.mu.Unlock()

	if m.disposeErr != nil {
		return m.disposeErr
	}
	if failures > 0 {
		return errors.New("dispose failed")
	}
	return nil
}

func (m *lifecycleModule) record(call string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls = append(m.calls, call)
}

func (m *lifecycleModule) Calls() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]string(nil), m.calls...)
}

func mustRegister(t *testing.T, registry *Registry, module Module) {
	t.Helper()
	if err := registry.Register(module); err != nil {
		t.Fatalf("Register(%q) returned error: %v", module.Info().Name, err)
	}
}

func TestRegisterAndLookup(t *testing.T) {
	registry := NewRegistry(nil, nil)
	module := &staticModule{info: ModuleInfo{
		Name:         "alpha",
		Description:  "Alpha module",
		Version:      "1.2.3",
		Capabilities: []string{"system.info"},
		Enabled:      true,
	}}

	mustRegister(t, registry, module)

	got, ok := registry.Get("alpha")
	if !ok {
		t.Fatal("Get(alpha) not found")
	}
	if got != module {
		t.Error("Get(alpha) returned a different module")
	}

	info, ok := registry.Info("alpha")
	if !ok {
		t.Fatal("Info(alpha) not found")
	}
	if info.Name != "alpha" || info.Description != "Alpha module" || info.Version != "1.2.3" {
		t.Errorf("Info(alpha) = %+v", info)
	}
	if !info.Enabled {
		t.Error("expected alpha to be enabled by default")
	}
	if len(info.Capabilities) != 1 || info.Capabilities[0] != "system.info" {
		t.Errorf("Capabilities = %v, want [system.info]", info.Capabilities)
	}

	state, ok := registry.State("alpha")
	if !ok || state != StateRegistered {
		t.Errorf("State(alpha) = %q, %v; want registered", state, ok)
	}

	if _, ok := registry.Get("missing"); ok {
		t.Error("Get(missing) unexpectedly found a module")
	}
	if _, ok := registry.Info("missing"); ok {
		t.Error("Info(missing) unexpectedly found a module")
	}
}

func TestRegisterRejectsDuplicateNames(t *testing.T) {
	registry := NewRegistry(nil, nil)
	first := &staticModule{info: ModuleInfo{Name: "duplicate", Enabled: true}}
	second := &staticModule{info: ModuleInfo{Name: "duplicate", Enabled: true}}

	mustRegister(t, registry, first)

	err := registry.Register(second)
	if err == nil {
		t.Fatal("expected duplicate registration to fail")
	}
	if !errors.Is(err, ErrDuplicateModule) {
		t.Errorf("error = %v, want ErrDuplicateModule", err)
	}

	got, ok := registry.Get("duplicate")
	if !ok || got != first {
		t.Error("duplicate registration replaced the original module")
	}
	if len(registry.Names()) != 1 {
		t.Errorf("Names() = %v, want one module", registry.Names())
	}
}

func TestRegisterRejectsInvalidModules(t *testing.T) {
	registry := NewRegistry(nil, nil)

	if err := registry.Register(nil); err == nil {
		t.Error("expected nil module registration to fail")
	}
	if err := registry.Register(&staticModule{}); err == nil {
		t.Error("expected empty module name registration to fail")
	}
	if err := registry.Register(&staticModule{info: ModuleInfo{
		Name:         "empty-capability",
		Capabilities: []string{""},
	}}); err == nil {
		t.Error("expected empty capability registration to fail")
	}
}

type panickingInfoModule struct{}

func (m *panickingInfoModule) Info() ModuleInfo {
	panic("Info panic")
}

func TestRegisterRecoversInfoPanic(t *testing.T) {
	registry := NewRegistry(nil, nil)

	err := registry.Register(&panickingInfoModule{})
	if err == nil {
		t.Fatal("expected registration to fail when Module.Info panics")
	}
	if !strings.Contains(err.Error(), "panicked") {
		t.Errorf("error = %v, want panic information", err)
	}
	if len(registry.Names()) != 0 {
		t.Errorf("Names() = %v, want empty registry", registry.Names())
	}
}

func TestListPreservesRegistrationOrder(t *testing.T) {
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "beta", Enabled: true}})
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "alpha", Enabled: true}})
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "gamma", Enabled: true}})

	names := registry.Names()
	want := []string{"beta", "alpha", "gamma"}
	if fmt.Sprint(names) != fmt.Sprint(want) {
		t.Errorf("Names() = %v, want %v", names, want)
	}

	list := registry.List()
	if len(list) != 3 {
		t.Fatalf("List() length = %d, want 3", len(list))
	}
	if list[0].Name != "beta" || list[1].Name != "alpha" || list[2].Name != "gamma" {
		t.Errorf("List() order = %v", []string{list[0].Name, list[1].Name, list[2].Name})
	}
}

func TestInfoAndListReturnCopies(t *testing.T) {
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{
		Name:         "alpha",
		Capabilities: []string{"system.info"},
		Enabled:      true,
	}})

	info, _ := registry.Info("alpha")
	info.Capabilities[0] = "mutated"
	info.Name = "mutated"

	again, _ := registry.Info("alpha")
	if again.Name != "alpha" {
		t.Errorf("Info name was mutated to %q", again.Name)
	}
	if again.Capabilities[0] != "system.info" {
		t.Errorf("Info capabilities were mutated: %v", again.Capabilities)
	}

	list := registry.List()
	list[0].Capabilities[0] = "mutated"
	again, _ = registry.Info("alpha")
	if again.Capabilities[0] != "system.info" {
		t.Errorf("List capabilities were mutated: %v", again.Capabilities)
	}
}

func TestEnabledAndDisabledModules(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Modules["forced-on"] = config.ModuleConfig{Enabled: true}
	cfg.Modules["forced-off"] = config.ModuleConfig{Enabled: false}

	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "default-on", Enabled: true}})
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "default-off", Enabled: false}})
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "forced-on", Enabled: false}})
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "forced-off", Enabled: true}})

	cases := []struct {
		name    string
		enabled bool
	}{
		{"default-on", true},
		{"default-off", false},
		{"forced-on", true},
		{"forced-off", false},
	}
	for _, c := range cases {
		if got := registry.IsEnabled(c.name); got != c.enabled {
			t.Errorf("IsEnabled(%q) = %v, want %v", c.name, got, c.enabled)
		}
		info, ok := registry.Info(c.name)
		if !ok {
			t.Fatalf("Info(%q) not found", c.name)
		}
		if info.Enabled != c.enabled {
			t.Errorf("Info(%q).Enabled = %v, want %v", c.name, info.Enabled, c.enabled)
		}
	}

	enabled := registry.EnabledNames()
	want := []string{"default-on", "forced-on"}
	if fmt.Sprint(enabled) != fmt.Sprint(want) {
		t.Errorf("EnabledNames() = %v, want %v", enabled, want)
	}
	if len(registry.List()) != 4 {
		t.Errorf("List() length = %d, want all 4 registered modules", len(registry.List()))
	}
}

func TestCapabilitiesDeclaredVersusGranted(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Permissions["system.info"] = true
	cfg.Permissions["window.manage"] = true
	cfg.Permissions["audio.control"] = false

	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{
		Name:         "alpha",
		Capabilities: []string{"system.info", "audio.control", "clipboard.read"},
		Enabled:      true,
	}})

	declared := registry.DeclaredCapabilities("alpha")
	if fmt.Sprint(declared) != fmt.Sprint([]string{"system.info", "audio.control", "clipboard.read"}) {
		t.Errorf("DeclaredCapabilities() = %v", declared)
	}

	granted := registry.GrantedCapabilities("alpha")
	if len(granted) != 1 || granted[0] != "system.info" {
		t.Errorf("GrantedCapabilities() = %v, want [system.info]", granted)
	}

	cases := []struct {
		capability string
		want       bool
	}{
		{"system.info", true},     // declared and granted
		{"audio.control", false},  // declared but explicitly denied
		{"clipboard.read", false}, // declared but not granted
		{"window.manage", false},  // granted but not declared
		{"network.access", false}, // neither declared nor granted
		{"", false},               // invalid
	}
	for _, c := range cases {
		if got := registry.HasCapability("alpha", c.capability); got != c.want {
			t.Errorf("HasCapability(alpha, %q) = %v, want %v", c.capability, got, c.want)
		}
	}

	if registry.HasCapability("missing", "system.info") {
		t.Error("HasCapability(missing, ...) must fail closed")
	}
	if got := registry.DeclaredCapabilities("missing"); len(got) != 0 {
		t.Errorf("DeclaredCapabilities(missing) = %v, want empty", got)
	}
	if got := registry.GrantedCapabilities("missing"); len(got) != 0 {
		t.Errorf("GrantedCapabilities(missing) = %v, want empty", got)
	}
}

func TestMissingPermissionsFailClosed(t *testing.T) {
	cfg := config.DefaultConfig()
	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{
		Name:         "alpha",
		Capabilities: []string{"system.info", "clipboard.read"},
		Enabled:      true,
	}})

	if registry.HasCapability("alpha", "system.info") {
		t.Error("missing permission grant must deny access")
	}

	cfg.Permissions["clipboard.read"] = false
	if registry.HasCapability("alpha", "clipboard.read") {
		t.Error("explicitly denied permission must deny access")
	}

	if registry.HasCapability("unknown", "system.info") {
		t.Error("unknown module must fail closed")
	}
}

func TestCapabilityRequiresEnabledModule(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Modules["alpha"] = config.ModuleConfig{Enabled: false}
	cfg.Permissions["system.info"] = true

	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{
		Name:         "alpha",
		Capabilities: []string{"system.info"},
		Enabled:      true,
	}})

	if registry.HasCapability("alpha", "system.info") {
		t.Error("disabled module must not receive a capability")
	}
	if got := registry.GrantedCapabilities("alpha"); len(got) != 1 || got[0] != "system.info" {
		// User grants are still visible; the runtime capability check is
		// what must fail closed for a disabled module.
		t.Errorf("GrantedCapabilities() = %v, want [system.info]", got)
	}
}

func TestConfigurationIsSnapshotted(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Permissions["system.info"] = true
	cfg.Modules["alpha"] = config.ModuleConfig{Enabled: true}

	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{
		Name:         "alpha",
		Capabilities: []string{"system.info"},
		Enabled:      true,
	}})

	cfg.Permissions["system.info"] = false
	cfg.Modules["alpha"] = config.ModuleConfig{Enabled: false}

	if !registry.HasCapability("alpha", "system.info") {
		t.Error("registry should retain its configuration snapshot")
	}
	if !registry.IsEnabled("alpha") {
		t.Error("registry should retain its configuration snapshot")
	}
}

func TestLifecycleTransitions(t *testing.T) {
	module := &lifecycleModule{info: ModuleInfo{Name: "lifecycle", Enabled: true}}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	assertState(t, registry, "lifecycle", StateInitialized)

	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	assertState(t, registry, "lifecycle", StateRunning)

	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	assertState(t, registry, "lifecycle", StateStopped)

	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() returned error: %v", err)
	}
	assertState(t, registry, "lifecycle", StateDisposed)

	want := []string{"initialize", "start", "stop", "dispose"}
	if fmt.Sprint(module.Calls()) != fmt.Sprint(want) {
		t.Errorf("lifecycle calls = %v, want %v", module.Calls(), want)
	}
}

func TestLifecycleSkipsDisabledModules(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Modules["lifecycle"] = config.ModuleConfig{Enabled: false}

	module := &lifecycleModule{info: ModuleInfo{Name: "lifecycle", Enabled: true}}
	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() returned error: %v", err)
	}

	assertState(t, registry, "lifecycle", StateRegistered)
	if len(module.Calls()) != 0 {
		t.Errorf("disabled module lifecycle calls = %v, want none", module.Calls())
	}
}

func TestInitializeFailureStillDisposes(t *testing.T) {
	module := &lifecycleModule{
		info:    ModuleInfo{Name: "failing-init", Enabled: true},
		initErr: errors.New("init failed"),
	}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err == nil {
		t.Fatal("expected Initialize() to report the module failure")
	}
	assertState(t, registry, "failing-init", StateFailed)

	// Initialize is required to clean up its own partial work. Failed
	// modules are not passed to Stop.
	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	assertState(t, registry, "failing-init", StateFailed)

	// Dispose is the final cleanup opportunity for a failed module.
	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() returned error: %v", err)
	}
	assertState(t, registry, "failing-init", StateDisposed)
	assertCalls(t, module, []string{"initialize", "dispose"})
}

func TestStartFailureStillDisposes(t *testing.T) {
	module := &lifecycleModule{
		info:     ModuleInfo{Name: "failing-start", Enabled: true},
		startErr: errors.New("start failed"),
	}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	if err := registry.Start(context.Background()); err == nil {
		t.Fatal("expected Start() to report the module failure")
	}
	assertState(t, registry, "failing-start", StateFailed)

	// Start is required to clean up its own partial work. Failed modules
	// are not passed to Stop.
	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	assertState(t, registry, "failing-start", StateFailed)

	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() returned error: %v", err)
	}
	assertState(t, registry, "failing-start", StateDisposed)
	assertCalls(t, module, []string{"initialize", "start", "dispose"})
}

func TestStopFailureStillDisposes(t *testing.T) {
	module := &lifecycleModule{
		info:    ModuleInfo{Name: "failing-stop", Enabled: true},
		stopErr: errors.New("stop failed"),
	}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if err := registry.Stop(context.Background()); err == nil {
		t.Fatal("expected Stop() to report the module failure")
	}
	assertState(t, registry, "failing-stop", StateFailed)

	// A failed Stop must not prevent final cleanup.
	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() returned error: %v", err)
	}
	assertState(t, registry, "failing-stop", StateDisposed)
	assertCalls(t, module, []string{"initialize", "start", "stop", "dispose"})
}

func TestRepeatedShutdownIsSafe(t *testing.T) {
	module := &lifecycleModule{info: ModuleInfo{Name: "healthy", Enabled: true}}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}
	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("Dispose() returned error: %v", err)
	}

	// Repeated shutdown calls are no-ops after successful cleanup.
	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("repeated Stop() returned error: %v", err)
	}
	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("repeated Dispose() returned error: %v", err)
	}

	assertState(t, registry, "healthy", StateDisposed)
	assertCalls(t, module, []string{"initialize", "start", "stop", "dispose"})
}

func TestDisposeFailureCanBeRetried(t *testing.T) {
	module := &lifecycleModule{
		info:            ModuleInfo{Name: "flaky-dispose", Enabled: true},
		disposeFailures: 1,
	}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}

	if err := registry.Dispose(context.Background()); err == nil {
		t.Fatal("expected first Dispose() to fail")
	}
	assertState(t, registry, "flaky-dispose", StateFailed)

	// A failed Dispose leaves the module eligible for a later retry.
	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("retry Dispose() returned error: %v", err)
	}
	assertState(t, registry, "flaky-dispose", StateDisposed)

	if err := registry.Dispose(context.Background()); err != nil {
		t.Fatalf("repeated Dispose() returned error: %v", err)
	}
	assertCalls(t, module, []string{"initialize", "start", "stop", "dispose", "dispose"})
}

func TestConcurrentDisposeIsSerialized(t *testing.T) {
	module := &lifecycleModule{info: ModuleInfo{Name: "concurrent", Enabled: true}}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	if err := registry.Stop(context.Background()); err != nil {
		t.Fatalf("Stop() returned error: %v", err)
	}

	const workers = 20
	errCh := make(chan error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errCh <- registry.Dispose(context.Background())
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Errorf("concurrent Dispose() error: %v", err)
		}
	}

	assertState(t, registry, "concurrent", StateDisposed)
	assertCalls(t, module, []string{"initialize", "start", "stop", "dispose"})
}

func TestLifecycleFailureMarksModuleFailed(t *testing.T) {
	failing := &lifecycleModule{
		info:    ModuleInfo{Name: "failing", Enabled: true},
		initErr: errors.New("init failed"),
	}
	healthy := &lifecycleModule{info: ModuleInfo{Name: "healthy", Enabled: true}}

	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, failing)
	mustRegister(t, registry, healthy)

	err := registry.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize() to report the module failure")
	}
	assertState(t, registry, "failing", StateFailed)
	assertState(t, registry, "healthy", StateInitialized)

	// Start skips the failed module but still starts healthy modules.
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	assertState(t, registry, "failing", StateFailed)
	assertState(t, registry, "healthy", StateRunning)
	if fmt.Sprint(failing.Calls()) != fmt.Sprint([]string{"initialize"}) {
		t.Errorf("failed module calls = %v, want only initialize", failing.Calls())
	}
}

func TestLifecyclePanicIsRecovered(t *testing.T) {
	module := &lifecycleModule{
		info:      ModuleInfo{Name: "panicking", Enabled: true},
		panicInit: true,
	}
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, module)

	err := registry.Initialize(context.Background())
	if err == nil {
		t.Fatal("expected Initialize() to report the panic")
	}
	if !strings.Contains(err.Error(), "panic") {
		t.Errorf("error = %v, want panic information", err)
	}
	assertState(t, registry, "panicking", StateFailed)

	// The process survived and the module is not restarted.
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	assertState(t, registry, "panicking", StateFailed)
}

func TestFailedModuleLosesCapabilities(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Permissions["system.info"] = true

	module := &lifecycleModule{
		info: ModuleInfo{
			Name:         "panicking",
			Capabilities: []string{"system.info"},
			Enabled:      true,
		},
		panicInit: true,
	}
	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, module)

	if !registry.HasCapability("panicking", "system.info") {
		t.Fatal("expected capability before failure")
	}
	if err := registry.Initialize(context.Background()); err == nil {
		t.Fatal("expected Initialize() to report the panic")
	}
	if registry.HasCapability("panicking", "system.info") {
		t.Error("failed module must not retain capabilities")
	}
}

func TestModulesWithoutLifecycleTransition(t *testing.T) {
	registry := NewRegistry(nil, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{Name: "static", Enabled: true}})

	if err := registry.Initialize(context.Background()); err != nil {
		t.Fatalf("Initialize() returned error: %v", err)
	}
	assertState(t, registry, "static", StateInitialized)
	if err := registry.Start(context.Background()); err != nil {
		t.Fatalf("Start() returned error: %v", err)
	}
	assertState(t, registry, "static", StateRunning)
}

func TestConcurrentRegistryAccess(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Permissions["system.info"] = true

	registry := NewRegistry(cfg, nil)
	mustRegister(t, registry, &staticModule{info: ModuleInfo{
		Name:         "base",
		Capabilities: []string{"system.info"},
		Enabled:      true,
	}})

	var wg sync.WaitGroup
	errCh := make(chan error, 64)

	for i := 0; i < 40; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			name := fmt.Sprintf("writer-%02d", i)
			if err := registry.Register(&staticModule{info: ModuleInfo{Name: name, Enabled: true}}); err != nil {
				errCh <- err
			}
		}()
	}

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_ = registry.List()
				_ = registry.Names()
				_ = registry.EnabledNames()
				_, _ = registry.Info("base")
				_, _ = registry.Get("base")
				_, _ = registry.State("base")
				_ = registry.HasCapability("base", "system.info")
				_ = registry.DeclaredCapabilities("base")
				_ = registry.GrantedCapabilities("base")
			}
		}()
	}

	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent Register() error: %v", err)
	}

	if got := len(registry.Names()); got != 41 {
		t.Errorf("registered modules = %d, want 41", got)
	}
}

func TestConcurrentDuplicateRegistration(t *testing.T) {
	registry := NewRegistry(nil, nil)

	const workers = 20
	start := make(chan struct{})
	results := make(chan error, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			results <- registry.Register(&staticModule{info: ModuleInfo{Name: "same", Enabled: true}})
		}()
	}

	close(start)
	wg.Wait()
	close(results)

	successes := 0
	duplicates := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrDuplicateModule):
			duplicates++
		default:
			t.Errorf("unexpected registration error: %v", err)
		}
	}

	if successes != 1 {
		t.Errorf("successful registrations = %d, want 1", successes)
	}
	if duplicates != workers-1 {
		t.Errorf("duplicate rejections = %d, want %d", duplicates, workers-1)
	}
}

func assertState(t *testing.T, registry *Registry, name string, want State) {
	t.Helper()
	got, ok := registry.State(name)
	if !ok {
		t.Fatalf("State(%q) not found", name)
	}
	if got != want {
		t.Errorf("State(%q) = %q, want %q", name, got, want)
	}
}

func assertCalls(t *testing.T, module *lifecycleModule, want []string) {
	t.Helper()
	if got := module.Calls(); fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("lifecycle calls = %v, want %v", got, want)
	}
}
