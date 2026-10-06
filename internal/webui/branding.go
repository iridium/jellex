package webui

import (
	"net/http"

	"github.com/iridium/jellex/assets"
)

// jellexCSS adjusts Plex Web's look: jellex's wordmark in the top bar, and
// hiding UI that has no meaning for jellex (the server activity dashboard,
// the plex.tv account menu, and server administration in settings). The
// selectors match the pinned client (PMSVersion); the wordmark's aspect
// ratio is its viewBox.
const jellexCSS = `[class^="NavBar-logoContainer-"] svg[viewBox="0 0 105 48"] {
  width: auto;
  aspect-ratio: 133.2 / 51;
  background: url("wordmark.svg") center / contain no-repeat;
}
[class^="NavBar-logoContainer-"] svg[viewBox="0 0 105 48"] > * {
  display: none;
}

/* Top bar: the activity dashboard and the account menu. */
[class^="NavBarActivityButton-container-"],
[data-testid="navbarAccountMenuTrigger"] {
  display: none !important;
}

/* Settings sidebar: everything from the server picker down (Status,
   Settings, Manage); only the Plex Web section stays. */
[class^="SettingsSidebar-sidebarContent-"] > [class*="ServerSettingsServerMenuButton-button-"],
[class^="SettingsSidebar-sidebarContent-"] > [class*="ServerSettingsServerMenuButton-button-"] ~ * {
  display: none !important;
}
`

// serveBranding serves jellex's own assets that the patched client refers to
// (patches/20-branding.patch links the stylesheet). It reports whether it
// handled the request.
func serveBranding(w http.ResponseWriter, r *http.Request) bool {
	switch r.URL.Path {
	case "/favicon.ico":
		ServeFavicon(w, r)
	case "/jellex/wordmark.svg":
		serveAsset(w, "image/svg+xml", assets.Wordmark)
	case "/jellex/jellex.css":
		serveAsset(w, "text/css; charset=utf-8", []byte(jellexCSS))
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
