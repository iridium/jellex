// Package store opens jellex's SQLite database, which holds everything
// jellex persists: the Plex ID map, sign-in sessions, stream selections and
// play queues. Live state tied to running processes (transcodes, now
// playing) stays in memory.
package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// FileName is the database file inside the data directory.
const FileName = "jellex.db"

// schema is created on open. jellex has no users to migrate yet: change it
// freely, and delete jellex.db if an existing one no longer fits.
const schema = `
	CREATE TABLE IF NOT EXISTS settings (
		key   TEXT PRIMARY KEY,
		value TEXT NOT NULL
	);
	CREATE TABLE IF NOT EXISTS ids (
		id  INTEGER PRIMARY KEY,
		key TEXT NOT NULL UNIQUE
	);
	CREATE TABLE IF NOT EXISTS sessions (
		id        TEXT PRIMARY KEY,
		user_id   TEXT NOT NULL,
		user_name TEXT NOT NULL,
		token     TEXT NOT NULL,
		device_id TEXT NOT NULL,
		server_id TEXT NOT NULL,
		created   INTEGER NOT NULL,
		seen      INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS stream_selections (
		part_id  INTEGER PRIMARY KEY,
		audio    INTEGER NOT NULL,
		subtitle INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS play_queues (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		uri      TEXT NOT NULL,
		selected INTEGER NOT NULL,
		created  INTEGER NOT NULL
	);
	CREATE TABLE IF NOT EXISTS play_queue_items (
		id       INTEGER PRIMARY KEY AUTOINCREMENT,
		queue_id INTEGER NOT NULL REFERENCES play_queues(id) ON DELETE CASCADE,
		pos      INTEGER NOT NULL,
		guid     TEXT NOT NULL
	);
	CREATE INDEX IF NOT EXISTS play_queue_items_queue ON play_queue_items(queue_id, pos);`

// Open opens (creating if needed) the database in dataDir.
func Open(dataDir string) (*sql.DB, error) {
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, err
	}
	path := filepath.Join(dataDir, FileName)
	// Sessions hold Jellyfin access tokens, so keep the database private.
	// SQLite gives the -wal and -shm files the database file's mode, so
	// create it 0600 up front.
	if f, err := os.OpenFile(path, os.O_CREATE|os.O_RDONLY, 0o600); err == nil {
		f.Close()
	}
	for _, p := range []string{path, path + "-wal", path + "-shm"} {
		os.Chmod(p, 0o600)
	}
	db, err := sql.Open("sqlite", "file:"+path+
		"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(1)&_pragma=synchronous(NORMAL)")
	if err != nil {
		return nil, err
	}
	// One connection: writes are tiny, and it rules out SQLITE_BUSY between
	// jellex's own goroutines.
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema in %s: %w", path, err)
	}
	return db, nil
}

// MachineID returns this jellex's Plex server identity, generated on first
// use. It's per database, so separate instances (even on one Jellyfin) never
// share one, and Plex Web doesn't mix them up.
func MachineID(db *sql.DB) (string, error) {
	var id string
	err := db.QueryRow("SELECT value FROM settings WHERE key = 'machine_id'").Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return "", err
	}
	b := make([]byte, 20) // PMS uses 40 hex digits
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id = hex.EncodeToString(b)
	_, err = db.Exec("INSERT INTO settings (key, value) VALUES ('machine_id', ?)", id)
	return id, err
}
