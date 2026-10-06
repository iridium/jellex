package plex

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

func (s *Server) searchRoutes() {
	s.mux.HandleFunc("GET /library/search", s.handleLibrarySearch)
	s.mux.HandleFunc("GET /hubs/search", s.handleHubSearch)
}

// searchKinds maps Plex search types to the Jellyfin item kinds they cover.
var searchKinds = map[string][]jellyfin.BaseItemKind{
	"movies": {jellyfin.BASEITEMKIND_MOVIE},
	"tv":     {jellyfin.BASEITEMKIND_SERIES, jellyfin.BASEITEMKIND_EPISODE},
	"music":  {jellyfin.BASEITEMKIND_MUSIC_ARTIST, jellyfin.BASEITEMKIND_MUSIC_ALBUM, jellyfin.BASEITEMKIND_AUDIO},
}

type searchHit struct {
	item *jellyfin.BaseItemDto
	sec  *section
}

// search runs a query against every library the user can see, keeping
// Jellyfin's relevance order within each library.
func (s *Server) search(ctx context.Context, query string, types []string, limit int) ([]searchHit, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	var kinds []jellyfin.BaseItemKind
	for _, t := range types {
		kinds = append(kinds, searchKinds[strings.TrimSpace(t)]...)
	}
	if len(types) == 0 {
		for _, k := range searchKinds {
			kinds = append(kinds, k...)
		}
	}
	if len(kinds) == 0 {
		return nil, nil
	}
	secs, err := s.sections(ctx)
	if err != nil {
		return nil, err
	}
	var hits []searchHit
	for i := range secs {
		res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ParentId(secs[i].GUID).Recursive(true).
			SearchTerm(query).IncludeItemTypes(kinds).Fields(listFields).Limit(int32(limit)).Execute()
		if err != nil {
			return nil, err
		}
		for j := range res.Items {
			hits = append(hits, searchHit{&res.Items[j], &secs[i]})
		}
	}
	return hits, nil
}

// handleLibrarySearch serves Plex Web's search: SearchResult elements that
// each wrap a single Metadata object and a relevance score.
func (s *Server) handleLibrarySearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 {
		limit = 50
	}
	var types []string
	if t := q.Get("searchTypes"); t != "" {
		types = strings.Split(t, ",")
	}
	hits, err := s.search(r.Context(), q.Get("query"), types, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	mc := Container().A("identifier", "com.plexapp.plugins.library")
	for i, h := range hits {
		md := s.metadata(h.item, h.sec, false)
		md.Single = true
		mc.Add(E("SearchResult").A("score", round2(1-float64(i)/float64(len(hits)+1))).Add(md))
	}
	write(w, r, mc)
}

// handleHubSearch serves the hub-shaped search other Plex clients use: one
// hub per result type.
func (s *Server) handleHubSearch(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 {
		limit = 10
	}
	hits, err := s.search(r.Context(), q.Get("query"), nil, limit)
	if err != nil {
		fail(w, r, err)
		return
	}
	order := []string{"movie", "show", "episode", "artist", "album", "track"}
	titles := map[string]string{"movie": "Movies", "show": "Shows", "episode": "Episodes", "artist": "Artists", "album": "Albums", "track": "Tracks"}
	byType := map[string][]*Element{}
	for _, h := range hits {
		md := s.metadata(h.item, h.sec, false)
		t, _ := md.Get("type").(string)
		byType[t] = append(byType[t], md)
	}
	mc := Container().A("identifier", "com.plexapp.plugins.library")
	for _, t := range order {
		items := byType[t]
		if len(items) == 0 {
			continue
		}
		mc.Add(hub(titles[t], t, "/hubs/search?query="+q.Get("query"), t, "hub.search."+t, items, limit))
	}
	write(w, r, mc)
}
