package plex

import (
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// The library browser's toolbar (type picker, filters, sorts, A-Z jump bar)
// is driven by a Meta block on /library/sections/{id}/all?includeMeta=1 and
// by per-filter value lists.

func (s *Server) libMetaRoutes() {
	s.mux.HandleFunc("GET /library/sections/{id}/firstCharacter", s.handleFirstCharacter)
	s.mux.HandleFunc("GET /library/sections/{id}/{filter}", s.handleFilterValues)
}

// sectionTypes lists the browsable Plex types per section type, the first
// being the default.
var sectionTypes = map[string][]int{
	"movie":  {typeMovie},
	"show":   {typeShow, typeSeason, typeEpisode},
	"artist": {typeArtist, typeAlbum, typeTrack},
}

var typeTitles = map[int]string{
	typeMovie: "Movies", typeShow: "Shows", typeSeason: "Seasons", typeEpisode: "Episodes",
	typeArtist: "Artists", typeAlbum: "Albums", typeTrack: "Tracks",
}

type filterDef struct{ filter, filterType, title string }

// typeFilters are the filters offered per type; each has a value list at
// /library/sections/{id}/{filter} except booleans.
func typeFilters(t int) []filterDef {
	switch t {
	case typeMovie, typeShow:
		return []filterDef{
			{"unwatched", "boolean", "Unplayed"},
			{"genre", "string", "Genre"},
			{"year", "integer", "Year"},
			{"decade", "integer", "Decade"},
			{"contentRating", "string", "Content Rating"},
		}
	case typeEpisode:
		return []filterDef{{"unwatched", "boolean", "Unplayed"}, {"year", "integer", "Year"}}
	case typeArtist, typeAlbum:
		return []filterDef{{"genre", "string", "Genre"}, {"year", "integer", "Year"}, {"decade", "integer", "Decade"}}
	}
	return nil
}

type sortDef struct{ key, title, dir string }

func typeSorts(t int) []sortDef {
	sorts := []sortDef{
		{"titleSort", "Title", "asc"},
		{"addedAt", "Date Added", "desc"},
	}
	switch t {
	case typeMovie, typeShow, typeEpisode:
		sorts = append(sorts,
			sortDef{"originallyAvailableAt", "Release Date", "desc"},
			sortDef{"audienceRating", "Audience Rating", "desc"},
			sortDef{"lastViewedAt", "Last Viewed", "desc"})
	case typeAlbum:
		sorts = append(sorts, sortDef{"year", "Year", "desc"})
	}
	return append(sorts, sortDef{"random", "Randomly", "asc"})
}

// sectionMeta builds the Meta block for a section, marking the type and
// sort ("key" or "key:desc") currently in use as active.
func (s *Server) sectionMeta(sec *section, active int, activeSort string) *Element {
	sortKey, sortDir, _ := strings.Cut(activeSort, ":")
	if sortKey == "" {
		sortKey = "titleSort"
	}
	if sortDir == "" {
		sortDir = "asc"
	}
	meta := E("Meta")
	meta.Single = true
	for _, t := range sectionTypes[sec.Type] {
		te := E("Type").
			A("key", fmt.Sprintf("/library/sections/%d/all?type=%d", sec.ID, t)).
			A("type", plexTypes[plexTypeNums[t]]).
			A("title", typeTitles[t]).
			A("active", t == active)
		for _, f := range typeFilters(t) {
			te.Add(E("Filter").
				A("filter", f.filter).
				A("filterType", f.filterType).
				A("key", fmt.Sprintf("/library/sections/%d/%s?type=%d", sec.ID, f.filter, t)).
				A("title", f.title).
				A("type", "filter"))
		}
		for i, so := range typeSorts(t) {
			e := E("Sort").
				A("defaultDirection", so.dir).
				A("descKey", so.key+":desc").
				A("key", so.key).
				A("title", so.title)
			if i == 0 {
				e.A("default", so.dir).
					A("firstCharacterKey", fmt.Sprintf("/library/sections/%d/firstCharacter", sec.ID))
			}
			if t == active && so.key == sortKey {
				e.A("active", true).A("activeDirection", sortDir)
			}
			te.Add(e)
		}
		te.Add(
			E("Field").A("key", "title").A("title", "Title").A("type", "string"),
			E("Field").A("key", "year").A("title", "Year").A("type", "integer"),
			E("Field").A("key", "genre").A("title", "Genre").A("type", "tag"),
		)
		meta.Add(te)
	}
	op := func(key, title string) *Element { return E("Operator").A("key", key).A("title", title) }
	meta.Add(
		E("FieldType").A("type", "tag").Add(op("=", "is"), op("!=", "is not")),
		E("FieldType").A("type", "integer").Add(op("=", "is"), op("!=", "is not"), op(">>=", "is greater than"), op("<<=", "is less than")),
		E("FieldType").A("type", "string").Add(op("=", "contains"), op("!=", "does not contain"), op("==", "is"), op("!==", "is not")),
		E("FieldType").A("type", "boolean").Add(op("=", "is true"), op("!=", "is false")),
	)
	return meta
}

// sectionFilters reads the Plex filter query parameters into a Jellyfin
// items request. Multiple values are comma-separated and ORed.
func sectionFilters(req jellyfin.ApiGetItemsRequest, q url.Values) jellyfin.ApiGetItemsRequest {
	list := func(k string) []string {
		var out []string
		for _, v := range strings.Split(q.Get(k), ",") {
			if v = strings.TrimSpace(v); v != "" {
				out = append(out, v)
			}
		}
		return out
	}
	if g := list("genre"); len(g) > 0 {
		req = req.Genres(g)
	}
	if r := list("contentRating"); len(r) > 0 {
		req = req.OfficialRatings(r)
	}
	var years []int32
	for _, y := range list("year") {
		if n, err := strconv.Atoi(y); err == nil {
			years = append(years, int32(n))
		}
	}
	for _, d := range list("decade") {
		if n, err := strconv.Atoi(d); err == nil {
			for y := n; y < n+10; y++ {
				years = append(years, int32(y))
			}
		}
	}
	if len(years) > 0 {
		req = req.Years(years)
	}
	if q.Get("unwatched") == "1" {
		req = req.IsPlayed(false)
	}
	// People filters, from cast & crew links. Jellyfin filters by one name.
	for key, kind := range map[string]string{"actor": "Actor", "role": "Actor", "director": "Director", "writer": "Writer", "producer": "Producer"} {
		if names := list(key); len(names) > 0 {
			req = req.Person(names[0]).PersonTypes([]string{kind})
		}
	}
	return req
}

// handleFilterValues lists the values for one filter, e.g. every genre in a
// section.
func (s *Server) handleFilterValues(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sec, err := s.sectionParam(r, r.PathValue("id"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	filter := r.PathValue("filter")
	t, err := strconv.Atoi(r.URL.Query().Get("type"))
	if err != nil {
		t = sectionItemType(sec.Type)
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	req := s.jf.FilterAPI.GetQueryFiltersLegacy(ctx).UserId(uid).ParentId(sec.GUID)
	if kind, ok := plexTypeNums[t]; ok {
		req = req.IncludeItemTypes([]jellyfin.BaseItemKind{kind})
	}
	f, _, err := req.Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	type value struct{ key, title string }
	var values []value
	switch filter {
	case "genre":
		for _, g := range f.GetGenres() {
			values = append(values, value{g, g})
		}
	case "contentRating":
		for _, c := range f.GetOfficialRatings() {
			values = append(values, value{c, c})
		}
	case "year":
		ys := f.GetYears()
		sort.Slice(ys, func(i, j int) bool { return ys[i] > ys[j] })
		for _, y := range ys {
			values = append(values, value{fmt.Sprint(y), fmt.Sprint(y)})
		}
	case "decade":
		seen := map[int32]bool{}
		var ds []int32
		for _, y := range f.GetYears() {
			if d := y / 10 * 10; !seen[d] {
				seen[d] = true
				ds = append(ds, d)
			}
		}
		sort.Slice(ds, func(i, j int) bool { return ds[i] > ds[j] })
		for _, d := range ds {
			values = append(values, value{fmt.Sprint(d), fmt.Sprintf("%ds", d)})
		}
	default:
		http.NotFound(w, r)
		return
	}
	mc := s.sectionContainer(sec).A("title2", filter)
	for _, v := range values {
		mc.Add(E("Directory").
			A("key", v.key).
			A("title", v.title).
			A("fastKey", fmt.Sprintf("/library/sections/%d/all?type=%d&%s=%s", sec.ID, t, filter, url.QueryEscape(v.key))))
	}
	write(w, r, mc)
}

// handleFirstCharacter counts items by the first character of their sort
// title, for the A-Z jump bar.
func (s *Server) handleFirstCharacter(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sec, err := s.sectionParam(r, r.PathValue("id"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	t, err := strconv.Atoi(r.URL.Query().Get("type"))
	if err != nil {
		t = sectionItemType(sec.Type)
	}
	kind, ok := plexTypeNums[t]
	if !ok {
		http.Error(w, "unsupported type", http.StatusBadRequest)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	req := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ParentId(sec.GUID).Recursive(true).
		IncludeItemTypes([]jellyfin.BaseItemKind{kind}).Fields([]jellyfin.ItemFields{"SortName"}).
		SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_SORT_NAME}).EnableImages(false).EnableUserData(false)
	res, _, err := sectionFilters(req, r.URL.Query()).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	var order []string
	counts := map[string]int{}
	for _, it := range res.Items {
		c := firstChar(it.GetSortName())
		if counts[c] == 0 {
			order = append(order, c)
		}
		counts[c]++
	}
	mc := s.sectionContainer(sec)
	for _, c := range order {
		mc.Add(E("Directory").A("size", counts[c]).A("key", c).A("title", c))
	}
	write(w, r, mc)
}

func firstChar(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) {
			return string(unicode.ToUpper(r))
		}
		return "#"
	}
	return "#"
}
