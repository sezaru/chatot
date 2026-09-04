package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"
)

// avatarCacheName derives a filesystem-safe cache filename for jid (whatsmeow
// JIDs contain '@' and sometimes ':', neither valid-looking in a filename).
func avatarCacheName(jid string) string {
	r := strings.NewReplacer("/", "_", "@", "_", ":", "_")
	return r.Replace(jid) + ".jpg"
}

// avatarEntry is what's memoized per jid: either a resolved cache path (with
// the picture ID it was fetched under, for a future ExistingID revalidation)
// or missing=true, meaning the contact has no picture / it's not visible to
// us — both are normal, not errors, so callers shouldn't re-fetch on every UI
// rebuild.
type avatarEntry struct {
	id      string
	path    string
	missing bool
}

// Avatar resolves jid's profile picture to a local file path, downloading and
// disk-caching it on first use. Returns ("", nil) if the contact has no
// picture (or it's not visible to us) — that's the normal case, not an
// error. Safe to call from a goroutine; never touches the GTK main loop.
func (w *Whatsmeow) Avatar(ctx context.Context, jid string) (string, error) {
	w.avatarMu.Lock()
	entry, ok := w.avatarMemo[jid]
	w.avatarMu.Unlock()
	if ok {
		return entry.path, nil // path is "" for a memoized "missing" entry too
	}
	// A picture fetched in an earlier run is still on disk: serve it without
	// a round trip. Every start used to re-ask the server for every chat,
	// which on a large account meant hundreds of IQs before the list had its
	// pictures. An events.Picture drops the file (invalidateAvatar), so a
	// changed picture is still re-fetched.
	if cached := filepath.Join(w.avatarDir, avatarCacheName(jid)); fileExists(cached) {
		w.memoAvatar(jid, avatarEntry{path: cached})
		return cached, nil
	}

	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: avatar: parse jid %q: %w", jid, err)
	}
	// The picture lives on the phone-number identity; ask for that one when
	// the chat is LID-addressed and the mapping is known.
	if to.Server == types.HiddenUserServer {
		if pn, err := w.wa.Store.LIDs.GetPNForLID(ctx, to); err == nil && !pn.IsEmpty() {
			to = pn
		}
	}

	info, err := w.wa.GetProfilePictureInfo(ctx, to, &whatsmeow.GetProfilePictureParams{Preview: true, ExistingID: entry.id})
	if err != nil {
		if avatarDefinitelyMissing(err) {
			// No picture, privacy-restricted, not in the group: normal, and
			// stable until an events.Picture says otherwise.
			w.memoAvatar(jid, avatarEntry{missing: true})
			return "", nil
		}
		// Not connected yet, IQ timeout, ...: transient. Not memoized, so the
		// next list rebuild (the one on Connected, for instance) retries.
		return "", fmt.Errorf("chatot/client: avatar %s: %w", jid, err)
	}
	if info == nil {
		// (nil, nil) means "unchanged from ExistingID". We only ever call with
		// ExistingID="" (invalidateAvatar drops the whole memo, id included),
		// so whatsmeow shouldn't return this for us — handled defensively
		// anyway since "no new data" is a fine answer either way.
		return "", nil
	}
	if info.URL == "" {
		w.memoAvatar(jid, avatarEntry{missing: true})
		return "", nil
	}

	// WhatsApp says a picture exists; a failed download of it is transient
	// (network blip, expired signed URL) and must not be remembered as
	// "no picture" — leave the memo empty so the next rebuild retries.
	resp, err := http.Get(info.URL)
	if err != nil {
		return "", fmt.Errorf("chatot/client: avatar %s: download: %w", jid, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("chatot/client: avatar %s: read: %w", jid, err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("chatot/client: avatar %s: download: HTTP %d", jid, resp.StatusCode)
	}

	path := filepath.Join(w.avatarDir, avatarCacheName(jid))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", fmt.Errorf("chatot/client: avatar: write cache file: %w", err)
	}
	w.memoAvatar(jid, avatarEntry{id: info.ID, path: path})
	return path, nil
}

// avatarDefinitelyMissing tells the "there is no picture to fetch" answers
// apart from transport failures, which must not be remembered.
func avatarDefinitelyMissing(err error) bool {
	return errors.Is(err, whatsmeow.ErrProfilePictureNotSet) ||
		errors.Is(err, whatsmeow.ErrProfilePictureUnauthorized) ||
		errors.Is(err, whatsmeow.ErrNotInGroup)
}

func (w *Whatsmeow) memoAvatar(jid string, entry avatarEntry) {
	w.avatarMu.Lock()
	if w.avatarMemo == nil {
		w.avatarMemo = make(map[string]avatarEntry)
	}
	w.avatarMemo[jid] = entry
	w.avatarMu.Unlock()
}

// invalidateAvatar drops jid's memo entry and cached file so the next Avatar
// call re-fetches it; called from handleRaw on *events.Picture.
func (w *Whatsmeow) invalidateAvatar(jid string) {
	w.avatarMu.Lock()
	delete(w.avatarMemo, jid)
	w.avatarMu.Unlock()
	if w.avatarDir != "" {
		os.Remove(filepath.Join(w.avatarDir, avatarCacheName(jid)))
	}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}
