package plex

import (
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

func (s *Server) playbackRoutes() {
	m := s.mux
	m.HandleFunc("GET /video/:/transcode/universal/decision", s.handleDecision)
	m.HandleFunc("GET /music/:/transcode/universal/decision", s.handleDecision)
	m.HandleFunc("GET /library/parts/{id}/{ts}/{file}", s.handlePart)
	m.HandleFunc("HEAD /library/parts/{id}/{ts}/{file}", s.handlePart)
	m.HandleFunc("GET /:/timeline", s.handleTimeline)
	m.HandleFunc("POST /:/timeline", s.handleTimeline)
	m.HandleFunc("GET /:/scrobble", s.handleScrobble(true))
	m.HandleFunc("PUT /:/scrobble", s.handleScrobble(true))
	m.HandleFunc("GET /:/unscrobble", s.handleScrobble(false))
	m.HandleFunc("PUT /:/unscrobble", s.handleScrobble(false))
	m.HandleFunc("PUT /:/rate", s.handleRate)
	m.HandleFunc("PUT /actions/removeFromContinueWatching", s.handleRemoveFromContinueWatching)
}

// handleDecision tells the client how to play an item: direct play of the
// original file when the client offers to, otherwise a DASH transcode.
func (s *Server) handleDecision(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	m := uriKey.FindStringSubmatch(r.URL.Query().Get("path"))
	if m == nil {
		http.Error(w, "bad path", http.StatusBadRequest)
		return
	}
	n, _ := strconv.Atoi(m[1])
	guid, ok := s.itemGUID(n)
	if !ok {
		http.NotFound(w, r)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Ids([]string{guid}).Fields(detailFields).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	if len(res.Items) == 0 {
		http.NotFound(w, r)
		return
	}
	sec, err := s.sectionFor(ctx, guid)
	if err != nil {
		fail(w, r, err)
		return
	}
	md := s.metadata(&res.Items[0], sec, true)
	mediaIndex, _ := strconv.Atoi(r.URL.Query().Get("mediaIndex"))
	// The client asks with directPlay=0 when it has decided it can't play
	// the file as is (codec, container, or a non-default audio track).
	if r.URL.Query().Get("directPlay") == "0" {
		write(w, r, s.transcodeDecision(md, mediaIndex))
		return
	}
	i := 0
	for _, c := range md.Children {
		if c.Tag != "Media" {
			continue
		}
		if i == mediaIndex {
			c.A("selected", true)
			for _, p := range c.Children {
				if p.Tag == "Part" {
					p.A("decision", "directplay").A("selected", true)
				}
			}
		}
		i++
	}
	write(w, r, Container().
		A("allowSync", false).
		A("directPlayDecisionCode", 1000).
		A("directPlayDecisionText", "Direct play OK.").
		A("generalDecisionCode", 1000).
		A("generalDecisionText", "Direct play OK.").
		A("mdeDecisionCode", 1000).
		A("mdeDecisionText", "Direct play OK.").
		A("identifier", "com.plexapp.plugins.library").
		Add(md))
}

var audioExts = map[string]bool{
	"mp3": true, "flac": true, "m4a": true, "aac": true, "ogg": true, "oga": true,
	"opus": true, "wav": true, "wma": true, "alac": true, "aiff": true, "ape": true,
}

// handlePart streams a media file from Jellyfin, passing Range requests
// through so clients can seek.
func (s *Server) handlePart(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	key, ok := s.ids.GUID(n)
	item, source, found := strings.Cut(strings.TrimPrefix(key, "part:"), ":")
	if !ok || !strings.HasPrefix(key, "part:") || !found {
		http.NotFound(w, r)
		return
	}
	kind := "Videos"
	if audioExts[strings.TrimPrefix(path.Ext(r.PathValue("file")), ".")] {
		kind = "Audio"
	}
	u := fmt.Sprintf("%s/%s/%s/stream?static=true&mediaSourceId=%s", strings.TrimRight(s.cfg.JellyfinURL, "/"), kind, item, source)
	req, err := http.NewRequestWithContext(r.Context(), r.Method, u, nil)
	if err != nil {
		fail(w, r, err)
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.cfg.JellyfinAPIKey))
	for _, h := range []string{"Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if v := r.Header.Get(h); v != "" {
			req.Header.Set(h, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail(w, r, err)
		return
	}
	defer resp.Body.Close()
	for _, h := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

// handleTimeline records playback progress the client reports while playing.
func (s *Server) handleTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	n, _ := strconv.Atoi(q.Get("ratingKey"))
	guid, ok := s.itemGUID(n)
	if !ok {
		write(w, r, Container())
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	pos, _ := strconv.ParseInt(q.Get("time"), 10, 64)
	dur, _ := strconv.ParseInt(q.Get("duration"), 10, 64)
	state := q.Get("state")
	s.sessions.notifyPlaying(s.sessions.update(r, guid, n, state, pos, dur))

	// Like PMS, treat an item as watched once 90% of it has played.
	if state == "stopped" && dur > 0 && pos >= dur*9/10 {
		if _, _, err := s.jf.UserDataAPI.MarkPlayedItem(ctx, guid).UserId(uid).DatePlayed(time.Now()).Execute(); err != nil {
			slog.Warn("mark played", "item", guid, "err", err)
		}
		pos = 0
	}
	if state != "buffering" {
		var ud jellyfin.UpdateUserItemDataDto
		ud.SetPlaybackPositionTicks(pos * ticksPerMs)
		ud.SetLastPlayedDate(time.Now())
		if _, _, err := s.jf.UserDataAPI.UpdateItemUserData(ctx, guid).UserId(uid).UpdateUserItemDataDto(ud).Execute(); err != nil {
			slog.Warn("update progress", "item", guid, "err", err)
		}
	}
	write(w, r, Container())
}

func (s *Server) handleScrobble(played bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		n, _ := strconv.Atoi(r.URL.Query().Get("key"))
		guid, ok := s.itemGUID(n)
		if !ok {
			http.NotFound(w, r)
			return
		}
		uid, err := s.user(ctx)
		if err != nil {
			fail(w, r, err)
			return
		}
		if played {
			_, _, err = s.jf.UserDataAPI.MarkPlayedItem(ctx, guid).UserId(uid).DatePlayed(time.Now()).Execute()
		} else {
			_, _, err = s.jf.UserDataAPI.MarkUnplayedItem(ctx, guid).UserId(uid).Execute()
		}
		if err != nil {
			fail(w, r, err)
			return
		}
		w.WriteHeader(http.StatusOK)
	}
}

// handleRate stores a user's star rating. Plex uses 0-10 (two per star);
// Jellyfin's user rating uses the same scale.
func (s *Server) handleRate(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	n, _ := strconv.Atoi(q.Get("key"))
	guid, ok := s.itemGUID(n)
	if !ok {
		http.NotFound(w, r)
		return
	}
	rating, err := strconv.ParseFloat(q.Get("rating"), 64)
	if err != nil {
		http.Error(w, "bad rating", http.StatusBadRequest)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	var ud jellyfin.UpdateUserItemDataDto
	if rating < 0 {
		ud.SetRatingNil()
	} else {
		ud.SetRating(rating)
	}
	if _, _, err := s.jf.UserDataAPI.UpdateItemUserData(ctx, guid).UserId(uid).UpdateUserItemDataDto(ud).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleRemoveFromContinueWatching clears an item's saved position, which
// takes it off Jellyfin's resume list. Next-up episodes that were never
// started can't be hidden this way; Jellyfin has no equivalent.
func (s *Server) handleRemoveFromContinueWatching(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, _ := strconv.Atoi(r.URL.Query().Get("ratingKey"))
	guid, ok := s.itemGUID(n)
	if !ok {
		http.NotFound(w, r)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	var ud jellyfin.UpdateUserItemDataDto
	ud.SetPlaybackPositionTicks(0)
	if _, _, err := s.jf.UserDataAPI.UpdateItemUserData(ctx, guid).UserId(uid).UpdateUserItemDataDto(ud).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}
