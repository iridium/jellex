package fmp4

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// muxedFMP4 has ffmpeg write a fragmented MP4 with interleaved audio and
// video, the way Jellyfin's HLS transcodes do, and returns its init segment
// and the media fragments after it.
func muxedFMP4(t *testing.T) (init, media []byte) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	f := filepath.Join(t.TempDir(), "muxed.mp4")
	out, err := exec.Command("ffmpeg", "-nostdin", "-loglevel", "error",
		"-f", "lavfi", "-i", "testsrc2=size=320x240:rate=24:duration=4",
		"-f", "lavfi", "-i", "sine=frequency=440:duration=4",
		"-c:v", "libx264", "-preset", "ultrafast", "-g", "24", "-pix_fmt", "yuv420p",
		"-c:a", "aac", "-shortest",
		"-movflags", "frag_keyframe+empty_moov+default_base_moof", f).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v: %s", err, out)
	}
	buf, err := os.ReadFile(f)
	if err != nil {
		t.Fatal(err)
	}
	top, err := boxes(buf, 0, len(buf))
	if err != nil {
		t.Fatal(err)
	}
	moov, ok := find(top, "moov")
	if !ok {
		t.Fatal("no moov")
	}
	return buf[:moov.end], buf[moov.end:]
}

// decodes reports whether ffmpeg can fully decode a file.
func decodes(t *testing.T, data []byte) (streams string) {
	t.Helper()
	f := filepath.Join(t.TempDir(), "split.mp4")
	if err := os.WriteFile(f, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("ffmpeg", "-nostdin", "-v", "error", "-xerror", "-i", f, "-f", "null", "-").CombinedOutput(); err != nil {
		t.Fatalf("decode failed: %v: %s", err, out)
	}
	out, err := exec.Command("ffprobe", "-v", "error", "-show_entries", "stream=codec_type", "-of", "csv=p=0", f).Output()
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(string(out))
}

func TestSplit(t *testing.T) {
	init, media := muxedFMP4(t)
	tracks, err := Tracks(init)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 2 {
		t.Fatalf("tracks = %+v, want 2", tracks)
	}
	for _, tr := range tracks {
		si, err := SplitInit(init, tr.ID)
		if err != nil {
			t.Fatal(err)
		}
		sm, err := SplitSegment(media, tr.ID)
		if err != nil {
			t.Fatal(err)
		}
		want := map[string]string{"vide": "video", "soun": "audio"}[tr.Handler]
		if got := decodes(t, append(si, sm...)); got != want {
			t.Errorf("track %d (%s): streams %q, want %q", tr.ID, tr.Handler, got, want)
		}
	}
}
