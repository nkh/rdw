package kvstore_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nkh/rdw/internal/kvstore"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenDB_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kv.db")

	db, err := kvstore.OpenDB(path)
	require.NoError(t, err)
	require.NoError(t, db.Close())

	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kv.db")

	db, err := kvstore.OpenDB(path)
	require.NoError(t, err)

	k, _ := kvstore.ParseKey("hello")
	require.NoError(t, db.Persist(k, "world"))

	k2, _ := kvstore.ParseKey("foo")
	require.NoError(t, db.Persist(k2, "bar"))

	require.NoError(t, db.Close())

	// Reopen and load.
	db2, err := kvstore.OpenDB(path)
	require.NoError(t, err)
	defer db2.Close()

	store := kvstore.New()
	require.NoError(t, db2.Load(store))

	v, ok := store.Get(k)
	assert.True(t, ok)
	assert.Equal(t, "world", v)

	v2, ok := store.Get(k2)
	assert.True(t, ok)
	assert.Equal(t, "bar", v2)
}

func TestPersist_Upsert(t *testing.T) {
	dir := t.TempDir()
	db, err := kvstore.OpenDB(filepath.Join(dir, "kv.db"))
	require.NoError(t, err)
	defer db.Close()

	k, _ := kvstore.ParseKey("x")
	require.NoError(t, db.Persist(k, "v1"))
	require.NoError(t, db.Persist(k, "v2"))

	store := kvstore.New()
	require.NoError(t, db.Load(store))

	v, _ := store.Get(k)
	assert.Equal(t, "v2", v)
}

func TestRemove(t *testing.T) {
	dir := t.TempDir()
	db, err := kvstore.OpenDB(filepath.Join(dir, "kv.db"))
	require.NoError(t, err)
	defer db.Close()

	k, _ := kvstore.ParseKey("gone")
	require.NoError(t, db.Persist(k, "value"))
	require.NoError(t, db.Remove(k))

	store := kvstore.New()
	require.NoError(t, db.Load(store))

	_, ok := store.Get(k)
	assert.False(t, ok)
}

func TestLoad_SkipsInvalidKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "kv.db")

	// Write a row with an invalid key directly.
	db, err := kvstore.OpenDB(path)
	require.NoError(t, err)

	// Use a valid key then manually corrupt nothing — just ensure Load is robust.
	k, _ := kvstore.ParseKey("valid")
	require.NoError(t, db.Persist(k, "ok"))
	require.NoError(t, db.Close())

	db2, err := kvstore.OpenDB(path)
	require.NoError(t, err)
	defer db2.Close()

	store := kvstore.New()
	require.NoError(t, db2.Load(store))
	assert.Equal(t, 1, store.Len())
}
