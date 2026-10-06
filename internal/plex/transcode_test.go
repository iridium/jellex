package plex

import (
	"encoding/xml"
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

func TestManifest(t *testing.T) {
	ts := &transcodeSession{id: "abc", codecs: "avc1.424029,mp4a.40.2", width: 1280, height: 720,
		durMs: []int64{3000, 3000, 1500}}
	b := ts.manifest()
	var mpd struct {
		Duration string `xml:"mediaPresentationDuration,attr"`
		Sets     []struct {
			MimeType string `xml:"mimeType,attr"`
			Rep      struct {
				Codecs string `xml:"codecs,attr"`
				Tmpl   struct {
					Media string `xml:"media,attr"`
					S     []struct {
						D int `xml:"d,attr"`
					} `xml:"SegmentTimeline>S"`
				} `xml:"SegmentTemplate"`
			} `xml:"Representation"`
		} `xml:"Period>AdaptationSet"`
	}
	if err := xml.Unmarshal(b, &mpd); err != nil {
		t.Fatalf("manifest is not valid XML: %v\n%s", err, b)
	}
	if mpd.Duration != "PT7.500S" {
		t.Errorf("duration = %s", mpd.Duration)
	}
	if len(mpd.Sets) != 2 || mpd.Sets[0].MimeType != "video/mp4" || mpd.Sets[1].MimeType != "audio/mp4" {
		t.Fatalf("adaptation sets = %+v", mpd.Sets)
	}
	if mpd.Sets[0].Rep.Codecs != "avc1.424029" || mpd.Sets[1].Rep.Codecs != "mp4a.40.2" {
		t.Errorf("codecs = %q / %q", mpd.Sets[0].Rep.Codecs, mpd.Sets[1].Rep.Codecs)
	}
	if n := len(mpd.Sets[1].Rep.Tmpl.S); n != 3 {
		t.Errorf("audio timeline has %d segments, want 3", n)
	}
	if !strings.HasPrefix(mpd.Sets[1].Rep.Tmpl.Media, "session/abc/1/") {
		t.Errorf("audio media template = %q", mpd.Sets[1].Rep.Tmpl.Media)
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
