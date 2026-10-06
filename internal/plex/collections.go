package plex

import (
	"context"
	"fmt"
	"net/http"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// Collections are Jellyfin box sets. A library's collections are the box
// sets containing items from it, which Jellyfin lists under the library.

// typeCollection is Plex's metadata type number for collections.
const typeCollection = 18

func (s *Server) collectionRoutes() {
	s.mux.HandleFunc("GET /library/sections/{id}/collections", s.handleSectionCollections)
	s.mux.HandleFunc("GET /library/collections/{id}/children", s.handleChildren)
	s.mux.HandleFunc("GET /library/collections/{id}/related", s.handleRelated)
	s.mux.HandleFunc("GET /library/collections/{id}", s.handleCollectionRedirect)
}

func (s *Server) collections(ctx context.Context, sec *section) ([]*Element, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ParentId(sec.GUID).Recursive(true).
		IncludeItemTypes([]jellyfin.BaseItemKind{jellyfin.BASEITEMKIND_BOX_SET}).Fields(listFields).
		SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_SORT_NAME}).Execute()
	if err != nil {
		return nil, err
	}
	var out []*Element
	for i := range res.Items {
		md := s.metadata(&res.Items[i], sec, false)
		if md.Get("thumb") == nil {
			s.collectionThumb(ctx, &res.Items[i], md)
		}
		out = append(out, md.A("subtype", sec.Type))
	}
	return out, nil
}

// collectionThumb uses the first member's poster for a collection without
// artwork of its own.
func (s *Server) collectionThumb(ctx context.Context, coll *jellyfin.BaseItemDto, md *Element) {
	uid, err := s.user(ctx)
	if err != nil {
		return
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).ParentId(coll.GetId()).Limit(1).
		SortBy([]jellyfin.ItemSortBy{jellyfin.ITEMSORTBY_PRODUCTION_YEAR}).Execute()
	if err != nil || len(res.Items) == 0 {
		return
	}
	md.Opt("thumb", s.imageURL(res.Items[0].GetId(), "thumb", primaryTag(&res.Items[0])))
}

func (s *Server) handleSectionCollections(w http.ResponseWriter, r *http.Request) {
	sec, err := s.sectionParam(r, r.PathValue("id"))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	items, err := s.collections(r.Context(), sec)
	if err != nil {
		fail(w, r, err)
		return
	}
	writeList(w, r, s.sectionContainer(sec).A("title2", "Collections").A("viewGroup", "collection"), items)
}

// handleCollectionRedirect sends /library/collections/{id} to the item's
// metadata, which is where collection details live.
func (s *Server) handleCollectionRedirect(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, fmt.Sprintf("/library/metadata/%s?%s", r.PathValue("id"), r.URL.RawQuery), http.StatusFound)
}
