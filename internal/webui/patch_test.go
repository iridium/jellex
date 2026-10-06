package webui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParsePatches(t *testing.T) {
	ps, err := parsePatches("x.patch", `# comment
file: js/main-*.js
--- find
a("x")
--- replace
b("y")
===
file: index.html
count: 2
--- find
<p>
  two lines
--- replace
<q>
`)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 2 {
		t.Fatalf("got %d patches", len(ps))
	}
	if ps[0].file != "js/main-*.js" || ps[0].find != `a("x")` || ps[0].replace != `b("y")` || ps[0].count != 1 {
		t.Errorf("patch 1 = %+v", ps[0])
	}
	if ps[1].count != 2 || ps[1].find != "<p>\n  two lines" || ps[1].replace != "<q>" {
		t.Errorf("patch 2 = %+v", ps[1])
	}
	if _, err := parsePatches("bad.patch", "file: x\n"); err == nil {
		t.Error("patch without find section accepted")
	}
}

func TestApplyPatches(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "js"), 0o755)
	os.WriteFile(filepath.Join(dir, "js", "main-1.js"), []byte(`x;a("x");y`), 0o644)
	p := patch{source: "t", file: "js/main-*.js", count: 1, find: `a("x")`, replace: `b("y")`}
	if err := applyPatches(dir, []patch{p}); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "js", "main-1.js"))
	if string(b) != `x;b("y");y` {
		t.Errorf("patched = %s", b)
	}
	// Applying again finds no match, which must fail rather than pass
	// silently: the client changed under the patch.
	if err := applyPatches(dir, []patch{p}); err == nil || !strings.Contains(err.Error(), "found 0 matches") {
		t.Errorf("err = %v", err)
	}
}

func TestStripIntegrity(t *testing.T) {
	dir := t.TempDir()
	page := `<link rel="stylesheet" href="/web/a.css" integrity="sha384-AbC+/1=" crossorigin="anonymous">` +
		`<script>b.sriHashes={45:"sha384-xyz",110:"sha384-q"};f.src=H,f.integrity=b.sriHashes[c],f.x=1</script>`
	os.WriteFile(filepath.Join(dir, "index.html"), []byte(page), 0o644)
	if err := stripIntegrity(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "index.html"))
	got := string(b)
	for _, gone := range []string{"integrity=\"", "sha384", "f.integrity="} {
		if strings.Contains(got, gone) {
			t.Errorf("still contains %q: %s", gone, got)
		}
	}
	if !strings.Contains(got, "b.sriHashes={}") || !strings.Contains(got, "f.src=H,void 0,f.x=1") {
		t.Errorf("unexpected result: %s", got)
	}
}

// TestEmbeddedPatchesParse keeps the shipped patch files valid.
func TestEmbeddedPatchesParse(t *testing.T) {
	ps, err := loadPatches()
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) == 0 {
		t.Fatal("no patches embedded")
	}
}

func TestRecolor(t *testing.T) {
	dir := t.TempDir()
	in := `a{color:#E5A00D;b:#cc7b19ff;c:rgba(229,160,13,.3);f:rgba(204, 123, 25, .3);g:rgb(1,2,3)}d{e:#e5a00db}` +
		`url("data:image/svg+xml,%3csvg stroke='%23F3B125'")`
	if err := os.WriteFile(filepath.Join(dir, "x.css"), []byte(in), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := recolor(dir); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "x.css"))
	want := `a{color:#00a4dc;b:#0083b0ff;c:rgba(0,164,220,.3);f:rgba(0,131,176, .3);g:rgb(1,2,3)}d{e:#e5a00db}` +
		`url("data:image/svg+xml,%3csvg stroke='%232cb9ec'")`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}
