package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	waSyncAction "go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types/events"
	"google.golang.org/protobuf/proto"
)

func newStickerFixture(t *testing.T) *Whatsmeow {
	t.Helper()
	w := newIngestFixture(t)
	w.stickerDir = filepath.Join(t.TempDir(), "stickers")
	return w
}

func writeTemp(t *testing.T, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestAddStickerCopiesIntoLibraryOnce(t *testing.T) {
	w := newStickerFixture(t)
	src := writeTemp(t, "pic.webp", []byte("RIFF....WEBP"))

	first, err := w.AddSticker(src)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Dir(first.Path) != w.stickerDir {
		t.Errorf("AddSticker path = %s, want a copy under %s", first.Path, w.stickerDir)
	}
	// The same picture from another path is the same entry.
	again, err := w.AddSticker(writeTemp(t, "copy.webp", []byte("RIFF....WEBP")))
	if err != nil {
		t.Fatal(err)
	}
	if again.Key != first.Key || again.Path != first.Path {
		t.Errorf("second AddSticker = %+v, want %+v", again, first)
	}
	got, _ := w.Stickers()
	if len(got) != 1 || got[0].FromWhatsApp {
		t.Errorf("Stickers = %+v, want the one local entry", got)
	}

	if err := w.RemoveSticker(context.Background(), first.Key); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(first.Path); !os.IsNotExist(err) {
		t.Errorf("sticker file still there after RemoveSticker: %v", err)
	}
	if got, _ := w.Stickers(); len(got) != 0 {
		t.Errorf("Stickers after remove = %+v, want empty", got)
	}
}

func favoriteEvent(fav bool) *events.AppState {
	return &events.AppState{
		Index: []string{appstate.IndexFavoriteSticker, "abc123"},
		SyncActionValue: &waSyncAction.SyncActionValue{StickerAction: &waSyncAction.StickerAction{
			DirectPath: proto.String("/v/t62.15575-24/x"), MediaKey: []byte{1}, FileEncSHA256: []byte{2},
			Mimetype: proto.String("image/webp"), IsFavorite: proto.Bool(fav),
		}},
	}
}

func waitStickers(t *testing.T, w *Whatsmeow, n int) []Sticker {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		got, err := w.Stickers()
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == n || time.Now().After(deadline) {
			return got
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestFavouriteStickerAppStateImportsAndHides(t *testing.T) {
	w := newStickerFixture(t)
	fetches := 0
	w.stickerFetch = func(ctx context.Context, act *waSyncAction.StickerAction) ([]byte, error) {
		fetches++
		return []byte("RIFF....WEBP"), nil
	}

	if !w.handleStickerAppState(favoriteEvent(true)) {
		t.Fatal("favouriteSticker mutation not recognised")
	}
	got := waitStickers(t, w, 1)
	if len(got) != 1 || !got[0].FromWhatsApp || got[0].Key != "wa:abc123" {
		t.Fatalf("Stickers = %+v, want the imported favourite", got)
	}
	if data, _ := os.ReadFile(got[0].Path); string(data) != "RIFF....WEBP" {
		t.Errorf("imported file = %q", data)
	}

	// A replay of the same favourite is no second download.
	w.handleStickerAppState(favoriteEvent(true))
	time.Sleep(50 * time.Millisecond)
	if fetches != 1 {
		t.Errorf("fetches = %d, want 1", fetches)
	}

	// Unstarred on the phone: gone from the picker.
	w.handleStickerAppState(favoriteEvent(false))
	if got := waitStickers(t, w, 0); len(got) != 0 {
		t.Errorf("Stickers after unfavourite = %+v, want empty", got)
	}
	// Removed here: a later replay of the favourite stays hidden.
	w.handleStickerAppState(favoriteEvent(true))
	time.Sleep(50 * time.Millisecond)
	if got, _ := w.Stickers(); len(got) != 0 {
		t.Errorf("Stickers after replay of a removed favourite = %+v, want empty", got)
	}
	if fetches != 1 {
		t.Errorf("fetches = %d, want 1: a hidden favourite is not downloaded again", fetches)
	}
}

func TestStickerAppStateIgnoresOtherMutations(t *testing.T) {
	w := newStickerFixture(t)
	if w.handleStickerAppState(&events.AppState{Index: []string{appstate.IndexPin, "x@s.whatsapp.net"}}) {
		t.Error("a pin mutation was taken for a sticker")
	}
}

// TestAddStickerKeepsAFavouriteAsItself: sending a favourite from the
// picker goes through AddSticker like any file; it must not be refiled as
// a second, local copy.
func TestAddStickerKeepsAFavouriteAsItself(t *testing.T) {
	w := newStickerFixture(t)
	w.stickerFetch = func(context.Context, *waSyncAction.StickerAction) ([]byte, error) {
		return []byte("RIFF....WEBP"), nil
	}
	w.handleStickerAppState(favoriteEvent(true))
	fav := waitStickers(t, w, 1)[0]

	got, err := w.AddSticker(fav.Path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Key != fav.Key || !got.FromWhatsApp {
		t.Errorf("AddSticker(favourite) = %+v, want the favourite %+v", got, fav)
	}
	if all, _ := w.Stickers(); len(all) != 1 {
		t.Errorf("Stickers = %+v, want the favourite alone", all)
	}
}
