// Package fmp4 splits muxed fragmented-MP4 (ISO BMFF) streams into one
// stream per track. Jellyfin's HLS transcodes interleave audio and video in
// each segment, while DASH players such as Shaka need them separately.
package fmp4

import (
	"encoding/binary"
	"errors"
	"fmt"
)

// Track describes one track of an init segment.
type Track struct {
	ID      uint32
	Handler string // "vide", "soun", ...
}

type box struct {
	typ        string
	start, end int // whole box, including header
	hdr        int // header length
}

func (b box) body(buf []byte) []byte { return buf[b.start+b.hdr : b.end] }

// boxes lists the boxes laid out back to back in buf[from:to].
func boxes(buf []byte, from, to int) ([]box, error) {
	var out []box
	for off := from; off < to; {
		if to-off < 8 {
			return nil, fmt.Errorf("truncated box header at %d", off)
		}
		size := int(binary.BigEndian.Uint32(buf[off:]))
		typ := string(buf[off+4 : off+8])
		hdr := 8
		switch size {
		case 1:
			if to-off < 16 {
				return nil, fmt.Errorf("truncated large box at %d", off)
			}
			size = int(binary.BigEndian.Uint64(buf[off+8:]))
			hdr = 16
		case 0:
			size = to - off
		}
		if size < hdr || off+size > to {
			return nil, fmt.Errorf("bad %q box size %d at %d", typ, size, off)
		}
		out = append(out, box{typ, off, off + size, hdr})
		off += size
	}
	return out, nil
}

func children(buf []byte, b box) ([]box, error) { return boxes(buf, b.start+b.hdr, b.end) }

func find(bs []box, typ string) (box, bool) {
	for _, b := range bs {
		if b.typ == typ {
			return b, true
		}
	}
	return box{}, false
}

func makeBox(typ string, payload ...[]byte) []byte {
	n := 8
	for _, p := range payload {
		n += len(p)
	}
	out := make([]byte, 8, n)
	binary.BigEndian.PutUint32(out, uint32(n))
	copy(out[4:], typ)
	for _, p := range payload {
		out = append(out, p...)
	}
	return out
}

// Tracks lists the tracks in an init segment.
func Tracks(init []byte) ([]Track, error) {
	top, err := boxes(init, 0, len(init))
	if err != nil {
		return nil, err
	}
	moov, ok := find(top, "moov")
	if !ok {
		return nil, errors.New("init segment has no moov")
	}
	kids, err := children(init, moov)
	if err != nil {
		return nil, err
	}
	var out []Track
	for _, k := range kids {
		if k.typ != "trak" {
			continue
		}
		t, err := trackInfo(init, k)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, nil
}

func trackInfo(buf []byte, trak box) (Track, error) {
	kids, err := children(buf, trak)
	if err != nil {
		return Track{}, err
	}
	tkhd, ok := find(kids, "tkhd")
	if !ok {
		return Track{}, errors.New("trak has no tkhd")
	}
	body := tkhd.body(buf)
	var t Track
	if body[0] == 1 { // version 1: 64-bit times
		t.ID = binary.BigEndian.Uint32(body[20:])
	} else {
		t.ID = binary.BigEndian.Uint32(body[12:])
	}
	if mdia, ok := find(kids, "mdia"); ok {
		mk, err := children(buf, mdia)
		if err != nil {
			return Track{}, err
		}
		if hdlr, ok := find(mk, "hdlr"); ok {
			hb := hdlr.body(buf)
			if len(hb) >= 12 {
				t.Handler = string(hb[8:12])
			}
		}
	}
	return t, nil
}

// SplitInit returns an init segment containing only the given track.
func SplitInit(init []byte, trackID uint32) ([]byte, error) {
	top, err := boxes(init, 0, len(init))
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, b := range top {
		if b.typ != "moov" {
			out = append(out, init[b.start:b.end]...)
			continue
		}
		kids, err := children(init, b)
		if err != nil {
			return nil, err
		}
		var parts [][]byte
		found := false
		for _, k := range kids {
			switch k.typ {
			case "trak":
				t, err := trackInfo(init, k)
				if err != nil {
					return nil, err
				}
				if t.ID == trackID {
					parts = append(parts, init[k.start:k.end])
					found = true
				}
			case "mvex":
				mk, err := children(init, k)
				if err != nil {
					return nil, err
				}
				var keep [][]byte
				for _, m := range mk {
					if m.typ == "trex" && binary.BigEndian.Uint32(m.body(init)[4:]) != trackID {
						continue
					}
					keep = append(keep, init[m.start:m.end])
				}
				parts = append(parts, makeBox("mvex", keep...))
			default:
				parts = append(parts, init[k.start:k.end])
			}
		}
		if !found {
			return nil, fmt.Errorf("track %d not in init segment", trackID)
		}
		out = append(out, makeBox("moov", parts...)...)
	}
	return out, nil
}

// tfhd flags.
const (
	tfhdBaseDataOffset     = 0x000001
	tfhdSampleDescription  = 0x000002
	tfhdDefaultDuration    = 0x000008
	tfhdDefaultSize        = 0x000010
	tfhdDefaultFlags       = 0x000020
	tfhdDefaultBaseIsMoof  = 0x020000
	trunDataOffset         = 0x000001
	trunFirstSampleFlags   = 0x000004
	trunSampleDuration     = 0x000100
	trunSampleSize         = 0x000200
	trunSampleFlags        = 0x000400
	trunSampleCompositeOff = 0x000800
)

// SplitSegment returns a media segment containing only the given track:
// each moof keeps just that track's traf, and its samples are packed into a
// new mdat with the trun data offsets rewritten to match.
func SplitSegment(seg []byte, trackID uint32) ([]byte, error) {
	top, err := boxes(seg, 0, len(seg))
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, b := range top {
		switch b.typ {
		case "moof":
			frag, err := splitFragment(seg, b, trackID)
			if err != nil {
				return nil, err
			}
			out = append(out, frag...)
		case "mdat", "sidx", "prft":
			// mdats are rebuilt per fragment; indexes would be stale.
		default:
			out = append(out, seg[b.start:b.end]...)
		}
	}
	return out, nil
}

type trunRef struct {
	trafOff    int // offset of the trun's data_offset field within the new traf
	start, len int // sample bytes in the source segment
}

func splitFragment(seg []byte, moof box, trackID uint32) ([]byte, error) {
	kids, err := children(seg, moof)
	if err != nil {
		return nil, err
	}
	var mfhd []byte
	var traf []byte
	var refs []trunRef
	for _, k := range kids {
		switch k.typ {
		case "mfhd":
			mfhd = seg[k.start:k.end]
		case "traf":
			t, rs, ok, err := keepTraf(seg, moof, k, trackID)
			if err != nil {
				return nil, err
			}
			if ok {
				traf, refs = t, rs
			}
		}
	}
	if traf == nil {
		return nil, nil // this fragment has no samples for the track
	}
	newMoof := makeBox("moof", mfhd, traf)
	trafStart := 8 + len(mfhd) // position of traf within newMoof
	dataStart := len(newMoof) + 8
	var mdat []byte
	for _, r := range refs {
		if r.start < 0 || r.start+r.len > len(seg) {
			return nil, fmt.Errorf("sample data out of range")
		}
		binary.BigEndian.PutUint32(newMoof[trafStart+r.trafOff:], uint32(dataStart+len(mdat)))
		mdat = append(mdat, seg[r.start:r.start+r.len]...)
	}
	return append(newMoof, makeBox("mdat", mdat)...), nil
}

// keepTraf copies a traf if it belongs to trackID, recording where each of
// its truns' samples live in the source and where their data_offset fields
// are in the copy.
func keepTraf(seg []byte, moof, traf box, trackID uint32) ([]byte, []trunRef, bool, error) {
	kids, err := children(seg, traf)
	if err != nil {
		return nil, nil, false, err
	}
	tfhd, ok := find(kids, "tfhd")
	if !ok {
		return nil, nil, false, errors.New("traf has no tfhd")
	}
	hb := tfhd.body(seg)
	if binary.BigEndian.Uint32(hb[4:]) != trackID {
		return nil, nil, false, nil
	}
	flags := binary.BigEndian.Uint32(hb[0:]) & 0xffffff
	if flags&tfhdBaseDataOffset != 0 {
		return nil, nil, false, errors.New("explicit base_data_offset not supported")
	}
	if flags&tfhdDefaultBaseIsMoof == 0 {
		return nil, nil, false, errors.New("only default-base-is-moof fragments are supported")
	}
	var defaultSize uint32
	p := 8
	if flags&tfhdSampleDescription != 0 {
		p += 4
	}
	if flags&tfhdDefaultDuration != 0 {
		p += 4
	}
	if flags&tfhdDefaultSize != 0 {
		defaultSize = binary.BigEndian.Uint32(hb[p:])
	}

	out := append([]byte(nil), seg[traf.start:traf.end]...)
	var refs []trunRef
	for _, k := range kids {
		if k.typ != "trun" {
			continue
		}
		tb := k.body(seg)
		tflags := binary.BigEndian.Uint32(tb[0:]) & 0xffffff
		count := int(binary.BigEndian.Uint32(tb[4:]))
		if tflags&trunDataOffset == 0 {
			return nil, nil, false, errors.New("trun without data_offset not supported")
		}
		dataOff := int(int32(binary.BigEndian.Uint32(tb[8:])))
		q := 12
		if tflags&trunFirstSampleFlags != 0 {
			q += 4
		}
		total := 0
		for i := 0; i < count; i++ {
			if tflags&trunSampleDuration != 0 {
				q += 4
			}
			if tflags&trunSampleSize != 0 {
				total += int(binary.BigEndian.Uint32(tb[q:]))
				q += 4
			} else {
				total += int(defaultSize)
			}
			if tflags&trunSampleFlags != 0 {
				q += 4
			}
			if tflags&trunSampleCompositeOff != 0 {
				q += 4
			}
		}
		refs = append(refs, trunRef{
			// data_offset sits after the trun header, version/flags and count.
			trafOff: (k.start - traf.start) + k.hdr + 8,
			start:   moof.start + dataOff,
			len:     total,
		})
	}
	return out, refs, true, nil
}
