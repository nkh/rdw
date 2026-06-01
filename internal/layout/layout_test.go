package layout_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nkh/rdw/internal/layout"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Layout.Validate
// ---------------------------------------------------------------------------

func validLayout() layout.Layout {
	return layout.Layout{
		SchemaVersion: layout.CurrentSchemaVersion,
		Name:          "test",
		Windows: []layout.WindowSpec{
			{
				Name: "main",
				Panes: []layout.PaneSpec{
					{TargetID: "build-log"},
				},
			},
		},
	}
}

func TestValidate_Valid(t *testing.T) {
	l := validLayout()
	assert.NoError(t, l.Validate())
}

func TestValidate_WrongSchemaVersion(t *testing.T) {
	l := validLayout()
	l.SchemaVersion = 99
	assert.Error(t, l.Validate())
}

func TestValidate_NoWindows(t *testing.T) {
	l := validLayout()
	l.Windows = nil
	assert.Error(t, l.Validate())
}

func TestValidate_WindowNoName(t *testing.T) {
	l := validLayout()
	l.Windows[0].Name = ""
	assert.Error(t, l.Validate())
}

func TestValidate_WindowNoPanes(t *testing.T) {
	l := validLayout()
	l.Windows[0].Panes = nil
	assert.Error(t, l.Validate())
}

func TestValidate_PaneNoTargetID(t *testing.T) {
	l := validLayout()
	l.Windows[0].Panes[0].TargetID = ""
	assert.Error(t, l.Validate())
}

func TestValidate_PaneNegativeScrollbackCap(t *testing.T) {
	l := validLayout()
	l.Windows[0].Panes[0].ScrollbackCap = -1
	assert.Error(t, l.Validate())
}

func TestValidate_PaneLimitExceeded(t *testing.T) {
	l := validLayout()
	panes := make([]layout.PaneSpec, 65)
	for i := range panes {
		panes[i] = layout.PaneSpec{TargetID: "pane"}
	}
	l.Windows[0].Panes = panes
	assert.Error(t, l.Validate())
}

func TestValidate_MultipleWindows(t *testing.T) {
	l := layout.Layout{
		SchemaVersion: layout.CurrentSchemaVersion,
		Windows: []layout.WindowSpec{
			{Name: "w1", Panes: []layout.PaneSpec{{TargetID: "p1"}}},
			{Name: "w2", Panes: []layout.PaneSpec{{TargetID: "p2"}}},
		},
	}
	assert.NoError(t, l.Validate())
}

func TestValidate_ValidSplitDirections(t *testing.T) {
	l := layout.Layout{
		SchemaVersion: layout.CurrentSchemaVersion,
		Windows: []layout.WindowSpec{
			{
				Name: "w",
				Panes: []layout.PaneSpec{
					{TargetID: "a"},
					{TargetID: "b", Split: layout.DirectionHorizontal, Size: "30%"},
					{TargetID: "c", Split: layout.DirectionVertical, Size: "50%"},
				},
			},
		},
	}
	assert.NoError(t, l.Validate())
}

func TestValidate_InvalidSplitDirection(t *testing.T) {
	l := validLayout()
	l.Windows[0].Panes = []layout.PaneSpec{
		{TargetID: "a"},
		{TargetID: "b", Split: "diagonal"},
	}
	assert.Error(t, l.Validate())
}

func TestValidate_InvalidPaneSize(t *testing.T) {
	l := validLayout()
	l.Windows[0].Panes[0].Size = "not-a-size"
	assert.Error(t, l.Validate())
}

// ---------------------------------------------------------------------------
// Parse
// ---------------------------------------------------------------------------

func TestParse_Valid(t *testing.T) {
	src := `
schema_version: 1
name: debug
windows:
  - name: build
    panes:
      - target_id: stdout
      - target_id: stderr
        split: h
        size: 30%
  - name: metrics
    panes:
      - target_id: cpu
      - target_id: mem
        split: v
        size: 50%
`
	l, err := layout.Parse([]byte(src))
	require.NoError(t, err)
	assert.Equal(t, "debug", l.Name)
	assert.Len(t, l.Windows, 2)
	assert.Equal(t, "build", l.Windows[0].Name)
	assert.Equal(t, "metrics", l.Windows[1].Name)
	assert.Len(t, l.Windows[0].Panes, 2)
	assert.Equal(t, "stderr", l.Windows[0].Panes[1].TargetID)
	assert.Equal(t, layout.DirectionHorizontal, l.Windows[0].Panes[1].Split)
	assert.Equal(t, "30%", l.Windows[0].Panes[1].Size)
}

func TestParse_InvalidYAML(t *testing.T) {
	_, err := layout.Parse([]byte("{{invalid yaml"))
	assert.Error(t, err)
}

func TestParse_FailsValidation(t *testing.T) {
	src := `
schema_version: 1
windows:
  - name: ""
    panes:
      - target_id: p1
`
	_, err := layout.Parse([]byte(src))
	assert.Error(t, err)
}

func TestParse_WrongSchemaVersion(t *testing.T) {
	src := `
schema_version: 99
windows:
  - name: w
    panes:
      - target_id: p
`
	_, err := layout.Parse([]byte(src))
	assert.Error(t, err)
}

func TestParse_AllPaneFields(t *testing.T) {
	src := `
schema_version: 1
windows:
  - name: main
    panes:
      - target_id: log
        group: ci
        split: v
        size: 40%
        private: true
        scrollback_cap: 5000
`
	l, err := layout.Parse([]byte(src))
	require.NoError(t, err)

	p := l.Windows[0].Panes[0]
	assert.Equal(t, "log", p.TargetID)
	assert.Equal(t, "ci", p.Group)
	assert.Equal(t, layout.DirectionVertical, p.Split)
	assert.Equal(t, "40%", p.Size)
	assert.True(t, p.Private)
	assert.Equal(t, 5000, p.ScrollbackCap)
}

// ---------------------------------------------------------------------------
// LoadFile
// ---------------------------------------------------------------------------

func TestLoadFile_Valid(t *testing.T) {
	src := `
schema_version: 1
windows:
  - name: main
    panes:
      - target_id: log
`
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.yaml")
	require.NoError(t, os.WriteFile(path, []byte(src), 0600))

	l, err := layout.LoadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "main", l.Windows[0].Name)
}

func TestLoadFile_MissingFile(t *testing.T) {
	_, err := layout.LoadFile("/nonexistent/layout.yaml")
	assert.Error(t, err)
}

func TestLoadFile_InvalidContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "layout.yaml")
	require.NoError(t, os.WriteFile(path, []byte("schema_version: 99\nwindows: []\n"), 0600))
	_, err := layout.LoadFile(path)
	assert.Error(t, err)
}

// ---------------------------------------------------------------------------
// ParseResizeArg
// ---------------------------------------------------------------------------

func TestParseResizeArg_Columns(t *testing.T) {
	v, unit, err := layout.ParseResizeArg("40")
	require.NoError(t, err)
	assert.Equal(t, 40, v)
	assert.Equal(t, layout.ResizeUnitColumns, unit)
}

func TestParseResizeArg_Pixels(t *testing.T) {
	v, unit, err := layout.ParseResizeArg("200px")
	require.NoError(t, err)
	assert.Equal(t, 200, v)
	assert.Equal(t, layout.ResizeUnitPixels, unit)
}

func TestParseResizeArg_Percent(t *testing.T) {
	v, unit, err := layout.ParseResizeArg("40%")
	require.NoError(t, err)
	assert.Equal(t, 40, v)
	assert.Equal(t, layout.ResizeUnitPercent, unit)
}

func TestParseResizeArg_PercentBounds(t *testing.T) {
	_, _, err := layout.ParseResizeArg("0%")
	assert.Error(t, err)

	_, _, err = layout.ParseResizeArg("101%")
	assert.Error(t, err)

	_, _, err = layout.ParseResizeArg("100%")
	assert.NoError(t, err)
}

func TestParseResizeArg_Empty(t *testing.T) {
	_, _, err := layout.ParseResizeArg("")
	assert.Error(t, err)
}

func TestParseResizeArg_Invalid(t *testing.T) {
	cases := []string{"abc", "px", "%", "12.5%", "12.5px"}
	for _, tc := range cases {
		_, _, err := layout.ParseResizeArg(tc)
		assert.Error(t, err, "expected error for %q", tc)
	}
}
