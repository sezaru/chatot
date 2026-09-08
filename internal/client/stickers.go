package client

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	waHistorySync "go.mau.fi/whatsmeow/proto/waHistorySync"
	"go.mau.fi/whatsmeow/types/events"

	"chatot/internal/store"
)

// Sticker is one entry of the sticker picker's library: a file the account
// can send again.
type Sticker struct {
	Key  string
	Path string
	// FromWhatsApp marks a sticker synced from the account (one starred or
	// recently sent on the phone). Removing it here only hides it on this
	// device; the phone keeps it.
	FromWhatsApp bool
}

// stickerRef is what a sticker download needs: the media path and keys
// a favourite (app state) or a recent (history sync) carries. FileSHA256
// is the plaintext hash, which only a recent knows.
type stickerRef struct {
	DirectPath    string
	FileEncSHA256 []byte
	FileSHA256    []byte
	MediaKey      []byte
}

// stickerFetcher downloads a sticker's bytes; a field so tests can stand in
// for the media servers.
type stickerFetcher func(ctx context.Context, ref stickerRef) ([]byte, error)

// recentStickerWorkers bounds how many of a history-sync chunk's recent
// stickers download at once.
const recentStickerWorkers = 4

// Stickers lists the library, most recently used first.
func (w *Whatsmeow) Stickers() ([]Sticker, error) {
	rows, err := w.store.Stickers()
	if err != nil {
		return nil, fmt.Errorf("chatot/client: list stickers: %w", err)
	}
	out := make([]Sticker, 0, len(rows))
	for _, r := range rows {
		out = append(out, stickerOf(r))
	}
	return out, nil
}

func stickerOf(r store.StickerRow) Sticker {
	return Sticker{Key: r.Key, Path: r.Path, FromWhatsApp: r.FromWhatsApp}
}

// AddSticker copies the file at path into the library, keyed by its
// content so the same picture added twice is one entry, and marks it used
// now. A file already in the library (by content, whether the user added it,
// a chat delivered it or the phone synced it) is only touched; one the user
// removed earlier comes back.
func (w *Whatsmeow) AddSticker(path string) (Sticker, error) {
	now := time.Now().Unix()
	// A library file (a picker tile, synced or not) is only touched.
	if st, ok, err := w.store.StickerByPath(path); err == nil && ok {
		return stickerOf(st), w.store.TouchSticker(path, now)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return Sticker{}, fmt.Errorf("chatot/client: add sticker: %w", err)
	}
	hash := contentHash(data)
	key := "file:" + hash
	ext := strings.ToLower(filepath.Ext(path))
	if ext == "" {
		ext = ".webp"
	}
	dest := filepath.Join(w.stickerDir, hash+ext)
	// Every synced sticker is filed under its content hash too, so the
	// same picture already there under another key (a favourite) is found
	// by its file.
	if st, ok := w.libraryFile(dest); ok {
		return stickerOf(st), w.store.TouchSticker(st.Path, now)
	}
	if st, ok, err := w.store.Sticker(key); err == nil && ok && !st.Hidden && st.Path != "" {
		if _, err := os.Stat(st.Path); err == nil {
			return stickerOf(st), w.store.TouchSticker(st.Path, now)
		}
	}
	if _, err := w.writeStickerFile(hash+ext, data); err != nil {
		return Sticker{}, err
	}
	row := store.StickerRow{Key: key, Path: dest, AddedTS: now, UsedTS: now}
	if err := w.store.AddSticker(row); err != nil {
		return Sticker{}, fmt.Errorf("chatot/client: add sticker: %w", err)
	}
	st, ok, err := w.store.Sticker(key)
	if err != nil || !ok {
		return Sticker{Key: key, Path: dest}, nil
	}
	return stickerOf(st), nil
}

// libraryFile finds the visible library entry whose file is path and is
// still on disk.
func (w *Whatsmeow) libraryFile(path string) (store.StickerRow, bool) {
	st, ok, err := w.store.StickerByPath(path)
	if err != nil || !ok {
		return store.StickerRow{}, false
	}
	if _, err := os.Stat(st.Path); err != nil {
		return store.StickerRow{}, false
	}
	return st, true
}

// RemoveSticker takes key out of the library and deletes its file. A
// WhatsApp sticker stays hidden here even when the phone syncs it again.
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
	ref := stickerRef{DirectPath: act.GetDirectPath(), FileEncSHA256: act.GetFileEncSHA256(), MediaKey: act.GetMediaKey()}
	go w.importFavoriteSticker(key, ref)
	return true
}

// importFavoriteSticker downloads a favourite and files it under key. Its
// file is named by content, so a copy the library already holds (a recent
// the phone synced, or one saved from a chat) is kept as it is rather than
// shown twice.
func (w *Whatsmeow) importFavoriteSticker(key string, ref stickerRef) {
	data, err := w.fetchSticker(ref)
	if err != nil {
		w.log.Warnf("chatot/client: download favourite sticker: %v", err)
		return
	}
	name := contentHash(data) + ".webp"
	if _, ok := w.libraryFile(filepath.Join(w.stickerDir, name)); ok {
		return
	}
	if w.stickerHidden(key) {
		return // unstarred or removed while the download ran
	}
	path, err := w.writeStickerFile(name, data)
	if err != nil {
		w.log.Warnf("chatot/client: %v", err)
		return
	}
	now := time.Now().Unix()
	if err := w.store.UpsertSticker(store.StickerRow{Key: key, Path: path, FromWhatsApp: true, AddedTS: now}); err != nil {
		w.log.Warnf("chatot/client: store favourite sticker: %v", err)
	}
}

// importRecentStickers brings a history-sync chunk's recent stickers (the
// ones the phone's picker shows) into the library, keyed by content like a
// file the user adds, so the same sticker saved from a chat is one entry.
// A sticker already here only takes the phone's last-use time; one the
// user removed stays hidden. Downloads run in the background.
func (w *Whatsmeow) importRecentStickers(list []*waHistorySync.StickerMetadata) {
	var todo []*waHistorySync.StickerMetadata
	for _, md := range list {
		if md.GetIsLottie() || md.GetDirectPath() == "" || len(md.GetMediaKey()) == 0 || len(md.GetFileSHA256()) == 0 {
			continue
		}
		hash := hex.EncodeToString(md.GetFileSHA256())
		if st, ok := w.libraryFile(filepath.Join(w.stickerDir, hash+".webp")); ok {
			if ts := recentStickerTS(md); ts > 0 {
				if err := w.store.TouchSticker(st.Path, ts); err != nil {
					w.log.Warnf("chatot/client: touch recent sticker: %v", err)
				}
			}
			continue
		}
		if st, ok, err := w.store.Sticker("file:" + hash); err == nil && ok && st.Hidden {
			continue
		}
		todo = append(todo, md)
	}
	if len(todo) == 0 {
		return
	}
	go func() {
		sem := make(chan struct{}, recentStickerWorkers)
		for _, md := range todo {
			sem <- struct{}{}
			go func(md *waHistorySync.StickerMetadata) {
				defer func() { <-sem }()
				w.importRecentSticker(md)
			}(md)
		}
	}()
}

// importRecentSticker downloads one recent sticker and files it by content.
func (w *Whatsmeow) importRecentSticker(md *waHistorySync.StickerMetadata) {
	ref := stickerRef{
		DirectPath: md.GetDirectPath(), FileEncSHA256: md.GetFileEncSHA256(),
		FileSHA256: md.GetFileSHA256(), MediaKey: md.GetMediaKey(),
	}
	data, err := w.fetchSticker(ref)
	if err != nil {
		w.log.Warnf("chatot/client: download recent sticker: %v", err)
		return
	}
	hash := contentHash(data)
	if w.stickerHidden("file:" + hash) {
		return // removed while the download ran
	}
	path, err := w.writeStickerFile(hash+".webp", data)
	if err != nil {
		w.log.Warnf("chatot/client: %v", err)
		return
	}
	now := time.Now().Unix()
	row := store.StickerRow{Key: "file:" + hash, Path: path, FromWhatsApp: true, AddedTS: now, UsedTS: recentStickerTS(md)}
	if err := w.store.UpsertSticker(row); err != nil {
		w.log.Warnf("chatot/client: store recent sticker: %v", err)
	}
}

// stickerHidden reports whether key is a library entry the user removed.
func (w *Whatsmeow) stickerHidden(key string) bool {
	st, ok, err := w.store.Sticker(key)
	return err == nil && ok && st.Hidden
}

// recentStickerTS is when the phone last sent a recent sticker, in unix
// seconds (the field is milliseconds on some builds); 0 when unknown.
func recentStickerTS(md *waHistorySync.StickerMetadata) int64 {
	ts := md.GetLastStickerSentTS()
	if ts > 1e12 {
		ts /= 1000
	}
	if ts < 0 {
		return 0
	}
	return ts
}

// fetchSticker downloads ref's bytes through the test stub or the media
// servers, with a timeout.
func (w *Whatsmeow) fetchSticker(ref stickerRef) ([]byte, error) {
	fetch := w.stickerFetch
	if fetch == nil {
		fetch = w.downloadSticker
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return fetch(ctx, ref)
}

func (w *Whatsmeow) downloadSticker(ctx context.Context, ref stickerRef) ([]byte, error) {
	if w.wa == nil {
		return nil, fmt.Errorf("not connected")
	}
	data, err := w.wa.DownloadMediaWithPath(ctx, ref.DirectPath, ref.FileEncSHA256, ref.FileSHA256, ref.MediaKey, whatsmeow.MediaImage, "", true)
	return acceptStickerDownload(ref, data, err)
}

// acceptStickerDownload passes a download's result through, except that a
// favourite (which carries no plaintext hash) is accepted on its MAC and
// encrypted hash alone: whatsmeow hands back the decrypted bytes together
// with ErrInvalidMediaSHA256 when it has no plaintext hash to check, and
// those bytes are good.
func acceptStickerDownload(ref stickerRef, data []byte, err error) ([]byte, error) {
	if err != nil && len(ref.FileSHA256) == 0 && len(data) > 0 && errors.Is(err, whatsmeow.ErrInvalidMediaSHA256) {
		return data, nil
	}
	return data, err
}

// contentHash is the hex sha256 of data: the library's name for a picture.
func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
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
