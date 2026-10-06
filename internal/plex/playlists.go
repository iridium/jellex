package plex

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// Playlists map onto the user's Jellyfin playlists. Plex identifies entries
// within a playlist by playlistItemID, which maps to Jellyfin's entry IDs.

func (s *Server) playlistRoutes() {
	m := s.mux
	m.HandleFunc("GET /playlists", s.handlePlaylists)
	m.HandleFunc("GET /playlists/all", s.handlePlaylists)
	m.HandleFunc("POST /playlists", s.handleCreatePlaylist)
	m.HandleFunc("GET /playlists/{id}", s.handlePlaylist)
	m.HandleFunc("PUT /playlists/{id}", s.handleRenamePlaylist)
	m.HandleFunc("DELETE /playlists/{id}", s.handleDeletePlaylist)
	m.HandleFunc("GET /playlists/{id}/items", s.handlePlaylistItems)
	m.HandleFunc("PUT /playlists/{id}/items", s.handleAddToPlaylist)
	m.HandleFunc("DELETE /playlists/{id}/items/{item}", s.handleRemoveFromPlaylist)
	m.HandleFunc("PUT /playlists/{id}/items/{item}/move", s.handleMovePlaylistItem)
	m.HandleFunc("GET /playlists/{id}/composite/{ts}", s.handlePlaylistComposite)
}

func playlistType(it *jellyfin.BaseItemDto) string {
	if it.GetMediaType() == jellyfin.MEDIATYPE_AUDIO {
		return "audio"
	}
	return "video"
}

func (s *Server) playlistMetadata(it *jellyfin.BaseItemDto) *Element {
	rk := s.rk(it.GetId())
	tag := primaryTag(it)
	m := E("Playlist").
		A("ratingKey", fmt.Sprint(rk)).
		A("key", fmt.Sprintf("/playlists/%d/items", rk)).
		A("guid", "com.plexapp.agents.none://"+it.GetId()).
		A("type", "playlist").
		A("title", it.GetName()).
		Opt("summary", it.GetOverview()).
		A("smart", false).
		A("playlistType", playlistType(it)).
		A("leafCount", int(it.GetChildCount())).
		A("composite", fmt.Sprintf("/playlists/%d/composite/%d", rk, imageStamp(tag)))
	if d := it.GetCumulativeRunTimeTicks(); d > 0 {
		m.A("duration", d/ticksPerMs)
	}
	if c := it.GetDateCreated(); !c.IsZero() {
		m.A("addedAt", c.Unix()).A("updatedAt", c.Unix())
	}
	m.JSONTag = "Metadata"
	return m
}

// listPlaylists returns the user's playlists, optionally only one type.
func (s *Server) listPlaylists(ctx context.Context, kind string) ([]*Element, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Recursive(true).
		IncludeItemTypes([]jellyfin.BaseItemKind{jellyfin.BASEITEMKIND_PLAYLIST}).
		Fields([]jellyfin.ItemFields{"ChildCount", "DateCreated", "Overview"}).
		SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_SORT_NAME}).Execute()
	if err != nil {
		return nil, err
	}
	var out []*Element
	for i := range res.Items {
		if kind != "" && playlistType(&res.Items[i]) != kind {
			continue
		}
		out = append(out, s.playlistMetadata(&res.Items[i]))
	}
	return out, nil
}

func (s *Server) handlePlaylists(w http.ResponseWriter, r *http.Request) {
	items, err := s.listPlaylists(r.Context(), r.URL.Query().Get("playlistType"))
	if err != nil {
		fail(w, r, err)
		return
	}
	writeList(w, r, Container().A("identifier", "com.plexapp.plugins.library").A("title1", "Playlists"), items)
}

// playlistParam resolves a playlist ratingKey to its Jellyfin item.
func (s *Server) playlistParam(r *http.Request) (*jellyfin.BaseItemDto, error) {
	guid, err := s.guidParam(r, "id")
	if err != nil {
		return nil, err
	}
	uid, err := s.user(r.Context())
	if err != nil {
		return nil, err
	}
	res, _, err := s.jf.LibraryAPI.GetItems(r.Context()).UserId(uid).Ids([]string{guid}).
		Fields([]jellyfin.ItemFields{"ChildCount", "DateCreated", "Overview"}).Execute()
	if err != nil {
		return nil, err
	}
	if len(res.Items) == 0 || res.Items[0].GetType() != jellyfin.BASEITEMKIND_PLAYLIST {
		return nil, errNotFound
	}
	return &res.Items[0], nil
}

func (s *Server) handlePlaylist(w http.ResponseWriter, r *http.Request) {
	pl, err := s.playlistParam(r)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	write(w, r, Container().A("identifier", "com.plexapp.plugins.library").Add(s.playlistMetadata(pl)))
}

func (s *Server) handlePlaylistItems(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pl, err := s.playlistParam(r)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	res, _, err := s.jf.PlaylistAPI.GetPlaylistItems(ctx, pl.GetId()).UserId(uid).Fields(listFields).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	mc := Container().
		A("identifier", "com.plexapp.plugins.library").
		A("ratingKey", s.rk(pl.GetId())).
		A("title", pl.GetName()).
		A("playlistType", playlistType(pl)).
		A("smart", false).
		A("leafCount", len(res.Items)).
		A("composite", s.playlistMetadata(pl).Get("composite"))
	var items []*Element
	for i := range res.Items {
		md := s.metadata(&res.Items[i], nil, false)
		md.A("playlistItemID", s.ids.ID("pli:"+res.Items[i].GetPlaylistItemId()))
		items = append(items, md)
	}
	writeList(w, r, mc, items)
}

// uriItems resolves a Plex item URI (server://…/library/metadata/1,2,3) to
// the playable Jellyfin items it covers, expanding shows, seasons and albums.
func (s *Server) uriItems(ctx context.Context, uri string) ([]string, error) {
	i := strings.Index(uri, "/library/metadata/")
	if i < 0 {
		return nil, fmt.Errorf("unsupported uri %q", uri)
	}
	keys := strings.TrimPrefix(uri[i:], "/library/metadata/")
	keys, _, _ = strings.Cut(keys, "/")
	var out []string
	for _, k := range strings.Split(keys, ",") {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		g, ok := s.itemGUID(n)
		if !ok {
			continue
		}
		items, _, err := s.expandPlayable(ctx, g, false)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	if len(out) == 0 {
		return nil, errNotFound
	}
	return out, nil
}

func (s *Server) handleCreatePlaylist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	if q.Get("smart") == "1" {
		http.Error(w, "smart playlists are not supported", http.StatusBadRequest)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	var ids []string
	if uri := q.Get("uri"); uri != "" {
		if ids, err = s.uriItems(ctx, uri); err != nil {
			failOr404(w, r, err)
			return
		}
	}
	var dto jellyfin.CreatePlaylistDto
	dto.SetName(q.Get("title"))
	dto.SetIds(ids)
	dto.SetUserId(uid)
	if q.Get("type") == "audio" {
		dto.SetMediaType(jellyfin.MEDIATYPE_AUDIO)
	} else {
		dto.SetMediaType(jellyfin.MEDIATYPE_VIDEO)
	}
	created, _, err := s.jf.PlaylistAPI.CreatePlaylist(ctx).CreatePlaylistDto(dto).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Ids([]string{created.GetId()}).
		Fields([]jellyfin.ItemFields{"ChildCount", "DateCreated"}).Execute()
	if err != nil || len(res.Items) == 0 {
		fail(w, r, fmt.Errorf("reload created playlist: %v", err))
		return
	}
	write(w, r, Container().Add(s.playlistMetadata(&res.Items[0])))
}

func (s *Server) handleAddToPlaylist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pl, err := s.playlistParam(r)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	ids, err := s.uriItems(ctx, r.URL.Query().Get("uri"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	if _, err := s.jf.PlaylistAPI.AddItemToPlaylist(ctx, pl.GetId()).Ids(ids).UserId(uid).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	pl.SetChildCount(pl.GetChildCount() + int32(len(ids)))
	write(w, r, Container().A("leafCountAdded", len(ids)).A("leafCountRequested", len(ids)).Add(s.playlistMetadata(pl)))
}

// entryParam maps a playlistItemID back to Jellyfin's playlist entry ID.
func (s *Server) entryParam(r *http.Request) (string, bool) {
	n, err := strconv.Atoi(r.PathValue("item"))
	if err != nil {
		return "", false
	}
	g, ok := s.ids.GUID(n)
	entry, found := strings.CutPrefix(g, "pli:")
	return entry, ok && found
}

func (s *Server) handleRemoveFromPlaylist(w http.ResponseWriter, r *http.Request) {
	pl, err := s.playlistParam(r)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	entry, ok := s.entryParam(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	if _, err := s.jf.PlaylistAPI.RemoveItemFromPlaylist(r.Context(), pl.GetId()).EntryIds([]string{entry}).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	s.handlePlaylistItems(w, r)
}

// handleMovePlaylistItem moves an entry to just after another (after=
// playlistItemID), or to the top when after is absent.
func (s *Server) handleMovePlaylistItem(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	pl, err := s.playlistParam(r)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	entry, ok := s.entryParam(r)
	if !ok {
		http.NotFound(w, r)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	res, _, err := s.jf.PlaylistAPI.GetPlaylistItems(ctx, pl.GetId()).UserId(uid).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	afterEntry := ""
	if after, err := strconv.Atoi(r.URL.Query().Get("after")); err == nil {
		g, _ := s.ids.GUID(after)
		afterEntry = strings.TrimPrefix(g, "pli:")
	}
	// Build the new order: everything except the moved entry, with it
	// reinserted after afterEntry (or first).
	var moved string
	var rest []jellyfin.BaseItemDto
	for _, it := range res.Items {
		if it.GetPlaylistItemId() == entry {
			moved = it.GetId()
			continue
		}
		rest = append(rest, it)
	}
	if moved == "" {
		http.NotFound(w, r)
		return
	}
	var order []string
	if afterEntry == "" {
		order = append(order, moved)
	}
	for _, it := range rest {
		order = append(order, it.GetId())
		if it.GetPlaylistItemId() == afterEntry {
			order = append(order, moved)
		}
	}
	// Jellyfin's move endpoint needs a user session, which an API key lacks,
	// so rewrite the playlist in the new order instead. Not atomic: a failure
	// between the two calls leaves the playlist empty.
	var entries []string
	for _, it := range res.Items {
		entries = append(entries, it.GetPlaylistItemId())
	}
	if _, err := s.jf.PlaylistAPI.RemoveItemFromPlaylist(ctx, pl.GetId()).EntryIds(entries).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	if _, err := s.jf.PlaylistAPI.AddItemToPlaylist(ctx, pl.GetId()).Ids(order).UserId(uid).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	s.handlePlaylistItems(w, r)
}

func (s *Server) handleRenamePlaylist(w http.ResponseWriter, r *http.Request) {
	pl, err := s.playlistParam(r)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	title := r.URL.Query().Get("title")
	if title == "" {
		w.WriteHeader(http.StatusOK)
		return
	}
	// The playlist update endpoint needs a user session, so rename through
	// the generic item update, which an admin API key may use.
	uid, err := s.user(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	full, _, err := s.jf.LibraryAPI.GetItem(r.Context(), pl.GetId()).UserId(uid).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	full.SetName(title)
	if _, err := s.jf.ItemUpdateAPI.UpdateItem(r.Context(), pl.GetId()).BaseItemDto(*full).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleDeletePlaylist(w http.ResponseWriter, r *http.Request) {
	pl, err := s.playlistParam(r)
	if err != nil {
		failOr404(w, r, err)
		return
	}
	if _, err := s.jf.LibraryAPI.DeleteItem(r.Context(), pl.GetId()).Execute(); err != nil {
		fail(w, r, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handlePlaylistComposite(w http.ResponseWriter, r *http.Request) {
	guid, err := s.guidParam(r, "id")
	if err != nil {
		failOr404(w, r, err)
		return
	}
	width, _ := strconv.Atoi(r.URL.Query().Get("width"))
	height, _ := strconv.Atoi(r.URL.Query().Get("height"))
	s.proxyImage(w, r, guid, "Primary", width, height)
}
