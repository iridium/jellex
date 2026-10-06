package plex

import (
	"context"
	"fmt"
	"net/http"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// Plex metadata type numbers, used in type= filters and section pivots.
const (
	typeMovie   = 1
	typeShow    = 2
	typeSeason  = 3
	typeEpisode = 4
	typeArtist  = 8
	typeAlbum   = 9
	typeTrack   = 10
)

// section is a Jellyfin library view exposed as a Plex library section.
type section struct {
	ID      int
	GUID    string
	Title   string
	Type    string // Plex section type: movie, show, artist
	Agent   string
	Scanner string
	Updated int64
}

// sectionKinds maps Jellyfin collection types to Plex section types.
var sectionKinds = map[jellyfin.CollectionType]struct {
	plexType, agent, scanner string
	itemType                 int
}{
	jellyfin.COLLECTIONTYPE_MOVIES:  {"movie", "tv.plex.agents.movie", "Plex Movie", typeMovie},
	jellyfin.COLLECTIONTYPE_TVSHOWS: {"show", "tv.plex.agents.series", "Plex TV Series", typeShow},
	jellyfin.COLLECTIONTYPE_MUSIC:   {"artist", "tv.plex.agents.music", "Plex Music", typeArtist},
}

func (s *Server) sections(ctx context.Context) ([]section, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	views, _, err := s.jf.UserViewAPI.GetUserViews(ctx).UserId(uid).Execute()
	if err != nil {
		return nil, fmt.Errorf("get user views: %w", err)
	}
	var out []section
	for _, v := range views.Items {
		kind, ok := sectionKinds[v.GetCollectionType()]
		if !ok {
			continue
		}
		out = append(out, section{
			ID:      s.ids.ID(v.GetId()),
			GUID:    v.GetId(),
			Title:   v.GetName(),
			Type:    kind.plexType,
			Agent:   kind.agent,
			Scanner: kind.scanner,
			Updated: v.GetDateCreated().Unix(),
		})
	}
	return out, nil
}

func sectionItemType(plexType string) int {
	for _, k := range sectionKinds {
		if k.plexType == plexType {
			return k.itemType
		}
	}
	return 0
}

// sectionDirectory renders a section. Under /media/providers keys are
// absolute; under /library/sections they are relative IDs.
func (s *Server) sectionDirectory(sec section, absolute bool) *Element {
	key := fmt.Sprint(sec.ID)
	if absolute {
		key = fmt.Sprintf("/library/sections/%d", sec.ID)
	}
	d := E("Directory").
		A("allowSync", false).
		A("art", fmt.Sprintf("/:/resources/%s-fanart.jpg", sec.Type)).
		A("composite", fmt.Sprintf("/library/sections/%d/composite/%d", sec.ID, sec.Updated)).
		A("filters", true).
		A("refreshing", false).
		A("thumb", fmt.Sprintf("/:/resources/%s.png", sec.Type)).
		A("key", key).
		A("type", sec.Type).
		A("title", sec.Title).
		A("agent", sec.Agent).
		A("scanner", sec.Scanner).
		A("language", "en-US").
		A("uuid", sec.GUID).
		A("updatedAt", sec.Updated).
		A("createdAt", sec.Updated).
		A("scannedAt", sec.Updated).
		A("content", true).
		A("directory", true).
		A("contentChangedAt", sec.Updated).
		A("hidden", 0)
	if absolute {
		d.A("id", fmt.Sprint(sec.ID)).A("hubKey", fmt.Sprintf("/hubs/sections/%d", sec.ID))
		d.Add(
			E("Pivot").A("id", "recommended").A("key", fmt.Sprintf("/hubs/sections/%d", sec.ID)).A("type", "hub").A("title", "Recommended").A("context", "content.discover").A("symbol", "star"),
			E("Pivot").A("id", "library").A("key", fmt.Sprintf("/library/sections/%d/all?type=%d", sec.ID, sectionItemType(sec.Type))).A("type", "list").A("title", "Library").A("context", "content.library").A("symbol", "library"),
		)
		if sec.Type == "movie" || sec.Type == "show" {
			d.Add(E("Pivot").A("id", "collections").A("key", fmt.Sprintf("/library/sections/%d/collections", sec.ID)).A("type", "list").A("title", "Collections").A("context", "content.collections").A("symbol", "stack"))
		}
	} else {
		d.Add(E("Location").A("id", sec.ID).A("path", "/"+sec.Title))
	}
	return d
}

func (s *Server) handleSections(w http.ResponseWriter, r *http.Request) {
	secs, err := s.sections(r.Context())
	if err != nil {
		fail(w, r, err)
		return
	}
	mc := Container().A("allowSync", false).A("title1", "Plex Library")
	for _, sec := range secs {
		mc.Add(s.sectionDirectory(sec, false))
	}
	write(w, r, mc)
}
