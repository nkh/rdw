package layout_test

import (
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
