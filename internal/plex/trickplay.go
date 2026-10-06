package plex

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"net/http"
	"strconv"
	"strings"
	"sync"

	jellyfin "github.com/sj14/jellyfin-go/api"
)

// Plex Web asks for seek previews one frame at a time:
// /library/parts/{part}/indexes/{sd|hd}/{offsetMs}. Jellyfin stores them as
// trickplay sprite sheets (a grid of frames at a fixed interval), so each
// request is answered by cropping one tile out of the right sheet.

// sheetCache keeps recently decoded sprite sheets; scrubbing hits the same
// sheet many times in a row.
type sheetCache struct {
	mu    sync.Mutex
	order []string
	byKey map[string]image.Image
}

const maxSheets = 16

func (c *sheetCache) get(key string) (image.Image, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	img, ok := c.byKey[key]
	return img, ok
}

func (c *sheetCache) put(key string, img image.Image) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.byKey == nil {
		c.byKey = map[string]image.Image{}
	}
	if _, ok := c.byKey[key]; ok {
		return
	}
	if len(c.order) >= maxSheets {
		delete(c.byKey, c.order[0])
		c.order = c.order[1:]
	}
	c.order = append(c.order, key)
	c.byKey[key] = img
}

func (s *Server) trickplayRoutes() {
	s.mux.HandleFunc("GET /library/parts/{id}/indexes/{index}/{offset}", s.handleIndexImage)
}

func (s *Server) handleIndexImage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	n, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	offset, err := strconv.ParseInt(r.PathValue("offset"), 10, 64)
	if err != nil || offset < 0 {
		http.NotFound(w, r)
		return
	}
	key, ok := s.ids.GUID(n)
	item, source, found := strings.Cut(strings.TrimPrefix(key, "part:"), ":")
	if !ok || !strings.HasPrefix(key, "part:") || !found {
		http.NotFound(w, r)
		return
	}
	info, err := s.trickplayInfo(ctx, item, source, r.PathValue("index") == "hd")
	if err != nil {
		failOr404(w, r, err)
		return
	}

	perSheet := int64(info.GetTileWidth() * info.GetTileHeight())
	frame := offset / int64(info.GetInterval())
	frame = min(frame, int64(info.GetThumbnailCount())-1)
	sheet := frame / perSheet
	cell := frame % perSheet
	tw, th := int(info.GetWidth()), int(info.GetHeight())

	img, err := s.trickplaySheet(ctx, item, source, int(info.GetWidth()), int(sheet))
	if err != nil {
		failOr404(w, r, err)
		return
	}
	x := int(cell%int64(info.GetTileWidth())) * tw
	y := int(cell/int64(info.GetTileWidth())) * th
	rect := image.Rect(x, y, x+tw, y+th).Intersect(img.Bounds())
	if rect.Empty() {
		http.NotFound(w, r)
		return
	}
	tile := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(tile, tile.Bounds(), img, rect.Min, draw.Src)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, tile, &jpeg.Options{Quality: 85}); err != nil {
		fail(w, r, err)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Write(buf.Bytes())
}

// trickplayInfo picks the trickplay resolution for a media source: the
// smallest for sd, the largest for hd.
func (s *Server) trickplayInfo(ctx context.Context, item, source string, hd bool) (*jellyfin.TrickplayInfoDto, error) {
	uid, err := s.user(ctx)
	if err != nil {
		return nil, err
	}
	res, _, err := s.jf.LibraryAPI.GetItems(ctx).UserId(uid).Ids([]string{item}).
		Fields([]jellyfin.ItemFields{jellyfin.ITEMFIELDS_TRICKPLAY}).Execute()
	if err != nil {
		return nil, err
	}
	if len(res.Items) == 0 {
		return nil, errNotFound
	}
	var best *jellyfin.TrickplayInfoDto
	for _, info := range res.Items[0].GetTrickplay()[source] {
		if info.GetInterval() <= 0 || info.GetTileWidth() <= 0 || info.GetTileHeight() <= 0 {
			continue
		}
		if best == nil || (hd && info.GetWidth() > best.GetWidth()) || (!hd && info.GetWidth() < best.GetWidth()) {
			best = &info
		}
	}
	if best == nil {
		return nil, errNotFound
	}
	return best, nil
}

func (s *Server) trickplaySheet(ctx context.Context, item, source string, width, sheet int) (image.Image, error) {
	key := fmt.Sprintf("%s/%s/%d/%d", item, source, width, sheet)
	if img, ok := s.sheets.get(key); ok {
		return img, nil
	}
	u := fmt.Sprintf("%s/Videos/%s/Trickplay/%d/%d.jpg?mediaSourceId=%s",
		strings.TrimRight(s.cfg.JellyfinURL, "/"), item, width, sheet, source)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", fmt.Sprintf(`MediaBrowser Token="%s"`, s.cfg.JellyfinAPIKey))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, errNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("trickplay sheet: %s", resp.Status)
	}
	img, err := jpeg.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("decode trickplay sheet: %w", err)
	}
	s.sheets.put(key, img)
	return img, nil
}
