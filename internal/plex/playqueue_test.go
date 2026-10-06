package plex

import (
	"database/sql"
	"errors"
	"reflect"
	"testing"

	"github.com/iridium/jellex/internal/store"
)

func TestURIKey(t *testing.T) {
	for uri, want := range map[string]string{
		"server://m/com.plexapp.plugins.library/library/metadata/7":                             "7",
		"server://m/com.plexapp.plugins.library/library/metadata/7/children":                    "7",
		"server://m/com.plexapp.plugins.library/library/metadata/7/children?excludeAllLeaves=1": "7",
		"server://m/com.plexapp.plugins.library/library/collections/9/items":                    "9",
		"/library/metadata/8965": "8965",
		"server://m/com.plexapp.plugins.library/library/sections/1/all": "",
	} {
		got := ""
		if m := uriKey.FindStringSubmatch(uri); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("uriKey(%q) = %q, want %q", uri, got, want)
		}
	}
}

func TestPlayQueueRoundTrip(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := &Server{db: db}

	a := &playQueue{URI: "server://m/x/library/metadata/1", Items: []string{"g1", "g2", "g3"}, Selected: 1}
	b := &playQueue{URI: "server://m/x/library/metadata/2", Items: []string{"g4"}}
	if err := s.addPlayQueue(a); err != nil {
		t.Fatal(err)
	}
	if err := s.addPlayQueue(b); err != nil {
		t.Fatal(err)
	}
	if a.ID == b.ID || len(a.ItemIDs) != 3 || b.ItemIDs[0] == a.ItemIDs[2] {
		t.Fatalf("ids not unique: %+v %+v", a, b)
	}
	got, err := s.playQueue(a.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, a) {
		t.Errorf("loaded %+v, want %+v", got, a)
	}
	if _, err := s.playQueue(999); !errors.Is(err, sql.ErrNoRows) {
		t.Errorf("missing queue: err = %v", err)
	}
}
