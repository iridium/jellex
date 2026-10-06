// Package auth signs browsers in to jellex with their Jellyfin account and
// keeps their sessions. Plex Web is served from jellex's own origin, so a
// session cookie set at login rides along on every request it makes.
package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// CookieName is the session cookie.
const CookieName = "jellex_session"

// sessionTTL is how long a session lasts without being used.
const sessionTTL = 30 * 24 * time.Hour

// Session is a signed-in browser.
type Session struct {
	UserID   string
	UserName string
	Token    string // the user's own Jellyfin access token
	DeviceID string
	ServerID string // the Jellyfin server it belongs to
	Created  time.Time
	Seen     time.Time
}

// Store holds sessions, persisted so restarts don't sign everyone out.
type Store struct {
	db          *sql.DB
	jellyfinURL string
	serverID    string
	http        *http.Client
}

// Open opens the sessions for the Jellyfin server with the given ID.
// Sessions from another server (jellex was pointed elsewhere) don't count.
func Open(db *sql.DB, jellyfinURL, serverID string) (*Store, error) {
	s := &Store{
		db:          db,
		jellyfinURL: strings.TrimRight(jellyfinURL, "/"),
		serverID:    serverID,
		http:        &http.Client{Timeout: 15 * time.Second},
	}
	// Forget sessions nobody will be able to use again.
	_, err := db.Exec("DELETE FROM sessions WHERE seen < ? OR server_id != ?",
		time.Now().Add(-sessionTTL).Unix(), serverID)
	return s, err
}

func (s *Store) insert(id string, ses Session) error {
	_, err := s.db.Exec(`INSERT OR REPLACE INTO sessions
		(id, user_id, user_name, token, device_id, server_id, created, seen)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		id, ses.UserID, ses.UserName, ses.Token, ses.DeviceID, ses.ServerID, ses.Created.Unix(), ses.Seen.Unix())
	return err
}

// Get returns the session for a session ID, if it's live.
func (s *Store) Get(id string) (Session, bool) {
	var ses Session
	var created, seen int64
	err := s.db.QueryRow(`SELECT user_id, user_name, token, device_id, server_id, created, seen
		FROM sessions WHERE id = ?`, id).
		Scan(&ses.UserID, &ses.UserName, &ses.Token, &ses.DeviceID, &ses.ServerID, &created, &seen)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("load session", "err", err)
		}
		return Session{}, false
	}
	ses.Created, ses.Seen = time.Unix(created, 0), time.Unix(seen, 0)
	if time.Since(ses.Seen) > sessionTTL || ses.ServerID != s.serverID {
		s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
		return Session{}, false
	}
	// Only record "seen" occasionally; it's for expiry, not auditing.
	if time.Since(ses.Seen) > time.Hour {
		ses.Seen = time.Now()
		s.db.Exec("UPDATE sessions SET seen = ? WHERE id = ?", ses.Seen.Unix(), id)
	}
	return ses, true
}

// FromRequest returns the session named by a request's cookie.
func (s *Store) FromRequest(r *http.Request) (string, Session, bool) {
	c, err := r.Cookie(CookieName)
	if err != nil || c.Value == "" {
		return "", Session{}, false
	}
	ses, ok := s.Get(c.Value)
	return c.Value, ses, ok
}

// Start records a new session and returns its ID.
func (s *Store) Start(ses Session) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	id := hex.EncodeToString(b)
	ses.Created, ses.Seen = time.Now(), time.Now()
	ses.ServerID = s.serverID
	return id, s.insert(id, ses)
}

// End removes a session and revokes its Jellyfin token.
func (s *Store) End(ctx context.Context, id string) {
	ses, ok := s.Get(id)
	s.db.Exec("DELETE FROM sessions WHERE id = ?", id)
	if ok {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.jellyfinURL+"/Sessions/Logout", nil)
		if err == nil {
			req.Header.Set("Authorization", authHeader(ses.DeviceID, ses.Token))
			if resp, err := s.http.Do(req); err == nil {
				resp.Body.Close()
			}
		}
	}
}

// authHeader is Jellyfin's client authorization header. Each browser login
// is its own Jellyfin device, so sessions show up separately in Jellyfin.
func authHeader(deviceID, token string) string {
	h := fmt.Sprintf(`MediaBrowser Client="jellex", Device="Plex Web", DeviceId="%s", Version="1"`, deviceID)
	if token != "" {
		h += fmt.Sprintf(`, Token="%s"`, token)
	}
	return h
}

// NewDeviceID returns a random Jellyfin device ID for a login.
func NewDeviceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return "jellex-" + hex.EncodeToString(b)
}

// ErrBadCredentials is returned when Jellyfin rejects a login.
var ErrBadCredentials = errors.New("wrong username or password")

type authResult struct {
	AccessToken string `json:"AccessToken"`
	User        struct {
		ID   string `json:"Id"`
		Name string `json:"Name"`
	} `json:"User"`
}

func (r authResult) session(deviceID string) Session {
	return Session{UserID: r.User.ID, UserName: r.User.Name, Token: r.AccessToken, DeviceID: deviceID}
}

// Password signs in to Jellyfin with a username and password.
func (s *Store) Password(ctx context.Context, user, password string) (Session, error) {
	device := NewDeviceID()
	body, _ := json.Marshal(map[string]string{"Username": user, "Pw": password})
	var res authResult
	status, err := s.post(ctx, "/Users/AuthenticateByName", device, body, &res)
	if status == http.StatusUnauthorized {
		return Session{}, ErrBadCredentials
	}
	if err != nil {
		return Session{}, err
	}
	return res.session(device), nil
}

// QuickConnect is a Quick Connect sign-in in progress: the user approves
// Code from a Jellyfin app they're already signed in to.
type QuickConnect struct {
	Code     string
	Secret   string
	DeviceID string
}

// ErrQuickConnectDisabled is returned when the server has Quick Connect off.
var ErrQuickConnectDisabled = errors.New("quick connect is disabled on this Jellyfin server")

// StartQuickConnect begins a Quick Connect sign-in.
func (s *Store) StartQuickConnect(ctx context.Context) (QuickConnect, error) {
	device := NewDeviceID()
	var res struct {
		Code   string `json:"Code"`
		Secret string `json:"Secret"`
	}
	status, err := s.post(ctx, "/QuickConnect/Initiate", device, nil, &res)
	if status == http.StatusUnauthorized || status == http.StatusForbidden {
		return QuickConnect{}, ErrQuickConnectDisabled
	}
	if err != nil {
		return QuickConnect{}, err
	}
	return QuickConnect{Code: res.Code, Secret: res.Secret, DeviceID: device}, nil
}

// PollQuickConnect checks a Quick Connect sign-in, returning a session once
// the user has approved it.
func (s *Store) PollQuickConnect(ctx context.Context, qc QuickConnect) (Session, bool, error) {
	var state struct {
		Authenticated bool `json:"Authenticated"`
	}
	if _, err := s.get(ctx, "/QuickConnect/Connect?secret="+url.QueryEscape(qc.Secret), qc.DeviceID, &state); err != nil {
		return Session{}, false, err
	}
	if !state.Authenticated {
		return Session{}, false, nil
	}
	body, _ := json.Marshal(map[string]string{"Secret": qc.Secret})
	var res authResult
	if _, err := s.post(ctx, "/Users/AuthenticateWithQuickConnect", qc.DeviceID, body, &res); err != nil {
		return Session{}, false, err
	}
	return res.session(qc.DeviceID), true, nil
}

func (s *Store) post(ctx context.Context, path, device string, body []byte, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.jellyfinURL+path, bytes.NewReader(body))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	return s.do(req, device, out)
}

func (s *Store) get(ctx context.Context, path, device string, out any) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.jellyfinURL+path, nil)
	if err != nil {
		return 0, err
	}
	return s.do(req, device, out)
}

func (s *Store) do(req *http.Request, device string, out any) (int, error) {
	req.Header.Set("Authorization", authHeader(device, ""))
	resp, err := s.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode/100 != 2 {
		return resp.StatusCode, fmt.Errorf("%s %s: %s: %.200s", req.Method, req.URL.Path, resp.Status, b)
	}
	if out != nil {
		if err := json.Unmarshal(b, out); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}
