package ids

import (
	"testing"

	"github.com/iridium/jellex/internal/store"
)

func TestPersist(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	m, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	a, b := m.ID("aaa"), m.ID("part:bbb")
	if a == b || m.ID("aaa") != a {
		t.Fatalf("ids not stable/unique: %d %d", a, b)
	}
	if g, ok := m.GUID(b); !ok || g != "part:bbb" {
		t.Errorf("GUID(%d) = %q, %v", b, g, ok)
	}

	// A fresh Map over the same database sees the same assignments and
	// keeps counting from there.
	m2, err := Open(db)
	if err != nil {
		t.Fatal(err)
	}
	if id := m2.ID("part:bbb"); id != b {
		t.Errorf("reloaded id = %d, want %d", id, b)
	}
	if id := m2.ID("ccc"); id != b+1 {
		t.Errorf("next id after reload = %d, want %d", id, b+1)
	}
}
