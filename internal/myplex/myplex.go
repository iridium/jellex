// Package myplex registers jellex with plex.tv the way Plex Media Server
// does: claiming it to an account and publishing the URLs it can be reached
// at, so Plex apps find it through the account's server list.
package myplex

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const baseURL = "https://plex.tv"

// Identity is how jellex presents itself to plex.tv.
type Identity struct {
	MachineID string
	Name      string
	Version   string
}

// state is persisted so the claim survives restarts.
type state struct {
	AuthToken string `json:"authToken"`
	Username  string `json:"username"`
}

type Client struct {
	id   Identity
	path string
	http *http.Client

	mu sync.Mutex
	st state
}

// Open loads any existing claim from dataDir.
func Open(dataDir string, id Identity) (*Client, error) {
	c := &Client{id: id, path: filepath.Join(dataDir, "myplex.json"), http: &http.Client{Timeout: 30 * time.Second}}
	b, err := os.ReadFile(c.path)
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &c.st); err != nil {
		return nil, fmt.Errorf("parse %s: %w", c.path, err)
	}
	return c, nil
}

// Claimed reports whether jellex is claimed, and by whom.
func (c *Client) Claimed() (username string, ok bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.Username, c.st.AuthToken != ""
}

func (c *Client) token() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.st.AuthToken
}

// Claim exchanges a claim token from https://plex.tv/claim for the server's
// own plex.tv token and stores it.
func (c *Client) Claim(ctx context.Context, claimToken string) error {
	q := url.Values{"token": {claimToken}}
	body, err := c.do(ctx, http.MethodPost, "/api/claim/exchange?"+q.Encode(), "")
	if err != nil {
		return fmt.Errorf("claim exchange: %w", err)
	}
	tok, user := parseUser(body)
	if tok == "" {
		return fmt.Errorf("claim exchange: no token in response: %.200s", body)
	}
	c.mu.Lock()
	c.st = state{AuthToken: tok, Username: user}
	err = c.saveLocked()
	c.mu.Unlock()
	if err != nil {
		return err
	}
	slog.Info("claimed by plex.tv account", "username", user)
	return nil
}

// parseUser reads the token and username from a plex.tv user response,
// which may be JSON or XML depending on the endpoint.
func parseUser(body []byte) (token, username string) {
	var j struct {
		AuthToken           string `json:"authToken"`
		AuthenticationToken string `json:"authenticationToken"`
		Username            string `json:"username"`
		User                *struct {
			AuthToken           string `json:"authToken"`
			AuthenticationToken string `json:"authenticationToken"`
			Username            string `json:"username"`
		} `json:"user"`
	}
	if json.Unmarshal(body, &j) == nil {
		if j.User != nil {
			return first(j.User.AuthToken, j.User.AuthenticationToken), j.User.Username
		}
		if t := first(j.AuthToken, j.AuthenticationToken); t != "" {
			return t, j.Username
		}
	}
	var x struct {
		AuthToken           string `xml:"authToken,attr"`
		AuthenticationToken string `xml:"authenticationToken,attr"`
		Username            string `xml:"username,attr"`
	}
	if xml.Unmarshal(body, &x) == nil {
		return first(x.AuthToken, x.AuthenticationToken), x.Username
	}
	return "", ""
}

func first(s ...string) string {
	for _, v := range s {
		if v != "" {
			return v
		}
	}
	return ""
}

// Publish tells plex.tv which URLs jellex can be reached at.
func (c *Client) Publish(ctx context.Context, urls []string) error {
	if c.token() == "" {
		return errors.New("not claimed")
	}
	q := url.Values{}
	for _, u := range urls {
		q.Add("Connection[][uri]", u)
	}
	if _, err := c.do(ctx, http.MethodPut, "/devices/"+url.PathEscape(c.id.MachineID)+"?"+q.Encode(), c.token()); err != nil {
		return fmt.Errorf("publish connections: %w", err)
	}
	return nil
}

// PublishLoop publishes now and then periodically, like PMS refreshing its
// device connections, until ctx ends. It does nothing while unclaimed.
func (c *Client) PublishLoop(ctx context.Context, urls []string, every time.Duration) {
	t := time.NewTicker(every)
	defer t.Stop()
	for {
		if c.token() != "" {
			if err := c.Publish(ctx, urls); err != nil {
				slog.Warn("plex.tv publish failed", "err", err)
			} else {
				slog.Info("published connections to plex.tv", "urls", urls)
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (c *Client) do(ctx context.Context, method, path, token string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	h := req.Header
	h.Set("Accept", "application/json")
	h.Set("X-Plex-Client-Identifier", c.id.MachineID)
	h.Set("X-Plex-Product", "Plex Media Server")
	h.Set("X-Plex-Version", c.id.Version)
	h.Set("X-Plex-Platform", platform())
	h.Set("X-Plex-Platform-Version", runtime.Version())
	h.Set("X-Plex-Device", "PC")
	h.Set("X-Plex-Device-Name", c.id.Name)
	h.Set("X-Plex-Provides", "server")
	if token != "" {
		h.Set("X-Plex-Token", token)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode/100 != 2 {
		return body, fmt.Errorf("%s %s: %s: %.300s", method, strings.SplitN(path, "?", 2)[0], resp.Status, body)
	}
	return body, nil
}

func (c *Client) saveLocked() error {
	b, err := json.Marshal(c.st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.path)
}

func platform() string {
	switch runtime.GOOS {
	case "linux":
		return "Linux"
	case "darwin":
		return "MacOSX"
	case "windows":
		return "Windows"
	}
	return runtime.GOOS
}
