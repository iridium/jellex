package plex

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Transcoding bridges Plex's transcoder API onto Jellyfin's HLS transcoder.
// Plex Web asks for /video/:/transcode/universal/start.m3u8 (patched to use
// HLS rather than DASH, see webui/patches/70-hls.patch); jellex starts a
// Jellyfin HLS transcode with fragmented-MP4 segments, reads its playlist,
// serves it with jellex URLs, and passes the segments through.

const transcodeDevice = "jellex"

type transcodeSession struct {
	id       string // Plex session ID, also used as Jellyfin's PlaySessionId
	base     string // Jellyfin /Videos/{item}/ URL the playlist is relative to
	init     string // init segment, relative to base
	segments []string
	durMs    []int64
	codecs   string
	width    int
	height   int
	bitrate  int
	seen     time.Time
}

type transcodeSessions struct {
	mu sync.Mutex
	by map[string]*transcodeSession
}

func (t *transcodeSessions) get(id string) (*transcodeSession, bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	ts, ok := t.by[id]
	if ok {
		ts.seen = time.Now()
	}
	return ts, ok
}

func (t *transcodeSessions) put(ts *transcodeSession) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.by == nil {
		t.by = map[string]*transcodeSession{}
	}
	ts.seen = time.Now()
	t.by[ts.id] = ts
}

func (t *transcodeSessions) remove(id string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	delete(t.by, id)
}

func (t *transcodeSessions) count() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return len(t.by)
}

func (s *Server) transcodeRoutes() {
	m := s.mux
	m.HandleFunc("GET /video/:/transcode/universal/start.m3u8", s.handleTranscodeStart)
	m.HandleFunc("GET /video/:/transcode/universal/session/{sid}/base/{file}", s.handleTranscodeFile)
	m.HandleFunc("GET /video/:/transcode/universal/ping", s.handleTranscodePing)
	m.HandleFunc("GET /video/:/transcode/universal/stop", s.handleTranscodeStop)
}

// jfGet fetches a Jellyfin URL with jellex's credentials.
func (s *Server) jfGet(ctx context.Context, u string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.cfg.JellyfinAPIKey))
	return http.DefaultClient.Do(req)
}

// transcodeTarget resolves the item and stream choices a transcode request
// refers to (by path, mediaIndex and the part's selected streams).
type transcodeTarget struct {
	item, source  string
	audioIndex    int // Jellyfin stream index, -1 for default
	subtitleIndex int // Jellyfin stream index to burn in, -1 for none
}

func (s *Server) resolveTranscode(ctx context.Context, q url.Values) (*transcodeTarget, error) {
	m := uriKey.FindStringSubmatch(q.Get("path"))
	if m == nil {
		return nil, errNotFound
	}
	n, _ := strconv.Atoi(m[1])
	guid, ok := s.itemGUID(n)
	if !ok {
		return nil, errNotFound
	}
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Ids([]string{guid}).Fields(detailFields).Execute()
	if err != nil {
		return nil, err
	}
	if len(res.Items) == 0 {
		return nil, errNotFound
	}
	it := &res.Items[0]
	sources := it.GetMediaSources()
	mediaIndex, _ := strconv.Atoi(q.Get("mediaIndex"))
	if mediaIndex >= len(sources) {
		return nil, errNotFound
	}
	ms := sources[mediaIndex]
	t := &transcodeTarget{item: guid, source: ms.GetId(), audioIndex: -1, subtitleIndex: -1}

	// The selected streams are on the rendered Part; map their Plex stream
	// IDs back to Jellyfin stream indexes.
	md := s.metadata(it, nil, true)
	var media []*Element
	for _, c := range md.Children {
		if c.Tag == "Media" {
			media = append(media, c)
		}
	}
	// "auto" (what Plex Web sends for HLS) leaves it to the server; the
	// decision told the client the selected subtitle is burned in.
	burn := q.Get("subtitles") == "burn" || q.Get("subtitles") == "auto"
	for _, p := range media[mediaIndex].Children {
		for _, st := range p.Children {
			if st.Get("selected") != true {
				continue
			}
			idx, ok := st.Get("index").(int)
			if !ok {
				// External subtitles have no container index; look it up.
				idx = s.streamIndex(st.Get("id"))
			}
			switch st.Get("streamType") {
			case 2:
				t.audioIndex = idx
			case 3:
				if burn {
					t.subtitleIndex = idx
				}
			}
		}
	}
	return t, nil
}

// streamIndex recovers a Jellyfin stream index from a Plex stream ID.
func (s *Server) streamIndex(id any) int {
	n, ok := id.(int)
	if !ok {
		return -1
	}
	key, ok := s.ids.GUID(n)
	if !ok {
		return -1
	}
	i := strings.LastIndexByte(key, ':')
	idx, err := strconv.Atoi(key[i+1:])
	if err != nil {
		return -1
	}
	return idx
}

func (s *Server) handleTranscodeStart(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	sid := q.Get("session")
	if sid == "" {
		http.Error(w, "session is required", http.StatusBadRequest)
		return
	}
	t, err := s.resolveTranscode(ctx, q)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	// A new start for an existing session (e.g. a seek or a track change)
	// replaces the old transcode.
	if _, ok := s.transcodes.get(sid); ok {
		s.stopTranscode(sid)
	}

	p := url.Values{}
	p.Set("MediaSourceId", t.source)
	p.Set("PlaySessionId", sid)
	p.Set("DeviceId", transcodeDevice)
	p.Set("VideoCodec", "h264")
	p.Set("AudioCodec", "aac")
	p.Set("SegmentContainer", "mp4")
	p.Set("TranscodingMaxAudioChannels", "2")
	if kbps, err := strconv.Atoi(q.Get("maxVideoBitrate")); err == nil && kbps > 0 {
		p.Set("VideoBitrate", fmt.Sprint(kbps*1000))
		p.Set("MaxStreamingBitrate", fmt.Sprint(kbps*1000))
	}
	if res := q.Get("videoResolution"); res != "" {
		if wh := strings.SplitN(res, "x", 2); len(wh) == 2 {
			p.Set("MaxWidth", wh[0])
			p.Set("MaxHeight", wh[1])
		}
	}
	if t.audioIndex >= 0 {
		p.Set("AudioStreamIndex", fmt.Sprint(t.audioIndex))
	}
	if t.subtitleIndex >= 0 {
		p.Set("SubtitleStreamIndex", fmt.Sprint(t.subtitleIndex))
		p.Set("SubtitleMethod", "Encode")
	}
	base := fmt.Sprintf("%s/Videos/%s/", strings.TrimRight(s.cfg.JellyfinURL, "/"), t.item)

	ts, err := s.openHLS(ctx, base, "master.m3u8?"+p.Encode())
	if err != nil {
		fail(w, r, fmt.Errorf("start jellyfin transcode: %w", err))
		return
	}
	ts.id = sid
	s.transcodes.put(ts)
	slog.Info("transcode started", "session", sid, "item", t.item, "segments", len(ts.segments),
		"audioIndex", t.audioIndex, "burnSubtitle", t.subtitleIndex)

	w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
	w.Write(ts.masterPlaylist())
}

// masterPlaylist is the HLS playlist start.m3u8 returns: one variant, the
// session's media playlist.
func (ts *transcodeSession) masterPlaylist() []byte {
	var b strings.Builder
	b.WriteString("#EXTM3U\n#EXT-X-VERSION:7\n")
	fmt.Fprintf(&b, "#EXT-X-STREAM-INF:BANDWIDTH=%d", max(ts.bitrate, 1))
	if ts.width > 0 && ts.height > 0 {
		fmt.Fprintf(&b, ",RESOLUTION=%dx%d", ts.width, ts.height)
	}
	if ts.codecs != "" {
		fmt.Fprintf(&b, `,CODECS="%s"`, ts.codecs)
	}
	fmt.Fprintf(&b, "\nsession/%s/base/index.m3u8\n", url.PathEscape(ts.id))
	return []byte(b.String())
}

// mediaPlaylist lists the session's segments (relative to
// session/{sid}/base/), all known up front, so the client can seek anywhere;
// Jellyfin transcodes whichever segment is asked for.
func (ts *transcodeSession) mediaPlaylist() []byte {
	target := int64(1)
	for _, d := range ts.durMs {
		target = max(target, (d+999)/1000)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "#EXTM3U\n#EXT-X-VERSION:7\n#EXT-X-TARGETDURATION:%d\n#EXT-X-MEDIA-SEQUENCE:0\n#EXT-X-PLAYLIST-TYPE:VOD\n#EXT-X-MAP:URI=\"header\"\n", target)
	for i, d := range ts.durMs {
		fmt.Fprintf(&b, "#EXTINF:%.3f,\n%d.m4s\n", float64(d)/1000, i)
	}
	b.WriteString("#EXT-X-ENDLIST\n")
	return []byte(b.String())
}

// proxyJellyfin streams a Jellyfin URL's body to w.
func (s *Server) proxyJellyfin(w http.ResponseWriter, r *http.Request, u string) {
	resp, err := s.jfGet(r.Context(), u)
	if err != nil {
		fail(w, r, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		fail(w, r, fmt.Errorf("GET %s: %s", strings.SplitN(u, "?", 2)[0], resp.Status))
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	if n := resp.Header.Get("Content-Length"); n != "" {
		w.Header().Set("Content-Length", n)
	}
	io.Copy(w, resp.Body)
}

// openHLS reads a Jellyfin master playlist and its first variant's media
// playlist into a session.
func (s *Server) openHLS(ctx context.Context, base, master string) (*transcodeSession, error) {
	lines, err := s.fetchLines(ctx, base+master)
	if err != nil {
		return nil, err
	}
	ts := &transcodeSession{base: base}
	variant := ""
	for i, l := range lines {
		if strings.HasPrefix(l, "#EXT-X-STREAM-INF:") {
			attrs := parseAttrs(strings.TrimPrefix(l, "#EXT-X-STREAM-INF:"))
			ts.codecs = attrs["CODECS"]
			ts.bitrate, _ = strconv.Atoi(attrs["BANDWIDTH"])
			fmt.Sscanf(attrs["RESOLUTION"], "%dx%d", &ts.width, &ts.height)
			if i+1 < len(lines) {
				variant = lines[i+1]
			}
			break
		}
	}
	if variant == "" {
		return nil, fmt.Errorf("no variant in master playlist")
	}
	lines, err = s.fetchLines(ctx, base+variant)
	if err != nil {
		return nil, err
	}
	var dur int64
	for _, l := range lines {
		switch {
		case strings.HasPrefix(l, "#EXT-X-MAP:"):
			ts.init = parseAttrs(strings.TrimPrefix(l, "#EXT-X-MAP:"))["URI"]
		case strings.HasPrefix(l, "#EXTINF:"):
			f, _ := strconv.ParseFloat(strings.SplitN(strings.TrimPrefix(l, "#EXTINF:"), ",", 2)[0], 64)
			dur = int64(f*1000 + 0.5)
		case l != "" && !strings.HasPrefix(l, "#"):
			ts.segments = append(ts.segments, l)
			ts.durMs = append(ts.durMs, dur)
		}
	}
	if ts.init == "" || len(ts.segments) == 0 {
		return nil, fmt.Errorf("unexpected media playlist (init %q, %d segments)", ts.init, len(ts.segments))
	}
	return ts, nil
}

func (s *Server) fetchLines(ctx context.Context, u string) ([]string, error) {
	resp, err := s.jfGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return nil, fmt.Errorf("GET %s: %s: %s", strings.SplitN(u, "?", 2)[0], resp.Status, b)
	}
	var out []string
	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() {
		out = append(out, strings.TrimSpace(sc.Text()))
	}
	return out, sc.Err()
}

// parseAttrs parses an HLS attribute list (KEY=VALUE,KEY="VALUE",...).
func parseAttrs(s string) map[string]string {
	out := map[string]string{}
	for s != "" {
		eq := strings.IndexByte(s, '=')
		if eq < 0 {
			break
		}
		key := strings.TrimSpace(s[:eq])
		s = s[eq+1:]
		var val string
		if strings.HasPrefix(s, `"`) {
			end := strings.IndexByte(s[1:], '"')
			if end < 0 {
				val, s = s[1:], ""
			} else {
				val, s = s[1:end+1], s[end+2:]
			}
		} else {
			end := strings.IndexByte(s, ',')
			if end < 0 {
				val, s = s, ""
			} else {
				val, s = s[:end], s[end:]
			}
		}
		out[key] = val
		s = strings.TrimPrefix(s, ",")
	}
	return out
}

// handleTranscodeFile serves session/{sid}/base/{file}: the media playlist,
// or the init segment or a media segment passed through from Jellyfin.
func (s *Server) handleTranscodeFile(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.transcodes.get(r.PathValue("sid"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	seg := r.PathValue("file")
	switch seg {
	case "index.m3u8":
		w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
		w.Write(ts.mediaPlaylist())
		return
	case "header":
		s.proxyJellyfin(w, r, ts.base+ts.init)
		return
	}
	n, err := strconv.Atoi(strings.TrimSuffix(seg, ".m4s"))
	if err != nil || n < 0 || n >= len(ts.segments) {
		http.NotFound(w, r)
		return
	}
	s.proxyJellyfin(w, r, ts.base+ts.segments[n])
}

func (s *Server) handleTranscodePing(w http.ResponseWriter, r *http.Request) {
	s.transcodes.get(r.URL.Query().Get("session"))
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleTranscodeStop(w http.ResponseWriter, r *http.Request) {
	s.stopTranscode(r.URL.Query().Get("session"))
	w.WriteHeader(http.StatusOK)
}

// stopTranscode ends a session and tells Jellyfin to kill its encoder.
func (s *Server) stopTranscode(sid string) {
	if sid == "" {
		return
	}
	s.transcodes.remove(sid)
	u := fmt.Sprintf("%s/Videos/ActiveEncodings?deviceId=%s&playSessionId=%s",
		strings.TrimRight(s.cfg.JellyfinURL, "/"), transcodeDevice, url.QueryEscape(sid))
	req, err := http.NewRequest(http.MethodDelete, u, nil)
	if err != nil {
		return
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.cfg.JellyfinAPIKey))
	if resp, err := http.DefaultClient.Do(req); err == nil {
		resp.Body.Close()
	}
}

// transcodeDecision answers a decision request the client made without
// direct play: Plex will stream an HLS transcode of the item.
func (s *Server) transcodeDecision(md *Element, mediaIndex int) *Element {
	const protocol = "hls"
	i := 0
	for _, c := range md.Children {
		if c.Tag != "Media" {
			continue
		}
		if i == mediaIndex {
			c.A("selected", true).A("protocol", protocol).A("container", "mp4").
				A("videoCodec", "h264").A("audioCodec", "aac").A("audioChannels", 2)
			for _, p := range c.Children {
				if p.Tag != "Part" {
					continue
				}
				p.A("decision", "transcode").A("selected", true).A("container", "mp4").A("protocol", "dash")
				for _, st := range p.Children {
					switch st.Get("streamType") {
					case 1:
						st.A("decision", "transcode")
					case 2:
						if st.Get("selected") == true {
							st.A("decision", "transcode")
						}
					case 3:
						if st.Get("selected") == true {
							st.A("decision", "burn")
						}
					}
				}
			}
		}
		i++
	}
	return Container().
		A("allowSync", false).
		A("directPlayDecisionCode", 3000).
		A("directPlayDecisionText", "App cannot direct play this item. Direct play is disabled.").
		A("transcodeDecisionCode", 1001).
		A("transcodeDecisionText", "Direct play not available; Conversion OK.").
		A("generalDecisionCode", 1001).
		A("generalDecisionText", "Direct play not available; Conversion OK.").
		A("mdeDecisionCode", 1000).
		A("mdeDecisionText", "Direct play OK.").
		A("identifier", "com.plexapp.plugins.library").
		Add(md)
}

// transcodeIdle is how long a session may go without segment requests or
// pings before jellex stops it, for clients that vanish without stopping.
const transcodeIdle = 2 * time.Minute

// reapTranscodes stops idle transcode sessions until ctx ends.
func (s *Server) reapTranscodes(ctx context.Context) {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		s.transcodes.mu.Lock()
		var idle []string
		for id, ts := range s.transcodes.by {
			if time.Since(ts.seen) > transcodeIdle {
				idle = append(idle, id)
			}
		}
		s.transcodes.mu.Unlock()
		for _, id := range idle {
			slog.Info("stopping idle transcode", "session", id)
			s.stopTranscode(id)
		}
	}
}
