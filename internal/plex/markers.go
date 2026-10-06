package plex

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// chapters renders an item's Jellyfin chapters as Plex Chapter elements.
// Chapter IDs only need to be unique within the item.
func (s *Server) chapters(it *jellyfin.BaseItemDto) []*Element {
	chs := it.GetChapters()
	rk := s.rk(it.GetId())
	var out []*Element
	for i, ch := range chs {
		start := ch.GetStartPositionTicks() / ticksPerMs
		end := it.GetRunTimeTicks() / ticksPerMs
		if i+1 < len(chs) {
			end = chs[i+1].GetStartPositionTicks() / ticksPerMs
		}
		e := E("Chapter").
			A("id", rk*1000+i).
			A("tag", ch.GetName()).
			A("index", i+1).
			A("startTimeOffset", start).
			A("endTimeOffset", end)
		if ch.GetImageTag() != "" {
			e.A("thumb", fmt.Sprintf("/library/metadata/%d/chapterImages/%d", rk, i))
		}
		out = append(out, e)
	}
	return out
}

var segmentMarkers = map[jellyfin.MediaSegmentType]string{
	jellyfin.MEDIASEGMENTTYPE_INTRO:      "intro",
	jellyfin.MEDIASEGMENTTYPE_OUTRO:      "credits",
	jellyfin.MEDIASEGMENTTYPE_COMMERCIAL: "commercial",
}

var (
	introChapter   = regexp.MustCompile(`(?i)^(intro|opening|opening credits|theme song)$`)
	creditsChapter = regexp.MustCompile(`(?i)^(credits|end credits|closing credits|outro|ending)$`)
)

// markers returns skip markers (intro, credits, commercials) for a playable
// item: Jellyfin media segments when a plugin provides them, otherwise
// chapters with recognizable names.
func (s *Server) markers(ctx context.Context, it *jellyfin.BaseItemDto) []*Element {
	rk := s.rk(it.GetId())
	type span struct {
		typ        string
		start, end int64
	}
	var spans []span
	res, _, err := s.jf.MediaSegmentAPI.GetItemSegments(ctx, it.GetId()).Execute()
	if err != nil {
		slog.Debug("media segments", "item", it.GetId(), "err", err)
	} else {
		for _, seg := range res.Items {
			if t, ok := segmentMarkers[seg.GetType()]; ok {
				spans = append(spans, span{t, seg.GetStartTicks() / ticksPerMs, seg.GetEndTicks() / ticksPerMs})
			}
		}
	}
	if len(spans) == 0 {
		for _, ch := range s.chapters(it) {
			name, _ := ch.Get("tag").(string)
			start, _ := ch.Get("startTimeOffset").(int64)
			end, _ := ch.Get("endTimeOffset").(int64)
			switch {
			case introChapter.MatchString(name):
				spans = append(spans, span{"intro", start, end})
			case creditsChapter.MatchString(name):
				spans = append(spans, span{"credits", start, end})
			}
		}
	}
	var out []*Element
	for i, sp := range spans {
		e := E("Marker").
			A("id", rk*1000+500+i).
			A("type", sp.typ).
			A("startTimeOffset", sp.start).
			A("endTimeOffset", sp.end)
		// Credits that run to the end are "final": Plex offers the next
		// item instead of just skipping ahead.
		if sp.typ == "credits" && sp.end >= it.GetRunTimeTicks()/ticksPerMs-1000 {
			e.A("final", true)
		}
		out = append(out, e)
	}
	return out
}

// addPlaybackExtras adds chapters and markers to a detail Metadata element
// when the request asks for them.
func (s *Server) addPlaybackExtras(ctx context.Context, r *http.Request, md *Element, it *jellyfin.BaseItemDto) {
	switch it.GetType() {
	case jellyfin.BASEITEMKIND_MOVIE, jellyfin.BASEITEMKIND_EPISODE:
	default:
		return
	}
	q := r.URL.Query()
	if q.Get("includeChapters") == "1" {
		md.Add(s.chapters(it)...)
	}
	if q.Get("includeMarkers") == "1" {
		md.Add(s.markers(ctx, it)...)
	}
}
