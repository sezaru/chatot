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

// An explicit add brings a removed WhatsApp sticker back, still marked as
// WhatsApp's so the next removal hides it again.
func TestAddStickerUnhidesARemovedEntry(t *testing.T) {
	s := newTestStore(t)
	must(t, s.UpsertSticker(StickerRow{Key: "file:a", Path: "/s/a.webp", FromWhatsApp: true, AddedTS: 10}))
	if _, err := s.RemoveSticker("file:a"); err != nil {
		t.Fatal(err)
	}
	// A plain upsert (a sync replay) leaves it hidden...
	must(t, s.UpsertSticker(StickerRow{Key: "file:a", Path: "/s/a.webp", FromWhatsApp: true, AddedTS: 20}))
	if got, _ := s.Stickers(); len(got) != 0 {
		t.Fatalf("Stickers after upsert of a hidden entry = %+v, want none", got)
	}
	// ...an add brings it back.
	must(t, s.AddSticker(StickerRow{Key: "file:a", Path: "/s/a.webp", AddedTS: 30, UsedTS: 30}))
	got, _ := s.Stickers()
	if len(got) != 1 || got[0].Key != "file:a" || !got[0].FromWhatsApp || got[0].Hidden {
		t.Fatalf("Stickers after AddSticker = %+v, want file:a visible and still from WhatsApp", got)
	}
	if path, _ := s.RemoveSticker("file:a"); path != "/s/a.webp" {
		t.Errorf("RemoveSticker path = %q", path)
	}
	if st, ok, _ := s.Sticker("file:a"); !ok || !st.Hidden {
		t.Errorf("a re-added WhatsApp sticker should hide on removal, got %+v", st)
	}
}
