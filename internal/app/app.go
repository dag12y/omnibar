// Package app implements the Omnibar backend application core.
//
// It is intentionally kept separate from cmd/omnibar so that startup
// behavior can be tested without invoking the binary.
package app

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"omnibar/internal/config"
	"omnibar/internal/events"
	"omnibar/internal/ipc"
	"omnibar/internal/ipc/jsonrpc"
	"omnibar/internal/logging"
	"omnibar/internal/modules"
	"omnibar/internal/modules/apps"
	"omnibar/internal/modules/audio"
	"omnibar/internal/modules/clipboard"
	"omnibar/internal/modules/core"
	"omnibar/internal/modules/fileshelf"
	"omnibar/internal/modules/keepawake"
	"omnibar/internal/modules/systemmonitor"
	"omnibar/internal/modules/windows"
)

// Backend is a running Omnibar backend: the IPC server plus its module
// registry.
type Backend struct {
	Server  *ipc.Server
	Modules *modules.Registry
	Events  *events.Hub

	logger *slog.Logger
}

// Shutdown gracefully stops the IPC server and then stops and disposes
// the registered modules.
//
// Module shutdown errors are logged but do not change the process exit
// status. Repeated calls are safe.
func (b *Backend) Shutdown() error {
	var serverErr error
	if b.Server != nil {
		serverErr = b.Server.Shutdown()
	}

	logger := b.logger
	if logger == nil {
		logger = slog.Default()
	}

	if b.Modules != nil {
		if err := b.Modules.Stop(context.Background()); err != nil {
			logger.Error("stop modules", "error", err)
		}
		if err := b.Modules.Dispose(context.Background()); err != nil {
			logger.Error("dispose modules", "error", err)
		}
	}

	return serverErr
}

// Run executes the Omnibar backend startup sequence and returns the
// started backend. The caller is responsible for shutting it down when
// appropriate (e.g., on signal in main, or in test cleanup).
//
// Startup sequence:
//  1. Parse command-line arguments (--help, --version, --config)
//  2. Handle --help and --version
//  3. Resolve configuration path ($XDG_CONFIG_HOME/omnibar/config.toml,
//     or --config override)
//  4. Load configuration (defaults if file missing)
//  5. Initialize structured logging (log/slog)
//  6. Validate configuration
//  7. Create the module registry and register built-in modules
//  8. Initialize and start enabled modules
//  9. Resolve IPC socket path ($XDG_RUNTIME_DIR/omnibar/omnibar.sock)
//  10. Start IPC server (Go owns the Unix domain socket server)
func Run(args []string, stdout, stderr io.Writer) (*Backend, error) {
	fs := flag.NewFlagSet("omnibar", flag.ContinueOnError)
	fs.SetOutput(stderr)
	help := fs.Bool("help", false, "Show help and exit")
	showVersion := fs.Bool("version", false, "Print the version and exit")
	configPath := fs.String("config", "", "Path to configuration file")
	if err := fs.Parse(args); err != nil {
		return nil, fmt.Errorf("parse arguments: %w", err)
	}

	if *help {
		printUsage(stdout)
		return nil, nil
	}

	if *showVersion {
		fmt.Fprintf(stdout, "omnibar %s\n", ipc.ServerVersion)
		return nil, nil
	}

	// Resolve configuration path.
	path := *configPath
	if path == "" {
		var err error
		path, err = config.ConfigPath()
		if err != nil {
			return nil, fmt.Errorf("resolve config path: %w", err)
		}
	}

	// Load configuration (defaults if file missing).
	cfg, err := config.Load(path)
	if err != nil {
		return nil, fmt.Errorf("load configuration: %w", err)
	}

	// Initialize logging before any further output.
	logger := logging.Init(cfg.Core.LogLevel, stderr)

	// Validate configuration.
	if err := cfg.Validate(); err != nil {
		logger.Error("configuration validation failed", "error", err)
		return nil, fmt.Errorf("validate configuration: %w", err)
	}

	// Build the static module registry.
	registry := modules.NewRegistry(cfg, logger)
	if err := registry.Register(core.New()); err != nil {
		logger.Error("register built-in module", "module", core.Name, "error", err)
		return nil, fmt.Errorf("register built-in module %q: %w", core.Name, err)
	}

	// Create the event hub before registering modules that publish events.
	hub := events.New(logger)

	// Register the system_monitor module. It declares the system.info
	// capability and publishes cpu/memory events while subscribed.
	sm := systemmonitor.New(cfg.Modules[systemmonitor.Name], logger, hub)
	if err := registry.Register(sm); err != nil {
		logger.Error("register built-in module", "module", systemmonitor.Name, "error", err)
		return nil, fmt.Errorf("register built-in module %q: %w", systemmonitor.Name, err)
	}

	awake := keepawake.New(hub, nil)
	if err := registry.Register(awake); err != nil {
		return nil, fmt.Errorf("register keep awake: %w", err)
	}

	mixer := audio.New(hub, nil)
	if err := registry.Register(mixer); err != nil {
		return nil, fmt.Errorf("register audio: %w", err)
	}
	var favorites clipboard.FavoritesStore
	if cfg.Permissions[clipboard.ReadCapability] {
		favorites = clipboard.DefaultFavoritesStore()
	}
	history := clipboard.NewWithStore(nil, favorites)
	if err := registry.Register(history); err != nil {
		return nil, fmt.Errorf("register clipboard: %w", err)
	}
	shelf := fileshelf.New(nil)
	if err := registry.Register(shelf); err != nil {
		return nil, fmt.Errorf("register file shelf: %w", err)
	}
	var layoutStore windows.LayoutStore
	if cfg.Permissions[windows.Capability] {
		layoutStore = windows.DefaultLayoutStore()
	}
	switcher := windows.NewWithLayoutStore(nil, layoutStore)
	if err := registry.Register(switcher); err != nil {
		return nil, fmt.Errorf("register window switcher: %w", err)
	}
	launcher := apps.New(nil)
	if err := registry.Register(launcher); err != nil {
		return nil, fmt.Errorf("register application launcher: %w", err)
	}
	for name := range cfg.Modules {
		if _, ok := registry.Info(name); !ok {
			logger.Warn("configuration references unknown module", "module", name)
		}
	}

	// Initialize and start enabled modules before accepting client
	// connections. A failed module is marked failed by the registry and
	// does not prevent other modules or the backend from starting.
	if err := registry.Initialize(context.Background()); err != nil {
		logger.Error("initialize modules", "error", err)
	}
	if err := registry.Start(context.Background()); err != nil {
		logger.Error("start modules", "error", err)
	}

	logger.Info("omnibar backend starting",
		"config", path,
		"log_level", cfg.Core.LogLevel,
		"modules", len(registry.Names()),
		"enabled_modules", len(registry.EnabledNames()),
		"permissions", len(cfg.Permissions),
	)

	// Create the JSON-RPC dispatcher and register the handshake, module
	// status, event, and system methods.
	dispatcher := jsonrpc.NewDispatcher()
	ipc.RegisterHandshake(dispatcher, registry)
	ipc.RegisterModuleMethods(dispatcher, registry)
	ipc.RegisterEventMethods(dispatcher, registry, hub)
	ipc.RegisterSystemMethods(dispatcher, registry, sm)
	ipc.RegisterKeepAwakeMethods(dispatcher, registry, awake)
	ipc.RegisterAudioMethods(dispatcher, registry, mixer)
	ipc.RegisterClipboardMethods(dispatcher, registry, history)
	ipc.RegisterFileShelfMethods(dispatcher, registry, shelf)
	ipc.RegisterWindowMethods(dispatcher, registry, switcher)
	ipc.RegisterAppMethods(dispatcher, registry, launcher)

	// Resolve IPC socket path.
	socketPath, err := ipc.SocketPath()
	if err != nil {
		logger.Error("resolve socket path", "error", err)
		shutdownModules(registry, logger)
		return nil, fmt.Errorf("resolve socket path: %w", err)
	}

	// Create and start the IPC server. The context-aware dispatcher gives
	// handlers such as module.subscribe access to the client's session.
	server := ipc.NewWithContext(socketPath, logger, dispatcher.HandleContext)
	if err := server.Start(); err != nil {
		logger.Error("start IPC server", "error", err)
		shutdownModules(registry, logger)
		return nil, fmt.Errorf("start IPC server: %w", err)
	}

	return &Backend{Server: server, Modules: registry, Events: hub, logger: logger}, nil
}

// shutdownModules stops and disposes modules after a partial startup
// failure.
func shutdownModules(registry *modules.Registry, logger *slog.Logger) {
	if err := registry.Stop(context.Background()); err != nil {
		logger.Error("stop modules", "error", err)
	}
	if err := registry.Dispose(context.Background()); err != nil {
		logger.Error("dispose modules", "error", err)
	}
}

// isHelpRequest reports whether args contains a help flag. It is
// exported so tests can exercise it without parsing full argument
// vectors.
func isHelpRequest(args []string) bool {
	for _, a := range args {
		if a == "-help" || a == "--help" || a == "-h" || a == "--h" {
			return true
		}
	}
	return false
}

// argsWithoutProgram removes a leading program name if present, so the
// function behaves consistently whether or not os.Args[0] is included.
func argsWithoutProgram(args []string) []string {
	if len(args) > 0 && strings.ContainsRune(args[0], os.PathSeparator) {
		return args[1:]
	}
	return args
}

// printUsage writes the command-line usage to the given writer.
func printUsage(w io.Writer) {
	fmt.Fprintf(w, "Usage: omnibar [flags]\n")
	fmt.Fprintf(w, "\n")
	fmt.Fprintf(w, "Flags:\n")
	fmt.Fprintf(w, "  -help     Show this help and exit\n")
	fmt.Fprintf(w, "  -config   Path to configuration file\n")
	fmt.Fprintf(w, "  -version  Print the version and exit\n")
}
