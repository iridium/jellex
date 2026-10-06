package plex

import (
	"strings"
	"testing"
)

func TestParseAttrs(t *testing.T) {
	got := parseAttrs(`BANDWIDTH=2128000,CODECS="avc1.424029,mp4a.40.2",RESOLUTION=1280x720,URI="hls1/main/-1.mp4?a=1&b=2"`)
	want := map[string]string{
		"BANDWIDTH":  "2128000",
		"CODECS":     "avc1.424029,mp4a.40.2",
		"RESOLUTION": "1280x720",
		"URI":        "hls1/main/-1.mp4?a=1&b=2",
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%s = %q, want %q", k, got[k], v)
		}
	}
}

func TestPlaylists(t *testing.T) {
	ts := &transcodeSession{id: "a b", codecs: "avc1.424029,mp4a.40.2", width: 1280, height: 720,
		bitrate: 2128000, durMs: []int64{3000, 3000, 1500}}
	master := string(ts.masterPlaylist())
	for _, want := range []string{"BANDWIDTH=2128000", "RESOLUTION=1280x720", `CODECS="avc1.424029,mp4a.40.2"`, "\nsession/a%20b/base/index.m3u8\n"} {
		if !strings.Contains(master, want) {
			t.Errorf("master playlist lacks %q:\n%s", want, master)
		}
	}
	media := string(ts.mediaPlaylist())
	for _, want := range []string{"#EXT-X-TARGETDURATION:3\n", "#EXT-X-PLAYLIST-TYPE:VOD", `#EXT-X-MAP:URI="header"`,
		"#EXTINF:3.000,\n0.m4s\n", "#EXTINF:1.500,\n2.m4s\n", "#EXT-X-ENDLIST"} {
		if !strings.Contains(media, want) {
			t.Errorf("media playlist lacks %q:\n%s", want, media)
		}
	}
}

func TestWithPlayRes(t *testing.T) {
	in := "[Script Info]\nTitle: x\n\n[Events]\n"
	out := string(withPlayRes([]byte(in)))
	if !strings.Contains(out, "PlayResX: 384") || !strings.Contains(out, "PlayResY: 288") {
		t.Errorf("PlayRes not added:\n%s", out)
	}
	kept := "[Script Info]\nPlayResX: 1920\nPlayResY: 1080\n"
	if string(withPlayRes([]byte(kept))) != kept {
		t.Error("existing PlayRes was changed")
	}
}
