package plex

import (
	"encoding/json"
	"encoding/xml"
	"net/http/httptest"
	"testing"
)

func sample() *Element {
	return Container().A("title", "Movies").Add(
		E("Directory").A("key", "1").A("hidden", false).Add(E("Pivot").A("id", "library")),
		E("Directory").A("key", "2").A("hidden", true),
		E("Metadata").A("ratingKey", "7").A("rating", 6.5),
		E("Preferences").Add(E("Setting").A("id", "x")),
	)
}

func TestElementXML(t *testing.T) {
	b, err := xml.Marshal(sample())
	if err != nil {
		t.Fatal(err)
	}
	want := `<MediaContainer title="Movies"><Directory key="1" hidden="0"><Pivot id="library"></Pivot></Directory>` +
		`<Directory key="2" hidden="1"></Directory><Metadata ratingKey="7" rating="6.5"></Metadata>` +
		`<Preferences><Setting id="x"></Setting></Preferences></MediaContainer>`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}

func TestElementJSON(t *testing.T) {
	e := sample()
	e.Children[3].Single = true
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"title":"Movies","Directory":[{"key":"1","hidden":false,"Pivot":[{"id":"library"}]},{"key":"2","hidden":true}],` +
		`"Metadata":[{"ratingKey":"7","rating":6.5}],"Preferences":{"Setting":[{"id":"x"}]}}`
	if string(b) != want {
		t.Errorf("got  %s\nwant %s", b, want)
	}
}

func TestWriteNegotiatesAndSizes(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("Accept", "application/json")
	w := httptest.NewRecorder()
	write(w, r, sample())
	var got struct {
		MediaContainer struct {
			Size int `json:"size"`
		}
	}
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.MediaContainer.Size != 4 {
		t.Errorf("size = %d, want 4", got.MediaContainer.Size)
	}

	w = httptest.NewRecorder()
	write(w, httptest.NewRequest("GET", "/", nil), sample())
	if ct := w.Header().Get("Content-Type"); ct != "text/xml;charset=utf-8" {
		t.Errorf("default content type = %q", ct)
	}
}
