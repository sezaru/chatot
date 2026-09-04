package ui

import (
	"context"
	_ "embed"
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/media"
	"chatot/internal/settings"
)

// NotificationSound gates the chime played alongside each desktop
// notification. GNotification carries no sound hint, and the daemons that
// display it (GNOME Shell, the portal, mako and friends) stay silent for app
// notifications, so the app rings on its own. Mirrors
// settings.NotificationSound; main.go sets it at startup and the Preferences
// window on change.
var NotificationSound = true

// notifyChime is the built-in chime, WhatsApp's notification tone (Ogg
// Vorbis, since MP3 aborts GtkMediaFile here), used unless the user supplies
// a sound file.
//
//go:embed assets/notify.oga
var notifyChime []byte

// notifySoundExts lists, in lookup order, the extensions a user-supplied
// notification sound may have (anything GStreamer decodes).
var notifySoundExts = []string{".oga", ".ogg", ".opus", ".flac", ".wav", ".mp3", ".m4a"}

// notifyStream is the stream of the chime currently playing, kept reachable
// so the finalizer cannot stop it mid-note; the next chime replaces it.
var notifyStream *gtk.MediaFile

// playNotificationSound plays the message chime: the user's own
// $XDG_CONFIG_HOME/chatot/notify.<ext> when present, the embedded chime
// otherwise. Main loop only (it touches GTK).
//
// A user file goes through media.PlayableAudio first, like every other audio
// the app plays: handing GtkMediaFile an MP3 aborts the process on this
// GStreamer. The transcode (cached after the first time) runs off the main
// loop; if it fails the built-in chime rings instead.
func playNotificationSound() {
	path := userSoundPath(settings.Dir())
	if path == "" {
		playChime(gtk.NewMediaFileForInputStream(gio.NewMemoryInputStreamFromBytes(glib.NewBytesWithGo(notifyChime))))
		return
	}
	go func() {
		out, err := media.PlayableAudio(context.Background(), playableCacheDir(), path, "")
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: notification sound %s: %v; using the built-in chime", path, err)
				playChime(gtk.NewMediaFileForInputStream(gio.NewMemoryInputStreamFromBytes(glib.NewBytesWithGo(notifyChime))))
				return
			}
			playChime(gtk.NewMediaFileForFilename(out))
		})
	}()
}

// playChime starts stream, cutting short a chime still ringing.
func playChime(stream *gtk.MediaFile) {
	if notifyStream != nil {
		notifyStream.Pause()
	}
	notifyStream = stream
	stream.Play()
}

// userSoundPath returns the user's notification sound in dir
// (notify.oga, notify.ogg, … per notifySoundExts), or "" when there is none.
func userSoundPath(dir string) string {
	for _, ext := range notifySoundExts {
		p := filepath.Join(dir, "notify"+ext)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return ""
}
