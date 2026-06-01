// Package layout defines the pane/window layout model and its serialisation
// schema.
//
// Windows are server-managed views rendered within a single browser page.
// The server maintains an ordered list of windows; the browser displays one
// at a time with a visible header bar showing all window names. Keyboard
// bindings move between windows; the header is also clickable.
//
// All layout files carry a schema_version field; the server rejects files
// with an unrecognised version.
//
// Layout description language
//
// Layouts are expressed in YAML. The root document has three keys:
//
//	schema_version: 1          # required; must equal 1
//	name: my-layout            # optional preset name
//	windows:                   # list of window specs
//	  - name: build            # window display name (shown in header bar)
//	    panes:                 # ordered list of pane specs
//	      - target_id: build-log    # stream name routed to this pane
//	        split: v                # how this pane splits from the previous (h|v)
//	        size: 60%               # width or height: N (cols), Npx, N%
//	        group: ci               # optional pane group name
//	        private: false          # hide from non-owner tokens
//	        scrollback_cap: 5000    # overrides server default (max 100 000)
//
// Split direction:
//   - "h" — split horizontally: the new pane appears below the previous one
//   - "v" — split vertically:   the new pane appears to the right
//
// The first pane in each window needs no split field; it occupies the whole
// window until a subsequent pane splits it.
//
// Size units:
//   - "40"    — columns (default unit)
//   - "40px"  — pixels
//   - "40%"   — percentage of the window dimension along the split axis
//
// Example — two windows with different pane arrangements:
//
//	schema_version: 1
//	name: debug
//	windows:
//	  - name: build
//	    panes:
//	      - target_id: stdout
//	      - target_id: stderr
//	        split: h
//	        size: 30%
//	  - name: metrics
//	    panes:
//	      - target_id: cpu
//	      - target_id: mem
//	        split: v
//	        size: 50%
//	      - target_id: disk
//	        split: h
//	        size: 25%
package layout

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

const CurrentSchemaVersion = 1

// ResizeUnit identifies the unit used for pane resize operations.
type ResizeUnit string

const (
	ResizeUnitColumns ResizeUnit = "columns" // default
	ResizeUnitPixels  ResizeUnit = "px"
	ResizeUnitPercent ResizeUnit = "%"
)

// Direction identifies the axis of a pane split or resize operation.
type Direction string

const (
	DirectionHorizontal Direction = "h"
	DirectionVertical   Direction = "v"
	DirectionLeft       Direction = "left"
	DirectionRight      Direction = "right"
	DirectionUp         Direction = "up"
	DirectionDown       Direction = "down"
)

// PaneSpec describes a single pane within a window layout.
type PaneSpec struct {
	// TargetID is the data stream name routed to this pane.
	TargetID string `yaml:"target_id" json:"target_id"`

	// Group is the optional pane group this pane belongs to.
	Group string `yaml:"group,omitempty" json:"group,omitempty"`

	// Split describes how this pane is divided from its sibling (h or v).
	// Omit for the first pane in a window.
	Split Direction `yaml:"split,omitempty" json:"split,omitempty"`

	// Size is the size specification along the split axis.
	// Accepts N (columns), Npx, N%.  Omit to use equal distribution.
	Size string `yaml:"size,omitempty" json:"size,omitempty"`

	// Private controls whether the pane is visible to non-owner tokens.
	Private bool `yaml:"private,omitempty" json:"private,omitempty"`

	// ScrollbackCap overrides the server default for this pane (max 100 000).
	ScrollbackCap int `yaml:"scrollback_cap,omitempty" json:"scrollback_cap,omitempty"`
}

// WindowSpec describes a server-managed view within the browser page.
// Windows are displayed one at a time; the browser shows a header bar listing
// all window names. Users switch between windows via keyboard bindings or by
// clicking the header.
type WindowSpec struct {
	// Name is the display name shown in the window header bar.
	Name string `yaml:"name" json:"name"`

	// Panes lists the panes within this window in layout order.
	Panes []PaneSpec `yaml:"panes" json:"panes"`
}

// Layout is the top-level layout document.
type Layout struct {
	// SchemaVersion must match CurrentSchemaVersion.
	SchemaVersion int `yaml:"schema_version" json:"schema_version"`

	// Name is an optional label for saved presets.
	Name string `yaml:"name,omitempty" json:"name,omitempty"`

	// Windows lists the windows in display order.
	// The browser renders a header bar with one entry per window.
	Windows []WindowSpec `yaml:"windows" json:"windows"`
}

// Validate checks that the layout is well-formed and carries the expected
// schema version.
func (l *Layout) Validate() error {
	if l.SchemaVersion != CurrentSchemaVersion {
		return fmt.Errorf(
			"unsupported layout schema version %d (current: %d)",
			l.SchemaVersion, CurrentSchemaVersion,
		)
	}

	if len(l.Windows) == 0 {
		return fmt.Errorf("layout must contain at least one window")
	}

	for i, w := range l.Windows {
		if w.Name == "" {
			return fmt.Errorf("window[%d]: name must not be empty", i)
		}

		if len(w.Panes) == 0 {
			return fmt.Errorf("window[%d] %q: must contain at least one pane", i, w.Name)
		}

		if len(w.Panes) > 64 {
			return fmt.Errorf("window[%d] %q: pane count %d exceeds maximum of 64",
				i, w.Name, len(w.Panes))
		}

		for j, p := range w.Panes {
			if p.TargetID == "" {
				return fmt.Errorf("window[%d] %q pane[%d]: target_id must not be empty",
					i, w.Name, j)
			}

			if p.ScrollbackCap < 0 {
				return fmt.Errorf("window[%d] %q pane[%d]: scrollback_cap must not be negative",
					i, w.Name, j)
			}

			if p.Size != "" {
				if _, _, err := ParseResizeArg(p.Size); err != nil {
					return fmt.Errorf("window[%d] %q pane[%d]: invalid size %q: %w",
						i, w.Name, j, p.Size, err)
				}
			}

			if p.Split != "" &&
				p.Split != DirectionHorizontal &&
				p.Split != DirectionVertical {
				return fmt.Errorf("window[%d] %q pane[%d]: split must be 'h' or 'v', got %q",
					i, w.Name, j, p.Split)
			}
		}
	}

	return nil
}

// LoadFile reads and validates a layout from a YAML file at path.
func LoadFile(path string) (*Layout, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading layout file %q: %w", path, err)
	}

	return Parse(data)
}

// Parse parses and validates a layout from raw YAML bytes.
func Parse(data []byte) (*Layout, error) {
	var l Layout

	if err := yaml.Unmarshal(data, &l); err != nil {
		return nil, fmt.Errorf("parsing layout YAML: %w", err)
	}

	if err := l.Validate(); err != nil {
		return nil, err
	}

	return &l, nil
}

// ParseResizeArg parses a resize argument string into a value and unit.
//
//	"40"    -> (40, ResizeUnitColumns)
//	"40px"  -> (40, ResizeUnitPixels)
//	"40%"   -> (40, ResizeUnitPercent)
//
// Fractional values are rejected.
func ParseResizeArg(s string) (value int, unit ResizeUnit, err error) {
	if s == "" {
		return 0, "", fmt.Errorf("resize argument must not be empty")
	}

	switch {
	case len(s) > 2 && s[len(s)-2:] == "px":
		n, e := parseWholeInt(s[:len(s)-2])
		if e != nil {
			return 0, "", fmt.Errorf("invalid pixel value %q", s)
		}

		return n, ResizeUnitPixels, nil

	case s[len(s)-1] == '%':
		n, e := parseWholeInt(s[:len(s)-1])
		if e != nil {
			return 0, "", fmt.Errorf("invalid percentage value %q", s)
		}

		if n < 1 || n > 100 {
			return 0, "", fmt.Errorf("percentage value %d out of range [1, 100]", n)
		}

		return n, ResizeUnitPercent, nil

	default:
		n, e := parseWholeInt(s)
		if e != nil {
			return 0, "", fmt.Errorf("invalid column value %q", s)
		}

		return n, ResizeUnitColumns, nil
	}
}

// parseWholeInt parses s as a non-negative decimal integer, rejecting any
// non-digit characters (including '.', '-', and spaces).
func parseWholeInt(s string) (int, error) {
	if s == "" {
		return 0, fmt.Errorf("empty")
	}

	n := 0

	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("non-digit character %q", c)
		}

		n = n*10 + int(c-'0')
	}

	return n, nil
}
