package kvstore

import (
	"database/sql"
	"fmt"

	_ "github.com/mattn/go-sqlite3"
)

// DB wraps a SQLite connection for KV persistence.
type DB struct {
	conn *sql.DB
}

// OpenDB opens (or creates) a SQLite database at path and returns a DB handle.
// The kv table is created if absent.
func OpenDB(path string) (*DB, error) {
	conn, err := sql.Open("sqlite3", path+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		return nil, fmt.Errorf("kvstore: open %s: %w", path, err)
	}

	if _, err = conn.Exec(
		`CREATE TABLE IF NOT EXISTS kv (
			key   TEXT PRIMARY KEY NOT NULL,
			value TEXT NOT NULL
		)`,
	); err != nil {
		conn.Close()
		return nil, fmt.Errorf("kvstore: create table: %w", err)
	}

	return &DB{conn: conn}, nil
}

// Close closes the underlying database connection.
func (db *DB) Close() error { return db.conn.Close() }

// Load reads all rows from the database into store, respecting size limits.
// Rows that violate limits are silently skipped.
func (db *DB) Load(store *Store) error {
	rows, err := db.conn.Query(`SELECT key, value FROM kv`)
	if err != nil {
		return fmt.Errorf("kvstore: load: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var ks, vs string
		if err := rows.Scan(&ks, &vs); err != nil {
			continue
		}

		k, err := ParseKey(ks)
		if err != nil {
			continue
		}

		_ = store.Set(k, vs) // ignore per-entry limit violations from old data
	}

	return rows.Err()
}

// Persist writes a single key-value pair to the database.
func (db *DB) Persist(k Key, value string) error {
	_, err := db.conn.Exec(
		`INSERT INTO kv (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		k.String(), value,
	)

	if err != nil {
		return fmt.Errorf("kvstore: persist %q: %w", k, err)
	}

	return nil
}

// Remove deletes a key from the database.
func (db *DB) Remove(k Key) error {
	_, err := db.conn.Exec(`DELETE FROM kv WHERE key = ?`, k.String())
	if err != nil {
		return fmt.Errorf("kvstore: remove %q: %w", k, err)
	}

	return nil
}
