// Package ids maps Jellyfin item GUIDs to the small integers Plex clients use
// as ratingKeys and section IDs. Assignments are persisted so keys stay stable
// across restarts, since clients cache them.
package ids

import (
	"database/sql"
	"log/slog"
	"sync"
)

// Map is the ID map: the ids table, fully cached in memory so lookups never
// touch the database.
type Map struct {
	db *sql.DB

	mu     sync.Mutex
	byGUID map[string]int
	byID   map[int]string
	next   int
}

// Open loads the map from db.
func Open(db *sql.DB) (*Map, error) {
	m := &Map{db: db, byGUID: map[string]int{}, byID: map[int]string{}, next: 1}
	rows, err := db.Query("SELECT id, key FROM ids")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int
		var key string
		if err := rows.Scan(&id, &key); err != nil {
			return nil, err
		}
		m.byGUID[key] = id
		m.byID[id] = key
		m.next = max(m.next, id+1)
	}
	return m, rows.Err()
}

// ID returns the integer for a Jellyfin GUID, assigning one if needed.
func (m *Map) ID(guid string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if id, ok := m.byGUID[guid]; ok {
		return id
	}
	id := m.next
	m.next++
	m.byGUID[guid] = id
	m.byID[id] = guid
	// Keep serving the ID even if it can't be saved; it just won't survive
	// a restart.
	if _, err := m.db.Exec("INSERT INTO ids (id, key) VALUES (?, ?)", id, guid); err != nil {
		slog.Error("save id", "id", id, "key", guid, "err", err)
	}
	return id
}

// GUID returns the Jellyfin GUID for an integer, if one was assigned.
func (m *Map) GUID(id int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.byID[id]
	return g, ok
}
