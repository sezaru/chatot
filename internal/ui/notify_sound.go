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

// NotificationSoundFile is the user's own chime from Preferences (mirrors
// settings.NotificationSoundFile); "" falls through to the defaults.
var NotificationSoundFile = ""

// notifySoundEnv names the environment variable a package (the Nix
// derivation's notificationSound argument) sets to replace the built-in
// chime with its own file.
const notifySoundEnv = "CHATOT_NOTIFY_SOUND"

// notifyChime is the built-in chime: a short synthesized two-note bell (Ogg
// Vorbis, since MP3 aborts GtkMediaFile here). Deliberately generic, so a
// packager can swap in a branded tone via CHATOT_NOTIFY_SOUND without the
// repository carrying it.
//
//go:embed assets/notify.oga
var notifyChime []byte

// notifySoundExts lists, in lookup order, the extensions a drop-in
// notification sound may have (anything GStreamer or ffmpeg decodes).
var notifySoundExts = []string{".oga", ".ogg", ".opus", ".flac", ".wav", ".mp3", ".m4a"}

// notifyStream is the stream of the chime currently playing, kept reachable
// so the finalizer cannot stop it mid-note; the next chime replaces it.
var notifyStream *gtk.MediaFile

// Sound sources, in the order notificationSoundSource consults them.
const (
	soundSourceCustom  = "custom"   // the file picked in Preferences
	soundSourceDropIn  = "drop-in"  // notify.<ext> in the config dir
	soundSourcePackage = "package"  // CHATOT_NOTIFY_SOUND
	soundSourceBuiltIn = "built-in" // the embedded chime
)

// notificationSoundSource resolves which file the chime plays: the file
// picked in Preferences, else a notify.<ext> dropped in the config dir,
// else the packager's CHATOT_NOTIFY_SOUND, else the built-in chime (path
// ""). A configured file that has since vanished is skipped, not an error.
func notificationSoundSource(custom, cfgDir, packaged string) (path, source string) {
	if custom != "" && fileExists(custom) {
		return custom, soundSourceCustom
	}
	if p := userSoundPath(cfgDir); p != "" {
		return p, soundSourceDropIn
	}
	if packaged != "" && fileExists(packaged) {
		return packaged, soundSourcePackage
	}
	return "", soundSourceBuiltIn
}

// currentNotificationSound is notificationSoundSource over the live
// settings and environment.
func currentNotificationSound() (path, source string) {
	return notificationSoundSource(NotificationSoundFile, settings.Dir(), os.Getenv(notifySoundEnv))
}

// playNotificationSound plays the message chime from whichever source
// currentNotificationSound picks. Main loop only (it touches GTK).
func playNotificationSound() {
	path, _ := currentNotificationSound()
	playSoundFile(path, nil)
}

// playSoundFile plays the audio at path, or the built-in chime when path is
// "". A file goes through media.PlayableAudio first, like every other audio
// the app plays: handing GtkMediaFile an MP3 aborts the process on this
// GStreamer. The transcode (cached after the first time) runs off the main
// loop; if it fails the built-in chime rings instead and onErr, if given,
// hears why. Main loop only.
func playSoundFile(path string, onErr func(error)) {
	if path == "" {
		playChime(builtInChime())
		return
	}
	go func() {
		out, err := media.PlayableAudio(context.Background(), playableCacheDir(), path, "")
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: notification sound %s: %v; using the built-in chime", path, err)
				if onErr != nil {
					onErr(err)
				}
				playChime(builtInChime())
				return
			}
			playChime(gtk.NewMediaFileForFilename(out))
		})
	}()
}

func builtInChime() *gtk.MediaFile {
	return gtk.NewMediaFileForInputStream(gio.NewMemoryInputStreamFromBytes(glib.NewBytesWithGo(notifyChime)))
}

// playChime starts stream, cutting short a chime still ringing.
func playChime(stream *gtk.MediaFile) {
	if notifyStream != nil {
		notifyStream.Pause()
	}
	notifyStream = stream
	stream.Play()
}

// userSoundPath returns the drop-in notification sound in dir (notify.oga,
// notify.ogg, … per notifySoundExts), or "" when there is none.
func userSoundPath(dir string) string {
	for _, ext := range notifySoundExts {
		p := filepath.Join(dir, "notify"+ext)
		if fileExists(p) {
			return p
		}
	}
	return ""
}

func fileExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}
