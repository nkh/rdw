package session_test

import (
	"fmt"
	"testing"

	"github.com/nkh/rdw/internal/session"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrollbackBuffer_BasicAppendAndLines(t *testing.T) {
	b := session.NewScrollbackBuffer(5)
	b.Append("line1")
	b.Append("line2")
	b.Append("line3")

	lines := b.Lines()
	require.Equal(t, []string{"line1", "line2", "line3"}, lines)
}

func TestScrollbackBuffer_Overflow_DropsOldest(t *testing.T) {
	b := session.NewScrollbackBuffer(3)
	b.Append("a")
	b.Append("b")
	b.Append("c")
	b.Append("d") // evicts "a"
	b.Append("e") // evicts "b"

	lines := b.Lines()
	assert.Equal(t, []string{"c", "d", "e"}, lines)
}

func TestScrollbackBuffer_Empty(t *testing.T) {
	b := session.NewScrollbackBuffer(10)
	assert.Equal(t, 0, b.Len())
	assert.Nil(t, b.Lines())
}

func TestScrollbackBuffer_Clear(t *testing.T) {
	b := session.NewScrollbackBuffer(5)
	b.Append("x")
	b.Append("y")
	b.Clear()

	assert.Equal(t, 0, b.Len())
	assert.Nil(t, b.Lines())
}

func TestScrollbackBuffer_DefaultCap(t *testing.T) {
	b := session.NewScrollbackBuffer(0)
	assert.Equal(t, session.DefaultScrollbackCap, b.Cap())
}

func TestScrollbackBuffer_ClampToMaxCap(t *testing.T) {
	b := session.NewScrollbackBuffer(session.MaxScrollbackCap + 1)
	assert.Equal(t, session.MaxScrollbackCap, b.Cap())
}

func TestScrollbackBuffer_ExactCapacity(t *testing.T) {
	cap := 4
	b := session.NewScrollbackBuffer(cap)

	for i := range cap {
		b.Append(fmt.Sprintf("line%d", i))
	}

	assert.Equal(t, cap, b.Len())
	lines := b.Lines()
	assert.Equal(t, "line0", lines[0])
	assert.Equal(t, "line3", lines[3])
}

func TestScrollbackBuffer_OrderAfterMultipleWraps(t *testing.T) {
	b := session.NewScrollbackBuffer(3)

	// Fill and wrap three times
	for i := range 9 {
		b.Append(fmt.Sprintf("%d", i))
	}

	// Only last 3 lines should remain: "6", "7", "8"
	lines := b.Lines()
	assert.Equal(t, []string{"6", "7", "8"}, lines)
}

func TestScrollbackBuffer_Concurrent(t *testing.T) {
	b := session.NewScrollbackBuffer(100)
	done := make(chan struct{})

	for i := range 50 {
		go func(n int) {
			b.Append(fmt.Sprintf("line%d", n))
			_ = b.Lines()
			done <- struct{}{}
		}(i)
	}

	for range 50 {
		<-done
	}
}
