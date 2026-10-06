package webui

import (
	"bufio"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// jellex customizes Plex Web by applying find-and-replace patches to a copy
// of the pristine client. The client's JS is minified (a few huge lines), so
// line-based diffs don't work well; exact snippets with an expected match
// count do, and fail loudly when a Plex Web update moves things.
//
// Patch files (patches/*.patch) hold one or more blocks separated by a line
// containing only "===":
//
//	# comments start with #
//	file: js/main-*.js      glob relative to the client root
//	count: 1                expected matches in total (default 1)
//	--- find
//	exact text to find
//	--- replace
//	replacement text
//
// The newline ending the find and replace sections isn't part of them.

//go:embed patches/*.patch
var patchFiles embed.FS

type patch struct {
	source  string // patch file and block, for errors
	file    string
	count   int
	find    string
	replace string
}

func loadPatches() ([]patch, error) {
	names, err := fs.Glob(patchFiles, "patches/*.patch")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var out []patch
	for _, name := range names {
		b, err := patchFiles.ReadFile(name)
		if err != nil {
			return nil, err
		}
		ps, err := parsePatches(filepath.Base(name), string(b))
		if err != nil {
			return nil, err
		}
		out = append(out, ps...)
	}
	return out, nil
}

func parsePatches(name, text string) ([]patch, error) {
	var out []patch
	for i, block := range splitBlocks(text) {
		p := patch{source: fmt.Sprintf("%s#%d", name, i+1), count: 1}
		var section string // "", "find" or "replace"
		var find, replace []string
		sc := bufio.NewScanner(strings.NewReader(block))
		sc.Buffer(make([]byte, 64<<10), 16<<20)
		for sc.Scan() {
			line := sc.Text()
			switch {
			case line == "--- find":
				section = "find"
			case line == "--- replace":
				section = "replace"
			case section == "find":
				find = append(find, line)
			case section == "replace":
				replace = append(replace, line)
			case strings.HasPrefix(line, "#") || strings.TrimSpace(line) == "":
			case strings.HasPrefix(line, "file:"):
				p.file = strings.TrimSpace(strings.TrimPrefix(line, "file:"))
			case strings.HasPrefix(line, "count:"):
				n, err := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "count:")))
				if err != nil || n < 1 {
					return nil, fmt.Errorf("%s: bad count %q", p.source, line)
				}
				p.count = n
			default:
				return nil, fmt.Errorf("%s: unexpected line %q", p.source, line)
			}
		}
		if err := sc.Err(); err != nil {
			return nil, fmt.Errorf("%s: %w", p.source, err)
		}
		p.find, p.replace = strings.Join(find, "\n"), strings.Join(replace, "\n")
		if p.file == "" || p.find == "" {
			return nil, fmt.Errorf("%s: needs file: and a --- find section", p.source)
		}
		out = append(out, p)
	}
	return out, nil
}

// splitBlocks splits a patch file on lines containing only "===".
func splitBlocks(text string) []string {
	var blocks []string
	var cur []string
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		if line == "===" {
			blocks = append(blocks, strings.Join(cur, "\n"))
			cur = nil
			continue
		}
		cur = append(cur, line)
	}
	blocks = append(blocks, strings.Join(cur, "\n"))
	var out []string
	for _, b := range blocks {
		if strings.TrimSpace(stripComments(b)) != "" {
			out = append(out, b)
		}
	}
	return out
}

func stripComments(block string) string {
	var keep []string
	for _, l := range strings.Split(block, "\n") {
		if !strings.HasPrefix(l, "#") {
			keep = append(keep, l)
		}
	}
	return strings.Join(keep, "\n")
}

// patchSetID identifies the patch set and patching logic, so a changed set
// produces a fresh patched copy. Bump the version when the patching code
// itself (stripIntegrity, recolor) changes.
func patchSetID(ps []patch) string {
	h := sha256.New()
	fmt.Fprintf(h, "v3\x00%v\x00", accentColors)
	for _, p := range ps {
		fmt.Fprintf(h, "%s\x00%d\x00%s\x00%s\x00", p.file, p.count, p.find, p.replace)
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// applyPatches applies every patch to the client in dir.
func applyPatches(dir string, ps []patch) error {
	for _, p := range ps {
		files, err := filepath.Glob(filepath.Join(dir, filepath.FromSlash(p.file)))
		if err != nil {
			return fmt.Errorf("%s: %w", p.source, err)
		}
		if len(files) == 0 {
			return fmt.Errorf("%s: no files match %s", p.source, p.file)
		}
		total := 0
		contents := map[string]string{}
		for _, f := range files {
			b, err := os.ReadFile(f)
			if err != nil {
				return err
			}
			contents[f] = string(b)
			total += strings.Count(string(b), p.find)
		}
		if total != p.count {
			return fmt.Errorf("%s: found %d matches in %s, want %d", p.source, total, p.file, p.count)
		}
		for f, s := range contents {
			if !strings.Contains(s, p.find) {
				continue
			}
			if err := os.WriteFile(f, []byte(strings.ReplaceAll(s, p.find, p.replace)), 0o644); err != nil {
				return err
			}
		}
	}
	return nil
}

var (
	integrityAttr = regexp.MustCompile(`\s+integrity="sha(?:256|384|512)-[A-Za-z0-9+/=]+"`)
	sriTable      = regexp.MustCompile(`sriHashes=\{[^}]*\}`)
	sriAssign     = regexp.MustCompile(`\w+\.integrity=\w+\.sriHashes\[\w+\]`)
)

// stripIntegrity removes Subresource Integrity checks from the client's HTML
// (tag attributes and webpack's chunk hash table), since patched files no
// longer match their hashes.
func stripIntegrity(dir string) error {
	htmls, err := filepath.Glob(filepath.Join(dir, "*.html"))
	if err != nil {
		return err
	}
	for _, f := range htmls {
		b, err := os.ReadFile(f)
		if err != nil {
			return err
		}
		s := integrityAttr.ReplaceAllString(string(b), "")
		s = sriTable.ReplaceAllString(s, "sriHashes={}")
		s = sriAssign.ReplaceAllString(s, "void 0")
		if err := os.WriteFile(f, []byte(s), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// accentColors maps Plex's orange accents (lowercase hex) to Jellyfin's
// blue: the brand accent, a darker shade for pressed and secondary states,
// and a lighter one for hover and focus.
var accentColors = map[string]string{
	"e5a00d": "00a4dc", // Plex accent
	"cc7b19": "0083b0", // darker
	"f9be03": "2cb9ec", // lighter
	"ebaf00": "2cb9ec",
	"f3b125": "2cb9ec",
	"f8ad18": "2cb9ec", // setup illustrations (in URL-encoded SVGs)
}

var (
	accentHex = regexp.MustCompile(`(?i)(#|%23)(e5a00d|cc7b19|f9be03|ebaf00|f3b125|f8ad18)((?:[0-9a-f]{2})?)\b`)
	// accentRGB matches the same colors written as rgb()/rgba(), e.g. the
	// seek bar's buffered range, rgba(204,123,25,.3).
	accentRGB = regexp.MustCompile(`(rgba?\()(\d{1,3}), ?(\d{1,3}), ?(\d{1,3})([,)])`)
)

// rgbHex returns the hex form of an rgb() triple.
func rgbHex(r, g, b string) string {
	n := func(s string) int { v, _ := strconv.Atoi(s); return v }
	return fmt.Sprintf("%02x%02x%02x", n(r), n(g), n(b))
}

// hexRGB returns the "r,g,b" form of a 6-digit hex color.
func hexRGB(hex string) string {
	v, _ := strconv.ParseUint(hex, 16, 32)
	return fmt.Sprintf("%d,%d,%d", v>>16, v>>8&0xff, v&0xff)
}

// recolor swaps Plex's accent colors for Jellyfin's throughout the client.
// They're hard-coded in hundreds of places across CSS and JS, so this is a
// rewrite rather than patch blocks.
func recolor(dir string) error {
	return filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		switch filepath.Ext(path) {
		case ".css", ".js", ".html", ".svg":
		default:
			return nil
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		s := accentHex.ReplaceAllStringFunc(string(b), func(m string) string {
			sm := accentHex.FindStringSubmatch(m) // prefix, color, alpha
			return sm[1] + accentColors[strings.ToLower(sm[2])] + sm[3]
		})
		s = accentRGB.ReplaceAllStringFunc(s, func(m string) string {
			sm := accentRGB.FindStringSubmatch(m) // prefix, r, g, b, terminator
			to, ok := accentColors[rgbHex(sm[2], sm[3], sm[4])]
			if !ok {
				return m
			}
			return sm[1] + hexRGB(to) + sm[5]
		})
		if s == string(b) {
			return nil
		}
		return os.WriteFile(path, []byte(s), 0o644)
	})
}

// patchedDir returns the directory holding the patched copy of the client in
// pristine, building it if needed. Copies for other patch sets are removed.
func patchedDir(pristine string) (string, error) {
	ps, err := loadPatches()
	if err != nil {
		return "", err
	}
	dir := pristine + "-patched-" + patchSetID(ps)
	if present(dir) {
		return dir, nil
	}
	old, _ := filepath.Glob(pristine + "-patched-*")
	tmp, err := os.MkdirTemp(filepath.Dir(pristine), filepath.Base(pristine)+"-patching-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmp)
	if err := os.CopyFS(tmp, os.DirFS(pristine)); err != nil {
		return "", fmt.Errorf("copy client: %w", err)
	}
	if err := stripIntegrity(tmp); err != nil {
		return "", fmt.Errorf("strip integrity: %w", err)
	}
	if err := applyPatches(tmp, ps); err != nil {
		return "", err
	}
	if err := recolor(tmp); err != nil {
		return "", fmt.Errorf("recolor: %w", err)
	}
	if err := os.Rename(tmp, dir); err != nil && !errors.Is(err, fs.ErrExist) {
		return "", err
	}
	for _, o := range old {
		os.RemoveAll(o)
	}
	return dir, nil
}
