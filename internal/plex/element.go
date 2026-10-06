package plex

import (
	"bytes"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Element is a node in a Plex response. Plex serves the same tree as XML
// (attributes plus child elements) or JSON (attributes as keys, children
// grouped into arrays by tag), so responses are built generically and encoded
// for whichever the client asked for.
type Element struct {
	Tag      string
	Attrs    []Attr
	Children []*Element
	// Single renders this element in JSON as an object rather than as a
	// one-item array, which Plex does for a few wrappers like Preferences.
	Single bool
	// JSONTag, if set, is the key this element is grouped under in JSON when
	// it differs from the XML tag (e.g. <Playlist> is "Metadata" in JSON).
	JSONTag string
}

func (e *Element) jsonTag() string {
	if e.JSONTag != "" {
		return e.JSONTag
	}
	return e.Tag
}

type Attr struct {
	Name  string
	Value any
}

func E(tag string) *Element { return &Element{Tag: tag} }

// Container starts a MediaContainer; size is filled in when it's written.
func Container() *Element { return E("MediaContainer") }

// A sets an attribute. Nil values are skipped so optional fields can be
// passed straight through.
func (e *Element) A(name string, v any) *Element {
	if v == nil {
		return e
	}
	for i := range e.Attrs {
		if e.Attrs[i].Name == name {
			e.Attrs[i].Value = v
			return e
		}
	}
	e.Attrs = append(e.Attrs, Attr{name, v})
	return e
}

// Opt sets an attribute only if v is not its zero value.
func (e *Element) Opt(name string, v any) *Element {
	switch x := v.(type) {
	case string:
		if x == "" {
			return e
		}
	case int:
		if x == 0 {
			return e
		}
	case int64:
		if x == 0 {
			return e
		}
	case float64:
		if x == 0 {
			return e
		}
	case bool:
		if !x {
			return e
		}
	}
	return e.A(name, v)
}

func (e *Element) Add(children ...*Element) *Element {
	for _, c := range children {
		if c != nil {
			e.Children = append(e.Children, c)
		}
	}
	return e
}

func (e *Element) Get(name string) any {
	for _, a := range e.Attrs {
		if a.Name == name {
			return a.Value
		}
	}
	return nil
}

func (e *Element) MarshalXML(enc *xml.Encoder, _ xml.StartElement) error {
	start := xml.StartElement{Name: xml.Name{Local: e.Tag}}
	for _, a := range e.Attrs {
		start.Attr = append(start.Attr, xml.Attr{Name: xml.Name{Local: a.Name}, Value: xmlValue(a.Value)})
	}
	if err := enc.EncodeToken(start); err != nil {
		return err
	}
	for _, c := range e.Children {
		if err := enc.Encode(c); err != nil {
			return err
		}
	}
	return enc.EncodeToken(start.End())
}

func xmlValue(v any) string {
	switch x := v.(type) {
	case bool:
		if x {
			return "1"
		}
		return "0"
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	default:
		return fmt.Sprint(x)
	}
}

func (e *Element) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	first := true
	key := func(k string) {
		if !first {
			b.WriteByte(',')
		}
		first = false
		kb, _ := json.Marshal(k)
		b.Write(kb)
		b.WriteByte(':')
	}
	for _, a := range e.Attrs {
		vb, err := json.Marshal(a.Value)
		if err != nil {
			return nil, err
		}
		key(a.Name)
		b.Write(vb)
	}
	// Group children by tag, keeping first-seen order.
	var order []string
	groups := map[string][]*Element{}
	for _, c := range e.Children {
		if _, ok := groups[c.jsonTag()]; !ok {
			order = append(order, c.jsonTag())
		}
		groups[c.jsonTag()] = append(groups[c.jsonTag()], c)
	}
	for _, tag := range order {
		g := groups[tag]
		key(tag)
		var vb []byte
		var err error
		if len(g) == 1 && g[0].Single {
			vb, err = json.Marshal(g[0])
		} else {
			vb, err = json.Marshal(g)
		}
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// write encodes a MediaContainer as XML (the Plex default) or JSON when the
// client asks for it.
func write(w http.ResponseWriter, r *http.Request, mc *Element) {
	if mc.Get("size") == nil {
		n := 0
		for _, c := range mc.Children {
			if !c.Single {
				n++
			}
		}
		mc.Attrs = append([]Attr{{"size", n}}, mc.Attrs...)
	}
	// Plex Web only renders a list when the container says how big the
	// whole list is, so unpaged responses report everything as one page.
	if mc.Get("totalSize") == nil && len(mc.Children) > 0 {
		mc.A("totalSize", mc.Get("size")).A("offset", 0)
	}
	writeRoot(w, r, mc)
}

// writeRoot encodes any root element, for the few responses that aren't a
// MediaContainer. In JSON the root is wrapped in an object keyed by its tag.
func writeRoot(w http.ResponseWriter, r *http.Request, e *Element) {
	if wantsJSON(r) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]*Element{e.Tag: e})
		return
	}
	w.Header().Set("Content-Type", "text/xml;charset=utf-8")
	w.Write([]byte(xml.Header))
	xml.NewEncoder(w).Encode(e)
}

func wantsJSON(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Accept"), "application/json") ||
		strings.Contains(r.URL.Query().Get("X-Plex-Accept"), "json")
}
