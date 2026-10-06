package webui

import (
	"bytes"
	"net/http"
	"os"
	"path/filepath"

	"github.com/iridium/jellex/assets"
)

// brandingCSS replaces the Plex wordmark in Plex Web's top bar with jellex's.
// The selectors match the pinned client (PMSVersion); the wordmark's aspect
// ratio is its viewBox.
const brandingCSS = `[class^="NavBar-logoContainer-"] svg[viewBox="0 0 105 48"] {
  width: auto;
  aspect-ratio: 133.2 / 51;
  background: url("wordmark.svg") center / contain no-repeat;
}
[class^="NavBar-logoContainer-"] svg[viewBox="0 0 105 48"] > * {
  display: none;
}
`

var brandingLink = []byte(`<link rel="stylesheet" href="/web/jellex/branding.css">`)

// serveBranding answers the requests jellex rebrands, leaving the client
// files on disk untouched. It reports whether it handled the request.
func serveBranding(w http.ResponseWriter, r *http.Request, dir string) bool {
	switch r.URL.Path {
	case "/favicon.ico":
		ServeFavicon(w, r)
	case "/jellex/wordmark.svg":
		serveAsset(w, "image/svg+xml", assets.Wordmark)
	case "/jellex/branding.css":
		serveAsset(w, "text/css; charset=utf-8", []byte(brandingCSS))
	case "/", "/index.html":
		page, err := os.ReadFile(filepath.Join(dir, "index.html"))
		if err != nil {
			return false
		}
		page = bytes.Replace(page, []byte("</head>"), append(brandingLink, "</head>"...), 1)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Write(page)
	default:
		return false
	}
	return true
}

// ServeFavicon serves the jellex logo as the favicon.
func ServeFavicon(w http.ResponseWriter, r *http.Request) {
	serveAsset(w, "image/svg+xml", assets.Logo)
}

func serveAsset(w http.ResponseWriter, contentType string, body []byte) {
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(body)
}
