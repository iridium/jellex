package plex

import (
	"context"
	"fmt"
	"math/rand/v2"
	"net/http"
	"regexp"
	"strconv"
	"sync"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// playQueue is an ordered list of items a client is playing. Queues live in
// memory only; clients recreate them freely.
type playQueue struct {
	ID       int
	Version  int
	URI      string
	Items    []string // Jellyfin item GUIDs
	ItemIDs  []int    // playQueueItemID per item
	Selected int      // index into Items
}

type playQueues struct {
	mu     sync.Mutex
	next   int
	byID   map[int]*playQueue
	itemID int
}

func (q *playQueues) add(pq *playQueue) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.byID == nil {
		q.byID = map[int]*playQueue{}
	}
	q.next++
	pq.ID = q.next
	pq.Version = 1
	for range pq.Items {
		q.itemID++
		pq.ItemIDs = append(pq.ItemIDs, q.itemID)
	}
	q.byID[pq.ID] = pq
}

func (q *playQueues) get(id int) (*playQueue, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	pq, ok := q.byID[id]
	return pq, ok
}

func (s *Server) playQueueRoutes() {
	s.mux.HandleFunc("POST /playQueues", s.handleCreatePlayQueue)
	s.mux.HandleFunc("GET /playQueues/{id}", s.handleGetPlayQueue)
}

// uriKey extracts the item key from a play queue source URI such as
// server://<machine>/com.plexapp.plugins.library/library/metadata/7.
var uriKey = regexp.MustCompile(`/library/(?:metadata|collections)/(\d+)(/children|/items)?$`)

func (s *Server) handleCreatePlayQueue(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	q := r.URL.Query()
	uri := q.Get("uri")
	if id, err := strconv.Atoi(q.Get("playlistID")); err == nil {
		s.createPlaylistQueue(w, r, id)
		return
	}
	if m := playlistURI.FindStringSubmatch(uri); m != nil {
		id, _ := strconv.Atoi(m[1])
		s.createPlaylistQueue(w, r, id)
		return
	}
	m := uriKey.FindStringSubmatch(uri)
	if m == nil {
		http.Error(w, "unsupported play queue uri", http.StatusBadRequest)
		return
	}
	n, _ := strconv.Atoi(m[1])
	guid, ok := s.itemGUID(n)
	if !ok {
		http.NotFound(w, r)
		return
	}
	items, start, err := s.expandPlayable(ctx, guid, q.Get("continuous") == "1")
	if err != nil {
		fail(w, r, err)
		return
	}
	if len(items) == 0 {
		http.Error(w, "nothing playable", http.StatusBadRequest)
		return
	}
	// key= selects where in the queue to start, e.g. a specific episode.
	if km := uriKey.FindStringSubmatch(q.Get("key")); km != nil {
		if kn, err := strconv.Atoi(km[1]); err == nil {
			if kg, ok := s.itemGUID(kn); ok {
				for i, g := range items {
					if g == kg {
						start = i
					}
				}
			}
		}
	}
	pq := &playQueue{URI: uri, Items: items, Selected: start}
	s.queues.add(pq)
	s.writePlayQueue(w, r, pq)
}

var playlistURI = regexp.MustCompile(`/playlists/(\d+)(/items)?$`)

// createPlaylistQueue starts a play queue from a playlist, in playlist order.
func (s *Server) createPlaylistQueue(w http.ResponseWriter, r *http.Request, playlistRK int) {
	ctx := r.Context()
	guid, ok := s.itemGUID(playlistRK)
	if !ok {
		http.NotFound(w, r)
		return
	}
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	res, _, err := s.jf.PlaylistAPI.GetPlaylistItems(ctx, guid).UserId(uid).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	var items []string
	for _, it := range res.Items {
		items = append(items, it.GetId())
	}
	if len(items) == 0 {
		http.Error(w, "playlist is empty", http.StatusBadRequest)
		return
	}
	pq := &playQueue{URI: fmt.Sprintf("library://x/playlist/%d", playlistRK), Items: items}
	if r.URL.Query().Get("shuffle") == "1" {
		rand.Shuffle(len(pq.Items), func(i, j int) { pq.Items[i], pq.Items[j] = pq.Items[j], pq.Items[i] })
	}
	s.queues.add(pq)
	s.writePlayQueue(w, r, pq)
}

func (s *Server) handleGetPlayQueue(w http.ResponseWriter, r *http.Request) {
	id, _ := strconv.Atoi(r.PathValue("id"))
	pq, ok := s.queues.get(id)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.writePlayQueue(w, r, pq)
}

// expandPlayable turns an item into the list of playable items it stands for
// (a show or season becomes its episodes, an album its tracks) and picks
// where to start: the first unwatched episode, or the beginning. With
// continuous set, an episode expands to its whole show, starting at itself.
func (s *Server) expandPlayable(ctx context.Context, guid string, continuous bool) ([]string, int, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, 0, err
	}
	it, _, err := s.jf.LibraryAPI.GetItem(ctx, guid).UserId(uid).Execute()
	if err != nil {
		return nil, 0, err
	}
	var list []jellyfin.BaseItemDto
	switch it.GetType() {
	case jellyfin.BASEITEMKIND_EPISODE:
		if !continuous {
			return []string{guid}, 0, nil
		}
		res, _, err := s.jf.ShowAPI.GetEpisodes(ctx, it.GetSeriesId()).UserId(uid).Execute()
		if err != nil {
			return nil, 0, err
		}
		out := make([]string, len(res.Items))
		start := 0
		for i := range res.Items {
			out[i] = res.Items[i].GetId()
			if out[i] == guid {
				start = i
			}
		}
		return out, start, nil
	case jellyfin.BASEITEMKIND_SERIES:
		res, _, err := s.jf.ShowAPI.GetEpisodes(ctx, guid).UserId(uid).Execute()
		if err != nil {
			return nil, 0, err
		}
		list = res.Items
	case jellyfin.BASEITEMKIND_SEASON:
		res, _, err := s.jf.ShowAPI.GetEpisodes(ctx, it.GetSeriesId()).SeasonId(guid).UserId(uid).Execute()
		if err != nil {
			return nil, 0, err
		}
		list = res.Items
	case jellyfin.BASEITEMKIND_BOX_SET:
		res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ParentId(guid).
			SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_PRODUCTION_YEAR, jellyfin.ITEMSORTBY_SORT_NAME}).Execute()
		if err != nil {
			return nil, 0, err
		}
		out := make([]string, len(res.Items))
		for i := range res.Items {
			out[i] = res.Items[i].GetId()
		}
		return out, 0, nil
	case jellyfin.BASEITEMKIND_MUSIC_ALBUM, jellyfin.BASEITEMKIND_MUSIC_ARTIST:
		req := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Recursive(true).
			IncludeItemTypes([]jellyfin.BaseItemKind{jellyfin.BASEITEMKIND_AUDIO}).
			SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_ALBUM, jellyfin.ITEMSORTBY_PARENT_INDEX_NUMBER, jellyfin.ITEMSORTBY_INDEX_NUMBER})
		if it.GetType() == jellyfin.BASEITEMKIND_MUSIC_ALBUM {
			req = req.ParentId(guid)
		} else {
			req = req.ArtistIds([]string{guid})
		}
		res, _, err := req.Execute()
		if err != nil {
			return nil, 0, err
		}
		list = res.Items
	default:
		return []string{guid}, 0, nil
	}
	start := 0
	for i := range list {
		if ud := list[i].GetUserData(); !ud.GetPlayed() {
			start = i
			break
		}
	}
	out := make([]string, len(list))
	for i := range list {
		out[i] = list[i].GetId()
	}
	return out, start, nil
}

func (s *Server) writePlayQueue(w http.ResponseWriter, r *http.Request, pq *playQueue) {
	ctx := r.Context()
	uid, err := s.user(ctx)
	if err != nil {
		fail(w, r, err)
		return
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Ids(pq.Items).Fields(detailFields).Execute()
	if err != nil {
		fail(w, r, err)
		return
	}
	byID := map[string]*jellyfin.BaseItemDto{}
	for i := range res.Items {
		byID[res.Items[i].GetId()] = &res.Items[i]
	}
	var sec *section
	if len(pq.Items) > 0 {
		if sec, err = s.sectionFor(ctx, pq.Items[pq.Selected]); err != nil {
			fail(w, r, err)
			return
		}
	}
	mc := Container().
		A("size", len(pq.Items)).
		A("identifier", "com.plexapp.plugins.library").
		A("mediaTagPrefix", "/system/bundle/media/flags/").
		A("mediaTagVersion", 1).
		A("playQueueID", pq.ID).
		A("playQueueSelectedItemID", pq.ItemIDs[pq.Selected]).
		A("playQueueSelectedItemOffset", pq.Selected).
		A("playQueueSelectedMetadataItemID", fmt.Sprint(s.rk(pq.Items[pq.Selected]))).
		A("playQueueShuffled", false).
		A("playQueueSourceURI", pq.URI).
		A("playQueueTotalCount", len(pq.Items)).
		A("playQueueVersion", pq.Version)
	for i, g := range pq.Items {
		it, ok := byID[g]
		if !ok {
			continue
		}
		md := s.metadata(it, sec, true).A("playQueueItemID", pq.ItemIDs[i])
		s.addPlaybackExtras(ctx, r, md, it)
		mc.Add(md)
	}
	write(w, r, mc)
}
