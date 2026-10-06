package plex

import (
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
)

// Stream selection: Plex clients pick audio and subtitle tracks per part
// with PUT /library/parts/{id}, and expect later responses to mark those
// streams selected. Jellyfin has no per-item track memory, so jellex keeps
// selections in its database (stream_selections).

type trackChoice struct {
	audio, subtitle int // stream IDs; 0 means not chosen; subtitle -1 means explicitly off
}

func (s *Server) selection(part int) (trackChoice, bool) {
	var c trackChoice
	err := s.db.QueryRow("SELECT audio, subtitle FROM stream_selections WHERE part_id = ?", part).
		Scan(&c.audio, &c.subtitle)
	if err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			slog.Error("load stream selection", "part", part, "err", err)
		}
		return trackChoice{}, false
	}
	return c, true
}

func (s *Server) streamRoutes() {
	s.mux.HandleFunc("PUT /library/parts/{id}", s.handleSelectStreams)
	s.mux.HandleFunc("GET /library/streams/{id}", s.handleStreamFile)
	s.mux.HandleFunc("GET /subtitles/:/transcode/universal/start", s.handleSidecarSubtitles)
}

func (s *Server) handleSelectStreams(w http.ResponseWriter, r *http.Request) {
	part, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	if _, ok := s.ids.GUID(part); !ok {
		http.NotFound(w, r)
		return
	}
	q := r.URL.Query()
	c, _ := s.selection(part)
	if v, err := strconv.Atoi(q.Get("audioStreamID")); err == nil {
		c.audio = v
	}
	if v, err := strconv.Atoi(q.Get("subtitleStreamID")); err == nil {
		if v == 0 {
			v = -1
		}
		c.subtitle = v
	}
	if _, err := s.db.Exec("INSERT OR REPLACE INTO stream_selections (part_id, audio, subtitle) VALUES (?, ?, ?)",
		part, c.audio, c.subtitle); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// applySelection marks the chosen audio and subtitle streams of a Part as
// selected, overriding the file's defaults.
func (s *Server) applySelection(part *Element, partID int) {
	c, ok := s.selection(partID)
	if !ok {
		return
	}
	for _, st := range part.Children {
		id, _ := st.Get("id").(int)
		switch st.Get("streamType") {
		case 2:
			if c.audio != 0 {
				setSelected(st, id == c.audio)
			}
		case 3:
			if c.subtitle != 0 {
				setSelected(st, id == c.subtitle)
			}
		}
	}
}

func setSelected(e *Element, on bool) {
	if on {
		e.A("selected", true)
		return
	}
	attrs := e.Attrs[:0]
	for _, a := range e.Attrs {
		if a.Name != "selected" {
			attrs = append(attrs, a)
		}
	}
	e.Attrs = attrs
}

// subtitleCodecs maps Jellyfin subtitle codec names to Plex's.
var subtitleCodecs = map[string]string{
	"subrip": "srt", "srt": "srt", "webvtt": "vtt", "vtt": "vtt",
	"ass": "ass", "ssa": "ssa", "mov_text": "mov_text",
	"pgssub": "pgs", "hdmv_pgs_subtitle": "pgs", "dvdsub": "vobsub", "dvd_subtitle": "vobsub",
}

// textSubtitle reports whether Jellyfin can deliver a subtitle codec as text.
func textSubtitle(codec string) bool {
	switch codec {
	case "srt", "vtt", "ass", "ssa", "mov_text":
		return true
	}
	return false
}

// handleStreamFile serves a subtitle stream as a file. Plex clients that
// direct play render text subtitles themselves from this. The format can be
// chosen with ?format=srt|vtt|ass; by default it's SRT.
func (s *Server) handleStreamFile(w http.ResponseWriter, r *http.Request) {
	n, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	s.serveSubtitle(w, r, n, r.URL.Query().Get("format"))
}

// handleSidecarSubtitles serves the selected subtitle track of an item being
// direct played. Plex Web renders it over the video with libjass, which
// parses this stream as ASS. The item and part come from the same parameters
// as a transcode decision.
func (s *Server) handleSidecarSubtitles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	m := uriKey.FindStringSubmatch(q.Get("path"))
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
	mediaIndex, _ := strconv.Atoi(q.Get("mediaIndex"))
	md := s.metadata(&res.Items[0], nil, true)
	var media []*Element
	for _, c := range md.Children {
		if c.Tag == "Media" {
			media = append(media, c)
		}
	}
	if mediaIndex >= len(media) {
		http.NotFound(w, r)
		return
	}
	for _, p := range media[mediaIndex].Children {
		for _, st := range p.Children {
			if st.Get("streamType") == 3 && st.Get("selected") == true && st.Get("key") != nil {
				s.serveSubtitle(w, r, st.Get("id").(int), "ass")
				return
			}
		}
	}
	http.NotFound(w, r)
}

func (s *Server) serveSubtitle(w http.ResponseWriter, r *http.Request, n int, format string) {
	key, ok := s.ids.GUID(n)
	// stream:part:{item}:{source}:{index}
	rest, isStream := strings.CutPrefix(key, "stream:part:")
	parts := strings.Split(rest, ":")
	if !ok || !isStream || len(parts) != 3 {
		http.NotFound(w, r)
		return
	}
	item, source, index := parts[0], parts[1], parts[2]
	switch format {
	case "vtt", "ass", "srt":
	default:
		format = "srt"
	}
	u := fmt.Sprintf("%s/Videos/%s/%s/Subtitles/%s/0/Stream.%s",
		strings.TrimRight(s.cfg.JellyfinURL, "/"), item, source, index, format)
	req, err := http.NewRequestWithContext(r.Context(), http.MethodGet, u, nil)
	if err != nil {
		fail(w, r, err)
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.cfg.JellyfinAPIKey))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		fail(w, r, err)
		return
	}
	defer resp.Body.Close()
	ct := map[string]string{"srt": "application/x-subrip", "vtt": "text/vtt", "ass": "text/x-ssa"}[format]
	w.Header().Set("Content-Type", ct+"; charset=utf-8")
	if format != "ass" || resp.StatusCode != http.StatusOK {
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
		return
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Write(withPlayRes(b))
}

// withPlayRes adds the ASS default script resolution when a script has none.
// Jellyfin's converted subtitles omit it, and without it libjass (Plex Web's
// renderer) can't scale or place text: it comes out tiny in a corner.
func withPlayRes(ass []byte) []byte {
	s := string(ass)
	if strings.Contains(s, "PlayResX") || !strings.Contains(s, "[Script Info]") {
		return ass
	}
	return []byte(strings.Replace(s, "[Script Info]", "[Script Info]\nPlayResX: 384\nPlayResY: 288", 1))
}
