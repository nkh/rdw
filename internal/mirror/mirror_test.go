package mirror_test

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nkh/rdw/internal/mirror"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFileSync_WritesData(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	sink, err := mirror.FileSync(path)
	require.NoError(t, err)

	_, err = sink.Write([]byte("hello\n"))
	require.NoError(t, err)
	require.NoError(t, sink.Close())

	data, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "hello\n", string(data))
}

func TestFileSync_Appends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	s1, _ := mirror.FileSync(path)
	_, _ = s1.Write([]byte("line1\n"))
	_ = s1.Close()

	s2, _ := mirror.FileSync(path)
	_, _ = s2.Write([]byte("line2\n"))
	_ = s2.Close()

	data, _ := os.ReadFile(path)
	assert.Equal(t, "line1\nline2\n", string(data))
}

func TestCmdSync_ReceivesData(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "cmd_out.txt")

	sink, err := mirror.CmdSync("cat > " + out)
	require.NoError(t, err)

	_, err = sink.Write([]byte("from cmd\n"))
	require.NoError(t, err)
	require.NoError(t, sink.Close())

	// Give the subprocess a moment.
	time.Sleep(50 * time.Millisecond)

	data, err := os.ReadFile(out)
	require.NoError(t, err)
	assert.Equal(t, "from cmd\n", string(data))
}

func TestTee_DeliversToBoth(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tee.txt")

	sink, err := mirror.FileSync(path)
	require.NoError(t, err)

	src := strings.NewReader("alpha\nbeta\ngamma\n")
	teed := mirror.Tee(src, sink)

	out, err := io.ReadAll(teed)
	require.NoError(t, err)
	assert.Equal(t, "alpha\nbeta\ngamma\n", string(out))

	// Give goroutine time to flush.
	time.Sleep(20 * time.Millisecond)

	fileData, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Equal(t, "alpha\nbeta\ngamma\n", string(fileData))
}
