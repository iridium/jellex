// Package assets holds jellex's own static files, embedded in the binary.
package assets

import _ "embed"

// Logo is the jellex logo as SVG.
//
//go:embed logo.svg
var Logo []byte

// Wordmark is the jellex wordmark in white, for dark backgrounds.
//
//go:embed wordmark.svg
var Wordmark []byte
