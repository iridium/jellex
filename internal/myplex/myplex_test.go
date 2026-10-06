package myplex

import "testing"

func TestParseUser(t *testing.T) {
	for name, body := range map[string]string{
		"json flat":   `{"authToken":"tok","username":"me"}`,
		"json nested": `{"user":{"authenticationToken":"tok","username":"me"}}`,
		"xml":         `<?xml version="1.0" encoding="UTF-8"?><user authenticationToken="tok" username="me" email="x"/>`,
	} {
		tok, user := parseUser([]byte(body))
		if tok != "tok" || user != "me" {
			t.Errorf("%s: got (%q, %q)", name, tok, user)
		}
	}
	if tok, _ := parseUser([]byte(`<html>nope</html>`)); tok != "" {
		t.Errorf("garbage: got token %q", tok)
	}
}
