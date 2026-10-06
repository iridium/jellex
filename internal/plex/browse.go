package plex

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

func (s *Server) browseRoutes() {
	m := s.mux
	m.HandleFunc("GET /hubs", s.handleHomeHubs)
	m.HandleFunc("GET /hubs/promoted", s.handlePromotedHubs)
	m.HandleFunc("GET /hubs/continueWatching", s.handleContinueWatching)
	m.HandleFunc("GET /hubs/sections/{id}", s.handleSectionHubs)
	m.HandleFunc("GET /library/sections/{id}/all", s.handleSectionAll)
	m.HandleFunc("GET /library/sections/{id}/prefs", s.handleEmpty)
	m.HandleFunc("GET /library/metadata/{id}", s.handleMetadata)
	m.HandleFunc("GET /library/metadata/{id}/children", s.handleChildren)
	m.HandleFunc("GET /library/metadata/{id}/related", s.handleRelated)
	m.HandleFunc("GET /library/metadata/{id}/similar", s.handleSimilarList)
	m.HandleFunc("GET /hubs/metadata/{id}/postplay", s.handlePostPlay)
	m.HandleFunc("GET /library/metadata/{id}/{kind}/{ts}", s.handleImage)
	m.HandleFunc("GET /photo/:/transcode", s.handlePhotoTranscode)
}

// errNotFound is returned when a Plex ID doesn't map to a Jellyfin item.
var errNotFound = errors.New("not found")

func (s *Server) guidParam(r *http.Request, name string) (string, error) {
	n, err := strconv.Atoi(r.PathValue(name))
	if err != nil {
		return "", errNotFound
	}
	g, ok := s.itemGUID(n)
	if !ok {
		return "", errNotFound
	}
	return g, nil
}

// itemGUID maps a ratingKey to a Jellyfin item ID. The ID map also holds
// parts, streams and playlist entries ("part:…" etc.), which aren't items.
func (s *Server) itemGUID(n int) (string, bool) {
	g, ok := s.ids.GUID(n)
	if !ok || strings.Contains(g, ":") {
		return "", false
	}
	return g, true
}

func (s *Server) sectionParam(r *http.Request, id string) (*section, error) {
	secs, err := s.sections(r.Context())
	if err != nil {
		return nil, err
	}
	for i := range secs {
		if fmt.Sprint(secs[i].ID) == id {
			return &secs[i], nil
		}
	}
	return nil, errNotFound
}

func failOr404(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, errNotFound) {
		http.NotFound(w, r)
		return
	}
	fail(w, r, err)
}

// paging reads Plex's container paging from headers or query parameters.
func paging(r *http.Request, defSize int) (start, size int) {
	get := func(k string) string {
		if v := r.Header.Get(k); v != "" {
			return v
		}
		return r.URL.Query().Get(k)
	}
	start, _ = strconv.Atoi(get("X-Plex-Container-Start"))
	size = defSize
	if v, err := strconv.Atoi(get("X-Plex-Container-Size")); err == nil {
		size = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("count")); err == nil && v > 0 {
		size = v
	}
	return start, size
}

// hub builds a hub from items fetched with one more than limit, so it can
// tell the client whether a "see all" list has more.
func hub(title, identifier, key, typ, context string, items []*Element, limit int) *Element {
	items, more := trimHub(items, limit)
	h := E("Hub").
		A("hubKey", key).
		A("key", key).
		A("title", title).
		A("type", typ).
		A("hubIdentifier", identifier).
		A("context", context).
		A("size", len(items)).
		A("more", more).
		A("style", "shelf").
		A("promoted", true)
	return h.Add(items...)
}

// latest returns recently added items for a section.
func (s *Server) latest(ctx context.Context, sec *section, limit int) ([]*Element, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	req := s.jf.LibraryAPI.GetLatestMedia(ctx).UserId(uid).ParentId(sec.GUID).Fields(listFields).Limit(int32(limit)).EnableUserData(true)
	if sec.Type == "artist" {
		req = req.IncludeItemTypes([]jellyfin.BaseItemKind{jellyfin.BASEITEMKIND_MUSIC_ALBUM})
	}
	items, _, err := req.Execute()
	if err != nil {
		return nil, fmt.Errorf("latest media: %w", err)
	}
	var out []*Element
	for i := range items {
		out = append(out, s.metadata(&items[i], sec, false))
	}
	return out, nil
}

// continueWatching returns in-progress items followed by next-up episodes.
func (s *Server) continueWatching(ctx context.Context, parent string, limit int) ([]*Element, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	resumeReq := s.jf.LibraryAPI.GetResumeItems(ctx).UserId(uid).Fields(listFields).Limit(int32(limit)).
		MediaTypes([]jellyfin.MediaType{jellyfin.MEDIATYPE_VIDEO})
	nextReq := s.jf.ShowAPI.GetNextUp(ctx).UserId(uid).Fields(listFields).Limit(int32(limit))
	if parent != "" {
		resumeReq = resumeReq.ParentId(parent)
		nextReq = nextReq.ParentId(parent)
	}
	resume, _, err := resumeReq.Execute()
	if err != nil {
		return nil, fmt.Errorf("resume items: %w", err)
	}
	next, _, err := nextReq.Execute()
	if err != nil {
		return nil, fmt.Errorf("next up: %w", err)
	}
	seen := map[string]bool{}
	var out []*Element
	for _, list := range [][]jellyfin.BaseItemDto{resume.Items, next.Items} {
		for i := range list {
			if seen[list[i].GetId()] || len(out) >= limit {
				continue
			}
			seen[list[i].GetId()] = true
			out = append(out, s.metadata(&list[i], nil, false))
		}
	}
	return out, nil
}

func (s *Server) sectionHubs(ctx context.Context, sec *section, limit int) ([]*Element, error) {
	var hubs []*Element
	if sec.Type != "artist" {
		cw, err := s.continueWatching(ctx, sec.GUID, limit+1)
		if err != nil {
			return nil, err
		}
		if len(cw) > 0 {
			hubs = append(hubs, hub("Continue Watching", sec.Type+".inprogress",
				fmt.Sprintf("/hubs/sections/%d/continueWatching", sec.ID), "mixed", "hub."+sec.Type+".inprogress", cw, limit))
		}
	}
	items, err := s.latest(ctx, sec, limit+1)
	if err != nil {
		return nil, err
	}
	hubs = append(hubs, recentlyAddedHub(sec, items, limit))
	return hubs, nil
}

// recentlyAddedHub is a section's "Recently Added" hub. Its type sets the
// card shape in Plex Web: "album" gets square cards, the others posters.
func recentlyAddedHub(sec *section, items []*Element, limit int) *Element {
	title := map[string]string{"movie": "Recently Added Movies", "show": "Recently Added TV", "artist": "Recently Added Music"}[sec.Type]
	typ := map[string]string{"movie": "movie", "show": "mixed", "artist": "album"}[sec.Type]
	ident := map[string]string{"movie": "movie.recentlyadded", "show": "tv.recentlyadded", "artist": "music.recent.added"}[sec.Type]
	return hub(title, ident, fmt.Sprintf("/library/sections/%d/recentlyAdded", sec.ID), typ, "hub."+ident, items, limit)
}

func (s *Server) handlePromotedHubs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, limit := paging(r, 12)
	mc := Container().A("identifier", "com.plexapp.plugins.library")
	for _, id := range strings.Split(r.URL.Query().Get("contentDirectoryID"), ",") {
		if id == "" {
			continue
		}
		sec, err := s.sectionParam(r, id)
		if err != nil {
			failOr404(w, r, err)
			return
		}
		items, err := s.latest(ctx, sec, limit+1)
		if err != nil {
			fail(w, r, err)
			return
		}
		mc.Add(recentlyAddedHub(sec, items, limit).A("librarySectionID", sec.ID))
	}
	write(w, r, mc)
}

func (s *Server) handleContinueWatching(w http.ResponseWriter, r *http.Request) {
	_, limit := paging(r, 20)
	items, err := s.continueWatching(r.Context(), "", limit+1)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, Container().A("identifier", "com.plexapp.plugins.library").Add(
		hub("Continue Watching", "home.continue", "/hubs/continueWatching/items", "mixed", "hub.home.continue", items, limit),
	))
}

func (s *Server) handleHomeHubs(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	_, limit := paging(r, 12)
	mc := Container().A("identifier", "com.plexapp.plugins.library")
	cw, err := s.continueWatching(ctx, "", limit+1)
	if err != nil {
		fail(w, r, err)
		return
	}
	if len(cw) > 0 {
		mc.Add(hub("Continue Watching", "home.continue", "/hubs/continueWatching/items", "mixed", "hub.home.continue", cw, limit))
	}
	secs, err := s.sections(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	for i := range secs {
		items, err := s.latest(ctx, &secs[i], limit+1)
		if err != nil {
			fail(w, r, err)
			return
		}
		mc.Add(hub("Recently Added in "+secs[i].Title, fmt.Sprintf("home.%d.recentlyadded", secs[i].ID),
			fmt.Sprintf("/library/sections/%d/recentlyAdded", secs[i].ID), "mixed", "hub.home.recentlyadded", items, limit))
	}
	write(w, r, mc)
}

func (s *Server) handleSectionHubs(w http.ResponseWriter, r *http.Request) {
	sec, err := s.sectionParam(r, r.PathValue("id"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	_, limit := paging(r, 12)
	hubs, err := s.sectionHubs(r.Context(), sec, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	write(w, r, Container().
		A("identifier", "com.plexapp.plugins.library").
		A("librarySectionID", sec.ID).
		A("librarySectionTitle", sec.Title).
		A("librarySectionUUID", sec.GUID).
		Add(hubs...))
}

// sortBy maps Plex sort parameters ("titleSort", "addedAt:desc") to Jellyfin.
func sortBy(plex string) ([]jellyfin.ItemSortBy, []jellyfin.SortOrder) {
	field, dir, _ := strings.Cut(plex, ":")
	order := jellyfin.SORTORDER_ASCENDING
	if dir == "desc" {
		order = jellyfin.SORTORDER_DESCENDING
	}
	by := map[string]jellyfin.ItemSortBy{
		"titleSort":             jellyfin.ITEMSORTBY_SORT_NAME,
		"title":                 jellyfin.ITEMSORTBY_SORT_NAME,
		"addedAt":               jellyfin.ITEMSORTBY_DATE_CREATED,
		"originallyAvailableAt": jellyfin.ITEMSORTBY_PREMIERE_DATE,
		"year":                  jellyfin.ITEMSORTBY_PRODUCTION_YEAR,
		"lastViewedAt":          jellyfin.ITEMSORTBY_DATE_PLAYED,
		"audienceRating":        jellyfin.ITEMSORTBY_COMMUNITY_RATING,
		"rating":                jellyfin.ITEMSORTBY_COMMUNITY_RATING,
		"random":                jellyfin.ITEMSORTBY_RANDOM,
		"index":                 jellyfin.ITEMSORTBY_INDEX_NUMBER,
	}[field]
	if by == "" {
		by = jellyfin.ITEMSORTBY_SORT_NAME
	}
	return []jellyfin.ItemSortBy{by, jellyfin.ITEMSORTBY_SORT_NAME}, []jellyfin.SortOrder{order, jellyfin.SORTORDER_ASCENDING}
}

func (s *Server) handleSectionAll(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sec, err := s.sectionParam(r, r.PathValue("id"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	q := r.URL.Query()
	plexType := sectionItemType(sec.Type)
	if t, err := strconv.Atoi(q.Get("type")); err == nil {
		plexType = t
	}
	if plexType == typeCollection {
		items, err := s.collections(ctx, sec)
		if err != nil {
			fail(w, r, err)
			return
		}
		writeList(w, r, s.sectionContainer(sec).A("title2", "Collections").A("viewGroup", "collection"), items)
		return
	}
	kind, ok := plexTypeNums[plexType]
	if !ok {
		http.Error(w, "unsupported type", http.StatusBadRequest)
		return
	}
	start, size := paging(r, 100000)
	by, order := sortBy(q.Get("sort"))
	req := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ParentId(sec.GUID).Recursive(true).
		IncludeItemTypes([]jellyfin.BaseItemKind{kind}).Fields(listFields).
		SortBy(by).SortOrder(order).StartIndex(int32(start)).Limit(int32(size)).EnableTotalRecordCount(true)
	res, _, err := sectionFilters(req, q).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	mc := Container().
		A("size", len(res.Items)).
		A("totalSize", int(res.GetTotalRecordCount())).
		A("offset", start).
		A("allowSync", false).
		A("identifier", "com.plexapp.plugins.library").
		A("librarySectionID", sec.ID).
		A("librarySectionTitle", sec.Title).
		A("librarySectionUUID", sec.GUID).
		A("title1", sec.Title).
		A("title2", "All").
		A("viewGroup", plexTypes[kind])
	if q.Get("includeMeta") == "1" {
		mc.Add(s.sectionMeta(sec, plexType, q.Get("sort")))
	}
	for i := range res.Items {
		mc.Add(s.metadata(&res.Items[i], sec, false))
	}
	w.Header().Set("X-Plex-Container-Start", fmt.Sprint(start))
	w.Header().Set("X-Plex-Container-Total-Size", fmt.Sprint(res.GetTotalRecordCount()))
	write(w, r, mc)
}

func (s *Server) handleMetadata(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var guids []string
	for _, part := range strings.Split(r.PathValue("id"), ",") {
		n, err := strconv.Atoi(part)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		g, ok := s.itemGUID(n)
		if !ok {
			http.NotFound(w, r)
			return
		}
		guids = append(guids, g)
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Ids(guids).Fields(detailFields).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	if len(res.Items) == 0 {
		http.NotFound(w, r)
		return
	}
	sec, err := s.sectionFor(ctx, guids[0])
	if err != nil {
		fail(w, r, err)
		return
	}
	mc := Container().A("identifier", "com.plexapp.plugins.library")
	if sec != nil {
		mc.A("librarySectionID", sec.ID).A("librarySectionTitle", sec.Title).A("librarySectionUUID", sec.GUID)
	}
	for i := range res.Items {
		md := s.metadata(&res.Items[i], sec, true)
		s.addPlaybackExtras(ctx, r, md, &res.Items[i])
		if res.Items[i].GetType() == jellyfin.BASEITEMKIND_BOX_SET && md.Get("thumb") == nil {
			s.collectionThumb(ctx, &res.Items[i], md)
		}
		if r.URL.Query().Get("includeExtras") == "1" {
			if xs, primary, err := s.extras(ctx, &res.Items[i], sec); err != nil {
				slog.Warn("extras", "item", res.Items[i].GetId(), "err", err)
			} else if len(xs) > 0 {
				ex := E("Extras").A("size", len(xs)).Add(xs...)
				ex.Single = true
				md.Add(ex).Opt("primaryExtraKey", primary)
			}
		}
		if r.URL.Query().Get("includeOnDeck") == "1" && res.Items[i].GetType() == jellyfin.BASEITEMKIND_SERIES {
			if od, err := s.onDeck(ctx, res.Items[i].GetId(), sec); err != nil {
				slog.Warn("on deck", "series", res.Items[i].GetId(), "err", err)
			} else if od != nil {
				md.Add(od)
			}
		}
		mc.Add(md)
	}
	write(w, r, mc)
}

// onDeck is a show's next episode to watch, shown as "up next" on the show
// page.
func (s *Server) onDeck(ctx context.Context, series string, sec *section) (*Element, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	res, _, err := s.jf.ShowAPI.GetNextUp(ctx).UserId(uid).SeriesId(series).Fields(listFields).
		Limit(1).EnableResumable(true).Execute()
	if err != nil {
		return nil, err
	}
	if len(res.Items) == 0 {
		return nil, nil
	}
	ep := s.metadata(&res.Items[0], sec, false)
	ep.Single = true
	od := E("OnDeck").Add(ep)
	od.Single = true
	return od, nil
}

func (s *Server) handleChildren(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	guid, err := s.guidParam(r, "id")
	if err != nil {
		failOr404(w, r, err)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	parent, _, err := s.jf.LibraryAPI.GetItem(ctx, guid).UserId(uid).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	sec, err := s.sectionFor(ctx, guid)
	if err != nil {
		fail(w, r, err)
		return
	}
	var items []jellyfin.BaseItemDto
	switch parent.GetType() {
	case jellyfin.BASEITEMKIND_SERIES:
		res, _, err := s.jf.ShowAPI.GetSeasons(ctx, guid).UserId(uid).Fields(listFields).Execute()
		if err != nil {
			fail(w, r, err)
			return
		}
		items = res.Items
	case jellyfin.BASEITEMKIND_SEASON:
		res, _, err := s.jf.ShowAPI.GetEpisodes(ctx, parent.GetSeriesId()).SeasonId(guid).UserId(uid).Fields(listFields).Execute()
		if err != nil {
			fail(w, r, err)
			return
		}
		items = res.Items
	default:
		req := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ParentId(guid).Fields(listFields).
			SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_PARENT_INDEX_NUMBER, jellyfin.ITEMSORTBY_INDEX_NUMBER, jellyfin.ITEMSORTBY_SORT_NAME})
		if parent.GetType() == jellyfin.BASEITEMKIND_MUSIC_ARTIST {
			req = s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ArtistIds([]string{guid}).Recursive(true).
				IncludeItemTypes([]jellyfin.BaseItemKind{jellyfin.BASEITEMKIND_MUSIC_ALBUM}).Fields(listFields).
				SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_PRODUCTION_YEAR, jellyfin.ITEMSORTBY_SORT_NAME})
		}
		res, _, err := req.Execute()
		if err != nil {
			fail(w, r, err)
			return
		}
		items = res.Items
	}
	pm := s.metadata(parent, sec, false)
	mc := Container().
		A("identifier", "com.plexapp.plugins.library").
		A("key", pm.Get("ratingKey")).
		A("title1", pm.Get("parentTitle")).
		A("title2", parent.GetName()).
		A("parentTitle", pm.Get("parentTitle")).
		A("parentIndex", pm.Get("index")).
		A("thumb", pm.Get("thumb")).
		A("art", pm.Get("art")).
		A("summary", parent.GetOverview()).
		A("viewGroup", map[jellyfin.BaseItemKind]string{
			jellyfin.BASEITEMKIND_SERIES: "season", jellyfin.BASEITEMKIND_SEASON: "episode",
			jellyfin.BASEITEMKIND_MUSIC_ARTIST: "album", jellyfin.BASEITEMKIND_MUSIC_ALBUM: "track",
		}[parent.GetType()])
	if sec != nil {
		mc.A("librarySectionID", sec.ID).A("librarySectionTitle", sec.Title).A("librarySectionUUID", sec.GUID)
	}
	start, size := paging(r, len(items))
	total := len(items)
	items = items[min(start, total):min(start+max(size, 0), total)]
	mc.A("size", len(items)).A("totalSize", total).A("offset", start)
	for i := range items {
		mc.Add(s.metadata(&items[i], sec, false))
	}
	w.Header().Set("X-Plex-Container-Start", fmt.Sprint(start))
	w.Header().Set("X-Plex-Container-Total-Size", fmt.Sprint(total))
	write(w, r, mc)
}

// imageKinds maps Plex image path segments to Jellyfin image types.
var imageKinds = map[string]string{
	"thumb": "Primary", "art": "Backdrop", "clearLogo": "Logo", "banner": "Banner", "theme": "",
}

func (s *Server) handleImage(w http.ResponseWriter, r *http.Request) {
	guid, err := s.guidParam(r, "id")
	if err != nil {
		failOr404(w, r, err)
		return
	}
	kind := imageKind(r.PathValue("kind"), r.PathValue("ts"))
	if kind == "" {
		http.NotFound(w, r)
		return
	}
	s.proxyImage(w, r, guid, kind, 0, 0)
}

// imageKind maps the {kind}/{ts} tail of a Plex image path to a Jellyfin
// image type. Chapter images are chapterImages/{index}.
func imageKind(kind, ts string) string {
	if kind == "chapterImages" {
		if _, err := strconv.Atoi(ts); err == nil {
			return "Chapter/" + ts
		}
		return ""
	}
	return imageKinds[kind]
}

// handlePhotoTranscode resizes an image named by url=, which is usually one
// of our own /library/metadata/... image paths.
func (s *Server) handlePhotoTranscode(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	width, _ := strconv.Atoi(q.Get("width"))
	height, _ := strconv.Atoi(q.Get("height"))
	u, err := url.Parse(q.Get("url"))
	if err != nil {
		http.Error(w, "bad url", http.StatusBadRequest)
		return
	}
	// /library/metadata/{id}/{kind}/{ts}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	if len(parts) >= 4 && parts[0] == "library" && parts[1] == "metadata" {
		n, err := strconv.Atoi(parts[2])
		g, ok := s.itemGUID(n)
		kind := imageKind(parts[3], parts[min(4, len(parts)-1)])
		if err != nil || !ok || kind == "" {
			http.NotFound(w, r)
			return
		}
		s.proxyImage(w, r, g, kind, width, height)
		return
	}
	// Other local images, such as seek previews at
	// /library/parts/{id}/indexes/sd/{offset} or playlist composites, are
	// served by their own routes, which may honor width and height.
	if u.Host == "" && (strings.HasPrefix(u.Path, "/library/parts/") || strings.HasPrefix(u.Path, "/playlists/")) {
		q := u.Query()
		if width > 0 {
			q.Set("width", fmt.Sprint(width))
		}
		if height > 0 {
			q.Set("height", fmt.Sprint(height))
		}
		u.RawQuery = q.Encode()
		r2 := r.Clone(r.Context())
		r2.URL = &url.URL{Path: u.Path, RawQuery: u.RawQuery}
		r2.RequestURI = u.RequestURI()
		s.mux.ServeHTTP(w, r2)
		return
	}
	http.NotFound(w, r)
}

func (s *Server) proxyImage(w http.ResponseWriter, r *http.Request, guid, kind string, width, height int) {
	u := fmt.Sprintf("%s/Items/%s/Images/%s?quality=90", strings.TrimRight(s.cfg.JellyfinURL, "/"), guid, kind)
	if width > 0 {
		u += fmt.Sprintf("&fillWidth=%d", width)
	}
	if height > 0 {
		u += fmt.Sprintf("&fillHeight=%d", height)
	}
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
	for _, h := range []string{"Content-Type", "Content-Length", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(h); v != "" {
			w.Header().Set(h, v)
		}
	}
	if resp.StatusCode == http.StatusOK {
		w.Header().Set("Cache-Control", "public, max-age=86400")
	}
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}
