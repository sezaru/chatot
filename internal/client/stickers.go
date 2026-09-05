package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waSyncAction "go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types/events"

	"chatot/internal/store"
)

// Sticker is one entry of the sticker picker's library: a file the account
// can send again.
type Sticker struct {
	Key  string
	Path string
	// FromWhatsApp marks a favourite synced from the account (a sticker
	// starred on the phone). Removing it here only hides it on this
	// device; the phone keeps its favourite.
	FromWhatsApp bool
}

// stickerFetcher downloads a favourite sticker's bytes; a field so tests
// can stand in for the media servers.
type stickerFetcher func(ctx context.Context, act *waSyncAction.StickerAction) ([]byte, error)

// Stickers lists the library, most recently used first.
func (w *Whatsmeow) Stickers() ([]Sticker, error) {
	rows, err := w.store.Stickers()
	if err != nil {
		return nil, fmt.Errorf("chatot/client: list stickers: %w", err)
	}
	out := make([]Sticker, 0, len(rows))
	for _, r := range rows {
		out = append(out, Sticker{Key: r.Key, Path: r.Path, FromWhatsApp: r.FromWhatsApp})
	}
	return out, nil
}

// AddSticker copies the file at path into the library, keyed by its
// content so the same picture added twice is one entry, and marks it used
// now. A file already in the library (by content) is only touched.
func (w *Whatsmeow) AddSticker(path string) (Sticker, error) {
	now := time.Now().Unix()
	// A library file (a picker tile, favourite or not) is only touched.
	if st, ok, err := w.store.StickerByPath(path); err == nil && ok {
		return Sticker{Key: st.Key, Path: st.Path, FromWhatsApp: st.FromWhatsApp}, w.store.TouchSticker(path, now)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Sticker{}, fmt.Errorf("chatot/client: add sticker: %w", err)
	}
	sum := sha256.Sum256(data)
	key := "file:" + hex.EncodeToString(sum[:])
	if st, ok, err := w.store.Sticker(key); err == nil && ok && !st.Hidden && st.Path != "" {
		if _, err := os.Stat(st.Path); err == nil {
			return Sticker{Key: key, Path: st.Path}, w.store.TouchSticker(st.Path, now)
		}
	}
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".webp"
	}
	dest, err := w.writeStickerFile(hex.EncodeToString(sum[:])+ext, data)
	if err != nil {
		return Sticker{}, err
	}
	row := store.StickerRow{Key: key, Path: dest, AddedTS: now, UsedTS: now}
	if err := w.store.UpsertSticker(row); err != nil {
		return Sticker{}, fmt.Errorf("chatot/client: add sticker: %w", err)
	}
	return Sticker{Key: key, Path: dest}, nil
}

// RemoveSticker takes key out of the library and deletes its file. A
// WhatsApp favourite stays hidden here even when the phone syncs it again.
func (w *Whatsmeow) RemoveSticker(ctx context.Context, key string) error {
	path, err := w.store.RemoveSticker(key)
	if err != nil {
		return fmt.Errorf("chatot/client: remove sticker: %w", err)
	}
	w.removeStickerFile(path)
	return nil
}

// touchSticker moves the sticker at path to the front of the library when
// it is in it (a send from the picker or of a fresh file).
func (w *Whatsmeow) touchSticker(path string) {
	if err := w.store.TouchSticker(path, time.Now().Unix()); err != nil {
		w.log.Warnf("chatot/client: touch sticker: %v", err)
	}
}

// handleStickerAppState applies a favouriteSticker app-state mutation: the
// phone starring a sticker downloads it into the library, unstarring one
// hides it. Reports whether evt was one.
func (w *Whatsmeow) handleStickerAppState(evt *events.AppState) bool {
	if len(evt.Index) < 2 || evt.Index[0] != appstate.IndexFavoriteSticker {
		return false
	}
	key := favoriteStickerKey(evt.Index)
	act := evt.GetStickerAction()
	if act == nil || !act.GetIsFavorite() {
		if _, err := w.store.RemoveSticker(key); err != nil {
			w.log.Warnf("chatot/client: unfavourite sticker: %v", err)
		}
		return true
	}
	if act.GetIsLottie() {
		return true // an animated Lottie sticker: nothing here can draw it
	}
	if st, ok, err := w.store.Sticker(key); err == nil && ok {
		if st.Hidden {
			return true
		}
		if _, err := os.Stat(st.Path); err == nil {
			return true
		}
	}
	go w.importFavoriteSticker(key, act)
	return true
}

// importFavoriteSticker downloads a favourite and files it under key.
func (w *Whatsmeow) importFavoriteSticker(key string, act *waSyncAction.StickerAction) {
	fetch := w.stickerFetch
	if fetch == nil {
		fetch = w.downloadFavoriteSticker
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	data, err := fetch(ctx, act)
	if err != nil {
		w.log.Warnf("chatot/client: download favourite sticker: %v", err)
		return
	}
	sum := sha256.Sum256([]byte(key))
	path, err := w.writeStickerFile(hex.EncodeToString(sum[:])+".webp", data)
	if err != nil {
		w.log.Warnf("chatot/client: %v", err)
		return
	}
	now := time.Now().Unix()
	if err := w.store.UpsertSticker(store.StickerRow{Key: key, Path: path, FromWhatsApp: true, AddedTS: now}); err != nil {
		w.log.Warnf("chatot/client: store favourite sticker: %v", err)
	}
}

func (w *Whatsmeow) downloadFavoriteSticker(ctx context.Context, act *waSyncAction.StickerAction) ([]byte, error) {
	if w.wa == nil {
		return nil, fmt.Errorf("not connected")
	}
	// A favourite carries no plaintext hash, so the download can only
	// check the encrypted one.
	return w.wa.DownloadMediaWithPath(ctx, act.GetDirectPath(), act.GetFileEncSHA256(), nil, act.GetMediaKey(), whatsmeow.MediaImage, "", true)
}

// favoriteStickerKey is the library key of a favouriteSticker mutation:
// its app-state index past the type name.
func favoriteStickerKey(index []string) string {
	return "wa:" + strings.Join(index[1:], "/")
}

// writeStickerFile stores data as name under the sticker directory and
// returns the path.
func (w *Whatsmeow) writeStickerFile(name string, data []byte) (string, error) {
	if err := os.MkdirAll(w.stickerDir, 0o700); err != nil {
		return "", fmt.Errorf("chatot/client: create sticker dir: %w", err)
	}
	path := filepath.Join(w.stickerDir, name)
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("chatot/client: write sticker: %w", err)
	}
	return path, nil
}

// removeStickerFile deletes path when it lives in the sticker directory;
// a file elsewhere (an older row pointing at the user's own picture) is
// left alone.
func (w *Whatsmeow) removeStickerFile(path string) {
	if path == "" {
		return
	}
	rel, err := filepath.Rel(w.stickerDir, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		w.log.Warnf("chatot/client: remove sticker file: %v", err)
	}
}
