package plex

import (
	"fmt"
	"net/http"
	"strings"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// The "see all" pages behind hub titles: each hub's key returns its full
// item list, paged like any other list.

func (s *Server) hubListRoutes() {
	s.mux.HandleFunc("GET /hubs/continueWatching/items", s.handleContinueWatchingList)
	s.mux.HandleFunc("GET /hubs/sections/{id}/continueWatching", s.handleSectionContinueWatchingList)
	s.mux.HandleFunc("GET /library/sections/{id}/recentlyAdded", s.handleRecentlyAddedList)
}

// maxHubList caps "see all" lists that Jellyfin can't page natively.
const maxHubList = 200

// writeList pages items per the request and writes them as a list.
func writeList(w http.ResponseWriter, r *http.Request, mc *Element, items []*Element) {
	start, size := paging(r, len(items))
	total := len(items)
	page := items[min(start, total):min(start+max(size, 0), total)]
	mc.A("size", len(page)).A("totalSize", total).A("offset", start).Add(page...)
	w.Header().Set("X-Plex-Container-Start", fmt.Sprint(start))
	w.Header().Set("X-Plex-Container-Total-Size", fmt.Sprint(total))
	write(w, r, mc)
}

// trimHub cuts a list fetched with one extra item down to limit, reporting
// whether there was more.
func trimHub(items []*Element, limit int) ([]*Element, bool) {
	if len(items) > limit {
		return items[:limit], true
	}
	return items, false
}

func (s *Server) handleContinueWatchingList(w http.ResponseWriter, r *http.Request) {
	parent := ""
	// contentDirectoryID narrows to one library when it names exactly one.
	if ids := strings.Split(r.URL.Query().Get("contentDirectoryID"), ","); len(ids) == 1 && ids[0] != "" {
		if sec, err := s.sectionParam(r, ids[0]); err == nil {
			parent = sec.GUID
		}
	}
	items, err := s.continueWatching(r.Context(), parent, maxHubList)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeList(w, r, Container().A("identifier", "com.plexapp.plugins.library").A("title1", "Continue Watching"), items)
}

func (s *Server) handleSectionContinueWatchingList(w http.ResponseWriter, r *http.Request) {
	sec, err := s.sectionParam(r, r.PathValue("id"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	items, err := s.continueWatching(r.Context(), sec.GUID, maxHubList)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeList(w, r, s.sectionContainer(sec).A("title2", "Continue Watching"), items)
}

func (s *Server) handleRecentlyAddedList(w http.ResponseWriter, r *http.Request) {
	sec, err := s.sectionParam(r, r.PathValue("id"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	items, err := s.latest(r.Context(), sec, maxHubList)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeList(w, r, s.sectionContainer(sec).A("title2", "Recently Added"), items)
}

func (s *Server) sectionContainer(sec *section) *Element {
	return Container().
		A("identifier", "com.plexapp.plugins.library").
		A("librarySectionID", sec.ID).
		A("librarySectionTitle", sec.Title).
		A("librarySectionUUID", sec.GUID).
		A("title1", sec.Title)
}

// handleRelated serves the "More Like This" hub on detail pages from
// Jellyfin's similar items.
func (s *Server) handleRelated(w http.ResponseWriter, r *http.Request) {
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
	const limit = 12
	res, _, err := s.jf.LibraryAPI.GetSimilarItems(ctx, guid).UserId(uid).Fields(listFields).Limit(limit + 1).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	sec, err := s.sectionFor(ctx, guid)
	if err != nil {
		fail(w, r, err)
		return
	}
	var items []*Element
	for i := range res.Items {
		items = append(items, s.metadata(&res.Items[i], sec, false))
	}
	mc := Container().A("identifier", "com.plexapp.plugins.library")
	if len(items) > 0 {
		typ, _ := items[0].Get("type").(string)
		mc.Add(hub("More Like This", typ+".similar", fmt.Sprintf("/library/metadata/%d/similar", s.rk(guid)),
			typ, "hub."+typ+".similar", items, limit))
	}
	write(w, r, mc)
}

// handleSimilarList is the "see all" list behind the More Like This hub.
func (s *Server) handleSimilarList(w http.ResponseWriter, r *http.Request) {
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
	res, _, err := s.jf.LibraryAPI.GetSimilarItems(ctx, guid).UserId(uid).Fields(listFields).Limit(50).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	sec, err := s.sectionFor(ctx, guid)
	if err != nil {
		fail(w, r, err)
		return
	}
	var items []*Element
	for i := range res.Items {
		items = append(items, s.metadata(&res.Items[i], sec, false))
	}
	writeList(w, r, Container().A("identifier", "com.plexapp.plugins.library").A("title1", "More Like This"), items)
}

// handlePostPlay serves the hubs shown when an item finishes: the following
// episodes for an episode, and similar items.
func (s *Server) handlePostPlay(w http.ResponseWriter, r *http.Request) {
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
	it, _, err := s.jf.LibraryAPI.GetItem(ctx, guid).UserId(uid).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	sec, err := s.sectionFor(ctx, guid)
	if err != nil {
		fail(w, r, err)
		return
	}
	mc := Container().A("identifier", "com.plexapp.plugins.library")
	const limit = 6
	if it.GetType() == jellyfin.BASEITEMKIND_EPISODE {
		res, _, err := s.jf.ShowAPI.GetEpisodes(ctx, it.GetSeriesId()).UserId(uid).StartItemId(guid).
			Fields(listFields).Limit(limit + 2).Execute()
		if err != nil {
			fail(w, r, err)
			return
		}
		var next []*Element
		for i := range res.Items {
			if res.Items[i].GetId() != guid {
				next = append(next, s.metadata(&res.Items[i], sec, false))
			}
		}
		if len(next) > 0 {
			mc.Add(hub("Up Next", "tv.upnext", fmt.Sprintf("/library/metadata/%d/children", s.rk(it.GetSeasonId())),
				"episode", "hub.tv.upnext", next, limit))
		}
	}
	sim, _, err := s.jf.LibraryAPI.GetSimilarItems(ctx, guid).UserId(uid).Fields(listFields).Limit(limit + 1).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	var related []*Element
	for i := range sim.Items {
		related = append(related, s.metadata(&sim.Items[i], sec, false))
	}
	if len(related) > 0 {
		typ, _ := related[0].Get("type").(string)
		mc.Add(hub("More Like This", typ+".similar", fmt.Sprintf("/library/metadata/%d/similar", s.rk(guid)),
			typ, "hub."+typ+".similar", related, limit))
	}
	write(w, r, mc)
}
