package ui

import (
	"context"
	"log"
	"mime"
	"path/filepath"
	"strconv"
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

// editTarget is the message being edited in the composer's edit mode.
type editTarget struct {
	MsgID string
	Text  string
}

// editAction is what SubmitEdit resolves to: the outbound edit to perform.
type editAction struct {
	JID   string
	MsgID string
	Text  string
}

// composeState is the composer's pure state machine, kept free of GTK so it
// can be unit-tested directly. Composer builds the widget on top of it.
// replyTo and editing are mutually exclusive — arming one clears the other.
type composeState struct {
	jid     string
	replyTo *client.Message
	editing *editTarget
}

// SetChat switches the active chat, clearing any pending reply/edit (both
// carry a target from the previous chat that a switch would misapply).
func (s *composeState) SetChat(jid string) {
	s.jid = jid
	s.replyTo = nil
	s.editing = nil
}

// StartReply arms reply mode, quoting msg on the next Submit.
func (s *composeState) StartReply(msg client.Message) {
	m := msg
	s.replyTo = &m
	s.editing = nil
}

// StartEdit arms edit mode, replacing msg's text on the next SubmitEdit.
func (s *composeState) StartEdit(msg client.Message) {
	s.editing = &editTarget{MsgID: msg.ID, Text: msg.Text}
	s.replyTo = nil
}

// CancelEdit clears edit mode without sending.
func (s *composeState) CancelEdit() { s.editing = nil }

// EditTarget returns the message being edited, if any.
func (s *composeState) EditTarget() (editTarget, bool) {
	if s.editing == nil {
		return editTarget{}, false
	}
	return *s.editing, true
}

// SubmitEdit resolves an edit submission from the current entry text. Fails
// (ok=false) if not in edit mode, no active chat, or text is blank; on
// success it clears edit mode.
func (s *composeState) SubmitEdit(text string) (editAction, bool) {
	text = strings.TrimSpace(text)
	if s.editing == nil || s.jid == "" || text == "" {
		return editAction{}, false
	}
	action := editAction{JID: s.jid, MsgID: s.editing.MsgID, Text: text}
	s.editing = nil
	return action, true
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

// locationAction is what a location-dialog submission resolves to.
type locationAction struct {
	JID     string
	Loc     client.Location
	ReplyTo *client.MsgRef
}

// SubmitLocation resolves a location send from the dialog's raw entry strings.
// Fails (ok=false) with no active chat or invalid coordinates; on success it
// clears any pending reply like the other sends.
func (s *composeState) SubmitLocation(name, address, lat, long string) (locationAction, bool) {
	if s.jid == "" {
		return locationAction{}, false
	}
	loc, ok := parseLocation(name, address, lat, long)
	if !ok {
		return locationAction{}, false
	}
	action := locationAction{JID: s.jid, Loc: loc}
	if s.replyTo != nil {
		action.ReplyTo = &client.MsgRef{ChatJID: s.replyTo.ChatJID, MsgID: s.replyTo.ID}
	}
	s.replyTo = nil
	return action, true
}

// parseLocation validates the dialog's raw lat/long strings into a
// client.Location, rejecting non-numeric or out-of-range coordinates. Name
// and address are trimmed free text.
func parseLocation(name, address, lat, long string) (client.Location, bool) {
	latF, err := strconv.ParseFloat(strings.TrimSpace(lat), 64)
	if err != nil || latF < -90 || latF > 90 {
		return client.Location{}, false
	}
	longF, err := strconv.ParseFloat(strings.TrimSpace(long), 64)
	if err != nil || longF < -180 || longF > 180 {
		return client.Location{}, false
	}
	return client.Location{
		Name: strings.TrimSpace(name), Address: strings.TrimSpace(address),
		Latitude: latF, Longitude: longF,
	}, true
}

// pollAction is what a poll-dialog submission resolves to.
type pollAction struct {
	JID        string
	Name       string
	Options    []string
	Selectable int
}

// SubmitPoll resolves a poll send from the dialog's raw strings. Fails
// (ok=false) with no active chat or a form parsePollForm rejects.
func (s *composeState) SubmitPoll(question string, options []string, selectable int) (pollAction, bool) {
	if s.jid == "" {
		return pollAction{}, false
	}
	name, opts, sel, ok := parsePollForm(question, options, selectable)
	if !ok {
		return pollAction{}, false
	}
	return pollAction{JID: s.jid, Name: name, Options: opts, Selectable: sel}, true
}

// parsePollForm trims the question and options, drops blank options, and
// requires a non-empty question plus at least two options. selectable is
// clamped into [1, len(opts)]. ok is false if the form is unusable.
func parsePollForm(question string, options []string, selectable int) (name string, opts []string, sel int, ok bool) {
	name = strings.TrimSpace(question)
	for _, o := range options {
		if o = strings.TrimSpace(o); o != "" {
			opts = append(opts, o)
		}
	}
	if name == "" || len(opts) < 2 {
		return "", nil, 0, false
	}
	sel = selectable
	if sel < 1 {
		sel = 1
	}
	if sel > len(opts) {
		sel = len(opts)
	}
	return name, opts, sel, true
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

	quoteBar    *gtk.Box
	quoteLabel  *gtk.Label
	editBar     *gtk.Box
	entry       *gtk.Entry
	attachBtn   *gtk.Button
	locationBtn *gtk.Button
	pollBtn     *gtk.Button
	recordBtn   *gtk.Button

	recorder  *audio.Recorder // non-nil only while a recording is in progress
	recording bool

	typing *typingModel // debounces our own composing/paused SendTyping calls

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

	editBar := gtk.NewBox(gtk.OrientationHorizontal, 6)
	editBar.AddCSSClass("chatot-composer-quote")
	editBar.SetVisible(false)

	editLabel := gtk.NewLabel("Editing message…")
	editLabel.SetXAlign(0)
	editLabel.SetHExpand(true)
	editBar.Append(editLabel)

	cancelEdit := gtk.NewButtonWithLabel("×")
	cancelEdit.AddCSSClass("flat")
	editBar.Append(cancelEdit)

	root.Append(editBar)

	entryRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	entryRow.SetMarginTop(6)
	entryRow.SetMarginBottom(6)
	entryRow.SetMarginStart(8)
	entryRow.SetMarginEnd(8)

	attachBtn := gtk.NewButtonWithLabel("📎")
	attachBtn.AddCSSClass("flat")
	attachBtn.SetSensitive(false)
	entryRow.Append(attachBtn)

	locationBtn := gtk.NewButtonWithLabel("📍")
	locationBtn.AddCSSClass("flat")
	locationBtn.SetSensitive(false)
	entryRow.Append(locationBtn)

	pollBtn := gtk.NewButtonWithLabel("📊")
	pollBtn.AddCSSClass("flat")
	pollBtn.SetSensitive(false)
	entryRow.Append(pollBtn)

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
		Box:         root,
		c:           c,
		quoteBar:    quoteBar,
		quoteLabel:  quoteLabel,
		editBar:     editBar,
		entry:       entry,
		attachBtn:   attachBtn,
		locationBtn: locationBtn,
		pollBtn:     pollBtn,
		recordBtn:   recordBtn,
		typing:      newTypingModel(typingDebounce),
	}

	entry.ConnectActivate(comp.submit)
	entry.ConnectChanged(comp.onEntryChanged)
	sendBtn.ConnectClicked(comp.submit)
	attachBtn.ConnectClicked(comp.pickAttachment)
	locationBtn.ConnectClicked(comp.pickLocation)
	pollBtn.ConnectClicked(comp.pickPoll)
	recordBtn.ConnectClicked(comp.toggleRecording)
	cancelQuote.ConnectClicked(func() {
		comp.state.CancelReply()
		comp.refreshQuoteBar()
	})
	cancelEdit.ConnectClicked(comp.cancelEdit)

	// Ticks the typing debounce every second so a burst of keystrokes with
	// no explicit "stopped typing" signal (send, clear, chat switch) still
	// resolves to paused after typingDebounce of silence. Runs for the
	// composer's lifetime — return true to keep repeating.
	glib.TimeoutAdd(1000, func() bool {
		comp.tickTyping()
		return true
	})

	return comp
}

// onEntryChanged drives the typing debounce off every entry edit: a
// non-empty entry records a keystroke (sending composing only on the first
// keystroke of a burst); an entry cleared back to empty forces paused
// immediately rather than waiting out the debounce window.
func (c *Composer) onEntryChanged() {
	jid := c.state.jid
	if jid == "" {
		return
	}
	if strings.TrimSpace(c.entry.Text()) == "" {
		if c.typing.Cleared() {
			c.sendTypingAsync(jid, false)
		}
		return
	}
	if send, composing := c.typing.Keystroke(time.Now()); send {
		c.sendTypingAsync(jid, composing)
	}
}

// tickTyping is invoked on the GTK main loop roughly once a second; it asks
// the debounce model whether the silence window has elapsed and, if so,
// sends the paused transition.
func (c *Composer) tickTyping() {
	jid := c.state.jid
	if jid == "" {
		return
	}
	if send, composing := c.typing.Tick(time.Now()); send {
		c.sendTypingAsync(jid, composing)
	}
}

// sendTypingAsync fires SendTyping in the background so a slow/failed
// network call never blocks the GTK main loop; failures are logged only,
// mirroring submit/sendMedia's error handling for outbound calls.
func (c *Composer) sendTypingAsync(jid string, typing bool) {
	go func() {
		if err := c.c.SendTyping(jid, typing); err != nil {
			log.Printf("chatot: send typing failed: %v", err)
		}
	}()
}

// SetWindow supplies the parent window gtk.FileDialog needs; call once
// before the attach button can be used (main.go does this after the window
// is constructed).
func (c *Composer) SetWindow(w *gtk.Window) { c.window = w }

// OnSent registers f to be called (on the GTK main loop) with the optimistic
// outbound message right after a successful send.
func (c *Composer) OnSent(f func(client.Message)) { c.onSent = f }

// SetChat switches the composer to jid, clearing any pending reply. If we
// were mid-burst composing in the previous chat, tell it we've stopped —
// the entry's text isn't cleared on a chat switch, so nothing else would
// trigger that paused transition.
func (c *Composer) SetChat(jid string) {
	prevJID := c.state.jid
	c.state.SetChat(jid)
	if c.typing.Cleared() && prevJID != "" {
		c.sendTypingAsync(prevJID, false)
	}
	c.attachBtn.SetSensitive(jid != "")
	c.locationBtn.SetSensitive(jid != "")
	c.pollBtn.SetSensitive(jid != "")
	c.recordBtn.SetSensitive(jid != "" && !c.recording)
	c.refreshQuoteBar()
	c.refreshEditBar()
}

// StartReply arms reply mode for msg; called from the conversation view's
// per-bubble reply affordance.
func (c *Composer) StartReply(msg client.Message) {
	c.state.StartReply(msg)
	c.refreshQuoteBar()
	c.refreshEditBar()
}

// StartEdit arms edit mode for msg: prefill the entry with its current text so
// the user amends it in place, and show the editing bar. Called from the
// conversation view's per-bubble edit affordance.
func (c *Composer) StartEdit(msg client.Message) {
	c.state.StartEdit(msg)
	c.entry.SetText(msg.Text)
	c.refreshQuoteBar()
	c.refreshEditBar()
}

// cancelEdit leaves edit mode, clearing the entry and hiding the editing bar.
func (c *Composer) cancelEdit() {
	c.state.CancelEdit()
	c.entry.SetText("")
	c.refreshEditBar()
}

func (c *Composer) refreshEditBar() {
	_, ok := c.state.EditTarget()
	c.editBar.SetVisible(ok)
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
	if _, editing := c.state.EditTarget(); editing {
		c.submitEdit()
		return
	}
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

// submitEdit resolves the current entry against edit mode and, if valid,
// clears edit mode immediately and sends the edit in the background. The
// EditMessage impl pushes an EventMessage so the open chat re-renders the
// amended bubble; nothing is appended here.
func (c *Composer) submitEdit() {
	action, ok := c.state.SubmitEdit(c.entry.Text())
	if !ok {
		return
	}
	c.entry.SetText("")
	c.refreshEditBar()

	go func() {
		if err := c.c.EditMessage(context.Background(), action.JID, action.MsgID, action.Text); err != nil {
			log.Printf("chatot: edit message failed: %v", err)
		}
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

// pickLocation opens a small modal with name/latitude/longitude entries and,
// on send, hands the parsed coordinates to sendLocation. No-ops if there's no
// active chat or no window set yet.
func (c *Composer) pickLocation() {
	if !c.locationBtn.Sensitive() || c.window == nil {
		return
	}

	dialog := gtk.NewWindow()
	dialog.SetTitle("Send location")
	dialog.SetTransientFor(c.window)
	dialog.SetModal(true)

	grid := gtk.NewGrid()
	grid.SetRowSpacing(6)
	grid.SetColumnSpacing(8)
	grid.SetMarginTop(12)
	grid.SetMarginBottom(12)
	grid.SetMarginStart(12)
	grid.SetMarginEnd(12)

	nameEntry := gtk.NewEntry()
	nameEntry.SetPlaceholderText("Name (optional)")
	latEntry := gtk.NewEntry()
	latEntry.SetPlaceholderText("Latitude")
	longEntry := gtk.NewEntry()
	longEntry.SetPlaceholderText("Longitude")

	grid.Attach(gtk.NewLabel("Name"), 0, 0, 1, 1)
	grid.Attach(nameEntry, 1, 0, 1, 1)
	grid.Attach(gtk.NewLabel("Latitude"), 0, 1, 1, 1)
	grid.Attach(latEntry, 1, 1, 1, 1)
	grid.Attach(gtk.NewLabel("Longitude"), 0, 2, 1, 1)
	grid.Attach(longEntry, 1, 2, 1, 1)

	sendBtn := gtk.NewButtonWithLabel("Send")
	sendBtn.AddCSSClass("suggested-action")
	sendBtn.ConnectClicked(func() {
		if c.sendLocation(nameEntry.Text(), "", latEntry.Text(), longEntry.Text()) {
			dialog.Close()
		}
	})
	grid.Attach(sendBtn, 1, 3, 1, 1)

	dialog.SetChild(grid)
	dialog.SetDefaultWidget(sendBtn)
	dialog.Present()
}

// sendLocation resolves the dialog's raw entries against composeState and
// sends the location in the background (mirroring submit's flow). Returns
// false — leaving the dialog open — if the coordinates don't parse.
func (c *Composer) sendLocation(name, address, lat, long string) bool {
	action, ok := c.state.SubmitLocation(name, address, lat, long)
	if !ok {
		return false
	}
	c.refreshQuoteBar()

	go func() {
		id, err := c.c.SendLocation(context.Background(), action.JID, action.Loc, action.ReplyTo)
		if err != nil {
			log.Printf("chatot: send location failed: %v", err)
			return
		}
		if c.onSent == nil {
			return
		}
		loc := action.Loc
		msg := client.Message{
			ID: id, ChatJID: action.JID, FromMe: true, TS: time.Now().Unix(),
			ReplyTo: action.ReplyTo, Location: &loc,
		}
		glib.IdleAdd(func() { c.onSent(msg) })
	}()
	return true
}

// pollOptionCount is how many option entries the create-poll dialog offers.
const pollOptionCount = 4

// pickPoll opens a small modal with a question entry and a few option entries
// and, on send, hands the parsed form to sendPoll. No-ops if there's no active
// chat or no window set yet.
func (c *Composer) pickPoll() {
	if !c.pollBtn.Sensitive() || c.window == nil {
		return
	}

	dialog := gtk.NewWindow()
	dialog.SetTitle("Create poll")
	dialog.SetTransientFor(c.window)
	dialog.SetModal(true)

	grid := gtk.NewGrid()
	grid.SetRowSpacing(6)
	grid.SetColumnSpacing(8)
	grid.SetMarginTop(12)
	grid.SetMarginBottom(12)
	grid.SetMarginStart(12)
	grid.SetMarginEnd(12)

	questionEntry := gtk.NewEntry()
	questionEntry.SetPlaceholderText("Question")
	grid.Attach(gtk.NewLabel("Question"), 0, 0, 1, 1)
	grid.Attach(questionEntry, 1, 0, 1, 1)

	optionEntries := make([]*gtk.Entry, pollOptionCount)
	for i := range optionEntries {
		e := gtk.NewEntry()
		e.SetPlaceholderText("Option")
		optionEntries[i] = e
		grid.Attach(gtk.NewLabel("Option"), 0, i+1, 1, 1)
		grid.Attach(e, 1, i+1, 1, 1)
	}

	sendBtn := gtk.NewButtonWithLabel("Create")
	sendBtn.AddCSSClass("suggested-action")
	sendBtn.ConnectClicked(func() {
		opts := make([]string, len(optionEntries))
		for i, e := range optionEntries {
			opts[i] = e.Text()
		}
		if c.sendPoll(questionEntry.Text(), opts) {
			dialog.Close()
		}
	})
	grid.Attach(sendBtn, 1, pollOptionCount+1, 1, 1)

	dialog.SetChild(grid)
	dialog.SetDefaultWidget(sendBtn)
	dialog.Present()
}

// sendPoll resolves the dialog's raw form against composeState and sends the
// poll in the background (mirroring submit's flow). Returns false — leaving the
// dialog open — if the form is unusable (blank question or <2 options). Single-
// select only from the composer; multi-select is out of scope.
func (c *Composer) sendPoll(question string, options []string) bool {
	action, ok := c.state.SubmitPoll(question, options, 1)
	if !ok {
		return false
	}

	go func() {
		id, err := c.c.CreatePoll(context.Background(), action.JID, action.Name, action.Options, action.Selectable)
		if err != nil {
			log.Printf("chatot: create poll failed: %v", err)
			return
		}
		if c.onSent == nil {
			return
		}
		opts := make([]client.PollOption, len(action.Options))
		for i, o := range action.Options {
			opts[i] = client.PollOption{Name: o}
		}
		msg := client.Message{
			ID: id, ChatJID: action.JID, FromMe: true, TS: time.Now().Unix(),
			Poll: &client.Poll{Name: action.Name, Options: opts, SelectableCount: action.Selectable},
		}
		glib.IdleAdd(func() { c.onSent(msg) })
	}()
	return true
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

	jid := c.state.jid
	go func() {
		if err := c.c.SendRecording(jid, true); err != nil {
			log.Printf("chatot: send recording presence failed: %v", err)
		}
	}()
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

	go func() {
		if err := c.c.SendRecording(jid, false); err != nil {
			log.Printf("chatot: send recording presence failed: %v", err)
		}
	}()

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
