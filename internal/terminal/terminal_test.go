package terminal_test

import (
	"testing"

	"github.com/nkh/rdw/internal/terminal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	m := terminal.New()
	assert.NotNil(t, m)
	assert.Len(t, m.All(), 0)
}

func TestGet_Missing(t *testing.T) {
	m := terminal.New()
	assert.Nil(t, m.Get("nope"))
}

func TestKill_Missing(t *testing.T) {
	m := terminal.New()
	assert.Error(t, m.Kill("nope"))
}

// TestLaunch_NoTTYD verifies Launch fails cleanly when neither ttyd nor socat
// is present. In CI both are absent so this exercises the error path.
func TestLaunch_ErrorPath(t *testing.T) {
	m := terminal.New()

	// We cannot guarantee ttyd/socat presence, so we only assert the Manager
	// state remains consistent regardless of which path is taken.
	port, err := m.Launch("pane1", "echo hello")
	if err != nil {
		// Expected in environments without ttyd/socat.
		assert.Equal(t, 0, port)
		assert.Len(t, m.All(), 0)
	} else {
		// ttyd or socat present: kill and verify cleanup.
		require.NoError(t, m.Kill("pane1"))
		assert.Len(t, m.All(), 0)
	}
}
