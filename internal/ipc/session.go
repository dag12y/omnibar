package ipc

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"sync"
	"time"

	"omnibar/internal/events"
)

// DefaultQueueSize is the default number of outbound frames a session
// buffers before a slow client is disconnected (ADR-0009).
const DefaultQueueSize = 64

// drainTimeout bounds how long a closing session waits for a client to
// accept already queued frames.
const drainTimeout = 5 * time.Second

type sessionContextKey struct{}

// SessionFromContext returns the client session that issued the request
// being handled. It is present for requests dispatched by the IPC server.
func SessionFromContext(ctx context.Context) (*Session, bool) {
	s, ok := ctx.Value(sessionContextKey{}).(*Session)
	return s, ok
}

func contextWithSession(ctx context.Context, s *Session) context.Context {
	return context.WithValue(ctx, sessionContextKey{}, s)
}

// Session is the server-side state of one client connection.
//
// It owns the connection's write side: a single goroutine drains a bounded
// queue, so replies and event notifications are never interleaved on the
// wire. Replies come from the connection's reader goroutine and apply
// backpressure to it. Event notifications come from other goroutines and
// never block: if the queue is full, the client is too slow and is
// disconnected so it cannot hold up publishers or other clients.
//
// Session implements events.Subscriber.
type Session struct {
	id     uint64
	conn   net.Conn
	logger *slog.Logger
	out    chan []byte

	// mu guards closed, which means out has been closed. Deliver holds the
	// read lock while sending so it can never send on a closed channel.
	mu     sync.RWMutex
	closed bool

	// dead is closed when the connection is forcibly shut down.
	dead     chan struct{}
	deadOnce sync.Once

	writerDone chan struct{}

	hookMu    sync.Mutex
	hooks     map[string]func()
	hookOrder []string
}

var _ events.Subscriber = (*Session)(nil)

func newSession(id uint64, conn net.Conn, logger *slog.Logger, queueSize int) *Session {
	if queueSize <= 0 {
		queueSize = DefaultQueueSize
	}
	return &Session{
		id:         id,
		conn:       conn,
		logger:     logger.With("session", id),
		out:        make(chan []byte, queueSize),
		dead:       make(chan struct{}),
		writerDone: make(chan struct{}),
		hooks:      make(map[string]func()),
	}
}

// ID returns a process-unique identifier for logging.
func (s *Session) ID() uint64 { return s.id }

// OnClose registers fn to run once when the session ends, before the
// connection is closed. Registering the same key again is a no-op, so
// handlers can call it on every request. Hooks run in registration order
// on the connection's reader goroutine.
func (s *Session) OnClose(key string, fn func()) {
	s.hookMu.Lock()
	defer s.hookMu.Unlock()
	if _, exists := s.hooks[key]; exists {
		return
	}
	s.hooks[key] = fn
	s.hookOrder = append(s.hookOrder, key)
}

func (s *Session) runHooks() {
	s.hookMu.Lock()
	order := s.hookOrder
	hooks := s.hooks
	s.hookOrder = nil
	s.hooks = make(map[string]func())
	s.hookMu.Unlock()

	for _, key := range order {
		func() {
			defer func() {
				if r := recover(); r != nil {
					s.logger.Error("session close hook panicked", "hook", key, "panic", r)
				}
			}()
			hooks[key]()
		}()
	}
}

// startWriter launches the single writer goroutine.
func (s *Session) startWriter() {
	go s.writeLoop(bufio.NewWriterSize(s.conn, 64*1024))
}

func (s *Session) writeLoop(w *bufio.Writer) {
	defer close(s.writerDone)

	for frame := range s.out {
		select {
		case <-s.dead:
			return
		default:
		}

		err := WriteFrame(w, frame)
		if err == nil {
			err = w.Flush()
		}
		if err != nil {
			s.noteWriteError(err)
			s.kill()
			return
		}
	}
}

func (s *Session) noteWriteError(err error) {
	select {
	case <-s.dead:
		return // already torn down; expected
	default:
	}
	if errors.Is(err, net.ErrClosed) {
		return
	}
	s.logger.Error("write frame error", "error", err)
}

// reply queues a response for the client. It blocks while the queue is
// full, which stalls only this connection's reader. It returns false if the
// session was shut down first.
func (s *Session) reply(frame []byte) bool {
	select {
	case s.out <- frame:
		return true
	case <-s.dead:
		return false
	}
}

type eventNotification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  eventParams `json:"params"`
}

type eventParams struct {
	Module string          `json:"module"`
	Event  string          `json:"event"`
	Data   json.RawMessage `json:"data"`
}

// Deliver queues a module.event notification for the client. It never
// blocks. A full queue disconnects the client; an event too large for one
// frame is dropped and logged.
func (s *Session) Deliver(ev events.Event) bool {
	frame, err := json.Marshal(eventNotification{
		JSONRPC: "2.0",
		Method:  "module.event",
		Params:  eventParams{Module: ev.Module, Event: ev.Name, Data: ev.Data},
	})
	if err != nil {
		s.logger.Error("encode event", "module", ev.Module, "event", ev.Name, "error", err)
		return false
	}
	if len(frame) > MaxFrameSize {
		s.logger.Error("dropping event larger than the frame limit",
			"module", ev.Module, "event", ev.Name, "size", len(frame), "limit", MaxFrameSize)
		return false
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		return false
	}
	select {
	case <-s.dead:
		return false
	default:
	}

	select {
	case s.out <- frame:
		return true
	default:
		s.logger.Warn("client too slow, disconnecting",
			"module", ev.Module, "event", ev.Name, "queue", cap(s.out))
		s.kill()
		return false
	}
}

// kill forcibly closes the connection. It is safe to call repeatedly and
// from any goroutine.
func (s *Session) kill() {
	s.deadOnce.Do(func() {
		close(s.dead)
		s.conn.Close()
	})
}

// finish ends the session after the reader has stopped: it runs close
// hooks, lets the writer flush queued replies (bounded by drainTimeout),
// and closes the connection.
func (s *Session) finish() {
	s.runHooks()

	s.mu.Lock()
	s.closed = true
	close(s.out)
	s.mu.Unlock()

	// A client that stopped reading must not hold the connection open.
	_ = s.conn.SetWriteDeadline(time.Now().Add(drainTimeout))
	<-s.writerDone

	s.kill()
}
