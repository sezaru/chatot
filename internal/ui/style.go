// Package ui holds the gotk4/libadwaita widgets (chat list, conversation,
// composer, ...) built up feature by feature; for now it only ships the
// shared stylesheet.
package ui

import _ "embed"

//go:embed style.css
var StyleCSS string
