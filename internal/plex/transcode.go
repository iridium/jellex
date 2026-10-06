package plex

import (
	"bufio"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/iridium/jellex/internal/fmp4"
)

// Transcoding bridges Plex's DASH transcoder API onto Jellyfin's HLS
// transcoder. Plex Web asks for /video/:/transcode/universal/start.mpd; jellex
// starts a Jellyfin HLS transcode with fragmented-MP4 segments, reads its
// playlist, and writes a DASH manifest whose segments come from Jellyfin's,
// split into separate video and audio streams.

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

	// Jellyfin muxes audio and video; DASH needs them apart. reps maps a
	// representation ID ("0" video, "1" audio) to its track in the segments.
	initData []byte
	reps     map[string]uint32

	mu    sync.Mutex
	cache map[int][]byte // recent raw segments, fetched once for both reps
	order []int
}

// maxCachedSegments bounds the raw segments kept per session. The video and
// audio streams fetch the same segment at about the same time.
const maxCachedSegments = 4

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
	m.HandleFunc("GET /video/:/transcode/universal/start.mpd", s.handleTranscodeStart)
	m.HandleFunc("GET /video/:/transcode/universal/session/{sid}/{rep}/header", s.handleTranscodeInit)
	m.HandleFunc("GET /video/:/transcode/universal/session/{sid}/{rep}/{seg}", s.handleTranscodeSegment)
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
	burn := q.Get("subtitles") == "burn"
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
	if err := s.loadInit(ctx, ts); err != nil {
		s.stopTranscode(sid)
		fail(w, r, fmt.Errorf("start jellyfin transcode: %w", err))
		return
	}
	s.transcodes.put(ts)
	slog.Info("transcode started", "session", sid, "item", t.item, "segments", len(ts.segments),
		"audioIndex", t.audioIndex, "burnSubtitle", t.subtitleIndex)

	w.Header().Set("Content-Type", "application/dash+xml")
	w.Write([]byte(xml.Header))
	w.Write(ts.manifest())
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

// manifest renders the session as a static DASH manifest with separate
// video and audio adaptation sets and an explicit segment timeline.
func (ts *transcodeSession) manifest() []byte {
	var total int64
	var timeline strings.Builder
	for i, d := range ts.durMs {
		if i == 0 {
			fmt.Fprintf(&timeline, `<S t="0" d="%d"/>`, d)
		} else {
			fmt.Fprintf(&timeline, `<S d="%d"/>`, d)
		}
		total += d
	}
	videoCodec, audioCodec := "avc1.640028", "mp4a.40.2"
	for _, c := range strings.Split(ts.codecs, ",") {
		c = strings.TrimSpace(c)
		switch {
		case strings.HasPrefix(c, "avc"), strings.HasPrefix(c, "hvc"), strings.HasPrefix(c, "hev"):
			videoCodec = c
		case strings.HasPrefix(c, "mp4a"), strings.HasPrefix(c, "ac-3"), strings.HasPrefix(c, "ec-3"), strings.HasPrefix(c, "opus"):
			audioCodec = c
		}
	}
	bw := ts.bitrate
	if bw <= 0 {
		bw = 4_000_000
	}
	sid := url.PathEscape(ts.id)
	seg := func(rep string) string {
		return fmt.Sprintf(`<SegmentTemplate timescale="1000" initialization="session/%s/%s/header" media="session/%s/%s/$Number$.m4s" startNumber="0"><SegmentTimeline>%s</SegmentTimeline></SegmentTemplate>`,
			sid, rep, sid, rep, timeline.String())
	}
	return []byte(fmt.Sprintf(`<MPD xmlns="urn:mpeg:dash:schema:mpd:2011" profiles="urn:mpeg:dash:profile:isoff-live:2011" type="static" minBufferTime="PT4S" mediaPresentationDuration="PT%.3fS">
<Period id="0" start="PT0S">
<AdaptationSet id="0" mimeType="video/mp4" contentType="video" segmentAlignment="true" startWithSAP="1">
<Representation id="0" codecs="%s" bandwidth="%d" width="%d" height="%d">%s</Representation>
</AdaptationSet>
<AdaptationSet id="1" mimeType="audio/mp4" contentType="audio" segmentAlignment="true" startWithSAP="1">
<Representation id="1" codecs="%s" bandwidth="128000" audioSamplingRate="48000"><AudioChannelConfiguration schemeIdUri="urn:mpeg:dash:23003:3:audio_channel_configuration:2011" value="2"/>%s</Representation>
</AdaptationSet>
</Period>
</MPD>
`, float64(total)/1000, videoCodec, bw, ts.width, ts.height, seg("0"), audioCodec, seg("1")))
}

// loadInit fetches the init segment and works out which track carries
// video and which audio.
func (s *Server) loadInit(ctx context.Context, ts *transcodeSession) error {
	b, err := s.fetchBytes(ctx, ts.base+ts.init)
	if err != nil {
		return err
	}
	tracks, err := fmp4.Tracks(b)
	if err != nil {
		return fmt.Errorf("parse init segment: %w", err)
	}
	ts.initData = b
	ts.reps = map[string]uint32{}
	for _, t := range tracks {
		switch t.Handler {
		case "vide":
			ts.reps["0"] = t.ID
		case "soun":
			ts.reps["1"] = t.ID
		}
	}
	if ts.reps["0"] == 0 || ts.reps["1"] == 0 {
		return fmt.Errorf("expected a video and an audio track, got %+v", tracks)
	}
	return nil
}

func (s *Server) fetchBytes(ctx context.Context, u string) ([]byte, error) {
	resp, err := s.jfGet(ctx, u)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", strings.SplitN(u, "?", 2)[0], resp.Status)
	}
	return io.ReadAll(resp.Body)
}

// segment returns raw segment n, from the cache or Jellyfin. The lock is
// held while fetching so both streams wait on one request.
func (s *Server) segment(ctx context.Context, ts *transcodeSession, n int) ([]byte, error) {
	ts.mu.Lock()
	defer ts.mu.Unlock()
	if b, ok := ts.cache[n]; ok {
		return b, nil
	}
	b, err := s.fetchBytes(ctx, ts.base+ts.segments[n])
	if err != nil {
		return nil, err
	}
	if ts.cache == nil {
		ts.cache = map[int][]byte{}
	}
	ts.cache[n] = b
	ts.order = append(ts.order, n)
	if len(ts.order) > maxCachedSegments {
		delete(ts.cache, ts.order[0])
		ts.order = ts.order[1:]
	}
	return b, nil
}

func (s *Server) handleTranscodeInit(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.transcodes.get(r.PathValue("sid"))
	track, okRep := ts.repTrack(r.PathValue("rep"))
	if !ok || !okRep {
		http.NotFound(w, r)
		return
	}
	b, err := fmp4.SplitInit(ts.initData, track)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Write(b)
}

func (ts *transcodeSession) repTrack(rep string) (uint32, bool) {
	if ts == nil {
		return 0, false
	}
	t, ok := ts.reps[rep]
	return t, ok
}

func (s *Server) handleTranscodeSegment(w http.ResponseWriter, r *http.Request) {
	ts, ok := s.transcodes.get(r.PathValue("sid"))
	track, okRep := ts.repTrack(r.PathValue("rep"))
	if !ok || !okRep {
		http.NotFound(w, r)
		return
	}
	n, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("seg"), ".m4s"))
	if err != nil || n < 0 || n >= len(ts.segments) {
		http.NotFound(w, r)
		return
	}
	raw, err := s.segment(r.Context(), ts, n)
	if err != nil {
		fail(w, r, err)
		return
	}
	b, err := fmp4.SplitSegment(raw, track)
	if err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "video/mp4")
	w.Write(b)
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
// direct play: Plex will stream a DASH transcode of the item.
func (s *Server) transcodeDecision(md *Element, mediaIndex int) *Element {
	i := 0
	for _, c := range md.Children {
		if c.Tag != "Media" {
			continue
		}
		if i == mediaIndex {
			c.A("selected", true).A("protocol", "dash").A("container", "mp4").
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
