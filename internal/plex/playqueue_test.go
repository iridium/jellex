package plex

import "testing"

func TestURIKey(t *testing.T) {
	for uri, want := range map[string]string{
		"server://m/com.plexapp.plugins.library/library/metadata/7":                             "7",
		"server://m/com.plexapp.plugins.library/library/metadata/7/children":                    "7",
		"server://m/com.plexapp.plugins.library/library/metadata/7/children?excludeAllLeaves=1": "7",
		"server://m/com.plexapp.plugins.library/library/collections/9/items":                    "9",
		"/library/metadata/8965": "8965",
		"server://m/com.plexapp.plugins.library/library/sections/1/all": "",
	} {
		got := ""
		if m := uriKey.FindStringSubmatch(uri); m != nil {
			got = m[1]
		}
		if got != want {
			t.Errorf("uriKey(%q) = %q, want %q", uri, got, want)
		}
	}
}
