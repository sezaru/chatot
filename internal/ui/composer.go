package ui

import (
	"context"
	"fmt"
	"log"
	"mime"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	coreglib "github.com/diamondburned/gotk4/pkg/core/glib"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/audio"
	"chatot/internal/client"
	"chatot/internal/geo"
)

// reactEmojis is the fixed quick-react set offered on every bubble.
var reactEmojis = []string{"👍", "❤️", "😂", "😮", "😢", "🙏"}

// SendReadReceipts decides whether a read here is reported to the senders
// (blue ticks). The account's own devices always learn about it, so the
// phone's badge stays in step either way.
var SendReadReceipts = false

// LocationAccess mirrors settings.Settings.LocationAccess: whether the
// Send-location sheet may ask the system for a position.
var LocationAccess = true

// SendTypingIndicators gates outbound SendTyping calls from the composer's
// typing-debounce state machine. Default true, matching WhatsApp's own
// default; the Preferences window's Privacy page toggles it.
var SendTypingIndicators = true

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

// liveLocationAction is what a live-location dialog submission resolves to.
type liveLocationAction struct {
	JID          string
	Latitude     float64
	Longitude    float64
	DurationSecs int
	ReplyTo      *client.MsgRef
}

// SubmitLiveLocation resolves a live-location send. Fails (ok=false) with no
// active chat or a non-positive duration; on success it clears any pending
// reply like the other sends.
func (s *composeState) SubmitLiveLocation(lat, lon float64, durationSecs int) (liveLocationAction, bool) {
	if s.jid == "" || durationSecs <= 0 {
		return liveLocationAction{}, false
	}
	action := liveLocationAction{JID: s.jid, Latitude: lat, Longitude: lon, DurationSecs: durationSecs}
	if s.replyTo != nil {
		action.ReplyTo = &client.MsgRef{ChatJID: s.replyTo.ChatJID, MsgID: s.replyTo.ID}
	}
	s.replyTo = nil
	return action, true
}

// contactAction is what a contact-pick submission resolves to.
type contactAction struct {
	JID     string
	Contact client.Contact
	ReplyTo *client.MsgRef
}

// SubmitContact resolves a contact send given the picked contact. Fails
// (ok=false) if there's no active chat; on success it clears any pending
// reply like the other sends.
func (s *composeState) SubmitContact(contact client.Contact) (contactAction, bool) {
	if s.jid == "" {
		return contactAction{}, false
	}
	action := contactAction{JID: s.jid, Contact: contact}
	if s.replyTo != nil {
		action.ReplyTo = &client.MsgRef{ChatJID: s.replyTo.ChatJID, MsgID: s.replyTo.ID}
	}
	s.replyTo = nil
	return action, true
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

// MarkReadOnOpen reports a just-opened chat's unread messages as read: to
// the account's other devices always, to the senders when SendReadReceipts
// allows. Runs the network call synchronously; callers invoke it from a
// goroutine to keep the GTK main loop unblocked.
func MarkReadOnOpen(ctx context.Context, c client.Client, jid string, msgs []client.Message, unreadCount int) {
	if unreadCount <= 0 {
		return
	}
	markRead(ctx, c, jid, unreadMessageIDs(msgs, unreadCount))
}

// MarkReadOnArrival handles a message that lands in the chat the user is
// looking at: it is read the moment it is shown, so the badge never counts
// it and the sender (receipts allowing) gets the tick.
func MarkReadOnArrival(ctx context.Context, c client.Client, msg client.Message) {
	if msg.FromMe {
		return
	}
	markRead(ctx, c, msg.ChatJID, []string{msg.ID})
}

// markRead sends the read for ids in jid. Opening the chat means the user
// has seen it, so the local badge clears even when the receipt cannot be
// sent (or there is no message to send it for).
func markRead(ctx context.Context, c client.Client, jid string, ids []string) {
	if len(ids) > 0 {
		err := c.MarkRead(ctx, jid, ids, SendReadReceipts)
		if err == nil {
			return
		}
		log.Printf("chatot: mark read failed: %v", err)
	}
	if err := c.ClearUnread(jid); err != nil {
		log.Printf("chatot: clear unread failed: %v", err)
	}
}

// Composer is the bottom bar: a reply-quote strip (shown only in reply
// mode), a text entry and a send button, backed by composeState.
type Composer struct {
	*gtk.Box

	c       client.Client
	state   composeState
	window  *gtk.Window  // parent for gtk.FileDialog; set via SetWindow
	avatars *avatarCache // the mention picker's rows

	quoteBar   *gtk.Box
	quoteLabel *gtk.Label
	quoteName  *gtk.Label
	// chatName is the open chat's display name, shown as the reply bar's
	// author line for an incoming message.
	chatName  string
	editBar   *gtk.Box
	entry     *composerInput
	attachBtn *gtk.MenuButton
	emojiBtn  *gtk.Button
	recordBtn *gtk.Button
	sendBtn   *gtk.Button

	// mentions is the @ autocomplete over the entry; people are the open
	// chat's candidates (fetched once per chat, peopleJID), and
	// mentionNames maps every name the picker inserted to its wire user so
	// submit can send "@user".
	mentions      *mentionPicker
	people        []mentionCandidate
	peopleJID     string
	peopleLoading string
	mentionNames  map[string]string

	gifProvider GIFProvider
	// pickerStack/pickerPopover back the GIF+Stickers picker, now reached
	// from the attach ＋ popover rather than a dedicated bar button.
	pickerStack   *gtk.Stack
	pickerPopover *gtk.Popover

	// tray is the send-preview an attach pick flows into; nil falls back to
	// sending the first picked file directly.
	tray *AttachTray

	recorder  *audio.Recorder // non-nil only while a recording is in progress
	recording bool
	// The idle strip and the recording strip swap places; recordTick is the
	// once-a-second timer driving the elapsed label, 0 when not recording.
	entryRow    *gtk.Box
	recordRow   *gtk.Box
	recordTime  *gtk.Label
	recordDot   *gtk.Box
	recordTrace *levelTrace  // the pill's level meter; fed every levelTickMS while recording
	liveShares  []*liveShare // own live locations in progress
	levelTick   glib.SourceHandle
	recordPause *gtk.Button
	recordTick  glib.SourceHandle

	typing *typingModel // debounces our own composing/paused SendTyping calls

	onSent       func(client.Message)
	onSendResult func(localID string, msg client.Message, err error)
	localSeq     int // see localMessageID
}

// NewComposer builds an empty, disabled-until-SetChat Composer backed by c.
func NewComposer(c client.Client) *Composer {
	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.AddCSSClass("chatot-composer")

	// The mockup's reply bar sits ABOVE the composer strip, on the chat
	// surface, inset 12px with a rounded top, a 3px accent left edge, and
	// two lines: the author in accent bold over the quoted text. The strip's
	// hairline runs beneath it, so the strip (and its border/padding) is its
	// own box below rather than the root.
	quoteBar := gtk.NewBox(gtk.OrientationHorizontal, 10)
	quoteBar.AddCSSClass("chatot-composer-quote")
	quoteBar.SetVisible(false)

	quoteCol := gtk.NewBox(gtk.OrientationVertical, 1)
	quoteCol.SetHExpand(true)
	quoteCol.SetVAlign(gtk.AlignCenter)

	quoteName := gtk.NewLabel("")
	quoteName.SetXAlign(0)
	quoteName.AddCSSClass("chatot-composer-quote-name")
	quoteCol.Append(quoteName)

	quoteLabel := gtk.NewLabel("")
	quoteLabel.SetXAlign(0)
	quoteLabel.SetEllipsize(pango.EllipsizeEnd)
	quoteLabel.AddCSSClass("chatot-composer-quote-text")
	quoteCol.Append(quoteLabel)
	quoteBar.Append(quoteCol)

	cancelQuote := gtk.NewButtonWithLabel("✕")
	cancelQuote.AddCSSClass("flat")
	cancelQuote.AddCSSClass("chatot-composer-quote-close")
	cancelQuote.SetVAlign(gtk.AlignCenter)
	quoteBar.Append(cancelQuote)

	root.Append(quoteBar)

	editBar := gtk.NewBox(gtk.OrientationHorizontal, 10)
	editBar.AddCSSClass("chatot-composer-quote")
	editBar.SetVisible(false)

	editLabel := gtk.NewLabel("Editing message…")
	editLabel.SetXAlign(0)
	editLabel.SetHExpand(true)
	editLabel.AddCSSClass("chatot-composer-quote-name")
	editBar.Append(editLabel)

	cancelEdit := gtk.NewButtonWithLabel("✕")
	cancelEdit.AddCSSClass("flat")
	cancelEdit.AddCSSClass("chatot-composer-quote-close")
	cancelEdit.SetVAlign(gtk.AlignCenter)
	editBar.Append(cancelEdit)

	root.Append(editBar)

	// Mockup: a 51px strip whose padding lives on .chatot-composer-strip (8px
	// 12px), with 7px between the buttons and the entry. Row margins here
	// would compound with that padding and make the strip too tall.
	strip := gtk.NewBox(gtk.OrientationVertical, 0)
	strip.AddCSSClass("chatot-composer-strip")
	root.Append(strip)
	entryRow := gtk.NewBox(gtk.OrientationHorizontal, 7)

	// Layout per the interactive mockup: 📎 attach · 🙂 emoji · [pill entry] ·
	// then 🎤 mic when idle, swapped for the green ➤ send once there's text.
	// Emoji glyphs, not symbolic icons — same call as the hover actions row.
	attachBtn := gtk.NewMenuButton()
	// SetChild, not SetLabel: a MenuButton's own label sits in a box that
	// reserves room for a dropdown arrow, which shoved the glyph off-centre.
	attachBtn.SetChild(gtk.NewLabel("📎"))
	attachBtn.AddCSSClass("flat")
	attachBtn.AddCSSClass("chatot-composer-iconbtn")
	// Centred, or the button fills the row's height and the 32px disc
	// stretches into a 36px oval.
	attachBtn.SetVAlign(gtk.AlignEnd)
	attachBtn.SetSensitive(false)
	entryRow.Append(attachBtn)

	emojiBtn := gtk.NewButtonWithLabel("🙂")
	emojiBtn.AddCSSClass("flat")
	// A label button is a .text-button, which Adwaita pads sideways into an
	// oval; this one is a disc.
	emojiBtn.RemoveCSSClass("text-button")
	emojiBtn.AddCSSClass("chatot-composer-iconbtn")
	emojiBtn.AddCSSClass("chatot-composer-emojibtn")
	emojiBtn.SetVAlign(gtk.AlignEnd)
	entryRow.Append(emojiBtn)

	entry := newComposerInput()
	// Nothing to write to until a chat is open (SetChat enables the strip).
	entry.SetSensitive(false)
	emojiBtn.SetSensitive(false)
	entryRow.Append(entry)

	recordBtn := gtk.NewButtonWithLabel("🎤")
	recordBtn.AddCSSClass("flat")
	recordBtn.AddCSSClass("chatot-composer-micbtn")
	recordBtn.SetTooltipText("Record voice message")
	recordBtn.SetSensitive(false)
	recordBtn.SetVAlign(gtk.AlignEnd)
	entryRow.Append(recordBtn)

	// Mic-XOR-send like the interactive mockup: idle shows only the mic and
	// the green send disc appears once the entry holds text
	// (updateSendVisibility drives the swap).
	sendBtn := gtk.NewButtonFromIconName("go-next-symbolic")
	sendBtn.AddCSSClass("chatot-send")
	sendBtn.SetTooltipText("Send")
	sendBtn.SetSensitive(false)
	sendBtn.SetVisible(false)
	sendBtn.SetVAlign(gtk.AlignEnd)
	entryRow.Append(sendBtn)

	strip.Append(entryRow)

	// Recording swaps the whole strip for a recording bar (see
	// newRecordingBar for what it holds and why).
	recordRow, recordTime, recordDot, recordTrace, recordCancel, recordPause, recordStop := newRecordingBar()
	recordRow.SetVisible(false)
	strip.Append(recordRow)

	comp := &Composer{
		Box:         root,
		c:           c,
		quoteBar:    quoteBar,
		quoteLabel:  quoteLabel,
		quoteName:   quoteName,
		editBar:     editBar,
		entry:       entry,
		attachBtn:   attachBtn,
		emojiBtn:    emojiBtn,
		recordBtn:   recordBtn,
		entryRow:    entryRow,
		recordRow:   recordRow,
		recordTime:  recordTime,
		recordDot:   recordDot,
		recordTrace: recordTrace,
		recordPause: recordPause,
		sendBtn:     sendBtn,
		gifProvider: settingsProvider{},
		typing:      newTypingModel(typingDebounce),
	}

	pickerPopover, pickerStack := newPickerPopover(comp)
	comp.pickerStack = pickerStack
	comp.pickerPopover = pickerPopover
	// Anchored to the emoji button, not the paperclip: the design puts all
	// three picker tabs behind the composer's smiley.
	pickerPopover.SetParent(emojiBtn)
	// The composer sits at the bottom of the window, so both popovers must
	// open upward or they fall off-screen.
	attachBtn.SetDirection(gtk.ArrowUp)
	attachBtn.SetPopover(newAttachPopover(comp))

	comp.avatars = newAvatarCache()
	comp.mentions = newMentionPicker(entry, comp.c, comp.avatars, comp.pickMention)
	// Captured before the entry's own bindings so Enter/Tab/arrows steer
	// the picker instead of sending or moving the cursor while it is up.
	mentionKeys := gtk.NewEventControllerKey()
	mentionKeys.SetPropagationPhase(gtk.PhaseCapture)
	mentionKeys.ConnectKeyPressed(func(keyval, _ uint, _ gdk.ModifierType) bool {
		return comp.mentions.Key(keyval)
	})
	entry.AddController(mentionKeys)
	// The fragment under the cursor changes with the cursor too, and the
	// picker (which never takes the focus) must not outlive it.
	entry.ConnectCursorMoved(comp.refreshMentionPicker)
	entry.ConnectPasteAttachments(comp.queueFiles, comp.queueImage)
	mentionFocus := gtk.NewEventControllerFocus()
	mentionFocus.ConnectLeave(func() { comp.mentions.Hide() })
	entry.AddController(mentionFocus)
	entry.ConnectActivate(comp.submit)
	entry.ConnectChanged(comp.onEntryChanged)
	sendBtn.ConnectClicked(comp.submit)
	emojiBtn.ConnectClicked(func() { comp.showPicker("emoji") })
	recordBtn.ConnectClicked(comp.toggleRecording)
	recordStop.ConnectClicked(comp.toggleRecording)
	recordCancel.ConnectClicked(comp.cancelRecording)
	recordPause.ConnectClicked(comp.togglePause)
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
	c.updateSendVisibility()
	c.refreshMentionPicker()
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

// sendEnabled reports whether the send button should be active: only when the
// entry holds non-whitespace text.
func sendEnabled(text string) bool {
	return strings.TrimSpace(text) != ""
}

// updateSendVisibility swaps mic for send off the entry's current text (the
// interactive mockup's idle composer shows only the mic; the green send disc
// replaces it once there's something to send). Safe to call with no active
// chat; an in-progress recording keeps the mic visible.
func (c *Composer) updateSendVisibility() {
	hasText := sendEnabled(c.entry.Text())
	c.sendBtn.SetVisible(hasText)
	c.sendBtn.SetSensitive(hasText)
	c.recordBtn.SetVisible(!hasText || c.recording)
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
	if !SendTypingIndicators {
		return
	}
	go func() {
		if err := c.c.SendTyping(jid, typing); err != nil {
			log.Printf("chatot: send typing failed: %v", err)
		}
	}()
}

// PopAttach opens the 📎 attachment popover — a dev/screenshot hook.
func (c *Composer) PopAttach() { c.attachBtn.Popup() }

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
	c.mentionNames = nil
	if c.mentions != nil {
		c.mentions.Hide()
	}
	c.attachBtn.SetSensitive(jid != "")
	c.emojiBtn.SetSensitive(jid != "")
	c.entry.SetSensitive(jid != "")
	c.recordBtn.SetSensitive(jid != "" && !c.recording)
	c.refreshQuoteBar()
	c.refreshEditBar()
}

// FocusInput puts the keyboard focus in the message entry so the user can
// type straight after opening a chat. No-op while no chat is open.
func (c *Composer) FocusInput() {
	if c.state.jid == "" || c.recording {
		return
	}
	c.entry.GrabFocus()
}

// SetChatName records the open chat's display name for the reply bar's
// author line. Separate from SetChat, which only ever sees a JID.
func (c *Composer) SetChatName(name string) {
	c.chatName = name
	c.refreshQuoteBar()
}

// StartReply arms reply mode for msg; called from the conversation view's
// per-bubble reply affordance.
func (c *Composer) StartReply(msg client.Message) {
	c.state.StartReply(msg)
	c.refreshQuoteBar()
	c.refreshEditBar()
	// The reply is typed next: the caret goes to the entry, as WhatsApp
	// does, instead of leaving the focus on the bubble's button.
	c.FocusInput()
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
	c.quoteName.SetLabel(replyAuthorName(target, c.chatName))
	c.quoteLabel.SetLabel(text)
	c.quoteBar.SetVisible(true)
}

// replyAuthorName is the accent line at the top of the reply bar: "You" for
// your own message, otherwise the chat's name. The mockup always shows an
// author; the old bar showed only the quoted text, which left a reply to
// yourself indistinguishable from a reply to them.
func replyAuthorName(target client.Message, chatName string) string {
	if target.FromMe {
		return "You"
	}
	if chatName != "" {
		return chatName
	}
	return "Reply"
}

// submit resolves the current entry text against composeState and, if
// valid, clears the entry/reply-mode immediately and sends in the
// background. Must run on the GTK main loop (entry.Text() isn't thread-safe).
func (c *Composer) submit() {
	if _, editing := c.state.EditTarget(); editing {
		c.submitEdit()
		return
	}
	action, ok := c.state.Submit(wireMentions(c.entry.Text(), c.mentionNames))
	if !ok {
		return
	}
	c.entry.SetText("")
	c.mentionNames = nil
	c.refreshQuoteBar()

	c.dispatch(client.Message{
		ChatJID: action.JID, FromMe: true,
		Text: action.Text, ReplyTo: action.ReplyTo,
	})
}

// dispatch sends msg the way WhatsApp does: the bubble appears at once with
// a pending clock, and the send runs in the background. When it returns the
// row settles into the sent message (its server id, a tick) or, on an
// error, into a failed bubble with a Retry. msg carries everything needed
// to send it, so a retry is a plain re-dispatch of the same value.
func (c *Composer) dispatch(msg client.Message) {
	msg.ID = c.localMessageID()
	msg.Status = client.MessageStatusPending
	msg.TS = time.Now().Unix()
	if msg.ChatJID == "" {
		return
	}
	if c.onSent != nil {
		c.onSent(msg)
	}
	pending := msg
	go func() {
		id, err := c.send(pending)
		if err != nil {
			log.Printf("chatot: send failed: %v", err)
		}
		sent := pending
		sent.ID = id
		glib.IdleAdd(func() {
			if c.onSendResult != nil {
				c.onSendResult(pending.ID, sent, err)
			}
		})
	}()
}

// send performs the network send for a dispatched message: text, or a file
// attachment. Anything else is a programming error (those kinds go through
// their own dialogs, not dispatch).
func (c *Composer) send(msg client.Message) (string, error) {
	ctx := context.Background()
	if msg.Attachment == nil {
		return c.c.SendText(ctx, msg.ChatJID, msg.Text, msg.ReplyTo)
	}
	a := *msg.Attachment
	return c.c.SendMedia(ctx, msg.ChatJID, client.Attachment{
		LocalPath: a.LocalPath, Filename: a.Filename, Caption: a.Caption,
		Thumbnail: a.Thumbnail, DurationSecs: a.DurationSecs,
		Width: a.Width, Height: a.Height,
	}, msg.ReplyTo)
}

// Resend is a failed bubble's Retry: the same content goes out again as a
// fresh pending message at the foot of the thread. Must run on the GTK main
// loop.
func (c *Composer) Resend(msg client.Message) {
	msg.Reactions = nil
	c.dispatch(msg)
}

// localMessageID mints the id an optimistic row carries until the server
// hands back the real one. Distinct from any WhatsApp id (those are hex
// upper-case), and unique within the process.
func (c *Composer) localMessageID() string {
	c.localSeq++
	return fmt.Sprintf("local-%d-%d", time.Now().UnixNano(), c.localSeq)
}

// OnSendResult registers f to be called (on the GTK main loop) once a
// dispatched send returns: localID is the optimistic row's id, msg the
// message with its server id on success, err the failure otherwise.
func (c *Composer) OnSendResult(f func(localID string, msg client.Message, err error)) {
	c.onSendResult = f
}

// submitEdit resolves the current entry against edit mode and, if valid,
// clears edit mode immediately and sends the edit in the background. The
// EditMessage impl pushes an EventMessage so the open chat re-renders the
// amended bubble; nothing is appended here.
func (c *Composer) submitEdit() {
	action, ok := c.state.SubmitEdit(wireMentions(c.entry.Text(), c.mentionNames))
	if !ok {
		return
	}
	c.entry.SetText("")
	c.mentionNames = nil
	c.refreshEditBar()

	go func() {
		if err := c.c.EditMessage(context.Background(), action.JID, action.MsgID, action.Text); err != nil {
			log.Printf("chatot: edit message failed: %v", err)
		}
	}()
}

// attachSource is one row of the 📎 menu: a tinted circle glyph, a label, and
// what picking it does.
type attachSource struct {
	Icon  string
	Label string
	Tint  string // hex of the circle's tint, at the mockup's 20% alpha
}

// attachSources is the mockup's exact seven-row attach list, in its order.
// Kept as a pure list so a drift from the design fails a test rather than only
// a screenshot.
//
// Note this replaces an earlier 3x3 tile grid that carried Camera and Event.
// Both were stubs that only opened a "not implemented" dialog, and the design
// has no row for either; a source with no backend is worse than no row.
func attachSources() []attachSource {
	return []attachSource{
		{"📷", "Photos", "#5a7ab5"},
		{"🎬", "Video", "#9c5b8a"},
		{"📄", "Document", "#4f8a8b"},
		{"👤", "Contact", "#b58a4a"},
		{"📍", "Location", "#c26b5c"},
		{"📊", "Poll", "#7a8b5a"},
		{"🎵", "Audio file", "#6a6a8c"},
	}
}

// attachActions maps each source label to its handler. Split from the list so
// the list stays testable without a Composer.
func (c *Composer) attachActions() map[string]func() {
	return map[string]func(){
		"Photos":     func() { c.pickAttachment(kindFilter("image/*")) },
		"Video":      func() { c.pickAttachment(kindFilter("video/*")) },
		"Document":   func() { c.pickAttachment(nil) },
		"Contact":    c.pickContact,
		"Location":   c.pickLocation,
		"Poll":       c.pickPoll,
		"Audio file": func() { c.pickAttachment(kindFilter("audio/*")) },
	}
}

// kindFilter builds a file-chooser filter for a single MIME pattern; nil
// patterns mean "any file".
func kindFilter(mime string) *gtk.FileFilter {
	f := gtk.NewFileFilter()
	f.AddMIMEType(mime)
	return f
}

// newAttachPopover builds the 📎 button's popover: the mockup's vertical list
// of seven sources, each a 28px tinted circle beside its label, in a 230px
// card. (GIF/Stickers live in the 🙂 picker, not here; live location is chosen
// inside the Location flow.)
func newAttachPopover(c *Composer) *gtk.Popover {
	popover := gtk.NewPopover()
	popover.SetHasArrow(false)
	popover.AddCSSClass("chatot-menu")
	popover.SetPosition(gtk.PosTop)

	list := gtk.NewBox(gtk.OrientationVertical, 1)
	list.SetSizeRequest(230, -1)
	actions := c.attachActions()
	for _, src := range attachSources() {
		activate := actions[src.Label]
		list.Append(newAttachRow(src, func() {
			popover.Popdown()
			if activate != nil {
				activate()
			}
		}))
	}
	popover.SetChild(list)
	return popover
}

// newAttachRow builds one attach-menu row: a tinted 28px circle carrying the
// glyph, then the label.
func newAttachRow(src attachSource, onClick func()) *gtk.Button {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)

	icon := gtk.NewLabel(src.Icon)
	icon.AddCSSClass("chatot-attach-glyph")
	icon.SetVAlign(gtk.AlignCenter)
	icon.SetSizeRequest(28, 28)
	// Per-instance provider: each row's circle carries its own tint, which a
	// shared stylesheet class can't express. "33" is the mockup's 20% alpha.
	css := gtk.NewCSSProvider()
	css.LoadFromString("label { background-color: " + src.Tint + "33; border-radius: 999px; }")
	icon.StyleContext().AddProvider(css, widgetPriority(uint(gtk.STYLE_PROVIDER_PRIORITY_APPLICATION)))
	row.Append(icon)

	label := gtk.NewLabel(src.Label)
	label.SetXAlign(0)
	label.SetHExpand(true)
	label.AddCSSClass("chatot-attach-label")
	row.Append(label)

	btn := gtk.NewButton()
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-menu-item")
	btn.SetChild(row)
	btn.ConnectClicked(onClick)
	return btn
}

// newRecordingBar builds the recording strip. It keeps the mockup's chrome
// (the 34px hairline pill in the composer band, the red dot, the mono timer)
// but lays the controls out the way WhatsApp does, which the mockup's
// Cancel-and-stop pair lacked: a 🗑 that discards, then inside the pill the
// dot, the timer, a dotted track and a ⏸/▶ that pauses without ending the
// note, and finally the green ➤ that sends. Returns the row plus every
// widget the composer drives.
func newRecordingBar() (row *gtk.Box, elapsed *gtk.Label, dot *gtk.Box, trace *levelTrace, cancel, pause, send *gtk.Button) {
	row = gtk.NewBox(gtk.OrientationHorizontal, 7)

	cancel = gtk.NewButtonWithLabel("🗑")
	cancel.AddCSSClass("flat")
	cancel.AddCSSClass("chatot-composer-iconbtn")
	cancel.AddCSSClass("chatot-record-discard")
	cancel.SetTooltipText("Discard recording")
	cancel.SetVAlign(gtk.AlignCenter)
	row.Append(cancel)

	pill := gtk.NewBox(gtk.OrientationHorizontal, 9)
	pill.AddCSSClass("chatot-record-pill")
	pill.SetHExpand(true)

	dot = gtk.NewBox(gtk.OrientationVertical, 0)
	dot.AddCSSClass("chatot-record-dot")
	dot.SetSizeRequest(8, 8)
	dot.SetVAlign(gtk.AlignCenter)
	pill.Append(dot)

	elapsed = gtk.NewLabel("0:00")
	elapsed.AddCSSClass("chatot-record-time")
	pill.Append(elapsed)

	// The dotted track is the level meter at rest: each dot swells into a
	// bar with the microphone level (see levelTrace).
	trace = newLevelTrace()
	pill.Append(trace)

	pause = gtk.NewButtonWithLabel("⏸")
	pause.AddCSSClass("flat")
	pause.AddCSSClass("chatot-record-pause")
	pause.SetTooltipText("Pause")
	pause.SetVAlign(gtk.AlignCenter)
	pill.Append(pause)
	row.Append(pill)

	send = gtk.NewButtonFromIconName("go-next-symbolic")
	send.AddCSSClass("chatot-send")
	send.SetTooltipText("Send voice message")
	send.SetVAlign(gtk.AlignCenter)
	row.Append(send)
	return row, elapsed, dot, trace, cancel, pause, send
}

// pickerPages are the mockup's three picker tabs, in its order. The GIF and
// Stickers pages keep their existing content; only the chrome is the design's.
var pickerPages = []segmentedPage{
	{"emoji", "Emoji"},
	{"gif", "GIF"},
	{"stickers", "Stickers"},
}

// newPickerPopover builds the mockup's 360px picker card: a segmented
// Emoji|GIF|Stickers pill over the selected page. It hangs off the 🙂 button —
// the design has no separate GIF/sticker entry point, and this replaces the
// native GtkEmojiChooser the 🙂 used to open.
func newPickerPopover(c *Composer) (*gtk.Popover, *gtk.Stack) {
	popover := gtk.NewPopover()
	popover.SetHasArrow(false)
	popover.AddCSSClass("chatot-menu")
	popover.SetPosition(gtk.PosTop)

	stack := gtk.NewStack()
	stack.AddNamed(newEmojiTab(c, popover), "emoji")
	stack.AddNamed(newGIFTab(c.gifProvider, c.onGIFChosen), "gif")
	stack.AddNamed(newStickerTab(c, popover), "stickers")

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.AddCSSClass("chatot-picker")
	box.SetSizeRequest(360, -1)
	box.Append(newSegmentedSwitcher(stack, pickerPages, true))
	box.Append(stack)

	popover.SetChild(box)
	return popover, stack
}

// showPicker pops the picker on the requested page.
func (c *Composer) showPicker(page string) {
	if c.pickerStack == nil || c.pickerPopover == nil {
		return
	}
	c.pickerStack.SetVisibleChildName(page)
	c.pickerPopover.Popup()
}

// onGIFChosen sends the picked GIF to the active chat: its mp4 is fetched
// into the cache and goes out as a looping video, which is what a GIF is
// on WhatsApp.
func (c *Composer) onGIFChosen(r GIFResult) {
	jid := c.state.jid
	if jid == "" || r.SendURL == "" {
		return
	}
	if c.pickerPopover != nil {
		c.pickerPopover.Popdown()
	}
	go func() {
		path, err := fetchGIFFile(context.Background(), r.SendURL, ".mp4")
		if err != nil {
			log.Printf("chatot: fetch gif: %v", err)
			return
		}
		att := client.Attachment{
			Kind: "video", MimeType: "video/mp4", LocalPath: path, Filename: filepath.Base(path),
			IsGIF: true, Width: r.Width, Height: r.Height,
		}
		id, err := c.c.SendMedia(context.Background(), jid, att, nil)
		if err != nil {
			log.Printf("chatot: send gif failed: %v", err)
			return
		}
		if c.onSent == nil {
			return
		}
		msg := client.Message{
			ID: id, ChatJID: jid, FromMe: true, TS: time.Now().Unix(),
			Attachment: &client.Attachment{Kind: "video", MimeType: "video/mp4", LocalPath: path, IsGIF: true, Width: r.Width, Height: r.Height},
		}
		glib.IdleAdd(func() { c.onSent(msg) })
	}()
}

// pickAttachment opens a file-choose dialog (filtered to images/videos when
// filter is non-nil) and, on a picked file, hands off to sendMedia. No-ops if
// there's no active chat or no window has been set yet (attachBtn is also
// disabled in that first case, but this guards against a stray click racing
// SetChat/SetWindow).
func (c *Composer) pickAttachment(filter *gtk.FileFilter) {
	if !c.attachBtn.Sensitive() || c.window == nil {
		return
	}
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Send file")
	if filter != nil {
		dialog.SetDefaultFilter(filter)
	}
	// OpenMultiple, not Open: the tray queues several files, and the mockup's
	// thumbnail strip exists precisely to review a multi-file pick.
	dialog.OpenMultiple(context.Background(), c.window, func(res gio.AsyncResulter) {
		files, err := dialog.OpenMultipleFinish(res)
		if err != nil {
			return // cancelled or failed; nothing to log, this is the common case
		}
		var paths []string
		for i := uint(0); i < files.NItems(); i++ {
			if f, ok := files.Item(i).Cast().(*gio.File); ok {
				paths = append(paths, f.Path())
			}
		}
		if len(paths) == 0 {
			return
		}
		// No tray wired (a bare Composer in a test) means the old behaviour:
		// send the first pick straight away rather than dropping it silently.
		if c.tray == nil {
			c.sendMedia(paths[0])
			return
		}
		c.openTray(paths)
	})
}

// SetTray gives the composer the send-preview tray its attach picks flow
// into. Without one the composer sends straight from the file chooser.
func (c *Composer) SetTray(tray *AttachTray) {
	c.tray = tray
	if tray != nil {
		tray.OnDiscard(c.restoreCaption)
	}
}

// openTray queues paths into the tray. Text already typed in the entry goes
// with the first file as its caption, the way WhatsApp carries a draft
// into the attach preview, so hitting Enter in the tray sends the picture
// with that text rather than dropping it. Files added to an open tray
// leave the entry alone.
func (c *Composer) openTray(paths []string) {
	seed := ""
	if c.tray.Empty() {
		seed = c.entry.Text()
	}
	c.tray.Open(paths)
	if seed != "" {
		c.tray.SeedCaption(seed)
		c.entry.SetText("")
	}
}

// restoreCaption is the tray's Cancel: the caption that came from the
// entry goes back there, unless something new was typed meanwhile.
func (c *Composer) restoreCaption(caption string) {
	if caption != "" && c.entry.Text() == "" {
		c.entry.SetText(caption)
		c.entry.SetPosition(-1)
	}
}

// SubmitText types text into the entry and sends it, as Enter would. A
// dev/screenshot aid for the send states.
func (c *Composer) SubmitText(text string) {
	c.entry.SetText(text)
	c.submit()
}

// queueFiles takes files that arrived by paste or drop the way a file
// chooser pick does: into the tray, or straight to a send without one.
// Nothing happens with no chat open.
func (c *Composer) queueFiles(paths []string) {
	if len(paths) == 0 || !c.attachBtn.Sensitive() {
		return
	}
	if c.tray == nil {
		c.sendMedia(paths[0])
		return
	}
	c.openTray(paths)
}

// queueImage takes a pasted or dropped picture: it is written out as a
// PNG under the temp dir, since a send (and the tray's preview) reads
// from a path.
func (c *Composer) queueImage(t *gdk.Texture) {
	if t == nil || !c.attachBtn.Sensitive() {
		return
	}
	path := filepath.Join(os.TempDir(), fmt.Sprintf("chatot-paste-%d.png", time.Now().UnixNano()))
	if !t.SaveToPNG(path) {
		log.Printf("chatot: could not write pasted image to %s", path)
		return
	}
	c.queueFiles([]string{path})
}

// DropTarget is a controller that takes files or a picture dropped on its
// widget into the composer, as a paste does. It is meant for the whole
// conversation pane, thread included: a drop is aimed at the chat, not at
// the entry.
func (c *Composer) DropTarget() *gtk.DropTarget {
	target := gtk.NewDropTarget(gdk.GTypeFileList, gdk.ActionCopy)
	target.SetGTypes([]coreglib.Type{gdk.GTypeFileList, gdk.GTypeTexture})
	target.ConnectDrop(func(v *coreglib.Value, _, _ float64) bool {
		switch x := v.GoValue().(type) {
		case *gdk.FileList:
			c.queueFiles(filePaths(x))
			return true
		case gdk.Texturer:
			c.queueImage(gdk.BaseTexture(x))
			return true
		}
		return false
	})
	return target
}

// ReopenFilePicker is the tray's ＋ button: pick more files into the open tray.
func (c *Composer) ReopenFilePicker() { c.pickAttachment(nil) }

// SendTrayItems sends every queued attachment in order, each with its own
// caption. The first send consumes any pending reply, so the rest are plain
// sends — a reply quotes one message, not a whole batch.
func (c *Composer) SendTrayItems(items []trayItem) {
	for _, item := range items {
		c.sendTrayItem(item)
	}
}

// sendMedia resolves the picked path (using the current entry text as
// caption) against composeState and sends it in the background, mirroring
// submit's clear-then-send-then-idle-append flow.
func (c *Composer) sendMedia(path string) {
	caption := c.entry.Text()
	c.entry.SetText("")
	c.sendTrayItem(trayItem{Path: path, Caption: caption})
}

// sendTrayItem sends one queued file with its own caption. The tray's
// preview (poster frame, PDF page, duration, pixel size) rides along as the
// message's embedded thumbnail and metadata, which is what lets the other
// side — and our own bubble — show a picture before the file is fetched.
func (c *Composer) sendTrayItem(item trayItem) {
	action, ok := c.state.SubmitMedia(item.Path, item.Caption)
	if !ok {
		return
	}
	// The entry is left alone: its text was moved into the tray's caption
	// when the tray opened (or belongs to the next message), and a file
	// send has no business clearing it.
	c.refreshQuoteBar()

	c.dispatch(client.Message{
		ChatJID: action.JID, FromMe: true, ReplyTo: action.ReplyTo,
		Attachment: &client.Attachment{
			Kind: guessAttachmentKind(action.Path), Filename: filepath.Base(action.Path),
			LocalPath: action.Path, Caption: action.Caption,
			Thumbnail: item.Preview.Image, DurationSecs: item.Preview.Seconds,
			Width: item.Preview.Width, Height: item.Preview.Height,
		},
	})
}

// pickSticker opens a file-choose dialog filtered to webp/images and, on a
// picked file, sends it as a sticker and pops popover down. No-ops if there's
// no active chat or no window set yet.
func (c *Composer) pickSticker(popover *gtk.Popover) {
	if c.state.jid == "" || c.window == nil {
		return
	}
	dialog := gtk.NewFileDialog()
	dialog.SetTitle("Send sticker")
	dialog.SetDefaultFilter(stickerFilter())
	dialog.Open(context.Background(), c.window, func(res gio.AsyncResulter) {
		file, err := dialog.OpenFinish(res)
		if err != nil {
			return // cancelled or failed; nothing to log, this is the common case
		}
		popover.Popdown()
		c.sendSticker(file.Path())
	})
}

// sendSticker sends path as a sticker to the active chat and files it in
// the library (a send moves it to the front), mirroring sendMedia's
// goroutine + IdleAdd flow. No reply support (stickers aren't quoted in
// practice, like voice notes).
func (c *Composer) sendSticker(path string) {
	jid := c.state.jid
	if jid == "" || path == "" {
		return
	}

	go func() {
		if st, err := c.c.AddSticker(path); err != nil {
			log.Printf("chatot: add sticker to library: %v", err)
		} else {
			path = st.Path
		}
		id, err := c.c.SendSticker(context.Background(), jid, path)
		if err != nil {
			log.Printf("chatot: send sticker failed: %v", err)
			return
		}
		if c.onSent == nil {
			return
		}
		msg := client.Message{
			ID: id, ChatJID: jid, FromMe: true, TS: time.Now().Unix(),
			Attachment: &client.Attachment{Kind: "sticker", MimeType: "image/webp", LocalPath: path},
		}
		glib.IdleAdd(func() { c.onSent(msg) })
	}()
}

// pickLocation opens the Send location sheet (see location_picker.go) and
// sends what it returns: a fixed point, or a live share that keeps a
// positioning session alive for its duration. No-ops if there's no active
// chat or no window set yet.
func (c *Composer) pickLocation() {
	if c.state.jid == "" || c.window == nil {
		return
	}
	showLocationPicker(c.window, LocationAccess, func(res locationResult) {
		if res.Live {
			c.sendLiveLocation(res)
			return
		}
		c.sendLocation(res)
	})
}

// sendLocation sends a fixed point in the background (mirroring submit's
// flow), with the sheet's map preview as the message thumbnail.
func (c *Composer) sendLocation(res locationResult) {
	action, ok := c.state.SubmitLocation(res.Name, res.Address, geo.FormatCoord(res.Lat), geo.FormatCoord(res.Lon))
	if !ok {
		return
	}
	action.Loc.Thumbnail = res.Thumbnail
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
}

// liveShare is an own live location in progress: the message it started
// and the timer that ends it. WhatsApp has no separate "stop" message, so
// ending is local (the bubble flips to "ended") — see Client.StopLiveLocation.
type liveShare struct {
	chatJID, msgID string
	expiry         glib.SourceHandle
}

// sendLiveLocation sends a live share and remembers it so the bubble's
// Stop sharing (or the chosen duration) can end it.
func (c *Composer) sendLiveLocation(res locationResult) {
	action, ok := c.state.SubmitLiveLocation(res.Lat, res.Lon, res.DurationSecs)
	if !ok {
		return
	}
	c.refreshQuoteBar()

	go func() {
		id, err := c.c.SendLiveLocation(context.Background(), action.JID, action.Latitude, action.Longitude, action.DurationSecs)
		if err != nil {
			log.Printf("chatot: send live location failed: %v", err)
			return
		}
		now := time.Now()
		msg := client.Message{
			ID: id, ChatJID: action.JID, FromMe: true, TS: now.Unix(), ReplyTo: action.ReplyTo,
			Location: &client.Location{
				Latitude: action.Latitude, Longitude: action.Longitude,
				IsLive: true, LiveUntil: now.Unix() + int64(action.DurationSecs),
				Thumbnail: res.Thumbnail,
			},
		}
		glib.IdleAdd(func() {
			c.trackLiveShare(action.JID, id, action.DurationSecs)
			if c.onSent != nil {
				c.onSent(msg)
			}
		})
	}()
}

// trackLiveShare registers a live share and arms its expiry.
func (c *Composer) trackLiveShare(chatJID, msgID string, durationSecs int) {
	share := &liveShare{chatJID: chatJID, msgID: msgID}
	share.expiry = glib.TimeoutSecondsAdd(uint(durationSecs), func() bool {
		share.expiry = 0
		c.endLiveShare(share)
		return false
	})
	c.liveShares = append(c.liveShares, share)
}

// StopLiveLocation is the bubble's Stop sharing: it ends the share early.
// A live location this composer didn't start (another device's, or one
// from before a restart) is still marked ended locally.
func (c *Composer) StopLiveLocation(msg client.Message) {
	for _, share := range c.liveShares {
		if share.chatJID == msg.ChatJID && share.msgID == msg.ID {
			if share.expiry != 0 {
				glib.SourceRemove(share.expiry)
				share.expiry = 0
			}
			c.endLiveShare(share)
			return
		}
	}
	go func() {
		if err := c.c.StopLiveLocation(context.Background(), msg.ChatJID, msg.ID); err != nil {
			log.Printf("chatot: stop live location failed: %v", err)
		}
	}()
}

func (c *Composer) endLiveShare(share *liveShare) {
	kept := c.liveShares[:0]
	for _, s := range c.liveShares {
		if s != share {
			kept = append(kept, s)
		}
	}
	c.liveShares = kept
	go func() {
		if err := c.c.StopLiveLocation(context.Background(), share.chatJID, share.msgID); err != nil {
			log.Printf("chatot: stop live location failed: %v", err)
		}
	}()
}

// sendPoll resolves the dialog's raw form against composeState and sends the
// poll in the background (mirroring submit's flow). Returns false — leaving the
// dialog open — if the form is unusable (blank question or <2 options). Single-
// select only from the composer; multi-select is out of scope.
func (c *Composer) sendPoll(question string, options []string, multi bool) bool {
	selectable := 1
	if multi {
		// "Allow multiple answers" = every option selectable; parsePollForm
		// clamps this to the number of options that survive trimming.
		selectable = len(options)
	}
	action, ok := c.state.SubmitPoll(question, options, selectable)
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

// sendContact resolves a contact send from the picked chat's name/JID against
// composeState and sends it in the background, mirroring sendLocation's flow.
func (c *Composer) sendContact(jid, name, phone string) {
	if phone == "" {
		phone = phoneFromJID(jid)
	}
	var phones []string
	if phone != "" {
		phones = []string{phone}
	}
	action, ok := c.state.SubmitContact(client.Contact{DisplayName: name, Phones: phones})
	if !ok {
		return
	}
	c.refreshQuoteBar()

	go func() {
		id, err := c.c.SendContact(context.Background(), action.JID, action.Contact, action.ReplyTo)
		if err != nil {
			log.Printf("chatot: send contact failed: %v", err)
			return
		}
		if c.onSent == nil {
			return
		}
		contact := action.Contact
		msg := client.Message{
			ID: id, ChatJID: action.JID, FromMe: true, TS: time.Now().Unix(),
			ReplyTo: action.ReplyTo, Contact: &contact,
		}
		glib.IdleAdd(func() { c.onSent(msg) })
	}()
}

// phoneFromJID extracts the phone-number portion of a WhatsApp JID
// ("15551234567@s.whatsapp.net" -> "+15551234567"), used as the picked
// chat's sole phone number since Chat carries no separate phone field.
func phoneFromJID(jid string) string {
	user, _, ok := strings.Cut(jid, "@")
	if !ok || user == "" {
		return ""
	}
	user, _, _ = strings.Cut(user, ":") // drop any device suffix (user:device@…)
	return "+" + user
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
	c.enterRecordingUI()

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

	go func() {
		// Stop runs off the main loop: a paused-and-resumed note is joined by
		// an ffmpeg re-encode there, which takes as long as the note is.
		oggBytes, dur, err := rec.Stop()
		if err != nil {
			log.Printf("chatot: stop recording failed: %v", err)
			return
		}
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
	c.leaveRecordingUI()
	c.recordBtn.SetSensitive(c.state.jid != "")
	c.entry.SetSensitive(c.state.jid != "")
	c.emojiBtn.SetSensitive(c.state.jid != "")
	c.attachBtn.SetSensitive(c.state.jid != "")
	c.sendBtn.SetVisible(true)
	c.updateSendVisibility()
}

// enterRecordingUI swaps the idle strip for the recording bar and starts the
// once-a-second timer that drives its elapsed label.
func (c *Composer) enterRecordingUI() {
	c.entryRow.SetVisible(false)
	c.recordRow.SetVisible(true)
	c.recordTime.SetLabel("0:00")
	c.setPausedUI(false)
	c.recordTick = glib.TimeoutAdd(1000, func() bool {
		if !c.recording || c.recorder == nil {
			return false
		}
		c.recordTime.SetLabel(recordingClock(c.recorder.Elapsed()))
		return true
	})
	c.recordTrace.Reset()
	c.levelTick = glib.TimeoutAdd(levelTickMS, func() bool {
		if !c.recording || c.recorder == nil {
			return false
		}
		if !c.recorder.Paused() {
			c.recordTrace.Push(c.recorder.Level())
		}
		return true
	})
}

// levelTickMS is how often the recording trace samples the microphone
// level: one column per tick, so ~5 s of speech spans the pill.
const levelTickMS = 50

// leaveRecordingUI restores the idle strip and stops the timer. Safe to call
// when no recording was running (stopRecording's error paths reach it).
func (c *Composer) leaveRecordingUI() {
	if c.recordTick != 0 {
		glib.SourceRemove(c.recordTick)
		c.recordTick = 0
	}
	if c.levelTick != 0 {
		glib.SourceRemove(c.levelTick)
		c.levelTick = 0
	}
	c.recordRow.SetVisible(false)
	c.entryRow.SetVisible(true)
}

// recordingClock formats the recording timer the way the mockup's mono label
// reads: m:ss, growing to h:mm:ss only past an hour.
func recordingClock(d time.Duration) string {
	secs := int(d.Seconds())
	if secs < 0 {
		secs = 0
	}
	return humanDuration2(secs)
}

// humanDuration2 is humanDuration with a floor of "0:00" — a recording that
// has just started still needs a clock, where an unknown media duration
// renders as nothing at all.
func humanDuration2(secs int) string {
	if secs <= 0 {
		return "0:00"
	}
	return humanDuration(secs)
}

// cancelRecording aborts the in-flight recording and discards it.
func (c *Composer) cancelRecording() {
	rec := c.recorder
	jid := c.state.jid
	c.recorder = nil
	c.recording = false
	c.resetRecordButton()
	if rec != nil {
		rec.Cancel()
	}
	go func() {
		if err := c.c.SendRecording(jid, false); err != nil {
			log.Printf("chatot: send recording presence failed: %v", err)
		}
	}()
}

// togglePause pauses or resumes the in-flight recording; the timer stops
// with it (Recorder.Elapsed excludes pauses) and the dot stops pulsing.
func (c *Composer) togglePause() {
	rec := c.recorder
	if rec == nil {
		return
	}
	if rec.Paused() {
		if err := rec.Resume(); err != nil {
			log.Printf("chatot: resume recording failed: %v", err)
			return
		}
		c.setPausedUI(false)
		return
	}
	if err := rec.Pause(); err != nil {
		log.Printf("chatot: pause recording failed: %v", err)
		return
	}
	c.setPausedUI(true)
}

// setPausedUI paints the pause button and dot for the paused/live state.
func (c *Composer) setPausedUI(paused bool) {
	if paused {
		c.recordPause.SetLabel("▶")
		c.recordPause.SetTooltipText("Resume")
		c.recordDot.AddCSSClass("chatot-record-dot-paused")
		return
	}
	c.recordPause.SetLabel("⏸")
	c.recordPause.SetTooltipText("Pause")
	c.recordDot.RemoveCSSClass("chatot-record-dot-paused")
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

// pollMinOptions is how many option entries the create-poll dialog opens
// with; ＋ Add option appends more.
const pollMinOptions = 2

// pickPoll opens the mockup's Create poll card: a question entry, option
// entries (two to start, "＋ Add option" for more), an "Allow multiple
// answers" check and a Cancel / Send poll footer. No-ops if there's no
// active chat or no window set yet.
func (c *Composer) pickPoll() {
	if c.state.jid == "" || c.window == nil {
		return
	}

	dialog := newCardDialog()
	dialog.SetTitle("Create poll")
	dialog.SetTransientFor(c.window)
	dialog.SetModal(true)
	dialog.SetDefaultSize(420, -1)

	body := dialogBody(10)

	question := gtk.NewEntry()
	question.SetPlaceholderText("Ask a question")
	question.AddCSSClass("chatot-dialog-entry")
	body.Append(question)

	options := gtk.NewBox(gtk.OrientationVertical, 10)
	body.Append(options)
	var entries []*gtk.Entry
	addOption := func() *gtk.Entry {
		e := gtk.NewEntry()
		e.SetPlaceholderText(fmt.Sprintf("Option %d", len(entries)+1))
		e.AddCSSClass("chatot-dialog-entry")
		e.AddCSSClass("chatot-poll-draft")
		entries = append(entries, e)
		options.Append(e)
		return e
	}
	for i := 0; i < pollMinOptions; i++ {
		addOption()
	}

	add := gtk.NewButtonWithLabel("＋ Add option")
	add.AddCSSClass("flat")
	add.AddCSSClass("chatot-link-btn")
	add.AddCSSClass("chatot-poll-add")
	add.SetHAlign(gtk.AlignStart)
	add.ConnectClicked(func() { addOption().GrabFocus() })
	body.Append(add)

	multi := gtk.NewCheckButtonWithLabel("Allow multiple answers")
	multi.AddCSSClass("chatot-poll-multi")
	body.Append(multi)

	footer := gtk.NewBox(gtk.OrientationHorizontal, 10)
	footer.AddCSSClass("chatot-dialog-footer")
	footer.SetHAlign(gtk.AlignEnd)
	cancel := gtk.NewButtonWithLabel("Cancel")
	cancel.AddCSSClass("chatot-chip-btn")
	cancel.ConnectClicked(func() { dialog.Close() })
	footer.Append(cancel)
	send := gtk.NewButtonWithLabel("Send poll")
	send.AddCSSClass("chatot-primary-btn")
	send.ConnectClicked(func() {
		opts := make([]string, len(entries))
		for i, e := range entries {
			opts[i] = e.Text()
		}
		if c.sendPoll(question.Text(), opts, multi.Active()) {
			dialog.Close()
		}
	})
	footer.Append(send)

	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.Append(body)
	root.Append(footer)
	dialog.SetChild(root)
	dialog.SetDefaultWidget(send)
	dialog.Present()
	question.GrabFocus()
}

// contactPick is one row of the Send contact card.
type contactPick struct {
	JID   string
	Name  string
	Phone string // "" when unknown
}

// contactPicks turns the chat list into the picker's rows: people only (no
// groups), with the phone the vCard will carry. Order is the chat list's.
func contactPicks(chats []client.Chat) []contactPick {
	out := make([]contactPick, 0, len(chats))
	for _, chat := range chats {
		if chat.IsGroup || strings.HasSuffix(chat.JID, "@newsletter") || chat.JID == "status@broadcast" {
			continue
		}
		out = append(out, contactPick{JID: chat.JID, Name: chat.Name, Phone: chat.Phone})
	}
	return out
}

// filterContactPicks keeps the rows whose name or phone contains query.
func filterContactPicks(picks []contactPick, query string) []contactPick {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return picks
	}
	out := make([]contactPick, 0, len(picks))
	for _, p := range picks {
		if strings.Contains(strings.ToLower(p.Name), query) || strings.Contains(p.Phone, query) {
			out = append(out, p)
		}
	}
	return out
}

// contactPhoneLabel is the picker row's mono subline: "+55 48 …" spaced
// the way the mockup writes numbers, or nothing when the number is unknown.
func contactPhoneLabel(phone string) string {
	if phone == "" {
		return ""
	}
	return "+" + phone
}

// pickContact opens the mockup's Send contact card: a pill search over a
// multi-select list of people (avatar, bold name, mono phone, round tick)
// and an "N selected · Send" footer. Every ticked person is sent as its
// own contact card. No-ops if there's no active chat or no window set yet.
func (c *Composer) pickContact() {
	if c.state.jid == "" || c.window == nil {
		return
	}

	// Every chat, not the list's default page: a contact to send may be
	// someone not talked to in a while.
	chats, err := c.c.Chats(contactPickLimit)
	if err != nil {
		log.Printf("chatot: load contacts failed: %v", err)
		return
	}
	picks := contactPicks(chats)

	dialog := newCardDialog()
	dialog.SetTitle("Send contact")
	dialog.SetTransientFor(c.window)
	dialog.SetModal(true)
	dialog.SetDefaultSize(400, 440)

	box := gtk.NewBox(gtk.OrientationVertical, 0)

	search := sidebarSearchEntry("Search contacts")
	searchRow := gtk.NewBox(gtk.OrientationVertical, 0)
	searchRow.AddCSSClass("chatot-forward-search")
	searchRow.Append(search)
	box.Append(searchRow)

	list := gtk.NewBox(gtk.OrientationVertical, 0)
	list.AddCSSClass("chatot-forward-list")
	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetVExpand(true)
	scroller.SetMinContentHeight(0)
	scroller.SetSizeRequest(-1, 120)
	scroller.SetChild(list)
	box.Append(scroller)

	footerRow := gtk.NewBox(gtk.OrientationHorizontal, 10)
	footerRow.AddCSSClass("chatot-dialog-footer")
	count := gtk.NewLabel("")
	count.SetXAlign(0)
	count.SetHExpand(true)
	count.SetVAlign(gtk.AlignCenter)
	count.AddCSSClass("chatot-card-value")
	footerRow.Append(count)
	send := gtk.NewButtonWithLabel("Send")
	send.AddCSSClass("chatot-primary-btn")
	send.SetSensitive(false)
	footerRow.Append(send)
	box.Append(footerRow)

	selected := map[string]contactPick{}
	cache := newAvatarCache()
	updateFooter := func() {
		count.SetText(contactSelectionLabel(len(selected)))
		send.SetSensitive(len(selected) > 0)
	}

	var rebuild func(string)
	rebuild = func(query string) {
		removeAllChildren(list)
		for _, p := range filterContactPicks(picks, query) {
			pick := p
			vm := chatRowVM(client.Chat{JID: p.JID, Name: p.Name}, time.Now())
			row := gtk.NewBox(gtk.OrientationHorizontal, 10)
			row.Append(buildAvatar(c.c, cache, p.JID, vm.Initial, contactPickAvatarSize))

			col := gtk.NewBox(gtk.OrientationVertical, 1)
			col.SetHExpand(true)
			col.SetVAlign(gtk.AlignCenter)
			name := gtk.NewLabel(p.Name)
			name.SetXAlign(0)
			name.SetEllipsize(pango.EllipsizeEnd)
			name.AddCSSClass("chatot-forward-name")
			col.Append(name)
			if phone := contactPhoneLabel(p.Phone); phone != "" {
				sub := gtk.NewLabel(phone)
				sub.SetXAlign(0)
				sub.AddCSSClass("chatot-contact-phone")
				col.Append(sub)
			}
			row.Append(col)

			_, on := selected[p.JID]
			check := newCheckGlyph(19, on)
			check.AddCSSClass("chatot-forward-check")
			if on {
				check.AddCSSClass("chatot-forward-check-on")
			}
			row.Append(check)

			btn := gtk.NewButton()
			btn.SetChild(row)
			btn.AddCSSClass("flat")
			btn.AddCSSClass("chatot-people-row")
			btn.ConnectClicked(func() {
				if _, ok := selected[pick.JID]; ok {
					delete(selected, pick.JID)
				} else {
					selected[pick.JID] = pick
				}
				updateFooter()
				rebuild(search.Text())
			})
			list.Append(btn)
		}
	}
	rebuild("")
	updateFooter()
	search.ConnectSearchChanged(func() { rebuild(search.Text()) })

	send.ConnectClicked(func() {
		// Keep the list's order rather than the map's.
		for _, p := range picks {
			if pick, ok := selected[p.JID]; ok {
				c.sendContact(pick.JID, pick.Name, pick.Phone)
			}
		}
		dialog.Close()
	})

	dialog.SetChild(box)
	dialog.SetDefaultWidget(send)
	dialog.Present()
}

// contactPickAvatarSize is the picker row's avatar, per the mockup.
const contactPickAvatarSize = 34

// contactPickLimit is how many chats the picker lists (the store's default
// page is 50).
const contactPickLimit = 5000

// contactSelectionLabel is the footer's count line.
func contactSelectionLabel(n int) string {
	if n == 0 {
		return "Pick people to send"
	}
	return fmt.Sprintf("%d selected", n)
}

// ShowLocationPicker / ShowPollDialog / ShowContactPicker open the attach
// sheets directly; screenshot hooks, so the dialogs can be captured without
// clicking through the attach menu. mode "manual" opens the location sheet
// on the map picker; "denied" opens it with system positioning off.
func (c *Composer) ShowLocationPicker(mode, point string) {
	if c.state.jid == "" || c.window == nil {
		return
	}
	// Only the plain hook talks to the positioning service; the staged
	// modes must never pop a system permission prompt during a capture.
	allow := LocationAccess && mode == ""
	p := showLocationPicker(c.window, allow, func(res locationResult) {
		if res.Live {
			c.sendLiveLocation(res)
			return
		}
		c.sendLocation(res)
	})
	if mode == "manual" {
		p.modeStack.SetVisibleChildName("manual")
		if lat, lon, ok := parseLatLon(point); ok {
			p.mapView.SetZoom(17)
			p.mapView.SetCentre(lat, lon)
			p.dropPin(lat, lon)
		}
	}
}

// parseLatLon reads "lat,lon".
func parseLatLon(s string) (lat, lon float64, ok bool) {
	parts := strings.Split(strings.TrimSpace(s), ",")
	if len(parts) != 2 {
		return 0, 0, false
	}
	lat, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	lon, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	return lat, lon, err1 == nil && err2 == nil
}

func (c *Composer) ShowPollDialog()    { c.pickPoll() }
func (c *Composer) ShowContactPicker() { c.pickContact() }
