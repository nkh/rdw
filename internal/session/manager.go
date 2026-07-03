// Package session provides the session manager that owns all runtime state
// for a single rdw server instance: windows, panes, the router, and the KV store.
package session

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/layout"
)

const maxPanesPerWindow = 64

// PaneState describes a live pane.
type PaneState struct {
	TargetID      TargetID `json:"target_id"`
	Label         string   `json:"label,omitempty"`
	Group         string   `json:"group,omitempty"`
	Private       bool     `json:"private,omitempty"`
	ScrollbackCap int      `json:"scrollback_cap"`
	Split         string   `json:"split,omitempty"`
	Size          string   `json:"size,omitempty"`
	Formatter     string   `json:"formatter,omitempty"`      // active formatter name
	SavedFormatter string  `json:"saved_formatter,omitempty"` // saved before image:/svg:
}

// WindowState describes a live window.
type WindowState struct {
	Name  string       `json:"name"`
	Panes []*PaneState `json:"panes"`
}

// Manager owns the session runtime state.
type Manager struct {
	mu      sync.RWMutex
	windows []*WindowState    // ordered list; index = display order
	active  int               // index of the currently-focused window
	kv      *kvstore.Store
}

// NewManager creates an empty session manager.
func NewManager(kv *kvstore.Store) *Manager {
	return &Manager{kv: kv}
}

// KV returns the session KV store.
func (m *Manager) KV() *kvstore.Store {
	return m.kv
}

// ApplyLayout creates windows and panes from a layout spec.
// Windows whose names already exist are left unchanged.
func (m *Manager) ApplyLayout(l *layout.Layout) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, ws := range l.Windows {
		if m.findWindow(ws.Name) != nil {
			continue
		}

		win := &WindowState{Name: ws.Name}

		for _, ps := range ws.Panes {
			id, err := ParseTargetID(ps.TargetID)
			if err != nil {
				return fmt.Errorf("window %q pane %q: %w", ws.Name, ps.TargetID, err)
			}

			win.Panes = append(win.Panes, &PaneState{
				TargetID:      id,
				Group:         ps.Group,
				Private:       ps.Private,
				ScrollbackCap: ps.ScrollbackCap,
				Split:         string(ps.Split),
				Size:          ps.Size,
			})
		}

		m.windows = append(m.windows, win)
	}

	return nil
}

// CreateWindow adds an empty window with the given name.
func (m *Manager) CreateWindow(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if name == "" {
		return fmt.Errorf("window name must not be empty")
	}

	if m.findWindow(name) != nil {
		return fmt.Errorf("window %q already exists", name)
	}

	m.windows = append(m.windows, &WindowState{Name: name})

	return nil
}

// CloseWindow removes the window with the given name.
// Returns an error if it is the last window in the session.
func (m *Manager) CloseWindow(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.windows) <= 1 {
		return fmt.Errorf("cannot close the last window")
	}

	idx := m.windowIndex(name)
	if idx < 0 {
		return fmt.Errorf("window %q not found", name)
	}

	m.windows = append(m.windows[:idx], m.windows[idx+1:]...)

	if m.active >= len(m.windows) {
		m.active = len(m.windows) - 1
	}

	return nil
}

// RenameWindow changes a window's display name.
func (m *Manager) RenameWindow(oldName, newName string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if newName == "" {
		return fmt.Errorf("new name must not be empty")
	}

	if m.findWindow(newName) != nil {
		return fmt.Errorf("window %q already exists", newName)
	}

	w := m.findWindow(oldName)
	if w == nil {
		return fmt.Errorf("window %q not found", oldName)
	}

	w.Name = newName

	return nil
}

// FocusWindow sets the active window by name.
func (m *Manager) FocusWindow(name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	idx := m.windowIndex(name)
	if idx < 0 {
		return fmt.Errorf("window %q not found", name)
	}

	m.active = idx

	return nil
}

// ActiveWindow returns the currently focused window.
func (m *Manager) ActiveWindow() *WindowState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if len(m.windows) == 0 {
		return nil
	}

	return m.windows[m.active]
}

// Windows returns an ordered snapshot of all windows.
func (m *Manager) Windows() []*WindowState {
	m.mu.RLock()
	defer m.mu.RUnlock()

	out := make([]*WindowState, len(m.windows))
	copy(out, m.windows)

	return out
}

// Window returns the named window or nil.
func (m *Manager) Window(name string) *WindowState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.findWindow(name)
}

// AddPane adds a pane to the named window.
func (m *Manager) AddPane(windowName string, pane *PaneState) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	w := m.findWindow(windowName)
	if w == nil {
		return fmt.Errorf("window %q not found", windowName)
	}

	if len(w.Panes) >= maxPanesPerWindow {
		return fmt.Errorf("window %q already has the maximum of %d panes",
			windowName, maxPanesPerWindow)
	}

	w.Panes = append(w.Panes, pane)

	return nil
}

// RemovePane removes the pane with the given Target ID from its window.
func (m *Manager) RemovePane(id TargetID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, w := range m.windows {
		for i, p := range w.Panes {
			if p.TargetID == id {
				w.Panes = append(w.Panes[:i], w.Panes[i+1:]...)
				return nil
			}
		}
	}

	return fmt.Errorf("pane %q not found in any window", id)
}

// FindPane returns the window and pane for the given Target ID.
func (m *Manager) FindPane(id TargetID) (*WindowState, *PaneState) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, w := range m.windows {
		for _, p := range w.Panes {
			if p.TargetID == id {
				return w, p
			}
		}
	}

	return nil, nil
}

// Snapshot returns a JSON-serialisable view of the session state.
func (m *Manager) Snapshot() ([]byte, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	type snapshot struct {
		Windows []*WindowState `json:"windows"`
		Active  int            `json:"active_window"`
	}

	wins := m.windows
	if wins == nil {
		wins = []*WindowState{}
	}
	return json.Marshal(snapshot{
		Windows: wins,
		Active:  m.active,
	})
}

// findWindow returns the window with the given name (caller must hold mu).
func (m *Manager) findWindow(name string) *WindowState {
	for _, w := range m.windows {
		if w.Name == name {
			return w
		}
	}

	return nil
}

// windowIndex returns the slice index for the named window, or -1.
func (m *Manager) windowIndex(name string) int {
	for i, w := range m.windows {
		if w.Name == name {
			return i
		}
	}

	return -1
}

// RestoreSnapshot deserialises a JSON snapshot produced by Snapshot() and
// replaces the current session state. Existing windows not present in the
// snapshot are dropped; routers/pipelines for their panes are not touched
// here — callers are responsible for reconciling the router.
func (m *Manager) RestoreSnapshot(data []byte) error {
	type snap struct {
		Windows []*WindowState `json:"windows"`
		Active  int            `json:"active_window"`
	}

	var s snap
	if err := json.Unmarshal(data, &s); err != nil {
		return fmt.Errorf("restore snapshot: %w", err)
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if s.Windows == nil {
		s.Windows = []*WindowState{}
	}

	m.windows = s.Windows
	m.active = s.Active

	if m.active >= len(m.windows) {
		m.active = 0
	}

	return nil
}

// AllPaneIDs returns the TargetID of every pane across all windows.
func (m *Manager) AllPaneIDs() []TargetID {
m.mu.RLock()
defer m.mu.RUnlock()

var ids []TargetID
for _, w := range m.windows {
	for _, p := range w.Panes {
		ids = append(ids, p.TargetID)
		}
	}

return ids
}
