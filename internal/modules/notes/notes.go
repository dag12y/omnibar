// Package notes manages explicitly authored local plain-text notes.
package notes

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"omnibar/internal/modules"
	"omnibar/internal/modules/clipboard"
)

const Name = "notes"
const Capability = "notes.manage"
const CopyCapability = "clipboard.write"
const MaxNotes = 100
const MaxBodyBytes = 32 * 1024
const MaxFileBytes = 2 * 1024 * 1024

// Revisions cross the JSON/QML boundary as exact JavaScript integers.
const maxRevision = 1<<53 - 1

var ErrConflict = errors.New("This note changed elsewhere. Reload the saved note or save your draft as a new note")

type Note struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Body     string `json:"body"`
	Pinned   bool   `json:"pinned"`
	Revision uint64 `json:"revision"`
}
type Summary struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Pinned   bool   `json:"pinned"`
	Revision uint64 `json:"revision"`
}
type Snapshot struct {
	Revision     uint64    `json:"revision"`
	Notes        []Summary `json:"notes"`
	CanCopy      bool      `json:"canCopy"`
	StorageError string    `json:"storageError,omitempty"`
}
type Result struct {
	Snapshot
	Note *Note `json:"note,omitempty"`
}
type Data struct {
	Version  int    `json:"version"`
	Revision uint64 `json:"revision"`
	Notes    []Note `json:"notes"`
}
type Store interface {
	Load() (Data, error)
	Save(Data) error
}
type Copier interface {
	Copy(context.Context, string) error
}
type Module struct {
	mu      sync.Mutex
	store   Store
	copier  Copier
	data    Data
	loadErr string
}

func New(store Store, copier Copier) *Module {
	if copier == nil {
		copier = clipboard.Wayland{}
	}
	return &Module{store: store, copier: copier, data: Data{Version: 1, Revision: 1, Notes: []Note{}}}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Local plain-text Scratchpad", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability, CopyCapability}}
}
func (m *Module) Initialize(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		m.loadErr = "Notes storage is unavailable"
		return nil
	}
	data, err := m.store.Load()
	if err == nil {
		err = validateData(data)
	}
	if err != nil {
		m.loadErr = "Notes could not be loaded; original file preserved"
		return nil
	}
	m.data = data
	m.loadErr = ""
	return nil
}
func (m *Module) Start(context.Context) error { return nil }
func (m *Module) Stop(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.data = Data{Version: 1, Revision: 1, Notes: []Note{}}
	return nil
}
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }
func ValidID(id string) bool {
	b, err := hex.DecodeString(id)
	return err == nil && len(b) == 16 && id == strings.ToLower(id)
}
func ValidRevision(r uint64) bool { return r > 0 && r <= maxRevision }
func ValidTitle(s string) bool {
	return s != "" && strings.TrimSpace(s) == s && len(s) <= 128 && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func ValidBody(s string) bool {
	return len(s) <= MaxBodyBytes && utf8.ValidString(s) && !strings.ContainsRune(s, 0)
}
func validateData(d Data) error {
	if d.Version != 1 || !ValidRevision(d.Revision) || len(d.Notes) > MaxNotes {
		return fmt.Errorf("invalid notes data")
	}
	ids := map[string]bool{}
	for _, n := range d.Notes {
		if !ValidID(n.ID) || !ValidTitle(n.Title) || !ValidBody(n.Body) || !ValidRevision(n.Revision) || n.Revision > d.Revision || ids[n.ID] {
			return fmt.Errorf("invalid note")
		}
		ids[n.ID] = true
	}
	return nil
}
func (m *Module) snapshot(query string) Snapshot {
	rows := []Summary{}
	tokens := strings.Fields(strings.ToLower(query))
	for _, n := range m.data.Notes {
		hay := strings.ToLower(n.Title + "\n" + n.Body)
		match := true
		for _, token := range tokens {
			if !strings.Contains(hay, token) {
				match = false
				break
			}
		}
		if match {
			rows = append(rows, Summary{n.ID, n.Title, n.Pinned, n.Revision})
		}
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Pinned != rows[j].Pinned {
			return rows[i].Pinned
		}
		if rows[i].Revision != rows[j].Revision {
			return rows[i].Revision > rows[j].Revision
		}
		return rows[i].ID < rows[j].ID
	})
	return Snapshot{Revision: m.data.Revision, Notes: rows, StorageError: m.loadErr}
}
func (m *Module) Snapshot(query string) Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.snapshot(query)
}
func (m *Module) find(id string) (int, error) {
	if m.loadErr != "" {
		return -1, errors.New(m.loadErr)
	}
	for i, n := range m.data.Notes {
		if n.ID == id {
			return i, nil
		}
	}
	return -1, fmt.Errorf("Note is no longer available; draft kept")
}
func (m *Module) Get(id, query string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, err := m.find(id)
	if err != nil {
		return Result{}, err
	}
	n := m.data.Notes[i]
	return Result{m.snapshot(query), &n}, nil
}
func (m *Module) commit(next Data) error {
	if m.loadErr != "" {
		return errors.New(m.loadErr)
	}
	if m.store == nil {
		return fmt.Errorf("Notes storage is unavailable")
	}
	if err := validateData(next); err != nil {
		return err
	}
	if err := m.store.Save(next); err != nil {
		return err
	}
	m.data = next
	return nil
}
func (m *Module) next() (Data, error) {
	if m.data.Revision >= maxRevision {
		return Data{}, fmt.Errorf("Note revision limit reached")
	}
	return Data{Version: 1, Revision: m.data.Revision + 1, Notes: append([]Note{}, m.data.Notes...)}, nil
}
func (m *Module) Create(title, body, query string) (Result, error) {
	if !ValidTitle(title) || !ValidBody(body) {
		return Result{}, fmt.Errorf("Use a title of 1–128 bytes and note text up to 32 KiB")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.data.Notes) >= MaxNotes {
		return Result{}, fmt.Errorf("Note limit reached (100)")
	}
	next, err := m.next()
	if err != nil {
		return Result{}, err
	}
	id := make([]byte, 16)
	if _, err := rand.Read(id); err != nil {
		return Result{}, fmt.Errorf("Could not create note ID")
	}
	n := Note{ID: hex.EncodeToString(id), Title: title, Body: body, Revision: next.Revision}
	next.Notes = append(next.Notes, n)
	if err := m.commit(next); err != nil {
		return Result{}, err
	}
	return Result{m.snapshot(query), &n}, nil
}
func (m *Module) Update(n Note, query string) (Result, error) {
	if !ValidID(n.ID) || !ValidRevision(n.Revision) || !ValidTitle(n.Title) || !ValidBody(n.Body) {
		return Result{}, fmt.Errorf("Invalid note; title limit 128 bytes, text limit 32 KiB")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	i, err := m.find(n.ID)
	if err != nil {
		return Result{}, err
	}
	old := m.data.Notes[i]
	if old.Revision != n.Revision {
		return Result{}, ErrConflict
	}
	if old == n {
		return Result{m.snapshot(query), &old}, nil
	}
	next, err := m.next()
	if err != nil {
		return Result{}, err
	}
	n.Revision = next.Revision
	next.Notes[i] = n
	if err := m.commit(next); err != nil {
		return Result{}, err
	}
	return Result{m.snapshot(query), &n}, nil
}
func (m *Module) Delete(id string, revision uint64, query string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, err := m.find(id)
	if err != nil {
		return Result{}, err
	}
	if m.data.Notes[i].Revision != revision {
		return Result{}, ErrConflict
	}
	next, err := m.next()
	if err != nil {
		return Result{}, err
	}
	next.Notes = append(next.Notes[:i], next.Notes[i+1:]...)
	if err := m.commit(next); err != nil {
		return Result{}, err
	}
	return Result{Snapshot: m.snapshot(query)}, nil
}
func (m *Module) Copy(ctx context.Context, id string, revision uint64, query string) (Result, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	i, err := m.find(id)
	if err != nil {
		return Result{}, err
	}
	n := m.data.Notes[i]
	if n.Revision != revision {
		return Result{}, ErrConflict
	}
	if n.Body == "" {
		return Result{}, fmt.Errorf("This note has no text to copy")
	}
	if err := m.copier.Copy(ctx, n.Body); err != nil {
		return Result{}, err
	}
	return Result{Snapshot: m.snapshot(query)}, nil
}
