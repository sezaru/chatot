# The attachment viewer — design + implementation notes

Lives in `Chatot Interactive.dc.html` as `pane: 'viewer'`. It replaces the
separate image window (`showImageViewer` / `showVideoViewer`) with one pane
that takes over the conversation column, the way `MediaPage` already does for
"Media, links and docs".

## Why a pane, not a window

The old modal was a second window over the chat: it owned the whole screen for
one photo, it could only show photos, and its four equal-weight buttons
(Forward / Copy / Save as… / Open) gave no sense of what the main action was.

The pane fixes all three:

- **It replaces the conversation, keeping the chat header, the chat list and
  the account rail.** You never lose your place, and `Esc` / `←` puts the
  conversation back exactly where it was.
- **It carries every attachment kind**, not just images.
- **One primary action per attachment**, in a fixed position, with everything
  else demoted to icon buttons and the details sidebar.

## Anatomy

```
┌ header ──────────────────────────────────────────────┬──────────┐
│ ←  [icon] title / from · when · size    n/N  ↩↪☆ℹ⋯ [Primary]   │
├──────────────────────────────────────────────────────┤ DETAILS  │
│                                                      │ sender   │
│  ‹            stage (per kind)                    ›  │ facts    │
│                                                      │ actions  │
├──────────────────────────────────────────────────────┤ keyboard │
│ control bar: zoom / pages, or transport              │ notice   │
├──────────────────────────────────────────────────────┤          │
│ caption + reactions (only when the message has them) │          │
├──────────────────────────────────────────────────────┤          │
│ filmstrip: every attachment in this chat, in order   │          │
└──────────────────────────────────────────────────────┴──────────┘
```

The filmstrip is the part that makes it one viewer instead of six: photos,
videos, PDFs, files, voice notes and locations are one ordered sequence per
chat, so `←`/`→` and the counter cross kinds. Thumbs are typed — hatched frame
with a glyph for media, coloured extension tile for documents, live map crop
for locations, duration chip for video and audio, `⬇` when a file has not been
downloaded yet.

## Per kind

| Kind | Stage | Control bar | Primary |
|---|---|---|---|
| Photo / GIF | Fit-to-window frame on a near-black stage | Zoom −, %, +, Fit | `Save…` |
| Video | Poster + centre play button | Play, elapsed, scrub, total, speed, mute, fullscreen | `Save…` |
| Voice / audio | Card with sender, waveform, played portion in accent | same transport, no fullscreen | `Save…` |
| PDF | White page sheet on the theme background | `‹ n / N ›` + zoom | `Open` |
| Other file | Card: extension tile, name, meta, "no preview" line | — | `Open` |
| Location | Real OSM crop, pin, address card, ± zoom | — | `Open in Maps` |

Not downloaded yet is a stage state, not a dead viewer: the stage shows the
kind, the size and one accent circle that fetches it; failure turns the circle
red with a retry line. The details sidebar collapses to `Download now` while
the file is missing, so it never offers `Copy image` for bytes we don't have.

Two things deliberately stay out of the viewer: **view-once photos** (opening
one is a one-shot action that belongs in the bubble) and **contact cards**
(they are not files).

## Rules the pane keeps

- **One accent-filled control on screen.** The header's primary action is the
  only filled button; the file card's own actions are outlined, and they hide
  entirely while the details sidebar is showing the same two.
- **Fit means fit.** The stage measures itself (`measureStage`) instead of
  assuming a pane width, because the details sidebar, the control bar and the
  caption row each change how much room the attachment gets.
- **The map is real.** The stage reuses the `map-porto-z1{6,7,8}.png` crops
  through `mapWindow`, which centres the shared point in the card and reports
  the true coordinates — same maths as the location picker, at a bigger size.
- **Nothing dead-ends in the viewer.** `Show in chat` returns to the message,
  `↩` returns with the reply bar already primed.

Keyboard, listed in the sidebar so it is discoverable: `Esc` back, `←`/`→`
previous/next, `+`/`−`/`0` zoom in/out/fit, `Space` play or pause.

## Wiring on the Go side

- Replace `showImageViewer` and `showVideoViewer` (`internal/ui/image_viewer.go`,
  `internal/ui/video_viewer.go`) with a viewer page built like
  `internal/ui/media_pane.go`: an `adw.NavigationPage` / stack child swapped
  into the conversation column with an `onBack`, not a `gtk.Window`.
- The attachment sequence comes from the same store query the media browser
  uses; the viewer needs message order across kinds, not just images, so extend
  the media query rather than filtering the loaded message list.
- Stage widgets already exist or have a clear owner: `GtkPicture` for photos,
  the existing GStreamer sink for video, `locationView`/`locationMap` for the
  map card, poppler-glib for PDF pages (already available in the devenv), and
  the voice-note waveform from the bubble for audio.
- Undownloaded attachments reuse the bubble's fetch path; the stage should
  react to the same download state so the pane and the bubble never disagree.
- Keep `Esc`, `←`/`→`, `+`/`−`/`0` and `Space` as pane-scoped
  `GtkShortcutController` actions so they don't fire while the composer or a
  dialog has focus.
