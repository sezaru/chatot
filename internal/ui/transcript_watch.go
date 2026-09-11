package ui

import (
	"context"
	"log"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"

	"chatot/internal/client"
	"chatot/internal/transcribe"
)

// arrivalDownloadTimeout bounds the fetch of a voice note that arrived,
// before its transcription.
const arrivalDownloadTimeout = 2 * time.Minute

// WatchVoiceNotes transcribes voice notes as they arrive, under the
// automatic preference, whether or not their chat is on screen: the text
// is what makes a note readable from the chat list and the search, and
// waiting for the reader to open the chat left every other note
// untouched. Each note is fetched (only where the download preference
// would fetch it anyway) and put on the queue behind any clicked run; the
// open thread, if it is the note's, shows the run the way it shows its
// own. events is a subscription taken before the client started; the loop
// runs until it closes.
func WatchVoiceNotes(c client.Client, events <-chan client.Event, cv *ConversationView) {
	for ev := range events {
		if ev.Kind != client.EventMessage || ev.Message == nil {
			continue
		}
		m := *ev.Message
		// The preferences are read where they are written, on the main
		// loop; the work itself goes off it.
		glib.IdleAdd(func() {
			if !wantsArrivalTranscript(m, AutoTranscribe, AutoDownload, time.Now()) ||
				!transcribe.EngineAvailable() || !transcribe.ModelReady(cacheDir()) {
				return
			}
			go transcribeArrival(c, cv, m)
		})
	}
}

// wantsArrivalTranscript reports whether m, just arrived, is a voice note
// to transcribe without a click: under the automatic preference, recent,
// without a transcript, and either already on disk or one the download
// preference fetches by itself (the run needs the file, and a reader who
// asked for no automatic downloads gets none).
func wantsArrivalTranscript(m client.Message, autoTranscribe bool, autoDownload string, now time.Time) bool {
	a := m.Attachment
	if a == nil || a.Kind != "audio" || m.Deleted {
		return false
	}
	if !autoTranscribeWants(autoTranscribe, a.Transcript, m.TS, now) {
		return false
	}
	return a.LocalPath != "" || autoDownloadWants(autoDownload, "audio", m.TS, now)
}

// transcribeArrival fetches m's note and queues its transcription. Runs
// off the main loop.
func transcribeArrival(c client.Client, cv *ConversationView, m client.Message) {
	ctx, cancel := context.WithTimeout(context.Background(), arrivalDownloadTimeout)
	defer cancel()
	path, err := c.DownloadMedia(ctx, m.ID)
	if err != nil {
		log.Printf("chatot: transcribe on arrival %s: download: %v", m.ID, err)
		return
	}
	id := m.ID
	glib.IdleAdd(func() { cv.SetLocalPath(id, path) })
	if !transcribe.Default.Enqueue(transcriptJob(c, cv, m.ChatJID, id, path, false)) {
		// The automatic backlog is full; the bubble asks again when the
		// chat is opened.
		trace(1, "transcribe on arrival: backlog full, %s dropped", id)
		return
	}
	trace(1, "transcribe on arrival: %s queued (%d waiting)", id, transcribe.Default.Pending())
	glib.IdleAdd(func() { cv.transcriptQueued(id) })
}
