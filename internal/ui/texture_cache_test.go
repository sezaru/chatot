package ui

import "testing"

func TestTextureCacheEvictsLeastRecentlyUsed(t *testing.T) {
	c := newTextureCache(100)
	c.put("a|1", nil, 40)
	c.put("b|1", nil, 40)
	if _, ok := c.get("a|1"); !ok {
		t.Fatal("a should be cached")
	}
	// a was just used, so b is the one to go.
	c.put("c|1", nil, 40)
	if _, ok := c.get("b|1"); ok {
		t.Fatal("b should have been evicted")
	}
	if _, ok := c.get("a|1"); !ok {
		t.Fatal("a should survive, it was used more recently than b")
	}
	if c.bytes != 80 {
		t.Fatalf("bytes = %d, want 80", c.bytes)
	}
	c.put("a|2", nil, 10)
	c.dropPrefix("a|")
	if _, ok := c.get("a|1"); ok {
		t.Fatal("dropPrefix should forget every side of a")
	}
	if _, ok := c.get("a|2"); ok {
		t.Fatal("dropPrefix should forget every side of a")
	}
	if _, ok := c.get("c|1"); !ok {
		t.Fatal("c must stay")
	}
	// An oversized entry evicts everything else and still lands.
	c.put("big|1", nil, 100)
	if c.order.Len() != 1 || c.bytes != 100 {
		t.Fatalf("after oversized put: %d entries, %d bytes", c.order.Len(), c.bytes)
	}
}
