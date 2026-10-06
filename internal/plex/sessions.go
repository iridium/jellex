package plex

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/coder/websocket"
	jellyfin "github.com/sj14/jellyfin-go/api"
)

// Now-playing sessions are built from the timeline reports players send
// while playing. They back /status/sessions (the dashboard) and the
// "playing" events pushed over the notifications websocket.

// sessionIdle is how long a session lingers without a timeline report.
const sessionIdle = 90 * time.Second

type playSession struct {
	Key        int // sessionKey
	Client     string
	Product    string
	Platform   string
	Device     string
	Address    string
	Item       string // Jellyfin GUID
	RatingKey  int
	State      string
	OffsetMs   int64
	DurationMs int64
	Seen       time.Time
}

type sessionTracker struct {
	mu   sync.Mutex
	next int
	by   map[string]*playSession // by client identifier

	subMu sync.Mutex
	subs  map[chan []byte]struct{}
}

// update records a timeline report and returns the session.
func (t *sessionTracker) update(r *http.Request, guid string, rk int, state string, offset, duration int64) playSession {
	client := plexParam(r, "X-Plex-Client-Identifier")
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.by == nil {
		t.by = map[string]*playSession{}
	}
	ps, ok := t.by[client]
	if !ok || ps.Item != guid {
		t.next++
		ps = &playSession{Key: t.next, Client: client}
		t.by[client] = ps
	}
	ps.Product = plexParam(r, "X-Plex-Product")
	ps.Platform = plexParam(r, "X-Plex-Platform")
	ps.Device = plexParam(r, "X-Plex-Device-Name")
	ps.Address = r.RemoteAddr
	ps.Item, ps.RatingKey, ps.State, ps.OffsetMs, ps.DurationMs, ps.Seen = guid, rk, state, offset, duration, time.Now()
	out := *ps
	if state == "stopped" {
		delete(t.by, client)
	}
	return out
}

// active returns live sessions, dropping idle ones.
func (t *sessionTracker) active() []playSession {
	t.mu.Lock()
	defer t.mu.Unlock()
	var out []playSession
	for k, ps := range t.by {
		if time.Since(ps.Seen) > sessionIdle {
			delete(t.by, k)
			continue
		}
		out = append(out, *ps)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out
}

// plexParam reads an X-Plex-* value from the headers or the query string,
// where Plex Web puts them.
func plexParam(r *http.Request, k string) string {
	if v := r.Header.Get(k); v != "" {
		return v
	}
	return r.URL.Query().Get(k)
}

func (t *sessionTracker) subscribe() chan []byte {
	ch := make(chan []byte, 16)
	t.subMu.Lock()
	if t.subs == nil {
		t.subs = map[chan []byte]struct{}{}
	}
	t.subs[ch] = struct{}{}
	t.subMu.Unlock()
	return ch
}

func (t *sessionTracker) unsubscribe(ch chan []byte) {
	t.subMu.Lock()
	delete(t.subs, ch)
	t.subMu.Unlock()
}

// broadcast sends a notification to every websocket listener, dropping it
// for listeners that are too far behind.
func (t *sessionTracker) broadcast(msg []byte) {
	t.subMu.Lock()
	defer t.subMu.Unlock()
	for ch := range t.subs {
		select {
		case ch <- msg:
		default:
		}
	}
}

// notifyPlaying pushes a PlaySessionStateNotification, which Plex clients
// use to refresh progress and the dashboard.
func (t *sessionTracker) notifyPlaying(ps playSession) {
	msg, err := json.Marshal(map[string]any{"NotificationContainer": map[string]any{
		"type": "playing",
		"size": 1,
		"PlaySessionStateNotification": []map[string]any{{
			"sessionKey":       fmt.Sprint(ps.Key),
			"clientIdentifier": ps.Client,
			"guid":             "",
			"ratingKey":        fmt.Sprint(ps.RatingKey),
			"url":              "",
			"key":              fmt.Sprintf("/library/metadata/%d", ps.RatingKey),
			"viewOffset":       ps.OffsetMs,
			"playQueueItemID":  0,
			"state":            ps.State,
		}},
	}})
	if err == nil {
		t.broadcast(msg)
	}
}

func (s *Server) sessionRoutes() {
	s.mux.HandleFunc("GET /status/sessions", s.handleSessions)
	s.mux.HandleFunc("GET /accounts", s.handleAccounts)
	s.mux.HandleFunc("GET /accounts/{id}", s.handleAccounts)
}

func (s *Server) handleSessions(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sessions := s.sessions.active()
	mc := Container()
	if len(sessions) == 0 {
		write(w, r, mc)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	var guids []string
	for _, ps := range sessions {
		guids = append(guids, ps.Item)
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Ids(guids).Fields(detailFields).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	byID := map[string]*jellyfin.BaseItemDto{}
	for i := range res.Items {
		byID[res.Items[i].GetId()] = &res.Items[i]
	}
	user := s.accountName(ctx)
	for _, ps := range sessions {
		it, ok := byID[ps.Item]
		if !ok {
			continue
		}
		md := s.metadata(it, nil, true).
			A("sessionKey", fmt.Sprint(ps.Key)).
			A("viewOffset", ps.OffsetMs)
		single := func(e *Element) *Element { e.Single = true; return e }
		md.Add(
			single(E("User").A("id", "1").A("title", user).A("thumb", "")),
			single(E("Player").
				A("address", ps.Address).
				A("machineIdentifier", ps.Client).
				A("platform", ps.Platform).
				A("product", ps.Product).
				A("title", ps.Device).
				A("state", ps.State).
				A("local", true).
				A("relayed", false).
				A("secure", false)),
			single(E("Session").A("id", ps.Client).A("bandwidth", 0).A("location", "lan")),
		)
		mc.Add(md)
	}
	write(w, r, mc)
}

// accountName is the name shown for the single local account: the plex.tv
// username when claimed, otherwise the Jellyfin user's name.
func (s *Server) accountName(ctx context.Context) string {
	if name, ok := s.myplex.Claimed(); ok && name != "" {
		return name
	}
	uid, err := s.user(ctx)
	if err != nil {
		return "jellex"
	}
	if u, _, err := s.jf.UserAPI.GetUserById(ctx, uid).Execute(); err == nil {
		return u.GetName()
	}
	return "jellex"
}

// handleAccounts lists the server's local accounts. jellex has one, the
// account every client acts as, with ID 1 like the PMS owner account.
func (s *Server) handleAccounts(w http.ResponseWriter, r *http.Request) {
	if id := r.PathValue("id"); id != "" {
		if n, err := strconv.Atoi(id); err != nil || n != 1 {
			http.NotFound(w, r)
			return
		}
	}
	write(w, r, Container().A("identifier", "com.plexapp.system.accounts").Add(
		E("Account").
			A("id", 1).
			A("key", "/accounts/1").
			A("name", s.accountName(r.Context())).
			A("defaultAudioLanguage", "en").
			A("autoSelectAudio", true).
			A("defaultSubtitleLanguage", "en").
			A("subtitleMode", 0).
			A("thumb", ""),
	))
}

// pushNotifications forwards broadcast notifications to one websocket until
// it closes, pinging it to keep it alive.
func (s *Server) pushNotifications(ctx context.Context, c *websocket.Conn) {
	ch := s.sessions.subscribe()
	defer s.sessions.unsubscribe(ch)
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case msg := <-ch:
			wctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Write(wctx, websocket.MessageText, msg)
			cancel()
			if err != nil {
				slog.Debug("websocket write", "err", err)
				return
			}
		case <-t.C:
			pctx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
