package ui

import (
	"context"
	"log"
	"mime"
	"path/filepath"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/audio"
	"chatot/internal/client"
)

// reactEmojis is the fixed quick-react set offered on every bubble.
var reactEmojis = []string{"👍", "❤️", "😂", "😮", "😢", "🙏"}

// SendReadReceipts gates MarkRead calls on chat open. Default false: chatot
// reads privately (whatsapp never learns a chat was opened here) until a
// user-facing setting exists.
var SendReadReceipts = false

// sendAction is what composeState.Submit resolves a submission to: the
// send it wants performed, if any.
type sendAction struct {
	JID     string
	Text    string
	ReplyTo *client.MsgRef
}

// composeState is the composer's pure state machine, kept free of GTK so it
// can be unit-tested directly. Composer builds the widget on top of it.
type composeState struct {
	jid     string
	replyTo *client.Message
}

// SetChat switches the active chat, clearing any pending reply (replying
// across a chat switch would attach the wrong context).
func (s *composeState) SetChat(jid string) {
	s.jid = jid
	s.replyTo = nil
}

// StartReply arms reply mode, quoting msg on the next Submit.
func (s *composeState) StartReply(msg client.Message) {
	m := msg
	s.replyTo = &m
}

// CancelReply clears reply mode without sending.
func (s *composeState) CancelReply() {
	s.replyTo = nil
}

// ReplyTarget returns the message being replied to, if any.
func (s *composeState) ReplyTarget() (client.Message, bool) {
	if s.replyTo == nil {
		return client.Message{}, false
	}
	return *s.replyTo, true
}

// mediaAction is what an attach-file pick resolves to.
type mediaAction struct {
	JID     string
	Path    string
	Caption string
	ReplyTo *client.MsgRef
}

// SubmitMedia resolves an attach-file send given the picked path and the
// composer's current entry text (used as the caption, then cleared like a
// text send). Fails (ok=false) if there's no active chat or path is blank.
func (s *composeState) SubmitMedia(path, caption string) (mediaAction, bool) {
	path = strings.TrimSpace(path)
	if path == "" || s.jid == "" {
		return mediaAction{}, false
	}
	action := mediaAction{JID: s.jid, Path: path, Caption: strings.TrimSpace(caption)}
	if s.replyTo != nil {
		action.ReplyTo = &client.MsgRef{ChatJID: s.replyTo.ChatJID, MsgID: s.replyTo.ID}
	}
	s.replyTo = nil
	return action, true
}

// Submit resolves a send attempt for text. It fails (ok=false) if there's no
// active chat or the text is blank/whitespace-only; nothing is cleared in
// that case. On success it returns the action to perform and clears any
// pending reply, so the caller's next Submit starts fresh.
func (s *composeState) Submit(text string) (sendAction, bool) {
	text = strings.TrimSpace(text)
	if text == "" || s.jid == "" {
		return sendAction{}, false
	}
	action := sendAction{JID: s.jid, Text: text}
	if s.replyTo != nil {
		action.ReplyTo = &client.MsgRef{ChatJID: s.replyTo.ChatJID, MsgID: s.replyTo.ID}
	}
	s.replyTo = nil
	return action, true
}

// unreadMessageIDs returns the IDs of the most recent `count` inbound
// (non-FromMe) messages in msgs (oldest-first, as ConversationView.Load
// returns them) — the best-effort set to mark read given only Chat's
// UnreadCount, since messages carry no per-row read flag.
func unreadMessageIDs(msgs []client.Message, count int) []string {
	if count <= 0 {
		return nil
	}
	var ids []string
	for i := len(msgs) - 1; i >= 0 && len(ids) < count; i-- {
		if !msgs[i].FromMe {
			ids = append(ids, msgs[i].ID)
		}
	}
	return ids
}

// MarkReadOnOpen sends read receipts for a just-opened chat, gated on
// SendReadReceipts. Runs the network call synchronously; callers invoke it
// from a goroutine to keep the GTK main loop unblocked.
func MarkReadOnOpen(ctx context.Context, c client.Client, jid string, msgs []client.Message, unreadCount int) {
	if !SendReadReceipts {
		return
	}
	ids := unreadMessageIDs(msgs, unreadCount)
	if len(ids) == 0 {
		return
	}
	if err := c.MarkRead(ctx, jid, ids); err != nil {
		log.Printf("chatot: mark read failed: %v", err)
	}
}

// Composer is the bottom bar: a reply-quote strip (shown only in reply
// mode), a text entry and a send button, backed by composeState.
type Composer struct {
	*gtk.Box

	c      client.Client
	state  composeState
	window *gtk.Window // parent for gtk.FileDialog; set via SetWindow

	quoteBar   *gtk.Box
	quoteLabel *gtk.Label
	entry      *gtk.Entry
	attachBtn  *gtk.Button
	recordBtn  *gtk.Button

	recorder  *audio.Recorder // non-nil only while a recording is in progress
	recording bool

	onSent func(client.Message)
}

// NewComposer builds an empty, disabled-until-SetChat Composer backed by c.
func NewComposer(c client.Client) *Composer {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-composer")

	quoteBar := gtk.NewBox(gtk.OrientationHorizontal, 6)
	quoteBar.AddCSSClass("chatot-composer-quote")
	quoteBar.SetVisible(false)

	quoteLabel := gtk.NewLabel("")
	quoteLabel.SetXAlign(0)
	quoteLabel.SetHExpand(true)
	quoteLabel.SetWrap(true)
	quoteBar.Append(quoteLabel)

	cancelQuote := gtk.NewButtonWithLabel("×")
	cancelQuote.AddCSSClass("flat")
	quoteBar.Append(cancelQuote)

	root.Append(quoteBar)

	entryRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	entryRow.SetMarginTop(6)
	entryRow.SetMarginBottom(6)
	entryRow.SetMarginStart(8)
	entryRow.SetMarginEnd(8)

	attachBtn := gtk.NewButtonWithLabel("📎")
	attachBtn.AddCSSClass("flat")
	attachBtn.SetSensitive(false)
	entryRow.Append(attachBtn)

	entry := gtk.NewEntry()
	entry.SetHExpand(true)
	entry.SetPlaceholderText("Message")
	entryRow.Append(entry)

	recordBtn := gtk.NewButtonWithLabel("🎤")
	recordBtn.AddCSSClass("flat")
	recordBtn.SetSensitive(false)
	entryRow.Append(recordBtn)

	sendBtn := gtk.NewButtonWithLabel("Send")
	sendBtn.AddCSSClass("suggested-action")
	entryRow.Append(sendBtn)

	root.Append(entryRow)

	comp := &Composer{
		Box:        root,
		c:          c,
		quoteBar:   quoteBar,
		quoteLabel: quoteLabel,
		entry:      entry,
		attachBtn:  attachBtn,
		recordBtn:  recordBtn,
	}

	entry.ConnectActivate(comp.submit)
	sendBtn.ConnectClicked(comp.submit)
	attachBtn.ConnectClicked(comp.pickAttachment)
	recordBtn.ConnectClicked(comp.toggleRecording)
	cancelQuote.ConnectClicked(func() {
		comp.state.CancelReply()
		comp.refreshQuoteBar()
	})

	return comp
}

// SetWindow supplies the parent window gtk.FileDialog needs; call once
// before the attach button can be used (main.go does this after the window
// is constructed).
func (c *Composer) SetWindow(w *gtk.Window) { c.window = w }

// OnSent registers f to be called (on the GTK main loop) with the optimistic
// outbound message right after a successful send.
func (c *Composer) OnSent(f func(client.Message)) { c.onSent = f }

// SetChat switches the composer to jid, clearing any pending reply.
func (c *Composer) SetChat(jid string) {
	c.state.SetChat(jid)
	c.attachBtn.SetSensitive(jid != "")
	c.recordBtn.SetSensitive(jid != "" && !c.recording)
	c.refreshQuoteBar()
}

// StartReply arms reply mode for msg; called from the conversation view's
// per-bubble reply affordance.
func (c *Composer) StartReply(msg client.Message) {
	c.state.StartReply(msg)
	c.refreshQuoteBar()
}

func (c *Composer) refreshQuoteBar() {
	target, ok := c.state.ReplyTarget()
	if !ok {
		c.quoteBar.SetVisible(false)
		return
	}
	text := target.Text
	if text == "" {
		text = "[media]"
	}
	c.quoteLabel.SetLabel(text)
	c.quoteBar.SetVisible(true)
}

// submit resolves the current entry text against composeState and, if
// valid, clears the entry/reply-mode immediately and sends in the
// background. Must run on the GTK main loop (entry.Text() isn't thread-safe).
func (c *Composer) submit() {
	action, ok := c.state.Submit(c.entry.Text())
	if !ok {
		return
	}
	c.entry.SetText("")
	c.refreshQuoteBar()

	go func() {
		id, err := c.c.SendText(context.Background(), action.JID, action.Text, action.ReplyTo)
		if err != nil {
			log.Printf("chatot: send failed: %v", err)
			return
		}
		if c.onSent == nil {
			return
		}
		msg := client.Message{
			ID: id, ChatJID: action.JID, FromMe: true,
			Text: action.Text, TS: time.Now().Unix(), ReplyTo: action.ReplyTo,
		}
		glib.IdleAdd(func() { c.onSent(msg) })
	}()
}

// pickAttachment opens a file-choose dialog and, on a picked file, hands off
// to sendMedia. No-ops if there's no active chat or no window has been set
// yet (attachBtn is also disabled in that first case, but this guards
// against a stray click racing SetChat/SetWindow).
func (c *Composer) pickAttachment() {
	if !c.attachBtn.Sensitive() || c.window == nil {
		return
	}
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Send file")
	dialog.Open(context.Background(), c.window, func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil {
			return // cancelled or failed; nothing to log, this is the common case
		}
		c.sendMedia(file.Path())
	})
}

// sendMedia resolves the picked path (using the current entry text as
// caption) against composeState and sends it in the background, mirroring
// submit's clear-then-send-then-idle-append flow.
func (c *Composer) sendMedia(path string) {
	action, ok := c.state.SubmitMedia(path, c.entry.Text())
	if !ok {
		return
	}
	c.entry.SetText("")
	c.refreshQuoteBar()
	c.attachBtn.SetSensitive(false)

	go func() {
		att := client.Attachment{LocalPath: action.Path, Filename: filepath.Base(action.Path), Caption: action.Caption}
		id, err := c.c.SendMedia(context.Background(), action.JID, att, action.ReplyTo)
		glib.IdleAdd(func() {
			c.attachBtn.SetSensitive(c.state.jid != "")
			if err != nil {
				log.Printf("chatot: send media failed: %v", err)
				return
			}
			if c.onSent == nil {
				return
			}
			msg := client.Message{
				ID: id, ChatJID: action.JID, FromMe: true, TS: time.Now().Unix(), ReplyTo: action.ReplyTo,
				Attachment: &client.Attachment{
					Kind: guessAttachmentKind(action.Path), Filename: att.Filename,
					LocalPath: action.Path, Caption: action.Caption,
				},
			}
			c.onSent(msg)
		})
	}()
}

// toggleRecording starts a voice-note recording on the first click and
// stops+sends it on the second. Guards against a stray click while there's
// no active chat (recordBtn is disabled then, but SetChat/click can race)
// or while a recording is already in flight.
func (c *Composer) toggleRecording() {
	if c.recording {
		c.stopRecording()
		return
	}
	if c.state.jid == "" || c.recorder != nil {
		return
	}

	rec := &audio.Recorder{}
	if err := rec.Start(); err != nil {
		log.Printf("chatot: start recording failed: %v", err)
		return
	}
	c.recorder = rec
	c.recording = true
	c.recordBtn.SetLabel("⏹")
	c.recordBtn.AddCSSClass("destructive-action")
	c.entry.SetSensitive(false)
	c.attachBtn.SetSensitive(false)
}

// stopRecording finalizes the in-flight recording and sends it in the
// background, mirroring submit/sendMedia's goroutine + IdleAdd pattern. Any
// failure (stop or send) resets the button without crashing.
func (c *Composer) stopRecording() {
	rec := c.recorder
	jid := c.state.jid
	c.recorder = nil
	c.recording = false
	c.resetRecordButton()

	if rec == nil {
		return
	}

	oggBytes, dur, err := rec.Stop()
	if err != nil {
		log.Printf("chatot: stop recording failed: %v", err)
		return
	}

	go func() {
		id, err := c.c.SendVoice(context.Background(), jid, oggBytes, dur)
		// SendVoice already cached the ogg + set local_path; DownloadMedia
		// returns that cached path without a network hit, so the just-sent
		// note renders inline (MediaControls) instead of a tap-to-load chip.
		var localPath string
		if err == nil {
			localPath, _ = c.c.DownloadMedia(context.Background(), id)
		}
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: send voice failed: %v", err)
				return
			}
			if c.onSent == nil {
				return
			}
			c.onSent(client.Message{
				ID: id, ChatJID: jid, FromMe: true, TS: time.Now().Unix(),
				Attachment: &client.Attachment{Kind: "audio", MimeType: "audio/ogg; codecs=opus", LocalPath: localPath},
			})
		})
	}()
}

// resetRecordButton restores the mic button to its idle appearance and
// re-enables the entry/attach controls it disabled while recording.
func (c *Composer) resetRecordButton() {
	c.recordBtn.SetLabel("🎤")
	c.recordBtn.RemoveCSSClass("destructive-action")
	c.recordBtn.SetSensitive(c.state.jid != "")
	c.entry.SetSensitive(true)
	c.attachBtn.SetSensitive(c.state.jid != "")
}

// guessAttachmentKind best-effort classifies path by extension, for the
// optimistic sent-bubble render only; Whatsmeow.SendMedia's own detection
// (sniffed from file bytes, see detectAttachmentKind) is authoritative and
// is what actually lands in the store row.
func guessAttachmentKind(path string) string {
	mt := mime.TypeByExtension(filepath.Ext(path))
	switch {
	case strings.HasPrefix(mt, "image/"):
		return "image"
	case strings.HasPrefix(mt, "video/"):
		return "video"
	case strings.HasPrefix(mt, "audio/"):
		return "audio"
	default:
		return "document"
	}
}
