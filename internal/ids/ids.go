// Package ids maps Jellyfin item GUIDs to the small integers Plex clients use
// as ratingKeys and section IDs. Assignments are persisted so keys stay stable
// across restarts, since clients cache them.
package ids

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

type Map struct {
	path string

	mu     sync.Mutex
	byGUID map[string]int
	byID   map[int]string
	next   int
}

// Open loads the mapping from path, creating it on first use.
func Open(path string) (*Map, error) {
	m := &Map{path: path, byGUID: map[string]int{}, byID: map[int]string{}, next: 1}
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return m, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &m.byGUID); err != nil {
		return nil, err
	}
	for g, id := range m.byGUID {
		m.byID[id] = g
		m.next = max(m.next, id+1)
	}
	return m, nil
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
	m.saveLocked()
	return id
}

// GUID returns the Jellyfin GUID for an integer, if one was assigned.
func (m *Map) GUID(id int) (string, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	g, ok := m.byID[id]
	return g, ok
}

// saveLocked rewrites the whole file. Fine for dev-sized libraries; swap for a
// real store once this matters.
func (m *Map) saveLocked() {
	b, err := json.Marshal(m.byGUID)
	if err != nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return
	}
	tmp := m.path + ".tmp"
	if os.WriteFile(tmp, b, 0o644) == nil {
		os.Rename(tmp, m.path)
	}
}
