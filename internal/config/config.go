// Package config loads jellex settings from the environment.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/iridium/jellex/internal/webui"
	"github.com/joho/godotenv"
)

type Config struct {
	// ListenAddr is where the emulated Plex API listens. Plex clients expect 32400.
	ListenAddr string
	// JellyfinURL is the base URL of the upstream Jellyfin server.
	JellyfinURL string
	// JellyfinAPIKey authenticates jellex against Jellyfin.
	JellyfinAPIKey string
	// ServerName is the friendly name advertised to Plex clients.
	ServerName string
	// MachineID is the stable identifier advertised to Plex clients and
	// plex.tv. Empty means derive it from the Jellyfin server ID.
	MachineID string
	// JellyfinUser is the Jellyfin user whose libraries and watch state are
	// served. Until auth exists every Plex client acts as this user; empty
	// means the first administrator.
	JellyfinUser string
	// WebDir caches the Plex Web client; it is downloaded there if missing.
	WebDir string
	// DataDir holds jellex state, such as the Jellyfin-to-Plex ID mapping.
	DataDir string
	// PublishURLs are the URLs published to plex.tv for Plex apps to connect
	// to, like PMS's custom server access URLs.
	PublishURLs []string
	// PlexClaim is a claim token from https://plex.tv/claim, used once to
	// claim jellex to a plex.tv account on startup.
	PlexClaim string
	// DisableCustomAssets serves the Plex Web client exactly as shipped,
	// without jellex's favicon and top-bar wordmark.
	DisableCustomAssets bool
}

// Load reads settings from the environment. A .env file in the working
// directory is loaded first if present; real environment variables win.
func Load() (Config, error) {
	if err := godotenv.Load(); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}
	c := Config{
		ListenAddr:     getenv("JELLEX_LISTEN_ADDR", ":32400"),
		JellyfinURL:    os.Getenv("JELLYFIN_URL"),
		JellyfinAPIKey: os.Getenv("JELLYFIN_API_KEY"),
		ServerName:     getenv("JELLEX_SERVER_NAME", "jellex"),
		MachineID:      os.Getenv("JELLEX_MACHINE_ID"),
		JellyfinUser:   os.Getenv("JELLYFIN_USER"),
		WebDir:         getenv("JELLEX_WEB_DIR", webui.DefaultDir()),
		DataDir:        getenv("JELLEX_DATA_DIR", defaultDataDir()),
		PublishURLs:    splitList(os.Getenv("JELLEX_PUBLISH_URLS")),
		PlexClaim:      os.Getenv("PLEX_CLAIM"),
	}
	if v := os.Getenv("JELLEX_DISABLE_CUSTOM_ASSETS"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return c, fmt.Errorf("JELLEX_DISABLE_CUSTOM_ASSETS: %w", err)
		}
		c.DisableCustomAssets = b
	}
	if c.JellyfinURL == "" {
		return c, fmt.Errorf("JELLYFIN_URL is required")
	}
	return c, nil
}

func defaultDataDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "jellex")
}

func splitList(s string) []string {
	var out []string
	for _, v := range strings.Split(s, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
