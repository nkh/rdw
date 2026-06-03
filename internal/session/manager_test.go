package session_test

import (
	"testing"

	"github.com/nkh/rdw/internal/kvstore"
	"github.com/nkh/rdw/internal/layout"
	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newManager() *session.Manager {
	return session.NewManager(kvstore.New())
}

func simpleLayout(windows ...string) *layout.Layout {
	l := &layout.Layout{SchemaVersion: layout.CurrentSchemaVersion}
	for _, name := range windows {
		l.Windows = append(l.Windows, layout.WindowSpec{
			Name:  name,
			Panes: []layout.PaneSpec{{TargetID: name + "-pane"}},
		})
	}
	return l
}

// ---------------------------------------------------------------------------
// ApplyLayout
// ---------------------------------------------------------------------------

func TestApplyLayout_CreatesWindows(t *testing.T) {
	m := newManager()
	err := m.ApplyLayout(simpleLayout("build", "metrics"))
	require.NoError(t, err)
	assert.Len(t, m.Windows(), 2)
}

func TestApplyLayout_SkipsExistingWindows(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("build")))
	require.NoError(t, m.ApplyLayout(simpleLayout("build", "metrics")))
	// "build" must not be duplicated.
	assert.Len(t, m.Windows(), 2)
}

func TestApplyLayout_InvalidTargetID(t *testing.T) {
	m := newManager()
	l := &layout.Layout{
		SchemaVersion: layout.CurrentSchemaVersion,
		Windows: []layout.WindowSpec{
			{Name: "w", Panes: []layout.PaneSpec{{TargetID: "-bad"}}},
		},
	}
	err := m.ApplyLayout(l)
	assert.Error(t, err)
}

func TestApplyLayout_PaneFields(t *testing.T) {
	m := newManager()
	l := &layout.Layout{
		SchemaVersion: layout.CurrentSchemaVersion,
		Windows: []layout.WindowSpec{
			{
				Name: "ci",
				Panes: []layout.PaneSpec{
					{
						TargetID:      "log",
						Group:         "ci-group",
						Private:       true,
						ScrollbackCap: 5000,
						Split:         layout.DirectionVertical,
						Size:          "40%",
					},
				},
			},
		},
	}
	require.NoError(t, m.ApplyLayout(l))
	_, p := m.FindPane(mustParseID(t, "log"))
	require.NotNil(t, p)
	assert.Equal(t, "ci-group", p.Group)
	assert.True(t, p.Private)
	assert.Equal(t, 5000, p.ScrollbackCap)
	assert.Equal(t, "v", p.Split)
	assert.Equal(t, "40%", p.Size)
}

// ---------------------------------------------------------------------------
// CreateWindow
// ---------------------------------------------------------------------------

func TestCreateWindow_Success(t *testing.T) {
	m := newManager()
	err := m.CreateWindow("new-win")
	require.NoError(t, err)
	assert.Len(t, m.Windows(), 1)
}

func TestCreateWindow_EmptyName(t *testing.T) {
	m := newManager()
	assert.Error(t, m.CreateWindow(""))
}

func TestCreateWindow_Duplicate(t *testing.T) {
	m := newManager()
	require.NoError(t, m.CreateWindow("x"))
	assert.Error(t, m.CreateWindow("x"))
}

// ---------------------------------------------------------------------------
// CloseWindow
// ---------------------------------------------------------------------------

func TestCloseWindow_Success(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("a", "b")))
	require.NoError(t, m.CloseWindow("a"))
	assert.Len(t, m.Windows(), 1)
	assert.Equal(t, "b", m.Windows()[0].Name)
}

func TestCloseWindow_LastWindowBlocked(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("only")))
	assert.Error(t, m.CloseWindow("only"))
}

func TestCloseWindow_NotFound(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("a", "b")))
	assert.Error(t, m.CloseWindow("ghost"))
}

func TestCloseWindow_ActiveIndexClamped(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("a", "b", "c")))
	require.NoError(t, m.FocusWindow("c"))
	require.NoError(t, m.CloseWindow("c"))
	// active should now point to "b" (last remaining)
	assert.Equal(t, "b", m.ActiveWindow().Name)
}

// ---------------------------------------------------------------------------
// RenameWindow
// ---------------------------------------------------------------------------

func TestRenameWindow_Success(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("old")))
	require.NoError(t, m.RenameWindow("old", "new"))
	assert.NotNil(t, m.Window("new"))
	assert.Nil(t, m.Window("old"))
}

func TestRenameWindow_EmptyNewName(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("x")))
	assert.Error(t, m.RenameWindow("x", ""))
}

func TestRenameWindow_DuplicateNewName(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("a", "b")))
	assert.Error(t, m.RenameWindow("a", "b"))
}

func TestRenameWindow_NotFound(t *testing.T) {
	m := newManager()
	assert.Error(t, m.RenameWindow("ghost", "new"))
}

// ---------------------------------------------------------------------------
// FocusWindow
// ---------------------------------------------------------------------------

func TestFocusWindow_Success(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("a", "b", "c")))
	require.NoError(t, m.FocusWindow("c"))
	assert.Equal(t, "c", m.ActiveWindow().Name)
}

func TestFocusWindow_NotFound(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("a")))
	assert.Error(t, m.FocusWindow("nope"))
}

func TestActiveWindow_InitiallyFirst(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("first", "second")))
	assert.Equal(t, "first", m.ActiveWindow().Name)
}

func TestActiveWindow_NoWindows(t *testing.T) {
	m := newManager()
	assert.Nil(t, m.ActiveWindow())
}

// ---------------------------------------------------------------------------
// AddPane / RemovePane / FindPane
// ---------------------------------------------------------------------------

func TestAddPane_Success(t *testing.T) {
	m := newManager()
	require.NoError(t, m.CreateWindow("w"))
	id := mustParseID(t, "my-pane")
	err := m.AddPane("w", &session.PaneState{TargetID: id})
	require.NoError(t, err)
	_, p := m.FindPane(id)
	assert.NotNil(t, p)
}

func TestAddPane_WindowNotFound(t *testing.T) {
	m := newManager()
	id := mustParseID(t, "p")
	assert.Error(t, m.AddPane("ghost", &session.PaneState{TargetID: id}))
}

func TestAddPane_MaxPanesEnforced(t *testing.T) {
	m := newManager()
	require.NoError(t, m.CreateWindow("w"))

	for i := range 64 {
		id := mustParseID(t, "pane")
		_ = id
		idStr := "p"
		if i < 10 {
			idStr = "p0"
		}
		id2, _ := session.ParseTargetID(idStr + string(rune('a'+i%26)) + string(rune('a'+i/26)))
		_ = m.AddPane("w", &session.PaneState{TargetID: id2})
	}

	overflow, _ := session.ParseTargetID("overflow")
	err := m.AddPane("w", &session.PaneState{TargetID: overflow})
	assert.Error(t, err)
}

func TestRemovePane_Success(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("w")))
	id := mustParseID(t, "w-pane")
	require.NoError(t, m.RemovePane(id))
	_, p := m.FindPane(id)
	assert.Nil(t, p)
}

func TestRemovePane_NotFound(t *testing.T) {
	m := newManager()
	assert.Error(t, m.RemovePane(mustParseID(t, "ghost")))
}

func TestFindPane_NotFound(t *testing.T) {
	m := newManager()
	w, p := m.FindPane(mustParseID(t, "nope"))
	assert.Nil(t, w)
	assert.Nil(t, p)
}

// ---------------------------------------------------------------------------
// Snapshot
// ---------------------------------------------------------------------------

func TestSnapshot_Valid(t *testing.T) {
	m := newManager()
	require.NoError(t, m.ApplyLayout(simpleLayout("a", "b")))

	data, err := m.Snapshot()
	require.NoError(t, err)
	assert.Contains(t, string(data), `"a"`)
	assert.Contains(t, string(data), `"b"`)
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func mustParseID(t *testing.T, s string) session.TargetID {
	t.Helper()
	id, err := session.ParseTargetID(s)
	require.NoError(t, err)
	return id
}
