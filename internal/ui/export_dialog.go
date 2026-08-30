package ui

import (
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// exportMessageLimit is passed to Client.Messages for an export: large
// enough to return everything the local store has synced for a chat.
// Exporting only covers locally-synced history, same as everything else in
// the conversation view — messages WhatsApp hasn't backfilled to this
// device yet (see RequestMoreHistory) aren't included.
const exportMessageLimit = 1 << 20

// formatChatExport renders msgs (oldest-first, as returned by
// Client.Messages) as plain text: one header line, then one
// "[2006-01-02 15:04] Sender: body" line per message. Non-text bodies
// (media, location, contact, poll, a revoked message) render a bracketed
// marker in place of their payload.
func formatChatExport(msgs []client.Message, contactName string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Chat with %s\nExported %s\n\n", contactName, time.Now().Format("2006-01-02 15:04"))
	for _, m := range msgs {
		sender := contactName
		if m.FromMe {
			sender = "You"
		}
		fmt.Fprintf(&b, "[%s] %s: %s\n", time.Unix(m.TS, 0).Format("2006-01-02 15:04"), sender, exportBody(m, contactName))
	}
	return b.String()
}

// exportBody renders a single message's body for formatChatExport.
func exportBody(m client.Message, contactName string) string {
	switch {
	case m.Deleted:
		return "[deleted]"
	case m.Attachment != nil:
		return exportAttachmentMarker(*m.Attachment)
	case m.Location != nil:
		if m.Location.Name != "" {
			return fmt.Sprintf("[Location: %s]", m.Location.Name)
		}
		return "[Location]"
	case m.Contact != nil:
		return fmt.Sprintf("[Contact: %s]", m.Contact.DisplayName)
	case m.Poll != nil:
		return fmt.Sprintf("[Poll: %s]", m.Poll.Name)
	default:
		return m.Text
	}
}

// exportAttachmentMarker renders the bracketed marker standing in for an
// attachment's raw bytes in the plain-text export.
func exportAttachmentMarker(a client.Attachment) string {
	switch a.Kind {
	case "image":
		return "[Photo]"
	case "video":
		if a.IsGIF {
			return "[GIF]"
		}
		return "[Video]"
	case "audio":
		return "[Voice message]"
	case "document":
		if a.Filename != "" {
			return fmt.Sprintf("[Document: %s]", a.Filename)
		}
		return "[Document]"
	case "sticker":
		return "[Sticker]"
	default:
		return fmt.Sprintf("[%s]", a.Kind)
	}
}

// slugForFilename lowercases name and replaces every run of characters that
// aren't letters/digits with a single '-', for a safe default export
// filename ("Ada Lovelace" -> "ada-lovelace").
func slugForFilename(name string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z' || r >= '0' && r <= '9':
			b.WriteRune(r)
			prevDash = false
		case !prevDash:
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		return "chat"
	}
	return slug
}

// defaultExportPath is ~/Documents/<slug(contactName)>.txt, falling back to
// the current directory if the home dir can't be resolved.
func defaultExportPath(contactName string) string {
	dir := "."
	if home, err := os.UserHomeDir(); err == nil {
		dir = filepath.Join(home, "Documents")
	}
	return filepath.Join(dir, slugForFilename(contactName)+".txt")
}

// exportMediaCount reports downloaded-media stats for jid's Include-media
// toggle subtitle: the number of image/video/document attachments and, when
// every one of them has already been downloaded (LocalPath set), a rough
// total size read via os.Stat. size is 0 (and ok false) if any file's size
// couldn't be determined, in which case the caller shows just the count.
func exportMediaCount(c client.Client, jid string) (count int, size int64, sizeKnown bool) {
	sizeKnown = true
	media, _ := c.ChatMedia(jid)
	for _, m := range media {
		count++
		if m.LocalPath == "" {
			sizeKnown = false
			continue
		}
		if info, err := os.Stat(m.LocalPath); err == nil {
			size += info.Size()
		} else {
			sizeKnown = false
		}
	}
	docs, _ := c.ChatDocs(jid)
	for _, d := range docs {
		count++
		if d.LocalPath == "" {
			sizeKnown = false
			continue
		}
		if info, err := os.Stat(d.LocalPath); err == nil {
			size += info.Size()
		} else {
			sizeKnown = false
		}
	}
	return count, size, sizeKnown
}

// humanBytes renders a byte count as a rough "about N MB"/"N KB" figure for
// the Include-media subtitle; no need for exactness here.
func humanBytes(n int64) string {
	const (
		kb = 1 << 10
		mb = 1 << 20
		gb = 1 << 30
	)
	switch {
	case n >= gb:
		return strconv.FormatFloat(float64(n)/gb, 'f', 1, 64) + " GB"
	case n >= mb:
		return strconv.FormatFloat(float64(n)/mb, 'f', 0, 64) + " MB"
	case n >= kb:
		return strconv.FormatFloat(float64(n)/kb, 'f', 0, 64) + " KB"
	default:
		return strconv.FormatInt(n, 10) + " B"
	}
}

// includeMediaSubtitle renders exportMediaCount's result the way the
// mockup's "48 files · about 96 MB" row does, falling back to just the
// count when a file's size (or the file itself) is unavailable.
func includeMediaSubtitle(count int, size int64, sizeKnown bool) string {
	noun := "files"
	if count == 1 {
		noun = "file"
	}
	if count == 0 {
		return "No media in this chat"
	}
	if sizeKnown {
		return fmt.Sprintf("%d %s · about %s", count, noun, humanBytes(size))
	}
	return fmt.Sprintf("%d %s", count, noun)
}

// copyChatMedia best-effort copies every downloaded image/video/document in
// jid into destDir (created if needed), skipping attachments that were
// never downloaded (LocalPath == ""). Failures on individual files are
// logged and otherwise ignored — a partial media export still leaves the
// text export intact.
func copyChatMedia(c client.Client, jid, destDir string) {
	var paths []string
	if media, _ := c.ChatMedia(jid); media != nil {
		for _, m := range media {
			if m.LocalPath != "" {
				paths = append(paths, m.LocalPath)
			}
		}
	}
	if docs, _ := c.ChatDocs(jid); docs != nil {
		for _, d := range docs {
			if d.LocalPath != "" {
				paths = append(paths, d.LocalPath)
			}
		}
	}
	if len(paths) == 0 {
		return
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		log.Printf("chatot: export media: create %s: %v", destDir, err)
		return
	}
	for _, src := range paths {
		dst := filepath.Join(destDir, filepath.Base(src))
		if err := copyFile(src, dst); err != nil {
			log.Printf("chatot: export media: copy %s: %v", src, err)
		}
	}
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ShowExportDialog opens the "Export chat" dialog for jid: Format and Range
// are shown as dropdowns but each carries only today's supported choice
// (plain text / all messages) — more formats and date-range export are
// deferred. Include-media, when on, also copies downloaded attachments into
// a "<file>_media/" folder alongside the text export.
func ShowExportDialog(parent *gtk.Window, c client.Client, jid, contactName string, toastOverlay *adw.ToastOverlay) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Export chat")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(420, 0)

	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.SetMarginTop(16)
	box.SetMarginBottom(16)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)

	box.Append(dropdownRow("Format", gtk.NewDropDownFromStrings([]string{"Plain text"})))
	box.Append(dropdownRow("Range", gtk.NewDropDownFromStrings([]string{"All messages"})))

	count, size, sizeKnown := exportMediaCount(c, jid)
	mediaRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	mediaText := gtk.NewBox(gtk.OrientationVertical, 2)
	mediaTitle := gtk.NewLabel("Include media")
	mediaTitle.SetXAlign(0)
	mediaText.Append(mediaTitle)
	mediaSubtitle := gtk.NewLabel(includeMediaSubtitle(count, size, sizeKnown))
	mediaSubtitle.SetXAlign(0)
	mediaSubtitle.AddCSSClass("dim-label")
	mediaText.Append(mediaSubtitle)
	mediaText.SetHExpand(true)
	mediaRow.Append(mediaText)
	includeMedia := gtk.NewSwitch()
	includeMedia.SetActive(count > 0)
	includeMedia.SetVAlign(gtk.AlignCenter)
	mediaRow.Append(includeMedia)
	box.Append(mediaRow)

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	saveLabel := gtk.NewLabel("SAVE TO")
	saveLabel.SetXAlign(0)
	saveLabel.AddCSSClass("dim-label")
	box.Append(saveLabel)

	pathLabel := gtk.NewLabel(defaultExportPath(contactName))
	pathLabel.SetXAlign(0)
	pathLabel.SetEllipsize(pango.EllipsizeMiddle)
	pathLabel.SetHExpand(true)
	chooseBtn := gtk.NewButtonWithLabel("Choose…")
	saveRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	saveRow.Append(pathLabel)
	saveRow.Append(chooseBtn)
	box.Append(saveRow)

	chooseBtn.ConnectClicked(func() {
		fd := gtk.NewFileDialog()
		fd.SetTitle("Export chat to")
		fd.SetInitialName(filepath.Base(pathLabel.Text()))
		fd.Save(context.Background(), dialog, func(res gio.AsyncResulter) {
			file, err := fd.SaveFinish(res)
			if err != nil {
				return // cancelled
			}
			pathLabel.SetText(file.Path())
		})
	})

	btnRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	btnRow.SetHAlign(gtk.AlignEnd)
	cancelBtn := gtk.NewButtonWithLabel("Cancel")
	exportBtn := gtk.NewButtonWithLabel("Export")
	exportBtn.AddCSSClass("suggested-action")
	btnRow.Append(cancelBtn)
	btnRow.Append(exportBtn)
	box.Append(btnRow)

	cancelBtn.ConnectClicked(func() { dialog.Close() })
	exportBtn.ConnectClicked(func() {
		path := pathLabel.Text()
		withMedia := includeMedia.Active()
		dialog.Close()

		go func() {
			msgs, err := c.Messages(jid, exportMessageLimit)
			if err != nil {
				log.Printf("chatot: export chat: load messages failed: %v", err)
				return
			}
			text := formatChatExport(msgs, contactName)
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				log.Printf("chatot: export chat: create dir: %v", err)
				return
			}
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				log.Printf("chatot: export chat: write %s: %v", path, err)
				return
			}
			if withMedia {
				ext := filepath.Ext(path)
				mediaDir := strings.TrimSuffix(path, ext) + "_media"
				copyChatMedia(c, jid, mediaDir)
			}
			if toastOverlay == nil {
				return
			}
			glib.IdleAdd(func() {
				toastOverlay.AddToast(adw.NewToast(fmt.Sprintf("Exported to %s", path)))
			})
		}()
	})

	dialog.SetChild(box)
	dialog.SetDefaultWidget(exportBtn)
	dialog.Present()
}

// dropdownRow pairs a left-aligned title with a right-hand dropdown, for the
// Format/Range rows (each a single-choice stand-in until more formats/date
// ranges are supported).
func dropdownRow(title string, dropdown *gtk.DropDown) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	label := gtk.NewLabel(title)
	label.SetXAlign(0)
	label.SetHExpand(true)
	row.Append(label)
	row.Append(dropdown)
	return row
}
