// Package plex serves an emulation of the Plex Media Server API backed by Jellyfin.
package plex

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/iridium/jellex/internal/config"
	"github.com/iridium/jellex/internal/ids"
	"github.com/iridium/jellex/internal/myplex"
	"github.com/iridium/jellex/internal/webui"
	jellyfin "github.com/sj14/jellyfin-go/api"
)

// Version is the Plex Media Server version jellex reports to clients.
const Version = "1.43.4.10903-jellex"

type Server struct {
	cfg        config.Config
	jf         *jellyfin.APIClient
	ids        *ids.Map
	mux        *http.ServeMux
	started    time.Time
	queues     playQueues
	sheets     sheetCache
	picks      selections
	sessions   sessionTracker
	transcodes transcodeSessions
	myplex     *myplex.Client

	userOnce sync.Mutex
	userID   string
}

func NewServer(ctx context.Context, cfg config.Config, jf *jellyfin.APIClient, mp *myplex.Client) (*Server, error) {
	m, err := ids.Open(filepath.Join(cfg.DataDir, "ids.json"))
	if err != nil {
		return nil, fmt.Errorf("open id map: %w", err)
	}
	s := &Server{cfg: cfg, jf: jf, ids: m, mux: http.NewServeMux(), started: time.Now(), myplex: mp}
	s.routes()
	go s.reapTranscodes(ctx)
	s.mux.Handle("GET /web/", http.StripPrefix("/web", webui.Handler(ctx, cfg.WebDir, !cfg.DisableCustomAssets)))
	if !cfg.DisableCustomAssets {
		s.mux.HandleFunc("GET /favicon.ico", webui.ServeFavicon)
	}
	return s, nil
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Plex Web probes the server from other origins (e.g. its loopback
	// address), so allow cross-origin requests like PMS does.
	if origin := r.Header.Get("Origin"); origin != "" {
		h := w.Header()
		h.Set("Access-Control-Allow-Origin", origin)
		h.Set("Access-Control-Allow-Credentials", "true")
		h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS, HEAD")
		h.Set("Access-Control-Max-Age", "86400")
		if reqHeaders := r.Header.Get("Access-Control-Request-Headers"); reqHeaders != "" {
			h.Set("Access-Control-Allow-Headers", reqHeaders)
		}
		h.Set("Access-Control-Expose-Headers", "Location, Date")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
	}
	_, pattern := s.mux.Handler(r)
	if pattern == "" {
		slog.Info("unhandled", "method", r.Method, "path", r.URL.Path, "query", stripPlexParams(r.URL.RawQuery))
	} else {
		slog.Debug("request", "method", r.Method, "path", r.URL.Path)
	}
	s.mux.ServeHTTP(w, r)
}

// stripPlexParams drops the X-Plex-* client headers that clients repeat as
// query parameters on every request, so logs show what actually matters.
func stripPlexParams(q string) string {
	var keep []string
	for _, p := range strings.Split(q, "&") {
		if p != "" && !strings.HasPrefix(p, "X-Plex-") {
			keep = append(keep, p)
		}
	}
	return strings.Join(keep, "&")
}

// user returns the Jellyfin user ID that all requests act as. It is resolved
// lazily so jellex can start before Jellyfin is up.
func (s *Server) user(ctx context.Context) (string, error) {
	s.userOnce.Lock()
	defer s.userOnce.Unlock()
	if s.userID != "" {
		return s.userID, nil
	}
	users, _, err := s.jf.UserAPI.GetUsers(ctx).Execute()
	if err != nil {
		return "", fmt.Errorf("list jellyfin users: %w", err)
	}
	for _, u := range users {
		if s.cfg.JellyfinUser != "" {
			if strings.EqualFold(u.GetName(), s.cfg.JellyfinUser) {
				s.userID = u.GetId()
			}
			continue
		}
		if p := u.GetPolicy(); p.GetIsAdministrator() {
			s.userID = u.GetId()
			break
		}
	}
	if s.userID == "" {
		return "", fmt.Errorf("jellyfin user %q not found", s.cfg.JellyfinUser)
	}
	return s.userID, nil
}

// fail logs err and reports it to the client as a 500. Requests the client
// abandoned (it navigated away) aren't errors and are only logged at debug.
func fail(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, context.Canceled) || r.Context().Err() != nil {
		slog.Debug("request canceled", "path", r.URL.Path, "err", err)
		return
	}
	slog.Error("request failed", "path", r.URL.Path, "err", err)
	http.Error(w, err.Error(), http.StatusInternalServerError)
}
