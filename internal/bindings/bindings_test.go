package bindings_test

import (
	"testing"

	"github.com/nkh/rdw/internal/bindings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Default bindings
// ---------------------------------------------------------------------------

func TestDefault_ContainsAllActions(t *testing.T) {
	b := bindings.Default()

	required := []bindings.Action{
		bindings.ActionWindowNext,
		bindings.ActionWindowPrev,
		bindings.ActionWindowFirst,
		bindings.ActionWindowLast,
		bindings.ActionWindowNew,
		bindings.ActionWindowClose,
		bindings.ActionWindowRename,
		bindings.ActionPaneFocusLeft,
		bindings.ActionPaneFocusDown,
		bindings.ActionPaneFocusUp,
		bindings.ActionPaneFocusRight,
		bindings.ActionPaneSplitH,
		bindings.ActionPaneSplitV,
		bindings.ActionPaneResizeLeft,
		bindings.ActionPaneResizeDown,
		bindings.ActionPaneResizeUp,
		bindings.ActionPaneResizeRight,
		bindings.ActionPaneClose,
		bindings.ActionPaneZoom,
		bindings.ActionPaneRename,
		bindings.ActionPaneSwap,
		bindings.ActionScrollUp,
		bindings.ActionScrollDown,
		bindings.ActionScrollTop,
		bindings.ActionScrollBottom,
		bindings.ActionScrollClear,
		bindings.ActionSearchOpen,
		bindings.ActionSearchNext,
		bindings.ActionSearchPrev,
		bindings.ActionLayoutSave,
		bindings.ActionLayoutReload,
		bindings.ActionEscape,
	}

	for _, action := range required {
		binding, ok := b[action]
		assert.True(t, ok, "missing action %q", action)
		assert.NotEmpty(t, binding.Keys, "action %q has no keys", action)
	}
}

func TestDefault_NoConflicts(t *testing.T) {
	b := bindings.Default()
	conflicts := bindings.Validate(b)
	assert.Empty(t, conflicts, "default bindings have conflicts: %v", conflicts)
}

func TestDefault_VimNavigationKeys(t *testing.T) {
	b := bindings.Default()

	cases := []struct {
		action bindings.Action
		key    string
	}{
		{bindings.ActionPaneFocusLeft, "h"},
		{bindings.ActionPaneFocusDown, "j"},
		{bindings.ActionPaneFocusUp, "k"},
		{bindings.ActionPaneFocusRight, "l"},
		{bindings.ActionScrollBottom, "G"},
		{bindings.ActionPaneClose, "q"},
		{bindings.ActionPaneZoom, "z"},
	}

	for _, tc := range cases {
		t.Run(string(tc.action), func(t *testing.T) {
			got, ok := bindings.Lookup(b, tc.key)
			require.True(t, ok, "key %q not bound", tc.key)
			assert.Equal(t, tc.action, got)
		})
	}
}

// ---------------------------------------------------------------------------
// Merge
// ---------------------------------------------------------------------------

func TestMerge_OverridesSingleBinding(t *testing.T) {
	base := bindings.Default()
	overrides := bindings.Bindings{
		bindings.ActionPaneFocusLeft: {Keys: []string{"Left"}},
	}

	merged := bindings.Merge(base, overrides)

	got, ok := bindings.Lookup(merged, "Left")
	require.True(t, ok)
	assert.Equal(t, bindings.ActionPaneFocusLeft, got)

	// Original key must be gone from merged
	_, still := bindings.Lookup(merged, "h")
	assert.False(t, still, "old key 'h' should not be bound after override")
}

func TestMerge_EmptyKeysRemovesAction(t *testing.T) {
	base := bindings.Default()
	overrides := bindings.Bindings{
		bindings.ActionPaneClose: {Keys: []string{}},
	}

	merged := bindings.Merge(base, overrides)
	_, ok := merged[bindings.ActionPaneClose]
	assert.False(t, ok, "action should be removed when override has empty keys")
}

func TestMerge_DoesNotMutateBase(t *testing.T) {
	base := bindings.Default()
	overrides := bindings.Bindings{
		bindings.ActionPaneFocusLeft: {Keys: []string{"Left"}},
	}

	_ = bindings.Merge(base, overrides)

	// base must be unchanged
	got, ok := bindings.Lookup(base, "h")
	require.True(t, ok)
	assert.Equal(t, bindings.ActionPaneFocusLeft, got)
}

func TestMerge_AddsNewAction(t *testing.T) {
	base := bindings.Bindings{}
	overrides := bindings.Bindings{
		bindings.ActionPaneZoom: {Keys: []string{"f"}},
	}

	merged := bindings.Merge(base, overrides)
	got, ok := bindings.Lookup(merged, "f")
	require.True(t, ok)
	assert.Equal(t, bindings.ActionPaneZoom, got)
}

// ---------------------------------------------------------------------------
// Validate
// ---------------------------------------------------------------------------

func TestValidate_ConflictDetected(t *testing.T) {
	b := bindings.Bindings{
		bindings.ActionPaneFocusLeft:  {Keys: []string{"h"}},
		bindings.ActionPaneFocusRight: {Keys: []string{"h"}}, // duplicate
	}

	conflicts := bindings.Validate(b)
	assert.Len(t, conflicts, 1)
	assert.Contains(t, conflicts[0], "h")
}

func TestValidate_NoConflict(t *testing.T) {
	b := bindings.Bindings{
		bindings.ActionPaneFocusLeft:  {Keys: []string{"h"}},
		bindings.ActionPaneFocusRight: {Keys: []string{"l"}},
	}

	assert.Empty(t, bindings.Validate(b))
}

func TestValidate_MultipleKeysPerAction_NoConflict(t *testing.T) {
	b := bindings.Bindings{
		bindings.ActionEscape: {Keys: []string{"Escape", "Control+c"}},
		bindings.ActionPaneClose: {Keys: []string{"q"}},
	}

	assert.Empty(t, bindings.Validate(b))
}

func TestValidate_MultipleKeysPerAction_WithConflict(t *testing.T) {
	b := bindings.Bindings{
		bindings.ActionEscape:    {Keys: []string{"Escape", "q"}},
		bindings.ActionPaneClose: {Keys: []string{"q"}},
	}

	conflicts := bindings.Validate(b)
	assert.NotEmpty(t, conflicts)
}

// ---------------------------------------------------------------------------
// Lookup
// ---------------------------------------------------------------------------

func TestLookup_Found(t *testing.T) {
	b := bindings.Default()
	action, ok := bindings.Lookup(b, "G")
	require.True(t, ok)
	assert.Equal(t, bindings.ActionScrollBottom, action)
}

func TestLookup_NotFound(t *testing.T) {
	b := bindings.Default()
	_, ok := bindings.Lookup(b, "F99")
	assert.False(t, ok)
}

func TestLookup_TrimsWhitespace(t *testing.T) {
	b := bindings.Bindings{
		bindings.ActionPaneZoom: {Keys: []string{"  z  "}},
	}

	action, ok := bindings.Lookup(b, "z")
	require.True(t, ok)
	assert.Equal(t, bindings.ActionPaneZoom, action)
}

// ---------------------------------------------------------------------------
// JSON
// ---------------------------------------------------------------------------

func TestJSON_ContainsAllActions(t *testing.T) {
	b := bindings.Default()
	j := bindings.JSON(b)

	assert.NotEmpty(t, j)

	for action := range b {
		_, ok := j[string(action)]
		assert.True(t, ok, "JSON missing action %q", action)
	}
}

func TestJSON_ValueIsKeySlice(t *testing.T) {
	b := bindings.Bindings{
		bindings.ActionPaneZoom: {Keys: []string{"z", "f"}},
	}

	j := bindings.JSON(b)
	keys, ok := j[string(bindings.ActionPaneZoom)]
	require.True(t, ok)
	assert.Equal(t, []string{"z", "f"}, keys)
}
