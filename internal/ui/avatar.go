package ui

import (
	"context"
	"strings"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// avatarCache is a view's own memo of jid -> resolved avatar path, so
// rebuilding a chat list / header doesn't re-fetch on every redraw. A jid
// present with path "" means "known to have no avatar" (still don't
// re-fetch); a jid absent means "not resolved yet".
type avatarCache struct {
	paths map[string]string
}

func newAvatarCache() *avatarCache {
	return &avatarCache{paths: make(map[string]string)}
}

// get reports the memoized path for jid and whether it's been resolved yet.
func (a *avatarCache) get(jid string) (path string, resolved bool) {
	path, resolved = a.paths[jid]
	return path, resolved
}

func (a *avatarCache) set(jid, path string) { a.paths[jid] = path }

// invalidate drops jid so the next buildAvatar call for it re-fetches.
func (a *avatarCache) invalidate(jid string) { delete(a.paths, jid) }

// buildAvatar returns a size x size container showing jid's avatar picture if
// known, else initial as an immediate fallback with an async fetch kicked off
// in the background (unless cache already knows jid has none). The fetch
// swaps the picture in via glib.IdleAdd when it completes; box is captured by
// that closure so a stale swap (the row/header having since been rebuilt or
// removed) is harmless — it just mutates a widget nobody's looking at anymore.
func buildAvatar(c client.Client, cache *avatarCache, jid, initial string, size int) *gtk.Box {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetSizeRequest(size, size)

	if path, resolved := cache.get(jid); resolved {
		if path != "" {
			box.Append(newAvatarPicture(path, size))
		} else {
			box.Append(newAvatarInitial(initial, size))
		}
		return box
	}

	box.Append(newAvatarInitial(initial, size))

	go func() {
		path, err := c.Avatar(context.Background(), jid)
		if err != nil {
			path = ""
		}
		glib.IdleAdd(func() {
			cache.set(jid, path)
			if path == "" {
				return
			}
			removeAllChildren(box)
			box.Append(newAvatarPicture(path, size))
		})
	}()

	return box
}

func newAvatarInitial(initial string, size int) *gtk.Label {
	label := gtk.NewLabel(initial)
	label.AddCSSClass("chatot-avatar")
	label.SetSizeRequest(size, size)
	return label
}

func newAvatarPicture(path string, size int) *gtk.Picture {
	pic := gtk.NewPictureForFilename(path)
	pic.SetContentFit(gtk.ContentFitCover)
	pic.SetCanShrink(true)
	pic.SetSizeRequest(size, size)
	pic.AddCSSClass("chatot-avatar-img")
	return pic
}

// initialFor derives the single-uppercase-letter fallback shown until (or
// instead of) a real avatar picture: the first rune of name, or "?" if name
// is empty.
func initialFor(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
}
