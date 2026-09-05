package store

import "testing"

func TestStickersOrderMostRecentlyUsedFirst(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertSticker(StickerRow{Key: "file:a", Path: "/s/a.webp", AddedTS: 10}))
	must(t, s.UpsertSticker(StickerRow{Key: "wa:b", Path: "/s/b.webp", FromWhatsApp: true, AddedTS: 20}))
	must(t, s.UpsertSticker(StickerRow{Key: "file:c", Path: "/s/c.webp", AddedTS: 30}))
	must(t, s.TouchSticker("/s/a.webp", 100))

	got, err := s.Stickers()
	must(t, err)
	want := []string{"file:a", "file:c", "wa:b"}
	if len(got) != len(want) {
		t.Fatalf("Stickers = %+v, want keys %v", got, want)
	}
	for i, k := range want {
		if got[i].Key != k {
			t.Errorf("Stickers[%d].Key = %q, want %q", i, got[i].Key, k)
		}
	}
	if !got[2].FromWhatsApp {
		t.Errorf("wa:b should be marked FromWhatsApp")
	}
}

func TestRemoveStickerHidesFavouriteAndDeletesLocal(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertSticker(StickerRow{Key: "wa:b", Path: "/s/b.webp", FromWhatsApp: true, AddedTS: 20}))
	must(t, s.UpsertSticker(StickerRow{Key: "file:a", Path: "/s/a.webp", AddedTS: 10}))

	path, err := s.RemoveSticker("wa:b")
	must(t, err)
	if path != "/s/b.webp" {
		t.Errorf("RemoveSticker path = %q, want /s/b.webp", path)
	}
	// The favourite's replayed app-state mutation must not resurrect it.
	must(t, s.UpsertSticker(StickerRow{Key: "wa:b", Path: "/s/b2.webp", FromWhatsApp: true, AddedTS: 25}))
	st, ok, err := s.Sticker("wa:b")
	must(t, err)
	if !ok || !st.Hidden {
		t.Errorf("wa:b after remove+upsert = %+v, want hidden", st)
	}
	if _, err := s.RemoveSticker("file:a"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.Sticker("file:a"); ok {
		t.Errorf("file:a still present after RemoveSticker")
	}
	got, _ := s.Stickers()
	if len(got) != 0 {
		t.Errorf("Stickers = %+v, want empty", got)
	}
	if _, err := s.RemoveSticker("missing"); err != nil {
		t.Errorf("RemoveSticker(missing) = %v, want nil", err)
	}
}

func TestStickerByPathSkipsHidden(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertSticker(StickerRow{Key: "wa:b", Path: "/s/b.webp", FromWhatsApp: true, AddedTS: 20}))
	if st, ok, err := s.StickerByPath("/s/b.webp"); err != nil || !ok || st.Key != "wa:b" {
		t.Errorf("StickerByPath = %+v, %v, %v", st, ok, err)
	}
	if _, err := s.RemoveSticker("wa:b"); err != nil {
		t.Fatal(err)
	}
	if _, ok, _ := s.StickerByPath("/s/b.webp"); ok {
		t.Error("a hidden favourite should not be found by path")
	}
}
