// Package timers owns durable, explicitly started countdowns.
package timers

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"omnibar/internal/events"
	"omnibar/internal/modules"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"
)

const Name = "timers"
const Capability = "timers.manage"
const MaxSeconds = 86400
const MaxUnfinished = 10
const MaxCompleted = 20
const MaxFileBytes = 64 * 1024
const maxRevision = 1<<53 - 1

var ErrConflict = errors.New("Timer changed; refresh before trying again")
var bootPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

type Record struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	State          string `json:"state"`
	Seconds        int    `json:"seconds"`
	Revision       uint64 `json:"revision"`
	RemainingMS    int64  `json:"remainingMs"`
	DeadlineWallMS int64  `json:"deadlineWallMs"`
	DeadlineBootNS int64  `json:"deadlineBootNs"`
}
type Data struct {
	Version  int      `json:"version"`
	Revision uint64   `json:"revision"`
	BootID   string   `json:"bootId"`
	Timers   []Record `json:"timers"`
}
type Row struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	State        string `json:"state"`
	Seconds      int    `json:"seconds"`
	Revision     uint64 `json:"revision"`
	RemainingMS  int64  `json:"remainingMs"`
	Notification string `json:"notification,omitempty"`
}
type Snapshot struct {
	Revision     uint64 `json:"revision"`
	Timers       []Row  `json:"timers"`
	Running      int    `json:"running"`
	Paused       int    `json:"paused"`
	Completed    int    `json:"completed"`
	StorageError string `json:"storageError,omitempty"`
}
type Store interface {
	Load() (Data, error)
	Save(Data) error
}
type Module struct {
	mu             sync.Mutex
	store          Store
	clock          Clock
	notifier       Notifier
	hub            *events.Hub
	data           Data
	loadErr, ioErr string
	alerts         map[string]string
	running        bool
	ctx            context.Context
	cancel         context.CancelFunc
	wake           *time.Timer
	generation     uint64
	workers        sync.WaitGroup
}

func New(store Store, clock Clock, notifier Notifier, hub *events.Hub) *Module {
	if clock == nil {
		clock = &LinuxClock{}
	}
	if notifier == nil {
		notifier = DesktopNotifier{}
	}
	return &Module{store: store, clock: clock, notifier: notifier, hub: hub, data: Data{Version: 1, Revision: 1, Timers: []Record{}}, alerts: map[string]string{}}
}
func (m *Module) Info() modules.ModuleInfo {
	return modules.ModuleInfo{Name: Name, Description: "Durable countdown timers", Version: "0.1.0", Enabled: true, Capabilities: []string{Capability}}
}
func ValidID(id string) bool {
	b, e := hex.DecodeString(id)
	return e == nil && len(b) == 16 && id == strings.ToLower(id)
}
func ValidName(s string) bool {
	return s != "" && strings.TrimSpace(s) == s && len(s) <= 64 && utf8.ValidString(s) && strings.IndexFunc(s, unicode.IsControl) < 0
}
func ValidRevision(r uint64) bool { return r > 0 && r <= maxRevision }
func validateData(d Data) error {
	if d.Version != 1 || !ValidRevision(d.Revision) || len(d.Timers) > MaxUnfinished+MaxCompleted || (d.BootID != "" && !bootPattern.MatchString(d.BootID)) {
		return fmt.Errorf("invalid timer data")
	}
	seen := map[string]bool{}
	unfinished, completed := 0, 0
	for _, r := range d.Timers {
		if !ValidID(r.ID) || seen[r.ID] || !ValidName(r.Name) || r.Seconds < 1 || r.Seconds > MaxSeconds || !ValidRevision(r.Revision) || r.Revision > d.Revision {
			return fmt.Errorf("invalid timer")
		}
		seen[r.ID] = true
		switch r.State {
		case "running":
			unfinished++
			if d.BootID == "" || r.DeadlineBootNS <= 0 || r.DeadlineWallMS <= 0 || r.DeadlineWallMS > maxRevision || r.RemainingMS != 0 {
				return fmt.Errorf("invalid running timer")
			}
		case "paused":
			unfinished++
			if r.RemainingMS < 1 || r.RemainingMS > int64(r.Seconds)*1000 || r.DeadlineBootNS != 0 || r.DeadlineWallMS != 0 {
				return fmt.Errorf("invalid paused timer")
			}
		case "completed":
			completed++
			if r.RemainingMS != 0 || r.DeadlineBootNS != 0 || r.DeadlineWallMS != 0 {
				return fmt.Errorf("invalid completed timer")
			}
		default:
			return fmt.Errorf("invalid timer state")
		}
	}
	if unfinished > MaxUnfinished || completed > MaxCompleted {
		return fmt.Errorf("timer limit exceeded")
	}
	return nil
}
func validSample(s Sample) bool {
	return bootPattern.MatchString(s.BootID) && s.BootNS > 0 && s.WallMS > 0 && s.WallMS <= maxRevision-int64(MaxSeconds)*1000
}
func (m *Module) sample() (Sample, error) {
	s, e := m.clock.Sample()
	if e != nil || !validSample(s) {
		return Sample{}, fmt.Errorf("timer clock unavailable")
	}
	return s, nil
}
func (m *Module) Initialize(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.store == nil {
		m.loadErr = "Timers storage is unavailable"
		return nil
	}
	d, e := m.store.Load()
	if e == nil {
		e = validateData(d)
	}
	if e != nil {
		m.loadErr = "Timers could not be loaded; original file preserved"
		return nil
	}
	m.data = d
	if !hasRunning(d) {
		return nil
	}
	now, e := m.sample()
	if e != nil {
		m.loadErr = e.Error()
		return nil
	}
	if d.BootID != now.BootID {
		next, e := m.next()
		if e != nil {
			m.loadErr = e.Error()
			return nil
		}
		next.BootID = now.BootID
		for i, r := range next.Timers {
			if r.State == "running" {
				left := max(int64(0), min(int64(r.Seconds)*1000, r.DeadlineWallMS-now.WallMS))
				r.DeadlineWallMS = now.WallMS + left
				r.DeadlineBootNS = now.BootNS + left*int64(time.Millisecond)
				r.Revision = next.Revision
				next.Timers[i] = r
			}
		}
		if e := m.commit(next); e != nil {
			m.loadErr = "Timers could not recover after reboot; original file preserved"
		}
	}
	return nil
}
func (m *Module) Start(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.running {
		return nil
	}
	m.ctx, m.cancel = context.WithCancel(context.Background())
	m.running = true
	m.schedule(0)
	return nil
}
func (m *Module) Stop(context.Context) error {
	m.mu.Lock()
	m.running = false
	m.generation++
	if m.wake != nil {
		m.wake.Stop()
		m.wake = nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.mu.Unlock()
	m.workers.Wait()
	return nil
}
func (m *Module) Dispose(ctx context.Context) error { return m.Stop(ctx) }
func hasRunning(d Data) bool {
	for _, r := range d.Timers {
		if r.State == "running" {
			return true
		}
	}
	return false
}
func remaining(r Record, d Data, now Sample) int64 {
	switch r.State {
	case "paused":
		return r.RemainingMS
	case "running":
		if now.BootID == d.BootID {
			return max(int64(0), min(int64(r.Seconds)*1000, (r.DeadlineBootNS-now.BootNS+int64(time.Millisecond)-1)/int64(time.Millisecond)))
		}
		return max(int64(0), min(int64(r.Seconds)*1000, r.DeadlineWallMS-now.WallMS))
	}
	return 0
}
func (m *Module) snapshot() Snapshot {
	out := Snapshot{Revision: m.data.Revision, Timers: []Row{}, StorageError: m.loadErr}
	if out.StorageError == "" {
		out.StorageError = m.ioErr
	}
	var now Sample
	if hasRunning(m.data) {
		var e error
		now, e = m.sample()
		if e != nil && out.StorageError == "" {
			out.StorageError = e.Error()
		}
	}
	for _, r := range m.data.Timers {
		row := Row{ID: r.ID, Name: r.Name, State: r.State, Seconds: r.Seconds, Revision: r.Revision, RemainingMS: remaining(r, m.data, now)}
		switch r.State {
		case "running":
			out.Running++
		case "paused":
			out.Paused++
		case "completed":
			out.Completed++
			row.Notification = m.alerts[r.ID]
			if row.Notification == "" {
				row.Notification = "attempted"
			}
		}
		out.Timers = append(out.Timers, row)
	}
	sort.SliceStable(out.Timers, func(i, j int) bool {
		a, b := out.Timers[i], out.Timers[j]
		if (a.State == "completed") != (b.State == "completed") {
			return a.State != "completed"
		}
		return a.Revision > b.Revision
	})
	return out
}
func (m *Module) Snapshot() Snapshot { m.mu.Lock(); defer m.mu.Unlock(); return m.snapshot() }
func (m *Module) publish() {
	if m.hub != nil {
		_ = m.hub.Publish(Name, "state", m.snapshot())
	}
}
func (m *Module) next() (Data, error) {
	if m.data.Revision >= maxRevision {
		return Data{}, fmt.Errorf("Timer revision limit reached")
	}
	d := m.data
	d.Revision++
	d.Timers = append([]Record{}, d.Timers...)
	return d, nil
}
func (m *Module) commit(d Data) error {
	if m.loadErr != "" {
		return errors.New(m.loadErr)
	}
	if m.store == nil {
		return fmt.Errorf("Timers storage unavailable")
	}
	if e := validateData(d); e != nil {
		return e
	}
	if e := m.store.Save(d); e != nil {
		m.ioErr = "Timer changes could not be saved; previous state kept"
		m.publish()
		return errors.New(m.ioErr)
	}
	m.data = d
	// Completion history is bounded, including ephemeral alert results.
	present := map[string]bool{}
	for _, r := range d.Timers {
		present[r.ID] = true
	}
	for id := range m.alerts {
		if !present[id] {
			delete(m.alerts, id)
		}
	}
	m.ioErr = ""
	return nil
}
func (m *Module) available() error {
	if !m.running {
		return fmt.Errorf("Timers unavailable")
	}
	if m.loadErr != "" {
		return errors.New(m.loadErr)
	}
	return nil
}
func (m *Module) Begin(id, name string, seconds int) (Snapshot, error) {
	if (id != "" && !ValidID(id)) || !ValidName(name) || seconds < 1 || seconds > MaxSeconds {
		return Snapshot{}, fmt.Errorf("Use a name up to 64 bytes and 1–86400 seconds")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.available(); e != nil {
		return Snapshot{}, e
	}
	if id != "" {
		for _, r := range m.data.Timers {
			if r.ID == id {
				if r.Name == name && r.Seconds == seconds {
					return m.snapshot(), nil
				}
				return Snapshot{}, fmt.Errorf("Timer ID already belongs to another request")
			}
		}
	}
	count := 0
	for _, r := range m.data.Timers {
		if r.State != "completed" {
			count++
		}
	}
	if count >= MaxUnfinished {
		return Snapshot{}, fmt.Errorf("Unfinished timer limit reached (10)")
	}
	now, e := m.sample()
	if e != nil {
		return Snapshot{}, e
	}
	next, e := m.next()
	if e != nil {
		return Snapshot{}, e
	}
	if id == "" {
		b := make([]byte, 16)
		if _, e := rand.Read(b); e != nil {
			return Snapshot{}, fmt.Errorf("Could not create timer ID")
		}
		id = hex.EncodeToString(b)
	}
	next.BootID = now.BootID
	next.Timers = append(next.Timers, Record{ID: id, Name: name, State: "running", Seconds: seconds, Revision: next.Revision, DeadlineBootNS: now.BootNS + int64(seconds)*int64(time.Second), DeadlineWallMS: now.WallMS + int64(seconds)*1000})
	if e := m.commit(next); e != nil {
		return Snapshot{}, e
	}
	m.schedule(0)
	m.publish()
	return m.snapshot(), nil
}
func (m *Module) Change(action, id string, revision uint64) (Snapshot, error) {
	if !ValidID(id) || !ValidRevision(revision) {
		return Snapshot{}, fmt.Errorf("Invalid timer ID or revision")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if e := m.available(); e != nil {
		return Snapshot{}, e
	}
	index := -1
	for i, r := range m.data.Timers {
		if r.ID == id {
			index = i
			break
		}
	}
	if index < 0 {
		return Snapshot{}, fmt.Errorf("Timer is no longer available")
	}
	r := m.data.Timers[index]
	if r.Revision != revision {
		return Snapshot{}, ErrConflict
	}
	next, e := m.next()
	if e != nil {
		return Snapshot{}, e
	}
	r.Revision = next.Revision
	switch action {
	case "pause":
		if r.State != "running" {
			return Snapshot{}, fmt.Errorf("Only running timers can be paused")
		}
		now, e := m.sample()
		if e != nil {
			return Snapshot{}, e
		}
		left := remaining(r, m.data, now)
		if left <= 0 {
			return Snapshot{}, fmt.Errorf("Timer has elapsed; refresh for completion")
		}
		r.State = "paused"
		r.RemainingMS = left
		r.DeadlineBootNS = 0
		r.DeadlineWallMS = 0
		next.Timers[index] = r
	case "resume":
		if r.State != "paused" {
			return Snapshot{}, fmt.Errorf("Only paused timers can be resumed")
		}
		now, e := m.sample()
		if e != nil {
			return Snapshot{}, e
		}
		next.BootID = now.BootID
		r.State = "running"
		r.DeadlineBootNS = now.BootNS + r.RemainingMS*int64(time.Millisecond)
		r.DeadlineWallMS = now.WallMS + r.RemainingMS
		r.RemainingMS = 0
		next.Timers[index] = r
	case "cancel", "dismiss":
		if (action == "dismiss") != (r.State == "completed") {
			return Snapshot{}, fmt.Errorf("Use Dismiss for completed timers and Cancel for unfinished timers")
		}
		next.Timers = append(next.Timers[:index], next.Timers[index+1:]...)
	default:
		return Snapshot{}, fmt.Errorf("Unknown timer action")
	}
	if e := m.commit(next); e != nil {
		return Snapshot{}, e
	}
	if action == "cancel" || action == "dismiss" {
		delete(m.alerts, id)
	}
	m.schedule(0)
	m.publish()
	return m.snapshot(), nil
}
func (m *Module) schedule(delay time.Duration) {
	m.generation++
	generation := m.generation
	if m.wake != nil {
		m.wake.Stop()
		m.wake = nil
	}
	if !m.running || m.loadErr != "" || !hasRunning(m.data) {
		return
	}
	if m.ioErr != "" {
		delay = 5 * time.Second
	}
	m.wake = time.AfterFunc(delay, func() {
		m.mu.Lock()
		valid := generation == m.generation && m.running
		m.mu.Unlock()
		if valid {
			m.tick()
		}
	})
}
func trimCompleted(d *Data) {
	for {
		count, index := 0, -1
		for i, r := range d.Timers {
			if r.State == "completed" {
				count++
				if index < 0 || r.Revision < d.Timers[index].Revision {
					index = i
				}
			}
		}
		if count <= MaxCompleted {
			return
		}
		d.Timers = append(d.Timers[:index], d.Timers[index+1:]...)
	}
}
func (m *Module) tick() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running || m.loadErr != "" {
		return
	}
	now, e := m.sample()
	if e != nil {
		m.ioErr = e.Error()
		m.publish()
		m.schedule(5 * time.Second)
		return
	}
	next, e := m.next()
	if e != nil {
		m.ioErr = e.Error()
		m.publish()
		m.schedule(5 * time.Second)
		return
	}
	jobs := []Record{}
	for i, r := range next.Timers {
		if r.State == "running" && remaining(r, m.data, now) == 0 {
			r.State = "completed"
			r.RemainingMS = 0
			r.DeadlineBootNS = 0
			r.DeadlineWallMS = 0
			r.Revision = next.Revision
			next.Timers[i] = r
			jobs = append(jobs, r)
		}
	}
	if len(jobs) > 0 {
		trimCompleted(&next)
		if e := m.commit(next); e != nil {
			m.schedule(5 * time.Second)
			return
		}
		for _, r := range jobs {
			m.alerts[r.ID] = "sending"
			m.workers.Add(1)
			go m.notify(m.ctx, r)
		}
		m.publish()
	}
	m.schedule(time.Second)
}
func (m *Module) notify(parent context.Context, r Record) {
	defer m.workers.Done()
	ctx, cancel := context.WithTimeout(parent, 3*time.Second)
	defer cancel()
	var err error
	if ctx.Err() != nil {
		return
	}
	err = m.notifier.Notify(ctx, r.Name)
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.running {
		return
	}
	for _, current := range m.data.Timers {
		if current.ID == r.ID && current.Revision == r.Revision && current.State == "completed" {
			if err == nil {
				m.alerts[r.ID] = "sent"
			} else {
				m.alerts[r.ID] = "unavailable"
			}
			m.publish()
			return
		}
	}
}
