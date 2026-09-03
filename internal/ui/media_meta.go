package ui

import (
	"fmt"
	"os"
	"strings"
	"time"

	"chatot/internal/client"
)

// docSubtitle renders a doc row's second line: "PDF · 1.2 MB · Aug 12, 2026",
// omitting the size segment when the file isn't downloaded (no LocalPath to
// stat — the store never persists a remote file's size).
func docSubtitle(d client.DocItem) string {
	parts := []string{docKindLabel(d.MimeType)}
	if size, ok := localFileSize(d.LocalPath); ok {
		parts = append(parts, formatFileSize(size))
	}
	parts = append(parts, time.Unix(d.TS, 0).Format("Jan 2, 2006"))
	return strings.Join(parts, " · ")
}

// docKindLabel derives a short uppercase type tag from a MIME type (e.g.
// "application/pdf" -> "PDF"), falling back to "FILE" if it can't parse one.
func docKindLabel(mime string) string {
	slash := strings.LastIndex(mime, "/")
	if slash < 0 || slash == len(mime)-1 {
		return "FILE"
	}
	sub := mime[slash+1:]
	if plus := strings.Index(sub, "+"); plus > 0 {
		sub = sub[:plus]
	}
	return strings.ToUpper(sub)
}

// formatFileSize renders n bytes as a human-readable "1.2 MB"-style string.
func formatFileSize(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for m := n / unit; m >= unit; m /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// linkSubtitle renders a link row's second line: "stay.example.com · Aug 12, 2026".
func linkSubtitle(l client.LinkItem) string {
	return l.Host + " · " + time.Unix(l.TS, 0).Format("Jan 2, 2006")
}

// mediaKindGlyph is the placeholder glyph for a Media-tab tile with no
// downloaded thumbnail: a video gets a play mark (there's no stored
// duration to show a proper badge — the schema has never persisted one).
func mediaKindGlyph(kind string) string {
	if kind == "video" {
		return "▶"
	}
	return "🖼"
}

// localFileSize stats path and returns its size, ok=false if path is empty
// or the stat fails (evicted cache, moved file, etc.).
func localFileSize(path string) (int64, bool) {
	if path == "" {
		return 0, false
	}
	info, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return info.Size(), true
}
