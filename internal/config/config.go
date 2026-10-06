// Package config loads jellex settings from the environment.
package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"

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
	// ServerName is the friendly name advertised to Plex clients. Empty
	// means use the Jellyfin server's name.
	ServerName string
	// MachineID is the server identity advertised to Plex clients, kept in
	// the database (store.MachineID) and filled in at startup.
	MachineID string
	// WebDir caches the Plex Web client; it is downloaded there if missing.
	WebDir string
	// DataDir holds jellex state, such as the Jellyfin-to-Plex ID mapping.
	DataDir string
	// JellyfinServerID is the connected Jellyfin server's ID, filled in at
	// startup rather than configured.
	JellyfinServerID string
	// Dev turns off sign-in: anyone who can reach jellex acts as the first
	// Jellyfin administrator. For local development and testing only.
	Dev bool
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
		ServerName:     os.Getenv("JELLEX_SERVER_NAME"),
		WebDir:         getenv("JELLEX_WEB_DIR", webui.DefaultDir()),
		DataDir:        getenv("JELLEX_DATA_DIR", defaultDataDir()),
	}
	var err error
	if c.Dev, err = getbool("JELLEX_DEV"); err != nil {
		return c, err
	}
	if c.DisableCustomAssets, err = getbool("JELLEX_DISABLE_CUSTOM_ASSETS"); err != nil {
		return c, err
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

func getbool(key string) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return false, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
