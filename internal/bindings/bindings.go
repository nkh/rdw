// Package bindings defines the keyboard binding model for the rdw browser UI.
//
// Every interactive action in the browser has a named Action. Each Action maps
// to one or more Key strings. The default set uses vim-like keys. Users supply
// a bindings section in their config to override individual keys without
// replacing the whole map.
package bindings

import (
	"fmt"
	"strings"
)

// Action names every interactive operation available in the browser UI.
type Action string

const (
	// --- Window navigation ---
	ActionWindowNext    Action = "window.next"     // gt  — move to next window
	ActionWindowPrev    Action = "window.prev"     // gT  — move to previous window
	ActionWindowFirst   Action = "window.first"    // g0  — jump to first window
	ActionWindowLast    Action = "window.last"     // g$  — jump to last window
	ActionWindowNew     Action = "window.new"      // gn  — create new empty window
	ActionWindowClose   Action = "window.close"    // gx  — close active window
	ActionWindowRename  Action = "window.rename"   // gr  — rename active window (opens prompt)

	// --- Pane navigation ---
	ActionPaneFocusLeft  Action = "pane.focus.left"   // h
	ActionPaneFocusDown  Action = "pane.focus.down"   // j
	ActionPaneFocusUp    Action = "pane.focus.up"     // k
	ActionPaneFocusRight Action = "pane.focus.right"  // l

	// --- Pane splitting ---
	ActionPaneSplitH Action = "pane.split.h"  // s   — split horizontally (new pane below)
	ActionPaneSplitV Action = "pane.split.v"  // v   — split vertically (new pane right)

	// --- Pane resizing (repeatable) ---
	ActionPaneResizeLeft  Action = "pane.resize.left"   // H
	ActionPaneResizeDown  Action = "pane.resize.down"   // J
	ActionPaneResizeUp    Action = "pane.resize.up"     // K
	ActionPaneResizeRight Action = "pane.resize.right"  // L

	// --- Pane lifecycle ---
	ActionPaneClose   Action = "pane.close"   // q   — close focused pane
	ActionPaneZoom    Action = "pane.zoom"    // z   — toggle zoom (full window)
	ActionPaneRename  Action = "pane.rename"  // r   — rename pane target ID (opens prompt)
	ActionPaneSwap    Action = "pane.swap"    // x   — enter swap mode: next hjkl picks target

	// --- Scrollback ---
	ActionScrollUp       Action = "scroll.up"        // Ctrl-u
	ActionScrollDown     Action = "scroll.down"       // Ctrl-d
	ActionScrollTop      Action = "scroll.top"        // gg
	ActionScrollBottom   Action = "scroll.bottom"     // G
	ActionScrollClear    Action = "scroll.clear"      // Ctrl-l

	// --- Search ---
	ActionSearchOpen     Action = "search.open"       // /
	ActionSearchNext     Action = "search.next"       // n
	ActionSearchPrev     Action = "search.prev"       // N

	// --- Layout persistence ---
	ActionLayoutSave     Action = "layout.save"       // <leader>w
	ActionLayoutReload   Action = "layout.reload"     // <leader>r

	// --- Mode ---
	ActionEscape         Action = "mode.escape"       // Escape / Ctrl-c — return to normal mode
)

// DefaultLeader is the prefix key for leader-prefixed bindings.
const DefaultLeader = " " // Space

// Binding holds the key string(s) that trigger an action.
type Binding struct {
	// Keys holds one or more key strings. Any of them triggers the action.
	// Format follows browser KeyboardEvent.key conventions:
	//   "h", "j", "k", "l", "G", "Escape", "Control+u", "Control+d", " "
	Keys []string
}

// Bindings maps every Action to its key set.
type Bindings map[Action]Binding

// Default returns the default vim-like binding set.
func Default() Bindings {
	return Bindings{
		ActionWindowNext:    {Keys: []string{"g t"}},
		ActionWindowPrev:    {Keys: []string{"g T"}},
		ActionWindowFirst:   {Keys: []string{"g 0"}},
		ActionWindowLast:    {Keys: []string{"g $"}},
		ActionWindowNew:     {Keys: []string{"g n"}},
		ActionWindowClose:   {Keys: []string{"g x"}},
		ActionWindowRename:  {Keys: []string{"g r"}},

		ActionPaneFocusLeft:  {Keys: []string{"h"}},
		ActionPaneFocusDown:  {Keys: []string{"j"}},
		ActionPaneFocusUp:    {Keys: []string{"k"}},
		ActionPaneFocusRight: {Keys: []string{"l"}},

		ActionPaneSplitH: {Keys: []string{"s"}},
		ActionPaneSplitV: {Keys: []string{"v"}},

		ActionPaneResizeLeft:  {Keys: []string{"H"}},
		ActionPaneResizeDown:  {Keys: []string{"J"}},
		ActionPaneResizeUp:    {Keys: []string{"K"}},
		ActionPaneResizeRight: {Keys: []string{"L"}},

		ActionPaneClose:  {Keys: []string{"q"}},
		ActionPaneZoom:   {Keys: []string{"z"}},
		ActionPaneRename: {Keys: []string{"R"}},
		ActionPaneSwap:   {Keys: []string{"x"}},

		ActionScrollUp:     {Keys: []string{"Control+u"}},
		ActionScrollDown:   {Keys: []string{"Control+d"}},
		ActionScrollTop:    {Keys: []string{"g g"}},
		ActionScrollBottom: {Keys: []string{"G"}},
		ActionScrollClear:  {Keys: []string{"Control+l"}},

		ActionSearchOpen: {Keys: []string{"/"}},
		ActionSearchNext: {Keys: []string{"n"}},
		ActionSearchPrev: {Keys: []string{"N"}},

		ActionLayoutSave:   {Keys: []string{"Control+w s"}},
		ActionLayoutReload: {Keys: []string{"Control+w r"}},

		ActionEscape: {Keys: []string{"Escape", "Control+c"}},
	}
}

// Merge returns a copy of base with overrides applied.
// Only the actions named in overrides are changed; all others keep their base
// binding. An action in overrides with an empty Keys slice removes that action.
func Merge(base, overrides Bindings) Bindings {
	result := make(Bindings, len(base))

	for action, binding := range base {
		result[action] = binding
	}

	for action, binding := range overrides {
		if len(binding.Keys) == 0 {
			delete(result, action)
		} else {
			result[action] = binding
		}
	}

	return result
}

// Validate checks for duplicate key assignments across all bindings.
// Returns a list of conflict descriptions, or nil if the map is clean.
func Validate(b Bindings) []string {
	seen := make(map[string]Action)
	var conflicts []string

	for action, binding := range b {
		for _, key := range binding.Keys {
			key = normaliseKey(key)

			if prev, ok := seen[key]; ok {
				conflicts = append(conflicts,
					fmt.Sprintf("key %q assigned to both %q and %q", key, prev, action))
			} else {
				seen[key] = action
			}
		}
	}

	return conflicts
}

// Lookup returns the Action bound to key, and whether one was found.
func Lookup(b Bindings, key string) (Action, bool) {
	key = normaliseKey(key)

	for action, binding := range b {
		for _, k := range binding.Keys {
			if normaliseKey(k) == key {
				return action, true
			}
		}
	}

	return "", false
}

// JSON returns a map suitable for serialisation into the browser UI's
// keybindings configuration object.
func JSON(b Bindings) map[string][]string {
	out := make(map[string][]string, len(b))

	for action, binding := range b {
		out[string(action)] = binding.Keys
	}

	return out
}

// normaliseKey lower-cases the non-modifier portion and standardises spacing.
func normaliseKey(k string) string {
	return strings.TrimSpace(k)
}
