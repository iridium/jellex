package plex

import (
	"context"
	"fmt"
	"hash/crc32"
	"path"
	"slices"
	"strings"
	"time"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// listFields are the Jellyfin fields needed to render items in lists and hubs.
var listFields = []jellyfin.ItemFields{
	"Overview", "Genres", "DateCreated", "ProviderIds", "SortName", "ParentId",
	"ChildCount", "RecursiveItemCount", "MediaSources", "OriginalTitle", "Taglines",
}

// detailFields add what a single-item page shows.
var detailFields = append(append([]jellyfin.ItemFields{}, listFields...),
	"People", "Studios", "MediaStreams", "Path", "ProductionLocations", "Trickplay", "Chapters", "LocalTrailerCount", "SpecialFeatureCount",
)

const ticksPerMs = 10_000

// plexTypes maps Jellyfin item kinds to Plex metadata types.
var plexTypes = map[jellyfin.BaseItemKind]string{
	jellyfin.BASEITEMKIND_MOVIE:        "movie",
	jellyfin.BASEITEMKIND_SERIES:       "show",
	jellyfin.BASEITEMKIND_SEASON:       "season",
	jellyfin.BASEITEMKIND_EPISODE:      "episode",
	jellyfin.BASEITEMKIND_MUSIC_ARTIST: "artist",
	jellyfin.BASEITEMKIND_MUSIC_ALBUM:  "album",
	jellyfin.BASEITEMKIND_AUDIO:        "track",
	jellyfin.BASEITEMKIND_TRAILER:      "clip",
	jellyfin.BASEITEMKIND_VIDEO:        "clip",
	jellyfin.BASEITEMKIND_BOX_SET:      "collection",
}

// plexTypeNums maps Plex type numbers (as used in type= filters) to Jellyfin kinds.
var plexTypeNums = map[int]jellyfin.BaseItemKind{
	typeMovie:   jellyfin.BASEITEMKIND_MOVIE,
	typeShow:    jellyfin.BASEITEMKIND_SERIES,
	typeSeason:  jellyfin.BASEITEMKIND_SEASON,
	typeEpisode: jellyfin.BASEITEMKIND_EPISODE,
	typeArtist:  jellyfin.BASEITEMKIND_MUSIC_ARTIST,
	typeAlbum:   jellyfin.BASEITEMKIND_MUSIC_ALBUM,
	typeTrack:   jellyfin.BASEITEMKIND_AUDIO,
}

// rk returns the Plex ratingKey for a Jellyfin item ID.
func (s *Server) rk(guid string) int { return s.ids.ID(guid) }

// imageURL builds a Plex-style image path for a Jellyfin image. The trailing
// number only busts caches, so a hash of the Jellyfin image tag is used.
func (s *Server) imageURL(guid, kind, tag string) string {
	if guid == "" || tag == "" {
		return ""
	}
	return fmt.Sprintf("/library/metadata/%d/%s/%d", s.rk(guid), kind, imageStamp(tag))
}

// imageStamp turns a Jellyfin image tag into the number Plex image paths end
// with; it only busts caches when the image changes.
func imageStamp(tag string) uint32 { return crc32.ChecksumIEEE([]byte(tag)) }

func primaryTag(it *jellyfin.BaseItemDto) string {
	if t := it.GetImageTags()["Primary"]; t != nil {
		return *t
	}
	return ""
}

// metadata renders a Jellyfin item as a Plex Metadata (or Directory for
// containers in some contexts) element. sec may be nil when unknown.
func (s *Server) metadata(it *jellyfin.BaseItemDto, sec *section, detail bool) *Element {
	typ := plexTypes[it.GetType()]
	id := it.GetId()
	rk := s.rk(id)
	m := E("Metadata").
		A("ratingKey", fmt.Sprint(rk)).
		A("key", fmt.Sprintf("/library/metadata/%d", rk)).
		// Like PMS's unmatched local items. Not plex://: clients look those
		// up on plex.tv's metadata service, and a card's actions menu waits
		// on that lookup, so a made-up plex:// GUID keeps it from opening.
		A("guid", fmt.Sprintf("local://%d", rk)).
		A("type", typ).
		A("title", it.GetName())
	if sec != nil {
		m.A("librarySectionTitle", sec.Title).
			A("librarySectionID", sec.ID).
			A("librarySectionKey", fmt.Sprintf("/library/sections/%d", sec.ID))
	}
	m.Opt("titleSort", it.GetSortName()).
		Opt("originalTitle", strings.TrimSpace(strings.TrimPrefix(it.GetOriginalTitle(), it.GetName()))).
		Opt("contentRating", it.GetOfficialRating()).
		Opt("summary", it.GetOverview()).
		Opt("audienceRating", round1(float64(it.GetCommunityRating()))).
		Opt("rating", round1(float64(it.GetCriticRating())/10)).
		Opt("year", int(it.GetProductionYear())).
		Opt("thumb", s.imageURL(id, "thumb", primaryTag(it)))
	if tl := it.GetTaglines(); len(tl) > 0 {
		m.A("tagline", tl[0])
	}
	if bd := it.GetBackdropImageTags(); len(bd) > 0 {
		m.A("art", s.imageURL(id, "art", bd[0]))
	}
	if d := it.GetRunTimeTicks(); d > 0 {
		m.A("duration", d/ticksPerMs)
	}
	if p := it.GetPremiereDate(); !p.IsZero() {
		m.A("originallyAvailableAt", p.Format(time.DateOnly))
	}
	if c := it.GetDateCreated(); !c.IsZero() {
		m.A("addedAt", c.Unix()).A("updatedAt", c.Unix())
	}

	ud := it.GetUserData()
	switch typ {
	case "show", "season", "artist", "album":
		m.A("key", fmt.Sprintf("/library/metadata/%d/children", rk))
	case "collection":
		m.A("key", fmt.Sprintf("/library/collections/%d/children", rk)).
			A("childCount", int(it.GetChildCount()))
	}
	switch typ {
	case "show":
		leaves := int(it.GetRecursiveItemCount())
		m.A("childCount", int(it.GetChildCount())).
			A("leafCount", leaves).
			A("viewedLeafCount", max(0, leaves-int(ud.GetUnplayedItemCount())))
		if leaves == 0 {
			m.A("viewedLeafCount", 0)
		}
	case "season":
		leaves := int(it.GetRecursiveItemCount())
		if leaves == 0 {
			leaves = int(it.GetChildCount())
		}
		m.A("index", int(it.GetIndexNumber())).
			A("leafCount", leaves).
			A("viewedLeafCount", max(0, leaves-int(ud.GetUnplayedItemCount()))).
			A("parentRatingKey", fmt.Sprint(s.rk(it.GetSeriesId()))).
			A("parentKey", fmt.Sprintf("/library/metadata/%d", s.rk(it.GetSeriesId()))).
			A("parentTitle", it.GetSeriesName()).
			Opt("parentThumb", s.imageURL(it.GetSeriesId(), "thumb", it.GetSeriesPrimaryImageTag()))
		if it.GetParentBackdropItemId() != "" {
			m.A("art", fmt.Sprintf("/library/metadata/%d/art/0", s.rk(it.GetParentBackdropItemId())))
		}
	case "episode":
		m.A("index", int(it.GetIndexNumber())).
			A("parentIndex", int(it.GetParentIndexNumber())).
			A("parentRatingKey", fmt.Sprint(s.rk(it.GetSeasonId()))).
			A("parentKey", fmt.Sprintf("/library/metadata/%d", s.rk(it.GetSeasonId()))).
			A("parentTitle", it.GetSeasonName()).
			A("parentThumb", fmt.Sprintf("/library/metadata/%d/thumb/0", s.rk(it.GetSeasonId()))).
			A("grandparentRatingKey", fmt.Sprint(s.rk(it.GetSeriesId()))).
			A("grandparentKey", fmt.Sprintf("/library/metadata/%d", s.rk(it.GetSeriesId()))).
			A("grandparentTitle", it.GetSeriesName()).
			Opt("grandparentThumb", s.imageURL(it.GetSeriesId(), "thumb", it.GetSeriesPrimaryImageTag()))
		if it.GetParentBackdropItemId() != "" {
			m.A("grandparentArt", fmt.Sprintf("/library/metadata/%d/art/0", s.rk(it.GetParentBackdropItemId())))
		}
	case "album":
		if artists := it.GetArtistItems(); len(artists) > 0 {
			a := artists[0]
			m.A("parentRatingKey", fmt.Sprint(s.rk(a.GetId()))).
				A("parentKey", fmt.Sprintf("/library/metadata/%d", s.rk(a.GetId()))).
				A("parentTitle", a.GetName()).
				A("parentThumb", fmt.Sprintf("/library/metadata/%d/thumb/0", s.rk(a.GetId())))
		}
		m.A("leafCount", int(it.GetChildCount()))
	case "track":
		m.A("index", int(it.GetIndexNumber())).
			Opt("parentIndex", int(it.GetParentIndexNumber()))
		if it.GetAlbumId() != "" {
			m.A("parentRatingKey", fmt.Sprint(s.rk(it.GetAlbumId()))).
				A("parentKey", fmt.Sprintf("/library/metadata/%d", s.rk(it.GetAlbumId()))).
				A("parentTitle", it.GetAlbum()).
				Opt("parentThumb", s.imageURL(it.GetAlbumId(), "thumb", it.GetAlbumPrimaryImageTag()))
		}
		if artists := it.GetArtistItems(); len(artists) > 0 {
			a := artists[0]
			m.A("grandparentRatingKey", fmt.Sprint(s.rk(a.GetId()))).
				A("grandparentKey", fmt.Sprintf("/library/metadata/%d", s.rk(a.GetId()))).
				A("grandparentTitle", a.GetName()).
				A("grandparentThumb", fmt.Sprintf("/library/metadata/%d/thumb/0", s.rk(a.GetId())))
		}
	}

	// Watch state.
	if pc := ud.GetPlayCount(); pc > 0 || ud.GetPlayed() {
		m.A("viewCount", max(1, int(pc)))
	}
	if off := ud.GetPlaybackPositionTicks(); off > 0 {
		m.A("viewOffset", off/ticksPerMs)
	}
	if lp := ud.GetLastPlayedDate(); !lp.IsZero() {
		m.A("lastViewedAt", lp.Unix())
	}
	if r := ud.GetRating(); r > 0 {
		m.A("userRating", r)
	}
	// Children: media for playable items, then tags.
	switch typ {
	case "movie", "episode", "track", "clip":
		for _, ms := range it.GetMediaSources() {
			m.Add(s.media(it, &ms, detail))
		}
	}
	for _, g := range it.GetGenres() {
		m.Add(E("Genre").A("tag", g).A("filter", "genre="+urlTag(g)))
	}
	for k, v := range it.GetProviderIds() {
		if v == nil || *v == "" {
			continue
		}
		switch strings.ToLower(k) {
		case "imdb", "tmdb", "tvdb":
			m.Add(E("Guid").A("id", fmt.Sprintf("%s://%s", strings.ToLower(k), *v)))
		}
	}
	if detail {
		for _, st := range it.GetStudios() {
			m.A("studio", st.GetName())
			break
		}
		for _, p := range it.GetPeople() {
			tag := map[jellyfin.PersonKind]string{
				"Actor": "Role", "GuestStar": "Role", "Director": "Director", "Writer": "Writer", "Producer": "Producer",
			}[p.GetType()]
			if tag == "" {
				continue
			}
			key := map[string]string{"Role": "actor"}[tag]
			if key == "" {
				key = strings.ToLower(tag)
			}
			e := E(tag).A("tag", p.GetName()).A("filter", key+"="+urlTag(p.GetName()))
			if tag == "Role" {
				e.Opt("role", p.GetRole())
			}
			if t := p.GetPrimaryImageTag(); t != "" {
				e.A("thumb", s.imageURL(p.GetId(), "thumb", t))
			}
			m.Add(e)
		}
		m.Add(E("Image").A("alt", it.GetName()).A("type", "coverPoster").A("url", m.Get("thumb")))
		if art := m.Get("art"); art != nil {
			m.Add(E("Image").A("alt", it.GetName()).A("type", "background").A("url", art))
		}
		if t := it.GetImageTags()["Logo"]; t != nil {
			m.Add(E("Image").A("alt", it.GetName()).A("type", "clearLogo").A("url", s.imageURL(id, "clearLogo", *t)))
		}
	}
	return m
}

func round1(f float64) float64 { return float64(int(f*10+0.5)) / 10 }

func urlTag(s string) string { return strings.ReplaceAll(s, " ", "%20") }

// media renders one Jellyfin media source as a Plex Media/Part/Stream tree.
func (s *Server) media(it *jellyfin.BaseItemDto, ms *jellyfin.MediaSourceInfo, detail bool) *Element {
	partKey := "part:" + it.GetId() + ":" + ms.GetId()
	partID := s.ids.ID(partKey)
	container := ms.GetContainer()
	if i := strings.IndexByte(container, ','); i >= 0 {
		container = container[:i]
	}
	m := E("Media").
		A("id", s.ids.ID("media:"+it.GetId()+":"+ms.GetId())).
		Opt("duration", ms.GetRunTimeTicks()/ticksPerMs).
		Opt("bitrate", int(ms.GetBitrate()/1000)).
		Opt("container", container)
	ext := strings.TrimPrefix(path.Ext(ms.GetPath()), ".")
	if ext == "" {
		ext = container
	}
	part := E("Part").
		A("id", partID).
		A("key", fmt.Sprintf("/library/parts/%d/%d/file.%s", partID, it.GetDateCreated().Unix(), ext)).
		Opt("duration", ms.GetRunTimeTicks()/ticksPerMs).
		Opt("file", ms.GetPath()).
		Opt("size", ms.GetSize()).
		Opt("container", container).
		A("accessible", true).
		A("exists", true)
	// Jellyfin trickplay images back Plex's seek preview thumbnails.
	if len(it.GetTrickplay()[ms.GetId()]) > 0 {
		part.A("indexes", "sd")
	}

	for _, st := range ms.GetMediaStreams() {
		switch st.GetType() {
		case jellyfin.MEDIASTREAMTYPE_VIDEO:
			w, h := int(st.GetWidth()), int(st.GetHeight())
			m.A("videoCodec", st.GetCodec()).
				Opt("width", w).Opt("height", h).
				A("videoResolution", videoResolution(w, h)).
				Opt("videoProfile", strings.ToLower(st.GetProfile())).
				A("videoFrameRate", frameRate(float64(st.GetRealFrameRate())))
			if h > 0 {
				m.A("aspectRatio", round2(float64(w)/float64(h)))
			}
		case jellyfin.MEDIASTREAMTYPE_AUDIO:
			if m.Get("audioCodec") == nil {
				m.A("audioCodec", st.GetCodec()).Opt("audioChannels", int(st.GetChannels()))
			}
		}
		if detail {
			if e := s.stream(partKey, &st); e != nil {
				part.Add(e)
			}
		}
	}
	selectFirstAudio(part)
	s.applySelection(part, partID)
	return m.Add(part)
}

func (s *Server) stream(partKey string, st *jellyfin.MediaStream) *Element {
	var kind int
	switch st.GetType() {
	case jellyfin.MEDIASTREAMTYPE_VIDEO:
		kind = 1
	case jellyfin.MEDIASTREAMTYPE_AUDIO:
		kind = 2
	case jellyfin.MEDIASTREAMTYPE_SUBTITLE:
		kind = 3
	default:
		return nil
	}
	e := E("Stream").
		A("id", s.ids.ID(fmt.Sprintf("stream:%s:%d", partKey, st.GetIndex()))).
		A("streamType", kind).
		Opt("default", st.GetIsDefault()).
		Opt("forced", st.GetIsForced()).
		A("codec", st.GetCodec()).
		A("index", int(st.GetIndex())).
		Opt("bitrate", int(st.GetBitRate()/1000)).
		Opt("languageCode", st.GetLanguage()).
		Opt("title", st.GetTitle()).
		A("displayTitle", st.GetDisplayTitle()).
		A("extendedDisplayTitle", st.GetDisplayTitle())
	switch kind {
	case 1:
		e.Opt("width", int(st.GetWidth())).Opt("height", int(st.GetHeight())).
			Opt("frameRate", round2(float64(st.GetRealFrameRate()))).
			Opt("profile", strings.ToLower(st.GetProfile())).
			Opt("bitDepth", int(st.GetBitDepth())).
			Opt("level", int(st.GetLevel()))
	case 2:
		e.Opt("channels", int(st.GetChannels())).
			Opt("samplingRate", int(st.GetSampleRate())).
			Opt("audioChannelLayout", st.GetChannelLayout()).
			Opt("profile", strings.ToLower(st.GetProfile()))
		if st.GetIsDefault() {
			e.A("selected", true)
		}
	case 3:
		codec := strings.ToLower(st.GetCodec())
		if c, ok := subtitleCodecs[codec]; ok {
			codec = c
		}
		e.A("codec", codec)
		if textSubtitle(codec) {
			e.A("key", fmt.Sprintf("/library/streams/%d", e.Get("id")))
		}
		// Sidecar files have no index in the container.
		if st.GetIsExternal() {
			e.Attrs = slices.DeleteFunc(e.Attrs, func(a Attr) bool { return a.Name == "index" })
		}
	}
	return e
}

// selectFirstAudio marks the first audio stream selected when none is the
// default (common for single-stream music files); Plex clients need a
// selected stream to analyze codecs.
func selectFirstAudio(part *Element) {
	var first *Element
	for _, st := range part.Children {
		if st.Get("streamType") != 2 {
			continue
		}
		if st.Get("selected") != nil {
			return
		}
		if first == nil {
			first = st
		}
	}
	if first != nil {
		first.A("selected", true)
	}
}

func round2(f float64) float64 { return float64(int(f*100+0.5)) / 100 }

func videoResolution(w, h int) string {
	switch {
	case w >= 3200 || h >= 1800:
		return "4k"
	case w >= 1700 || h >= 1000:
		return "1080"
	case w >= 1200 || h >= 700:
		return "720"
	case h >= 540:
		return "576"
	case h >= 400:
		return "480"
	}
	return "sd"
}

func frameRate(f float64) string {
	switch {
	case f == 0:
		return ""
	case f < 24.5:
		return "24p"
	case f < 26:
		return "PAL"
	case f < 31:
		return "NTSC"
	case f < 51:
		return "50p"
	}
	return "60p"
}

// sectionFor finds which library section an item belongs to.
func (s *Server) sectionFor(ctx context.Context, guid string) (*section, error) {
	secs, err := s.sections(ctx)
	if err != nil {
		return nil, err
	}
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	anc, _, err := s.jf.LibraryAPI.GetAncestors(ctx, guid).UserId(uid).Execute()
	if err != nil {
		return nil, err
	}
	for _, a := range anc {
		for i := range secs {
			if secs[i].GUID == a.GetId() {
				return &secs[i], nil
			}
		}
	}
	return nil, nil
}
