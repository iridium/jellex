package plex

import (
	"context"
	"fmt"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// Plex extra types, keyed by Jellyfin's.
var extraTypes = map[jellyfin.ExtraType]struct {
	num     int
	subtype string
}{
	jellyfin.EXTRATYPE_TRAILER:           {1, "trailer"},
	jellyfin.EXTRATYPE_DELETED_SCENE:     {2, "deletedScene"},
	jellyfin.EXTRATYPE_INTERVIEW:         {3, "interview"},
	jellyfin.EXTRATYPE_BEHIND_THE_SCENES: {5, "behindTheScenes"},
	jellyfin.EXTRATYPE_SCENE:             {6, "sceneOrSample"},
	jellyfin.EXTRATYPE_SAMPLE:            {6, "sceneOrSample"},
	jellyfin.EXTRATYPE_FEATURETTE:        {10, "featurette"},
	jellyfin.EXTRATYPE_SHORT:             {11, "short"},
	jellyfin.EXTRATYPE_CLIP:              {10, "featurette"},
}

// extras returns an item's trailers and special features as Plex clips,
// trailers first, and the key of the first trailer (Plex's primaryExtraKey).
func (s *Server) extras(ctx context.Context, it *jellyfin.BaseItemDto, sec *section) ([]*Element, string, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, "", err
	}
	var items []jellyfin.BaseItemDto
	if it.GetLocalTrailerCount() > 0 {
		trailers, _, err := s.jf.LibraryAPI.GetLocalTrailers(ctx, it.GetId()).UserId(uid).Execute()
		if err != nil {
			return nil, "", err
		}
		items = append(items, trailers...)
	}
	if it.GetSpecialFeatureCount() > 0 {
		features, _, err := s.jf.LibraryAPI.GetSpecialFeatures(ctx, it.GetId()).UserId(uid).Execute()
		if err != nil {
			return nil, "", err
		}
		items = append(items, features...)
	}
	var out []*Element
	primary := ""
	for i := range items {
		x := &items[i]
		t, ok := extraTypes[x.GetExtraType()]
		if !ok {
			continue
		}
		md := s.metadata(x, sec, false).
			A("type", "clip").
			A("subtype", t.subtype).
			A("extraType", t.num)
		if primary == "" && t.num == 1 {
			primary = fmt.Sprint(md.Get("key"))
		}
		out = append(out, md)
	}
	return out, primary, nil
}
