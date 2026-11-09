// Package ipc implements the Unix domain socket server for Omnibar.
//
// Go owns and listens on the socket at $XDG_RUNTIME_DIR/omnibar/omnibar.sock.
// The QML UI connects later using Quickshell.Io.Socket.
//
// The server provides transport and framing only: newline-delimited
// messages with a 1 MiB maximum frame size. No JSON-RPC or application
// logic is implemented here.
package ipc

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Handler processes an incoming message frame and returns an optional
// reply. A nil reply means no response is sent. This is used only for
// testing framing and connection behavior; no application logic lives
// here.
type Handler func([]byte) ([]byte, error)

// lifecycleState tracks the server lifecycle.
type lifecycleState int

const (
	stateNew lifecycleState = iota
	stateRunning
	stateStopped
)

// Server is a Unix domain socket server. It accepts multiple client
// connections, reads newline-delimited frames from each, and dispatches
// them to a Handler. The server owns its socket file and removes it on
// shutdown.
type Server struct {
	path     string
	lockPath string
	logger   *slog.Logger
	handler  Handler

	mu         sync.Mutex
	state      lifecycleState
	listener   net.Listener
	conns      map[net.Conn]struct{}
	wg         sync.WaitGroup
	ctx        context.Context
	cancel     context.CancelFunc
	lockFd     int
	socketInfo os.FileInfo
}

// New creates a Server bound to path. It does not bind or listen;
// call Start to begin accepting connections.
//
// lockFd is initialized to -1 so that Shutdown() on an unstarted server
// never closes stdin or any other unrelated file descriptor.
func New(path string, logger *slog.Logger, handler Handler) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		path:     path,
		lockPath: filepath.Join(filepath.Dir(path), lockFileName),
		logger:   logger,
		handler:  handler,
		conns:    make(map[net.Conn]struct{}),
		ctx:      ctx,
		cancel:   cancel,
		lockFd:   -1,
		state:    stateNew,
	}
}

// Start binds the Unix domain socket and begins accepting connections.
//
// Single-instance safety is provided by an exclusive, non-blocking flock
// on a lock file inside the private runtime directory. The lock is held
// for the entire lifetime of the server and released on shutdown or
// startup failure. The lock file is intentionally not deleted, because
// deleting lock files can introduce inode races.
//
// The complete socket startup sequence — stale socket inspection, stale
// socket removal, and net.Listen() — is serialized by the lock, so no
// race exists between checking and acting.
//
// The lifecycle state is held across the whole startup sequence so a
// concurrent Shutdown cannot observe or clean up a half-constructed
// server. A failed startup returns the server to the new state so a
// later Start may retry.
//
// The socket file is created with mode 0600.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	switch s.state {
	case stateRunning:
		return errors.New("server already started")
	case stateStopped:
		return errors.New("server already stopped")
	}
	s.state = stateRunning

	// Ensure the runtime directory exists with safe permissions.
	if err := EnsureRuntimeDir(s.path); err != nil {
		s.state = stateNew
		return err
	}

	// Acquire the exclusive lock before any socket inspection or
	// modification. This serializes all startup attempts.
	lockFd, err := acquireLock(s.lockPath)
	if err != nil {
		s.state = stateNew
		return err
	}
	s.lockFd = lockFd

	// failStart cleans up any partially created socket, releases the
	// lock, and returns the server to the new state. It is only called
	// with the mutex held.
	failStart := func(listener net.Listener, err error) error {
		s.cleanupSocket(listener)
		s.listener = nil
		if s.lockFd >= 0 {
			releaseLock(s.lockFd)
			s.lockFd = -1
		}
		s.state = stateNew
		return err
	}

	// Conservative stale socket check and removal (serialized by the
	// lock).
	stale, err := IsStaleSocket(s.path)
	if err != nil {
		return failStart(nil, fmt.Errorf("check existing socket %q: %w", s.path, err))
	}
	if stale {
		if err := os.Remove(s.path); err != nil {
			return failStart(nil, fmt.Errorf("remove stale socket %q: %w", s.path, err))
		}
	}

	listener, err := net.Listen("unix", s.path)
	if err != nil {
		return failStart(nil, fmt.Errorf("listen on %q: %w", s.path, err))
	}

	// Go's UnixListener unlinks the socket path by default when it is
	// closed. That would remove whatever entry currently owns the path,
	// including a replacement created after Listen. Disable the
	// automatic unlink so cleanup can verify ownership first.
	if ul, ok := listener.(*net.UnixListener); ok {
		ul.SetUnlinkOnClose(false)
	}
	s.listener = listener

	// Record socket identity immediately after successful Listen, so
	// that shutdown and failed-startup cleanup can verify the path
	// still refers to the socket this server created.
	info, err := os.Lstat(s.path)
	if err != nil {
		return failStart(listener, fmt.Errorf("stat socket %q: %w", s.path, err))
	}
	s.socketInfo = info

	if err := os.Chmod(s.path, 0o600); err != nil {
		return failStart(listener, fmt.Errorf("set socket permissions on %q: %w", s.path, err))
	}

	s.logger.Info("ipc server listening", "path", s.path)

	s.wg.Add(1)
	go s.acceptLoop()

	return nil
}

// cleanupSocket closes listener and removes the socket file only if the
// path still identifies the socket this server created. It is shared by
// failed startup cleanup and normal shutdown so both paths apply the
// same ownership-verified safety rules.
//
// The listener is configured with SetUnlinkOnClose(false) before this
// helper runs, so closing it never removes the path on its own. That
// prevents a listener close from unlinking a replacement entry that
// another process may have created at the socket path.
func (s *Server) cleanupSocket(listener net.Listener) {
	if listener != nil {
		if err := listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
			s.logger.Error("close listener error", "error", err)
		}
	}

	if s.socketInfo == nil {
		return
	}

	current, err := os.Lstat(s.path)
	if err != nil {
		// The socket is already gone or the path is inaccessible; there
		// is nothing safe to remove.
		return
	}
	if !os.SameFile(s.socketInfo, current) {
		// A different entry now owns the path. Never remove it.
		return
	}
	if err := os.Remove(s.path); err != nil && !os.IsNotExist(err) {
		s.logger.Error("remove socket error", "error", err)
	}
}

// acceptLoop accepts connections until the listener is closed.
//
// Connection registration and WaitGroup accounting happen while holding
// the mutex, in the same critical section that checks the lifecycle
// state. Shutdown marks the server stopped under the mutex before it
// closes connections and waits, so once Shutdown has begun waiting no
// handler can be added to the WaitGroup.
func (s *Server) acceptLoop() {
	defer s.wg.Done()

	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.ctx.Done():
				return
			default:
			}
			s.logger.Error("accept error", "error", err)
			return
		}

		s.mu.Lock()
		if s.state != stateRunning {
			// Shutdown has started: close the newly accepted connection
			// immediately and do not register it or start a handler.
			s.mu.Unlock()
			conn.Close()
			continue
		}
		s.conns[conn] = struct{}{}
		s.wg.Add(1)
		s.mu.Unlock()

		go s.handleConn(conn)
	}
}

// handleConn reads frames from conn and dispatches them to the handler.
func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		s.wg.Done()
	}()

	reader := bufio.NewReaderSize(conn, 64*1024)
	writer := bufio.NewWriterSize(conn, 64*1024)

	for {
		msg, err := ReadFrame(reader)
		if err != nil {
			if err == io.EOF {
				return
			}
			if err == io.ErrUnexpectedEOF {
				s.logger.Warn("client disconnected with incomplete frame")
				return
			}
			s.logger.Error("read frame error", "error", err)
			return
		}

		s.logger.Debug("received frame", "size", len(msg))

		if s.handler != nil {
			reply, err := s.handler(msg)
			if err != nil {
				s.logger.Error("handler error", "error", err)
				continue
			}
			if reply != nil {
				if err := WriteFrame(writer, reply); err != nil {
					s.logger.Error("write frame error", "error", err)
					return
				}
				if err := writer.Flush(); err != nil {
					s.logger.Error("flush error", "error", err)
					return
				}
			}
		}
	}
}

// Shutdown stops the server gracefully.
//
// It marks the server as stopping under the mutex before any cleanup, so
// the accept loop can no longer register connections or add handlers to
// the WaitGroup. It cancels the context, closes all registered
// connections, closes the listener, removes the socket file only if this
// server still owns it, waits for every handler to finish, and only then
// releases the flock.
//
// Safe to call on an unstarted server (lockFd == -1). Repeated calls are
// safe.
func (s *Server) Shutdown() error {
	s.mu.Lock()
	if s.state == stateStopped {
		s.mu.Unlock()
		return nil
	}
	s.state = stateStopped

	listener := s.listener
	conns := s.conns
	s.conns = make(map[net.Conn]struct{})

	lockFd := s.lockFd
	s.lockFd = -1
	s.mu.Unlock()

	s.cancel()

	// Unblock every handler that is waiting in ReadFrame by closing its
	// connection and expiring any read deadline.
	for conn := range conns {
		conn.SetReadDeadline(time.Now())
		conn.Close()
	}

	// Close the listener and remove the socket while still holding the
	// flock. This unblocks Accept and performs ownership-verified
	// cleanup before waiting for handlers.
	s.mu.Lock()
	s.cleanupSocket(listener)
	s.mu.Unlock()

	s.wg.Wait()

	// Release the lock only after socket cleanup and handler shutdown
	// are complete.
	releaseLock(lockFd)

	s.logger.Info("ipc server shutdown", "path", s.path)
	return nil
}
