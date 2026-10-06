package auth

import (
	"testing"

	"github.com/iridium/jellex/internal/store"
)

func TestSessions(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s, err := Open(db, "http://jellyfin", "srv")
	if err != nil {
		t.Fatal(err)
	}
	id, err := s.Start(Session{UserID: "u2", UserName: "guest", Token: "t2", DeviceID: "d2"})
	if err != nil {
		t.Fatal(err)
	}
	if ses, ok := s.Get(id); !ok || ses.UserID != "u2" || ses.ServerID != "srv" {
		t.Errorf("session = %+v, %v", ses, ok)
	}
	if _, ok := s.Get("nope"); ok {
		t.Error("unknown session accepted")
	}

	// Pointed at a different Jellyfin server, old sessions are gone.
	other, err := Open(db, "http://jellyfin", "other")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := other.Get(id); ok {
		t.Error("session from another server accepted")
	}
}
