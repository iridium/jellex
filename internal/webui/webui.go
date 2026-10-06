// Package webui serves the Plex Web client at /web like PMS does. The client
// isn't redistributed with jellex: on first start it is pulled out of a pinned
// Plex Media Server .deb and cached on disk.
package webui

import (
	"archive/tar"
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/ulikunitz/xz"
)

const (
	// PMSVersion is the Plex Media Server release the web client is taken from.
	PMSVersion = "1.43.4.10903-e5521bd8c"
	debURL     = "https://downloads.plex.tv/plex-media-server-new/" + PMSVersion + "/debian/plexmediaserver_" + PMSVersion + "_amd64.deb"
	debSHA256  = "6f6a1c8336d779e1f20151a6934349884bad6f6a66b18a2b189b96e55c3b3edb"
	// bundlePrefix is where the web client lives inside the package's data.tar.
	bundlePrefix = "./usr/lib/plexmediaserver/Resources/Plug-ins-e5521bd8c/WebClient.bundle/Contents/Resources/"
)

// DefaultDir is the cache directory used when none is configured.
func DefaultDir() string {
	base, err := os.UserCacheDir()
	if err != nil {
		base = os.TempDir()
	}
	return filepath.Join(base, "jellex", "plex-web-"+PMSVersion)
}

// Handler serves the web client from dir, mounted under /web/ with the prefix
// stripped. If dir has no client yet, it is downloaded in the background and
// requests get 503 until it's ready. If custom, it serves a patched copy of
// the client (patch.go) with jellex's branding; otherwise the client exactly
// as shipped.
func Handler(ctx context.Context, dir string, custom bool) http.Handler {
	// files is set once the client is on disk (and patched, if custom).
	var files atomic.Pointer[http.Handler]
	prepare := func() {
		serve := dir
		if custom {
			p, err := patchedDir(dir)
			if err != nil {
				slog.Error("patching plex web client failed; serving it unpatched", "err", err)
			} else {
				serve = p
			}
		}
		h := http.FileServerFS(os.DirFS(serve))
		files.Store(&h)
		slog.Info("plex web client ready", "dir", serve)
	}
	if present(dir) {
		prepare()
	} else {
		go func() {
			slog.Info("plex web client not found, downloading", "dir", dir, "url", debURL)
			if err := fetch(ctx, dir); err != nil {
				slog.Error("plex web client download failed", "err", err)
				return
			}
			prepare()
		}()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fs := files.Load()
		if fs == nil {
			w.Header().Set("Retry-After", "10")
			http.Error(w, "Plex web client is still downloading, try again shortly.", http.StatusServiceUnavailable)
			return
		}
		if custom && serveBranding(w, r) {
			return
		}
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			// Hashed assets can be cached forever; the page that names
			// them must be revalidated.
			w.Header().Set("Cache-Control", "no-cache")
			if custom {
				w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
			}
		}
		(*fs).ServeHTTP(w, r)
	})
}

func present(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "index.html"))
	return err == nil
}

// fetch downloads the pinned .deb, verifies it, and extracts the web client
// into dir. Extraction goes to a temp dir first so a partial result is never
// mistaken for a complete one.
func fetch(ctx context.Context, dir string) error {
	parent := filepath.Dir(dir)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return err
	}
	deb, err := os.CreateTemp(parent, "pms-*.deb")
	if err != nil {
		return err
	}
	defer os.Remove(deb.Name())
	defer deb.Close()

	if err := download(ctx, deb); err != nil {
		return err
	}
	if _, err := deb.Seek(0, io.SeekStart); err != nil {
		return err
	}

	tmp, err := os.MkdirTemp(parent, "plex-web-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := extract(deb, tmp); err != nil {
		return err
	}
	if !present(tmp) {
		return errors.New("web client not found in package")
	}
	return os.Rename(tmp, dir)
}

func download(ctx context.Context, dst io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, debURL, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", debURL, resp.Status)
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(dst, h), resp.Body); err != nil {
		return err
	}
	if got := hex.EncodeToString(h.Sum(nil)); got != debSHA256 {
		return fmt.Errorf("checksum mismatch: got %s, want %s", got, debSHA256)
	}
	return nil
}

// extract finds data.tar.xz in the .deb (an ar archive) and writes the web
// client files from it into dir.
func extract(deb io.Reader, dir string) error {
	data, err := arMember(bufio.NewReader(deb), "data.tar.xz")
	if err != nil {
		return err
	}
	xr, err := xz.NewReader(data)
	if err != nil {
		return err
	}
	tr := tar.NewReader(xr)
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		rel, ok := strings.CutPrefix(hdr.Name, bundlePrefix)
		if !ok || rel == "" || hdr.Typeflag != tar.TypeReg {
			continue
		}
		if !filepath.IsLocal(rel) {
			return fmt.Errorf("unsafe path in package: %s", hdr.Name)
		}
		dst := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			return err
		}
		_, err = io.Copy(f, tr)
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return err
		}
	}
}

// arMember returns a reader over the named member of an ar archive.
func arMember(r io.Reader, name string) (io.Reader, error) {
	magic := make([]byte, 8)
	if _, err := io.ReadFull(r, magic); err != nil {
		return nil, err
	}
	if string(magic) != "!<arch>\n" {
		return nil, errors.New("not an ar archive")
	}
	hdr := make([]byte, 60)
	for {
		if _, err := io.ReadFull(r, hdr); err != nil {
			return nil, fmt.Errorf("%s not found in package: %w", name, err)
		}
		size, err := strconv.ParseInt(strings.TrimSpace(string(hdr[48:58])), 10, 64)
		if err != nil {
			return nil, fmt.Errorf("bad ar header: %w", err)
		}
		if strings.TrimRight(strings.TrimSpace(string(hdr[0:16])), "/") == name {
			return io.LimitReader(r, size), nil
		}
		// Members are padded to an even length.
		if _, err := io.CopyN(io.Discard, r, size+size%2); err != nil {
			return nil, err
		}
	}
}

// contentSecurityPolicy keeps Plex Web talking only to jellex: no plex.tv,
// analytics, Sentry or other third parties. Patches already switch most of
// that off; this blocks whatever is left. The exceptions: Google's Cast SDK
// (scripts from www.gstatic.com) for Chromecast, and eval, which Plex Web's
// templates need.
const contentSecurityPolicy = "default-src 'self'; " +
	"script-src 'self' 'unsafe-inline' 'unsafe-eval' www.gstatic.com; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data: blob:; " +
	"media-src 'self' blob:; " +
	"font-src 'self' data:; " +
	"connect-src 'self'; " +
	"worker-src 'self' blob:; " +
	"frame-src 'self'; " +
	"object-src 'none'; base-uri 'self'"
