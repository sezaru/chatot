package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
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
	w.stickerFetch = func(ctx context.Context, ref stickerRef) ([]byte, error) {
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
	w.stickerFetch = func(context.Context, stickerRef) ([]byte, error) {
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

// recentSticker is one history-sync recent sticker whose download yields
// data.
func recentSticker(data []byte, sentTS int64) *waHistorySync.StickerMetadata {
	sum := sha256.Sum256(data)
	return &waHistorySync.StickerMetadata{
		DirectPath: proto.String("/v/t62.15575-24/" + hex.EncodeToString(sum[:4])), MediaKey: []byte{1},
		FileEncSHA256: []byte{2}, FileSHA256: sum[:], Mimetype: proto.String("image/webp"),
		LastStickerSentTS: proto.Int64(sentTS),
	}
}

// stubStickerFetch serves each recent sticker's bytes from its direct path
// and counts the downloads (recents download concurrently).
func stubStickerFetch(w *Whatsmeow, files map[string][]byte) *atomic.Int32 {
	fetches := &atomic.Int32{}
	w.stickerFetch = func(ctx context.Context, ref stickerRef) ([]byte, error) {
		fetches.Add(1)
		if data, ok := files[ref.DirectPath]; ok {
			return data, nil
		}
		return []byte("RIFF....WEBP"), nil
	}
	return fetches
}

// The phone's recent stickers arrive in a history-sync chunk and fill the
// picker in the phone's order; a replay downloads nothing again, and one
// removed here stays away.
func TestHistorySyncRecentStickersFillTheLibrary(t *testing.T) {
	w := newStickerFixture(t)
	old, fresh := []byte("RIFF.old.WEBP"), []byte("RIFF.new.WEBP")
	mdOld, mdNew := recentSticker(old, 1700000000), recentSticker(fresh, 1800000000000) // the second in milliseconds
	fetches := stubStickerFetch(w, map[string][]byte{mdOld.GetDirectPath(): old, mdNew.GetDirectPath(): fresh})
	lottie := recentSticker([]byte("lottie"), 1900000000)
	lottie.IsLottie = proto.Bool(true)
	chunk := &waHistorySync.HistorySync{RecentStickers: []*waHistorySync.StickerMetadata{mdOld, lottie, mdNew}}

	w.applyHistorySync(chunk)
	got := waitStickers(t, w, 2)
	if len(got) != 2 {
		t.Fatalf("Stickers = %+v, want the two drawable recents", got)
	}
	sum := sha256.Sum256(fresh)
	if got[0].Key != "file:"+hex.EncodeToString(sum[:]) || !got[0].FromWhatsApp {
		t.Errorf("Stickers[0] = %+v, want the most recently sent one, from WhatsApp", got[0])
	}
	if data, _ := os.ReadFile(got[0].Path); string(data) != string(fresh) {
		t.Errorf("imported file = %q", data)
	}
	if fetches.Load() != 2 {
		t.Errorf("fetches = %d, want 2 (the Lottie one is skipped)", fetches.Load())
	}

	w.applyHistorySync(chunk)
	time.Sleep(50 * time.Millisecond)
	if fetches.Load() != 2 {
		t.Errorf("fetches after replay = %d, want 2", fetches.Load())
	}

	if err := w.RemoveSticker(context.Background(), got[0].Key); err != nil {
		t.Fatal(err)
	}
	w.applyHistorySync(chunk)
	time.Sleep(50 * time.Millisecond)
	if got, _ := w.Stickers(); len(got) != 1 || fetches.Load() != 2 {
		t.Errorf("after removing a recent and replaying: Stickers = %+v, fetches = %d; want 1 and 2", got, fetches.Load())
	}
}

// Saving a sticker from a chat is the same entry as the phone's recent of
// it, in either order; and it brings back one the user removed.
func TestAddStickerFromChatMatchesRecent(t *testing.T) {
	w := newStickerFixture(t)
	pic := []byte("RIFF.pic.WEBP")
	md := recentSticker(pic, 1700000000)
	stubStickerFetch(w, map[string][]byte{md.GetDirectPath(): pic})

	w.importRecentStickers([]*waHistorySync.StickerMetadata{md})
	recent := waitStickers(t, w, 1)[0]
	saved, err := w.AddSticker(writeTemp(t, "3EB0.webp", pic))
	if err != nil {
		t.Fatal(err)
	}
	if saved.Key != recent.Key || saved.Path != recent.Path || !saved.FromWhatsApp {
		t.Errorf("AddSticker(chat copy) = %+v, want the recent %+v", saved, recent)
	}
	if all, _ := w.Stickers(); len(all) != 1 {
		t.Errorf("Stickers = %+v, want one entry", all)
	}

	if err := w.RemoveSticker(context.Background(), recent.Key); err != nil {
		t.Fatal(err)
	}
	back, err := w.AddSticker(writeTemp(t, "again.webp", pic))
	if err != nil {
		t.Fatal(err)
	}
	if all, _ := w.Stickers(); len(all) != 1 || all[0].Key != back.Key || back.Key != recent.Key {
		t.Errorf("after re-adding a removed recent: Stickers = %+v, AddSticker = %+v", all, back)
	}
	if data, _ := os.ReadFile(back.Path); string(data) != string(pic) {
		t.Errorf("re-added file = %q", data)
	}
	if !back.FromWhatsApp {
		t.Errorf("a re-added recent should stay a WhatsApp sticker so removing it hides it again")
	}

	// The other order: the chat copy first, then the phone's recent of it.
	w2 := newStickerFixture(t)
	fetches := stubStickerFetch(w2, map[string][]byte{md.GetDirectPath(): pic})
	first, err := w2.AddSticker(writeTemp(t, "3EB0.webp", pic))
	if err != nil {
		t.Fatal(err)
	}
	w2.importRecentStickers([]*waHistorySync.StickerMetadata{md})
	time.Sleep(50 * time.Millisecond)
	if all, _ := w2.Stickers(); len(all) != 1 || all[0].Key != first.Key || fetches.Load() != 0 {
		t.Errorf("recent of a saved sticker: Stickers = %+v, fetches = %d; want the one entry, no download", all, fetches.Load())
	}
}

// A favourite and a recent of the same picture are one tile, whichever
// arrives first.
func TestFavouriteAndRecentOfOnePictureAreOneTile(t *testing.T) {
	pic := []byte("RIFF.pic.WEBP")
	md := recentSticker(pic, 1700000000)

	// The favourite and the recent are the same picture behind two paths.
	files := map[string][]byte{md.GetDirectPath(): pic, favoriteEvent(true).GetStickerAction().GetDirectPath(): pic}

	w := newStickerFixture(t)
	stubStickerFetch(w, files)
	w.handleStickerAppState(favoriteEvent(true))
	fav := waitStickers(t, w, 1)[0]
	w.importRecentStickers([]*waHistorySync.StickerMetadata{md})
	time.Sleep(50 * time.Millisecond)
	if all, _ := w.Stickers(); len(all) != 1 || all[0].Key != fav.Key {
		t.Errorf("favourite then recent: Stickers = %+v, want the favourite alone", all)
	}

	w = newStickerFixture(t)
	stubStickerFetch(w, files)
	w.importRecentStickers([]*waHistorySync.StickerMetadata{md})
	recent := waitStickers(t, w, 1)[0]
	w.handleStickerAppState(favoriteEvent(true))
	time.Sleep(50 * time.Millisecond)
	if all, _ := w.Stickers(); len(all) != 1 || all[0].Key != recent.Key {
		t.Errorf("recent then favourite: Stickers = %+v, want the recent alone", all)
	}
}

// whatsmeow returns a favourite's decrypted bytes together with a hash
// error, since a favourite carries no plaintext hash to check; those bytes
// are kept. A recent (which does carry one) or an empty result is not.
func TestAcceptStickerDownloadKeepsAFavouriteWithoutPlaintextHash(t *testing.T) {
	fav := stickerRef{DirectPath: "/v/x", FileEncSHA256: []byte{2}, MediaKey: []byte{1}}
	data, err := acceptStickerDownload(fav, []byte("RIFF"), whatsmeow.ErrInvalidMediaSHA256)
	if err != nil || string(data) != "RIFF" {
		t.Errorf("favourite with hash error = %q, %v; want the bytes and no error", data, err)
	}
	if _, err := acceptStickerDownload(fav, nil, whatsmeow.ErrInvalidMediaSHA256); err == nil {
		t.Error("an empty download must stay an error")
	}
	if _, err := acceptStickerDownload(fav, []byte("RIFF"), whatsmeow.ErrMediaDownloadFailedWith404); err == nil {
		t.Error("another error must pass through")
	}
	recent := stickerRef{DirectPath: "/v/x", FileEncSHA256: []byte{2}, FileSHA256: make([]byte, 32), MediaKey: []byte{1}}
	if _, err := acceptStickerDownload(recent, []byte("RIFF"), whatsmeow.ErrInvalidMediaSHA256); err == nil {
		t.Error("a recent whose plaintext hash mismatches must stay an error")
	}
}
