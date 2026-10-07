package plex

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"strings"
)

// Jellyfin decides how a transcode is made: given a description of what
// the browser can play (a device profile), its PlaybackInfo API picks
// which streams to copy and which to re-encode, and returns the HLS URL to
// use. Copying matters: a 4K HEVC file with DTS audio only needs its audio
// converted for a browser that plays HEVC, which keeps full quality and
// costs almost no CPU.

// streamPlan is how Jellyfin will deliver a transcode.
type streamPlan struct {
	base      string // Jellyfin URL the playlist is relative to, ending in /
	master    string // master playlist, relative to base, with query
	copyVideo bool   // the video stream is passed through, not re-encoded
	reasons   []string
}

// defaultMaxBitrate caps a transcode when the client sets no limit
// ("Maximum" quality): high enough that Jellyfin copies any real file.
const defaultMaxBitrate = 400_000_000

// planTranscode asks Jellyfin how to stream t to the client making the
// request q. sid becomes Jellyfin's PlaySessionId, so stopTranscode can end
// the encode.
func (s *Server) planTranscode(ctx context.Context, t *transcodeTarget, q url.Values, sid string) (*streamPlan, error) {
	// The client always takes H.264. With directStream=1 it has checked it
	// can also play the source video as is (e.g. HEVC with hardware
	// decoding), so Jellyfin may pass it through.
	videoCodecs := []string{"h264"}
	if q.Get("directStream") == "1" && t.videoCodec != "" && t.videoCodec != "h264" {
		videoCodecs = append([]string{t.videoCodec}, videoCodecs...)
	}
	maxBitrate := defaultMaxBitrate
	if kbps, err := strconv.Atoi(q.Get("maxVideoBitrate")); err == nil && kbps > 0 {
		maxBitrate = kbps * 1000
	}
	codecProfiles := []map[string]any{} // Jellyfin requires the field
	for _, c := range videoCodecs {
		conds := []map[string]any{}
		if c == "hevc" {
			// Browsers don't do Dolby Vision; Jellyfin falls back to the
			// HDR10 base layer or re-encodes.
			conds = append(conds, cond("EqualsAny", "VideoRangeType", "SDR|HDR10|HLG"))
		}
		if wh := strings.SplitN(q.Get("videoResolution"), "x", 2); len(wh) == 2 {
			conds = append(conds, cond("LessThanEqual", "Width", wh[0]), cond("LessThanEqual", "Height", wh[1]))
		}
		if len(conds) > 0 {
			codecProfiles = append(codecProfiles, map[string]any{"Type": "Video", "Codec": c, "Conditions": conds})
		}
	}
	profile := map[string]any{
		"Name":                "jellex",
		"MaxStreamingBitrate": maxBitrate,
		"DirectPlayProfiles":  []any{},
		"TranscodingProfiles": []any{map[string]any{
			"Container":           "mp4",
			"Type":                "Video",
			"Protocol":            "hls",
			"VideoCodec":          strings.Join(videoCodecs, ","),
			"AudioCodec":          "aac",
			"Context":             "Streaming",
			"MaxAudioChannels":    "2",
			"BreakOnNonKeyFrames": true,
			"MinSegments":         1,
		}},
		"CodecProfiles": codecProfiles,
		// No subtitle profiles: a subtitle we pass is burned in.
		"SubtitleProfiles": []any{},
	}
	body := map[string]any{
		"UserId":               t.user,
		"MediaSourceId":        t.source,
		"MaxStreamingBitrate":  maxBitrate,
		"AudioStreamIndex":     t.audioIndex,
		"SubtitleStreamIndex":  t.subtitleIndex,
		"DeviceProfile":        profile,
		"EnableDirectPlay":     false,
		"EnableDirectStream":   false,
		"EnableTranscoding":    true,
		"AllowVideoStreamCopy": true,
		"AllowAudioStreamCopy": true,
		"AutoOpenLiveStream":   false,
	}
	if t.audioIndex < 0 {
		delete(body, "AudioStreamIndex")
	}
	var res struct {
		MediaSources []playbackSource `json:"MediaSources"`
	}
	if err := s.jfPost(ctx, fmt.Sprintf("/Items/%s/PlaybackInfo", t.item), body, &res); err != nil {
		return nil, err
	}
	var src *playbackSource
	for i := range res.MediaSources {
		if res.MediaSources[i].ID == t.source {
			src = &res.MediaSources[i]
		}
	}
	if src == nil || src.TranscodingURL == "" {
		return nil, fmt.Errorf("jellyfin offered no transcode for %s", t.item)
	}
	u, err := url.Parse(src.TranscodingURL)
	if err != nil {
		return nil, err
	}
	// Our own session and device IDs, so stopTranscode can find the encode;
	// jellex authenticates with a header, not the URL's key.
	uq := u.Query()
	uq.Set("PlaySessionId", sid)
	uq.Set("DeviceId", transcodeDevice)
	uq.Del("ApiKey")
	uq.Del("api_key")
	reasons := src.TranscodeReasons
	if len(reasons) == 0 {
		reasons = strings.Split(uq.Get("TranscodeReasons"), ",")
	}
	plan := &streamPlan{
		base:      strings.TrimRight(s.cfg.JellyfinURL, "/") + path.Dir(u.Path) + "/",
		master:    path.Base(u.Path) + "?" + uq.Encode(),
		copyVideo: t.subtitleIndex < 0,
		reasons:   reasons,
	}
	for _, r := range reasons {
		if strings.HasPrefix(r, "Video") || strings.HasPrefix(r, "Subtitle") {
			plan.copyVideo = false
		}
	}
	return plan, nil
}

type playbackSource struct {
	ID               string   `json:"Id"`
	TranscodingURL   string   `json:"TranscodingUrl"`
	TranscodeReasons []string `json:"TranscodeReasons"`
}

func cond(condition, property, value string) map[string]any {
	return map[string]any{"Condition": condition, "Property": property, "Value": value, "IsRequired": false}
}

// jfPost posts JSON to a Jellyfin API path with jellex's credentials and
// decodes the JSON reply into out.
func (s *Server) jfPost(ctx context.Context, apiPath string, body, out any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.JellyfinURL, "/")+apiPath, bytes.NewReader(b))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.cfg.JellyfinAPIKey))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode/100 != 2 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("POST %s: %s: %s", apiPath, resp.Status, msg)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
