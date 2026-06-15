package cycle_test

import (
	"context"
	"testing"
	"time"

	"github.com/nkh/rdw/internal/cycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_Valid(t *testing.T) {
	c, err := cycle.New([]string{"a", "b", "c"}, 100*time.Millisecond)
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, c.Windows())
	assert.Equal(t, 100*time.Millisecond, c.Interval())
}

func TestNew_EmptyWindows(t *testing.T) {
	_, err := cycle.New([]string{}, time.Second)
	assert.Error(t, err)
}

func TestNew_ZeroInterval(t *testing.T) {
	_, err := cycle.New([]string{"a"}, 0)
	assert.Error(t, err)
}

func TestRun_ReceivesFirstEvent(t *testing.T) {
	c, _ := cycle.New([]string{"home", "logs"}, 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ch := c.Run(ctx)
	ev := <-ch
	assert.Equal(t, "home", ev.Window)
}

func TestRun_Rotates(t *testing.T) {
	c, _ := cycle.New([]string{"a", "b", "c"}, 30*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	ch := c.Run(ctx)
	seen := map[string]bool{}

	for ev := range ch {
		seen[ev.Window] = true
	}

	assert.True(t, seen["a"])
	assert.True(t, seen["b"])
	assert.True(t, seen["c"])
}

func TestRun_StopsOnCancel(t *testing.T) {
	c, _ := cycle.New([]string{"x"}, 10*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	ch := c.Run(ctx)
	<-ch   // first event
	cancel()

	// Channel must close after cancel.
	_, open := <-ch
	assert.False(t, open)
}
