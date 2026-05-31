// Package layout defines the pane/window layout model and its serialisation
// schema. All layout files carry a schema_version field; the server rejects
// files with an unrecognised version.
package layout

import (
	"fmt"
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

	// Split describes how this pane is divided from its sibling.
	Split Direction `yaml:"split,omitempty" json:"split,omitempty"`

	// Size is the size specification for this pane (e.g. "40%", "80px", "40").
	Size string `yaml:"size,omitempty" json:"size,omitempty"`

	// Private controls whether the pane is visible to non-owner tokens.
	Private bool `yaml:"private,omitempty" json:"private,omitempty"`

	// ScrollbackCap overrides the server default for this pane.
	ScrollbackCap int `yaml:"scrollback_cap,omitempty" json:"scrollback_cap,omitempty"`
}

// WindowSpec describes a browser tab / window.
type WindowSpec struct {
	// Name is the display name of the window.
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

	// Windows lists the windows in tab order.
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
			return fmt.Errorf("window[%d] %q: pane count %d exceeds maximum of 64", i, w.Name, len(w.Panes))
		}

		for j, p := range w.Panes {
			if p.TargetID == "" {
				return fmt.Errorf("window[%d] %q pane[%d]: target_id must not be empty", i, w.Name, j)
			}

			if p.ScrollbackCap < 0 {
				return fmt.Errorf("window[%d] %q pane[%d]: scrollback_cap must not be negative", i, w.Name, j)
			}
		}
	}

	return nil
}

// ParseResizeArg parses a resize argument string into a value and unit.
// "40" -> (40, ResizeUnitColumns)
// "40px" -> (40, ResizeUnitPixels)
// "40%" -> (40, ResizeUnitPercent)
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

// parseWholeInt parses s as a decimal integer, rejecting any non-digit
// characters (including '.', '-', and spaces).
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
