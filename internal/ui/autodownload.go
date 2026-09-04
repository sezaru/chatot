package ui

import (
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// AutoDownload mirrors settings.AutoDownload: which incoming media is
// fetched as soon as its bubble is built rather than on a click.
var AutoDownload = "photos"

// autoDownloadMaxAge keeps automatic fetches to recent messages: scrolling
// back through months of history must not start hundreds of downloads.
const autoDownloadMaxAge = 7 * 24 * time.Hour

// autoDownloadLabel is the display name for a settings.AutoDownload value.
func autoDownloadLabel(mode string) string {
	switch mode {
	case "always":
		return "Always"
	case "never":
		return "Never"
	}
	return "Photos and voice notes"
}

// autoDownloadWants reports whether media of kind, sent at unix time ts,
// should be fetched without a click under mode.
func autoDownloadWants(mode, kind string, ts int64, now time.Time) bool {
	if ts <= 0 || now.Unix()-ts > int64(autoDownloadMaxAge/time.Second) {
		return false
	}
	switch mode {
	case "always":
		return true
	case "photos":
		return kind == "image" || kind == "sticker" || kind == "audio"
	}
	return false
}

// maybeAutoDownload schedules start on the main loop when the preference
// wants this media fetched now. Deferred to an idle so the placeholder is
// in the tree before the swap replaces it.
func maybeAutoDownload(start func(), kind string, ts int64) {
	if !autoDownloadWants(AutoDownload, kind, ts, time.Now()) {
		return
	}
	glib.IdleAdd(start)
}
