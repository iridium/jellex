package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestOpenTwice(t *testing.T) {
	dir := t.TempDir()
	for range 2 {
		db, err := Open(dir)
		if err != nil {
			t.Fatal(err)
		}
		db.Close()
	}
}

func TestPrivate(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec("INSERT INTO ids (id, key) VALUES (1, 'x')"); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{FileName, FileName + "-wal"} {
		fi, err := os.Stat(filepath.Join(dir, name))
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Errorf("%s mode = %v, want 0600", name, fi.Mode().Perm())
		}
	}
}

func TestMachineID(t *testing.T) {
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, err := MachineID(db)
	if err != nil || len(a) != 40 {
		t.Fatalf("MachineID = %q, %v", a, err)
	}
	db.Close()
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if b, _ := MachineID(db); b != a {
		t.Errorf("machine id changed across reopen: %q then %q", a, b)
	}
	other, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	if c, _ := MachineID(other); c == a {
		t.Error("two databases got the same machine id")
	}
}
