package ui

import (
	"container/list"
	"strings"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
)

// textureCache memoises decoded pictures by key (path|side), evicting the
// least recently used past a byte budget. The previous cache held 96
// entries and was dropped wholesale when full, so a chat list with a few
// hundred avatars decoded them again on every rebuild, and each row showed
// its initials until the decode landed: the avatar "flicker". Main loop
// only.
type textureCache struct {
	budget  int
	bytes   int
	entries map[string]*list.Element
	order   *list.List // front = most recently used
}

type textureEntry struct {
	key   string
	tex   *gdk.Texture
	bytes int
}

func newTextureCache(budget int) *textureCache {
	return &textureCache{budget: budget, entries: map[string]*list.Element{}, order: list.New()}
}

// get reports the cached texture for key, marking it recently used.
func (c *textureCache) get(key string) (*gdk.Texture, bool) {
	el, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(el)
	return el.Value.(*textureEntry).tex, true
}

// put stores tex (size bytes) under key, evicting the least recently used
// entries until the budget holds it.
func (c *textureCache) put(key string, tex *gdk.Texture, size int) {
	if el, ok := c.entries[key]; ok {
		c.remove(el)
	}
	for c.bytes+size > c.budget && c.order.Len() > 0 {
		c.remove(c.order.Back())
	}
	el := c.order.PushFront(&textureEntry{key: key, tex: tex, bytes: size})
	c.entries[key] = el
	c.bytes += size
}

// dropPrefix forgets every entry whose key starts with prefix: a file
// rewritten in place has stale decodes at every side.
func (c *textureCache) dropPrefix(prefix string) {
	for key, el := range c.entries {
		if strings.HasPrefix(key, prefix) {
			c.remove(el)
		}
	}
}

func (c *textureCache) remove(el *list.Element) {
	e := el.Value.(*textureEntry)
	c.order.Remove(el)
	delete(c.entries, e.key)
	c.bytes -= e.bytes
}

// textureBytes is what a texture costs in memory: RGBA at its pixel size.
func textureBytes(t *gdk.Texture) int { return t.Width() * t.Height() * 4 }
