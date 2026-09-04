package client

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// A picture already on disk is served without the server, and dropping it
// removes the file so the next call fetches again.
func TestAvatarServesDiskCacheFirst(t *testing.T) {
	dir := t.TempDir()
	w := &Whatsmeow{avatarDir: dir}
	jid := "5511999@s.whatsapp.net"
	path := filepath.Join(dir, avatarCacheName(jid))
	if err := os.WriteFile(path, []byte("jpg"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := w.Avatar(context.Background(), jid)
	if err != nil || got != path {
		t.Fatalf("Avatar = %q, %v; want the cached file", got, err)
	}
	if _, memo := w.avatarMemo[jid]; !memo {
		t.Fatal("the disk hit should be memoized")
	}
	w.invalidateAvatar(jid)
	if fileExists(path) {
		t.Fatal("invalidate should drop the cached file")
	}
}
