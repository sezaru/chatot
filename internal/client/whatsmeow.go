package client

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/proto/waMmsRetry"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	wastore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"chatot/internal/media"
	"chatot/internal/store"
)

// maxMediaCacheBytes caps the on-disk attachment cache; DownloadMedia
// triggers eviction (oldest mtime first) after every successful download.
const maxMediaCacheBytes = 1 << 30 // 1 GiB

var _ Client = (*Whatsmeow)(nil)

// Whatsmeow is the whatsmeow-backed Client. Store reads (Chats, Messages,
// Search) and every write except lifecycle calls are owned by later
// features; see the TODO on each stub method below.
type Whatsmeow struct {
	log       waLog.Logger
	container *sqlstore.Container
	device    *wastore.Device
	store     *store.Store
	wa        *whatsmeow.Client
	mediaDir  string
	avatarDir string

	events  *eventBus
	qrCodes chan string

	presenceMu         sync.Mutex
	presenceSubscribed map[string]bool // jid -> SubscribePresence already requested

	avatarMu   sync.Mutex
	avatarMemo map[string]avatarEntry

	blockMu sync.Mutex
	blocked map[string]bool // jid -> blocked, warmed on connect and kept live by SetBlocked + inbound events
}

// NewWhatsmeow opens (or creates) the whatsmeow auth/session store under
// stateDir and constructs the client. stateDir defaults to
// $XDG_STATE_HOME/chatot, falling back to ~/.local/state/chatot.
func NewWhatsmeow(stateDir string) (*Whatsmeow, error) {
	if stateDir == "" {
		var err error
		stateDir, err = defaultStateDir()
		if err != nil {
			return nil, err
		}
	}
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		return nil, fmt.Errorf("chatot/client: create state dir: %w", err)
	}

	dbLog := waLog.Stdout("Database", "ERROR", false)
	dbPath := filepath.Join(stateDir, "session.db")
	dsn := fmt.Sprintf("file:%s?_foreign_keys=on", dbPath)
	container, err := sqlstore.New(context.Background(), "sqlite3", dsn, dbLog)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: open whatsmeow store: %w", err)
	}

	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			device = container.NewDevice()
		} else {
			return nil, fmt.Errorf("chatot/client: get device: %w", err)
		}
	}

	msgStore, err := store.Open(filepath.Join(stateDir, "chatot.db"))
	if err != nil {
		return nil, fmt.Errorf("chatot/client: open message store: %w", err)
	}

	clientLog := waLog.Stdout("Client", "ERROR", false)
	wa := whatsmeow.NewClient(device, clientLog)

	avatarDir := filepath.Join(stateDir, "avatars")
	if err := os.MkdirAll(avatarDir, 0o700); err != nil {
		return nil, fmt.Errorf("chatot/client: create avatar cache dir: %w", err)
	}

	w := &Whatsmeow{
		log:       clientLog,
		container: container,
		device:    device,
		wa:        wa,
		store:     msgStore,
		mediaDir:  filepath.Join(stateDir, "media"),
		avatarDir: avatarDir,
		events:    newEventBus(clientLog.Warnf),
		qrCodes:   make(chan string, 8),
		blocked:   make(map[string]bool),
	}
	wa.AddEventHandler(w.handleRaw)
	return w, nil
}

func defaultStateDir() (string, error) {
	if dir := os.Getenv("XDG_STATE_HOME"); dir != "" {
		return filepath.Join(dir, "chatot"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("chatot/client: resolve home dir: %w", err)
	}
	return filepath.Join(home, ".local", "state", "chatot"), nil
}

func (w *Whatsmeow) handleRaw(evt interface{}) {
	// Poll votes arrive as an events.Message wrapping a PollUpdateMessage;
	// decrypt + tally them here (like applyHistorySync's early branch) and
	// return, so they're never ingested as a blank text message.
	if v, ok := evt.(*events.Message); ok && v.Message.GetPollUpdateMessage() != nil {
		w.handlePollVote(v)
		return
	}
	if hs, ok := evt.(*events.HistorySync); ok {
		w.applyHistorySync(hs.Data)
	}
	if mr, ok := evt.(*events.MediaRetry); ok {
		w.handleMediaRetry(mr)
		return
	}
	// Avatars aren't stored in sqlite (no ingest path); just drop the memo so
	// the next Avatar() call re-fetches, and tell the UI to refresh.
	if p, ok := evt.(*events.Picture); ok {
		jid := p.JID.String()
		w.invalidateAvatar(jid)
		w.pushEvent(Event{Kind: EventAvatar, Avatar: &Avatar{JID: jid}})
		return
	}
	// App-state chat-organization events: not messages, so update the store
	// directly and push a refresh rather than routing through translate.
	if p, ok := evt.(*events.Pin); ok {
		w.applyChatUpdate(p.JID.String(), func(jid string) error {
			return w.store.SetChatPinned(jid, p.Action.GetPinned())
		})
		return
	}
	if m, ok := evt.(*events.Mute); ok {
		w.applyChatUpdate(m.JID.String(), func(jid string) error {
			return w.store.SetChatMuted(jid, m.Action.GetMuted())
		})
		return
	}
	if a, ok := evt.(*events.Archive); ok {
		w.applyChatUpdate(a.JID.String(), func(jid string) error {
			return w.store.SetChatArchived(jid, a.Action.GetArchived())
		})
		return
	}
	if r, ok := evt.(*events.MarkChatAsRead); ok {
		w.applyChatUpdate(r.JID.String(), func(jid string) error {
			return w.store.SetChatUnread(jid, !r.Action.GetRead())
		})
		return
	}
	// Star is a per-message app-state event (not per-chat), so it can't reuse
	// applyChatUpdate: it updates one message row and pushes the same
	// reaction-style reload React/StarMessage use to refresh an open thread.
	if st, ok := evt.(*events.Star); ok {
		jid := st.ChatJID.String()
		if err := w.store.SetMessageStarred(jid, st.MessageID, st.Action.GetStarred()); err != nil {
			w.log.Warnf("chatot/client: apply star app-state: %v", err)
		}
		w.pushEvent(Event{Kind: EventReaction, Reaction: &Reaction{ChatJID: jid, MsgID: st.MessageID}})
		return
	}
	// Label app-state: a LabelEdit updates the label registry; a
	// LabelAssociationChat toggles a chat's membership in a label. Both push
	// an EventLabelUpdate so the sidebar's label filter and chat list refresh.
	if le, ok := evt.(*events.LabelEdit); ok {
		if err := w.store.UpsertLabel(le.LabelID, le.Action.GetName(), int(le.Action.GetColor()), le.Action.GetDeleted()); err != nil {
			w.log.Warnf("chatot/client: apply label-edit app-state: %v", err)
		}
		w.pushEvent(Event{Kind: EventLabelUpdate})
		return
	}
	if la, ok := evt.(*events.LabelAssociationChat); ok {
		if err := w.store.SetChatLabel(la.LabelID, la.JID.String(), la.Action.GetLabeled()); err != nil {
			w.log.Warnf("chatot/client: apply label-association app-state: %v", err)
		}
		w.pushEvent(Event{Kind: EventLabelUpdate})
		return
	}
	// Blocklist changes: either a full list (Action == "modify", re-fetch) or
	// a batch of individual Changes applied to the cached set in place.
	if bl, ok := evt.(*events.Blocklist); ok {
		w.applyBlocklistEvent(bl)
		return
	}
	// Group metadata/membership changed. events.GroupInfo carries deltas
	// (Join/Leave/Promote/Demote/Name/Topic); rather than reassemble state
	// from those, just re-fetch the group in the background.
	if gi, ok := evt.(*events.GroupInfo); ok {
		w.refreshGroupInfo(gi.JID.String())
		return
	}
	if _, ok := evt.(*events.Connected); ok {
		// whatsmeow only delivers other users' presence after we've sent our
		// own at least once; do it on every (re)connect rather than tracking
		// whether it already succeeded.
		go func() {
			if err := w.SendPresence(true); err != nil {
				w.log.Warnf("chatot/client: send initial presence: %v", err)
			}
		}()
		// Warm the blocked-set cache so IsBlocked is meaningful before the
		// first explicit Blocklist() call.
		go func() {
			if _, err := w.Blocklist(context.Background()); err != nil {
				w.log.Warnf("chatot/client: warm blocklist cache: %v", err)
			}
		}()
	}
	e := translate(evt)
	if e == nil {
		return
	}
	w.ingestEvent(*e)
	w.pushEvent(*e)
}

func (w *Whatsmeow) pushEvent(e Event) { w.events.Publish(e) }

// applyChatUpdate runs a store mutation for an inbound app-state
// chat-organization event and pushes an EventChatUpdate so the chat list
// refreshes, regardless of whether the store write succeeded.
func (w *Whatsmeow) applyChatUpdate(jid string, mutate func(jid string) error) {
	if err := mutate(jid); err != nil {
		w.log.Warnf("chatot/client: apply chat-update app-state: %v", err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
}

// Start connects to WhatsApp. If no device is paired yet, it opens the QR
// channel *before* connecting (whatsmeow requires this ordering) and fans
// QR codes onto QRCodes(); pairing completion arrives as an EventPairSuccess
// on Events() once whatsmeow's own handler processes events.PairSuccess.
func (w *Whatsmeow) Start(ctx context.Context) error {
	if proxy := os.Getenv("CHATOT_PROXY"); proxy != "" {
		if err := w.wa.SetProxyAddress(proxy); err != nil {
			w.log.Warnf("failed to set proxy %q: %v", proxy, err)
		}
	}

	if w.wa.Store.ID == nil {
		qrChan, err := w.wa.GetQRChannel(ctx)
		if err != nil {
			return fmt.Errorf("chatot/client: get QR channel: %w", err)
		}
		go w.pumpQR(qrChan)
	}

	if err := w.wa.Connect(); err != nil {
		return fmt.Errorf("chatot/client: connect: %w", err)
	}

	context.AfterFunc(ctx, w.wa.Disconnect)
	return nil
}

func (w *Whatsmeow) pumpQR(qrChan <-chan whatsmeow.QRChannelItem) {
	for item := range qrChan {
		switch {
		case item.Event == whatsmeow.QRChannelEventCode:
			select {
			case w.qrCodes <- item.Code:
			default:
				w.log.Warnf("QR code channel full, dropping code")
			}
		case item == whatsmeow.QRChannelSuccess:
			return
		default:
			w.log.Warnf("QR pairing ended: %+v", item)
			return
		}
	}
}

func (w *Whatsmeow) QRCodes() <-chan string { return w.qrCodes }

func (w *Whatsmeow) LoggedIn() bool {
	return w.wa.Store.ID != nil && w.wa.IsLoggedIn()
}

func (w *Whatsmeow) Logout(ctx context.Context) error {
	return w.wa.Logout(ctx)
}

func (w *Whatsmeow) Events() <-chan Event { return w.events.Subscribe() }

// Chats reads the chat list from the local store.
func (w *Whatsmeow) Chats(limit int) ([]Chat, error) {
	rows, err := w.store.Chats(limit)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: chats: %w", err)
	}
	out := make([]Chat, len(rows))
	for i, c := range rows {
		out[i] = chatFromStore(c)
	}
	return out, nil
}

// Messages reads a conversation's messages from the local store. Opening a
// chat is the natural "the user is looking at this JID now" signal, so this
// is also where we lazily ask whatsmeow to start pushing that contact's
// presence (see subscribePresence) — there's no separate "open chat" seam
// in the Client interface, and adding one just for this would be overkill.
func (w *Whatsmeow) Messages(jid string, limit int) ([]Message, error) {
	w.subscribePresence(jid)

	rows, err := w.store.Messages(jid, limit)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: messages: %w", err)
	}
	out := make([]Message, len(rows))
	for i, m := range rows {
		out[i] = messageFromStore(m, w.ownJID())
	}
	return out, nil
}

// MessagesBefore reads an older page of a conversation from the local store,
// for the conversation view's scroll-up paging.
func (w *Whatsmeow) MessagesBefore(jid, beforeMsgID string, limit int) ([]Message, error) {
	rows, err := w.store.MessagesBefore(jid, beforeMsgID, limit)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: messages before: %w", err)
	}
	out := make([]Message, len(rows))
	for i, m := range rows {
		out[i] = messageFromStore(m, w.ownJID())
	}
	return out, nil
}

// subscribePresence asks whatsmeow to start pushing presence updates for
// jid, once per jid for this client's lifetime (SubscribePresence itself is
// idempotent server-side, but there's no reason to re-request it on every
// Messages() call for a chat that's already open). Runs the actual request
// in a goroutine since Messages is called from UI code that shouldn't block
// on a network round-trip just to render a chat.
func (w *Whatsmeow) subscribePresence(jid string) {
	w.presenceMu.Lock()
	if w.presenceSubscribed == nil {
		w.presenceSubscribed = make(map[string]bool)
	}
	if w.presenceSubscribed[jid] {
		w.presenceMu.Unlock()
		return
	}
	w.presenceSubscribed[jid] = true
	w.presenceMu.Unlock()

	to, err := types.ParseJID(jid)
	if err != nil {
		return
	}
	go func() {
		if err := w.wa.SubscribePresence(context.Background(), to); err != nil {
			w.log.Warnf("chatot/client: subscribe presence for %s: %v", jid, err)
		}
	}()
}

// Search runs an fts5 query over the local store's message text plus a
// chat-name match.
func (w *Whatsmeow) Search(query string, limit int) ([]SearchHit, error) {
	rows, err := w.store.Search(query, limit)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: search: %w", err)
	}
	out := make([]SearchHit, len(rows))
	for i, r := range rows {
		out[i] = SearchHit{ChatJID: r.ChatJID, MsgID: r.MsgID, ChatName: r.ChatName, Snippet: r.Snippet, TS: r.TS}
	}
	return out, nil
}

// SendText sends a plain-text (optionally reply) message. On success the
// sent message is upserted into the local store immediately (optimistic
// echo), rather than waiting for whatsmeow to redeliver it as an event.
func (w *Whatsmeow) SendText(ctx context.Context, jid, text string, replyTo *MsgRef) (string, error) {
	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	waMsg := &waE2E.Message{}
	if replyTo == nil {
		waMsg.Conversation = proto.String(text)
	} else {
		waMsg.ExtendedTextMessage = &waE2E.ExtendedTextMessage{
			Text:        proto.String(text),
			ContextInfo: w.replyContextInfo(jid, *replyTo),
		}
	}

	id := w.wa.GenerateMessageID()
	if _, err := w.wa.SendMessage(ctx, to, waMsg, whatsmeow.SendRequestExtra{ID: id}); err != nil {
		return "", fmt.Errorf("chatot/client: send text: %w", err)
	}

	out := Message{ID: id, ChatJID: jid, FromJID: w.ownJID(), FromMe: true, Text: text, TS: time.Now().Unix(), ReplyTo: replyTo}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of sent message failed: %v", err)
	}
	return id, nil
}

// EditMessage edits an own text message (WhatsApp's ~15-min window) via
// BuildEdit + SendMessage, then optimistically rewrites the stored row (text
// + edited flag) and pushes an EventMessage so the open chat re-renders. The
// echo edit whatsmeow later redelivers upserts the same row idempotently.
func (w *Whatsmeow) EditMessage(ctx context.Context, chatJID, msgID, newText string) error {
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", chatJID, err)
	}
	edited := w.wa.BuildEdit(chat, msgID, &waE2E.Message{Conversation: proto.String(newText)})
	if _, err := w.wa.SendMessage(ctx, chat, edited); err != nil {
		return fmt.Errorf("chatot/client: send edit: %w", err)
	}

	out := Message{ID: msgID, ChatJID: chatJID, FromJID: w.ownJID(), FromMe: true, Text: newText, TS: time.Now().Unix(), Edited: true}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of edited message failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventMessage, Message: &out})
	return nil
}

// DeleteMessage revokes ("delete for everyone") an own message via
// BuildRevoke + SendMessage, then optimistically marks the stored row deleted
// and pushes an EventRevoke so the open chat re-renders the tombstone. The
// echo revoke whatsmeow later redelivers applies idempotently.
func (w *Whatsmeow) DeleteMessage(ctx context.Context, chatJID, msgID string) error {
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", chatJID, err)
	}
	own, err := types.ParseJID(w.ownJID())
	if err != nil {
		return fmt.Errorf("chatot/client: delete message: not logged in: %w", err)
	}
	revoke := w.wa.BuildRevoke(chat, own, msgID)
	if _, err := w.wa.SendMessage(ctx, chat, revoke); err != nil {
		return fmt.Errorf("chatot/client: send revoke: %w", err)
	}

	ts := time.Now().Unix()
	if err := w.store.MarkMessageDeleted(chatJID, msgID, ts); err != nil {
		w.log.Warnf("chatot/client: optimistic mark-deleted failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventRevoke, Revoke: &Revoke{ChatJID: chatJID, MsgID: msgID, TS: ts}})
	return nil
}

// replyContextInfo builds the ContextInfo for a reply, quoting the target
// message's text and, for group chats, naming its original sender so
// WhatsApp can resolve the "@X replied" attribution. Best-effort: if the
// target isn't in the local store (e.g. store lookup race), it still sends
// a bare StanzaID reply.
func (w *Whatsmeow) replyContextInfo(jid string, replyTo MsgRef) *waE2E.ContextInfo {
	ctx := &waE2E.ContextInfo{StanzaID: proto.String(replyTo.MsgID)}
	quoted, ok, err := w.store.MessageByID(replyTo.ChatJID, replyTo.MsgID)
	if err != nil || !ok {
		return ctx
	}
	ctx.QuotedMessage = &waE2E.Message{Conversation: proto.String(quoted.Text)}
	if !quoted.FromMe && quoted.FromJID != "" {
		ctx.Participant = proto.String(quoted.FromJID)
	}
	return ctx
}

// ownJID returns this device's own JID as a string, or "" if not logged in.
func (w *Whatsmeow) ownJID() string {
	if w.wa.Store.ID == nil {
		return ""
	}
	return w.wa.Store.ID.String()
}

// SendMedia uploads m's bytes and sends it as an image/video/audio/document
// message. Like SendText, the sent message (plus its media row, with
// ProtoBlob and a local cache copy so it renders inline without a
// re-download) is upserted into the local store optimistically.
func (w *Whatsmeow) SendMedia(ctx context.Context, jid string, m Attachment, replyTo *MsgRef) (string, error) {
	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	data := m.Data
	if len(data) == 0 {
		if m.LocalPath == "" {
			return "", errors.New("chatot/client: send media: attachment has no data or local path")
		}
		if data, err = os.ReadFile(m.LocalPath); err != nil {
			return "", fmt.Errorf("chatot/client: send media: read file: %w", err)
		}
	}

	mimeType := m.MimeType
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	kind, mediaType := detectAttachmentKind(mimeType, m.Filename)

	resp, err := w.wa.Upload(ctx, data, mediaType)
	if err != nil {
		return "", fmt.Errorf("chatot/client: send media: upload: %w", err)
	}

	var ctxInfo *waE2E.ContextInfo
	if replyTo != nil {
		ctxInfo = w.replyContextInfo(jid, *replyTo)
	}
	waMsg, mediaProto := buildMediaMessage(kind, mimeType, m, &resp, ctxInfo)

	id := w.wa.GenerateMessageID()
	if _, err := w.wa.SendMessage(ctx, to, waMsg, whatsmeow.SendRequestExtra{ID: id}); err != nil {
		return "", fmt.Errorf("chatot/client: send media: %w", err)
	}

	localPath, cacheErr := w.writeMediaFile(jid, id, mimeType, data)
	if cacheErr != nil {
		w.log.Warnf("chatot/client: cache outbound media failed: %v", cacheErr)
	}

	// AudioMessage carries no caption on the wire; don't show one locally.
	caption := m.Caption
	if kind == "audio" {
		caption = ""
	}
	out := Message{
		ID: id, ChatJID: jid, FromJID: w.ownJID(), FromMe: true, TS: time.Now().Unix(), ReplyTo: replyTo,
		Attachment: &Attachment{
			Kind: kind, Filename: attachmentFilename(m), MimeType: mimeType,
			LocalPath: localPath, Caption: caption, ProtoBlob: marshalMedia(mediaProto),
		},
	}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of sent media failed: %v", err)
	}
	return id, nil
}

// SendLocation sends a static location pin. Live-location sharing isn't sent
// from chatot (only received), so IsLive is ignored here. Optimistic echo
// mirrors SendText.
func (w *Whatsmeow) SendLocation(ctx context.Context, jid string, loc Location, replyTo *MsgRef) (string, error) {
	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	locMsg := &waE2E.LocationMessage{
		DegreesLatitude:  proto.Float64(loc.Latitude),
		DegreesLongitude: proto.Float64(loc.Longitude),
	}
	if loc.Name != "" {
		locMsg.Name = proto.String(loc.Name)
	}
	if loc.Address != "" {
		locMsg.Address = proto.String(loc.Address)
	}
	if replyTo != nil {
		locMsg.ContextInfo = w.replyContextInfo(jid, *replyTo)
	}

	id := w.wa.GenerateMessageID()
	if _, err := w.wa.SendMessage(ctx, to, &waE2E.Message{LocationMessage: locMsg}, whatsmeow.SendRequestExtra{ID: id}); err != nil {
		return "", fmt.Errorf("chatot/client: send location: %w", err)
	}

	sent := loc
	sent.IsLive = false
	out := Message{ID: id, ChatJID: jid, FromJID: w.ownJID(), FromMe: true, TS: time.Now().Unix(), ReplyTo: replyTo, Location: &sent}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of sent location failed: %v", err)
	}
	return id, nil
}

// SendContact shares contact as a vCard. Optimistic echo mirrors SendLocation.
func (w *Whatsmeow) SendContact(ctx context.Context, jid string, contact Contact, replyTo *MsgRef) (string, error) {
	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	contactMsg := &waE2E.ContactMessage{
		DisplayName: proto.String(contact.DisplayName),
		Vcard:       proto.String(buildVCard(contact)),
	}
	if replyTo != nil {
		contactMsg.ContextInfo = w.replyContextInfo(jid, *replyTo)
	}

	id := w.wa.GenerateMessageID()
	if _, err := w.wa.SendMessage(ctx, to, &waE2E.Message{ContactMessage: contactMsg}, whatsmeow.SendRequestExtra{ID: id}); err != nil {
		return "", fmt.Errorf("chatot/client: send contact: %w", err)
	}

	sent := contact
	out := Message{ID: id, ChatJID: jid, FromJID: w.ownJID(), FromMe: true, TS: time.Now().Unix(), ReplyTo: replyTo, Contact: &sent}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of sent contact failed: %v", err)
	}
	return id, nil
}

// CreatePoll sends a poll-creation message and optimistically upserts it into
// the local store (like SendText), so it renders immediately with zero votes.
func (w *Whatsmeow) CreatePoll(ctx context.Context, jid, name string, options []string, selectable int) (string, error) {
	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	waMsg := w.wa.BuildPollCreation(name, options, selectable)
	id := w.wa.GenerateMessageID()
	if _, err := w.wa.SendMessage(ctx, to, waMsg, whatsmeow.SendRequestExtra{ID: id}); err != nil {
		return "", fmt.Errorf("chatot/client: create poll: %w", err)
	}

	opts := make([]PollOption, len(options))
	for i, o := range options {
		opts[i] = PollOption{Name: o}
	}
	out := Message{
		ID: id, ChatJID: jid, FromJID: w.ownJID(), FromMe: true, TS: time.Now().Unix(),
		Poll: &Poll{Name: name, Options: opts, SelectableCount: selectable},
	}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of created poll failed: %v", err)
	}
	return id, nil
}

// VotePoll casts the local user's vote on pollMsgID. It reconstructs the poll
// message's MessageInfo (which BuildPollVote needs to derive the shared secret
// keying the encrypted vote) from the stored poll row, sends the vote, then
// optimistically records it locally and emits an EventPollVote so the tally
// updates without waiting for a server echo.
func (w *Whatsmeow) VotePoll(ctx context.Context, chatJID, pollMsgID string, options []string) error {
	chat, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", chatJID, err)
	}
	target, ok, err := w.store.MessageByID(chatJID, pollMsgID)
	if err != nil {
		return fmt.Errorf("chatot/client: vote poll: lookup: %w", err)
	}
	if !ok {
		return fmt.Errorf("chatot/client: vote poll: poll %s not found", pollMsgID)
	}

	sender := chat
	if target.FromMe {
		if own := w.wa.Store.ID; own != nil {
			sender = *own
		}
	} else if target.FromJID != "" {
		if parsed, perr := types.ParseJID(target.FromJID); perr == nil {
			sender = parsed
		}
	}

	info := &types.MessageInfo{
		MessageSource: types.MessageSource{
			Chat: chat, Sender: sender, IsFromMe: target.FromMe,
			IsGroup: strings.HasSuffix(chatJID, "@g.us"),
		},
		ID: pollMsgID,
	}
	voteMsg, err := w.wa.BuildPollVote(ctx, info, options)
	if err != nil {
		return fmt.Errorf("chatot/client: vote poll: build: %w", err)
	}
	if _, err := w.wa.SendMessage(ctx, chat, voteMsg); err != nil {
		return fmt.Errorf("chatot/client: vote poll: send: %w", err)
	}

	hashes := make([][]byte, len(options))
	for i, o := range options {
		hashes[i] = hashPollOption(o)
	}
	if err := w.store.SetPollVotes(chatJID, pollMsgID, w.ownJID(), hashes); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of own vote failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventPollVote, PollVote: &PollVote{ChatJID: chatJID, PollMsgID: pollMsgID}})
	return nil
}

// handlePollVote decrypts an incoming poll vote, replaces the voter's stored
// selections, and emits an EventPollVote so the open chat refreshes its tally.
// DecryptPollVote relies on whatsmeow having stored the poll creation's
// message secret; if the poll was never seen by this device the decrypt fails
// and the vote is dropped (logged).
func (w *Whatsmeow) handlePollVote(v *events.Message) {
	ctx := context.Background()
	vote, err := w.wa.DecryptPollVote(ctx, v)
	if err != nil {
		w.log.Warnf("chatot/client: decrypt poll vote: %v", err)
		return
	}
	key := v.Message.GetPollUpdateMessage().GetPollCreationMessageKey()
	pollMsgID := key.GetID()
	chatJID := key.GetRemoteJID()
	if chatJID == "" {
		chatJID = v.Info.Chat.String()
	}
	voter := v.Info.Sender.String()
	if err := w.store.SetPollVotes(chatJID, pollMsgID, voter, vote.GetSelectedOptions()); err != nil {
		w.log.Warnf("chatot/client: store poll vote: %v", err)
		return
	}
	w.pushEvent(Event{Kind: EventPollVote, PollVote: &PollVote{ChatJID: chatJID, PollMsgID: pollMsgID}})
}

// detectAttachmentKind maps a mime type to chatot's media kind and
// whatsmeow's upload MediaType. If mimeType is empty or the generic sniffed
// fallback, it tries the filename extension before giving up. Anything that
// isn't image/video/audio is sent as a document.
func detectAttachmentKind(mimeType, filename string) (string, whatsmeow.MediaType) {
	mt := strings.ToLower(strings.TrimSpace(strings.SplitN(mimeType, ";", 2)[0]))
	if mt == "" || mt == "application/octet-stream" {
		if ext := filepath.Ext(filename); ext != "" {
			if guessed := mime.TypeByExtension(ext); guessed != "" {
				mt = strings.ToLower(strings.SplitN(guessed, ";", 2)[0])
			}
		}
	}
	switch {
	case strings.HasPrefix(mt, "image/"):
		return "image", whatsmeow.MediaImage
	case strings.HasPrefix(mt, "video/"):
		return "video", whatsmeow.MediaVideo
	case strings.HasPrefix(mt, "audio/"):
		return "audio", whatsmeow.MediaAudio
	default:
		return "document", whatsmeow.MediaDocument
	}
}

// attachmentFilename derives a document's display filename: the caller's
// explicit name, else the source file's basename, else a generic fallback.
func attachmentFilename(m Attachment) string {
	if m.Filename != "" {
		return m.Filename
	}
	if m.LocalPath != "" {
		return filepath.Base(m.LocalPath)
	}
	return "file"
}

// buildMediaMessage constructs the waE2E.Message envelope and returns the
// concrete media sub-message alongside it (so the caller can marshalMedia it
// for ProtoBlob, mirroring how extractText stashes inbound descriptors).
// Dimensions/duration are left nil — deriving them would need decoding the
// file, which SendMedia deliberately doesn't block on.
func buildMediaMessage(kind, mimeType string, m Attachment, resp *whatsmeow.UploadResponse, ctxInfo *waE2E.ContextInfo) (*waE2E.Message, proto.Message) {
	switch kind {
	case "image":
		img := &waE2E.ImageMessage{
			URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
			Mimetype: proto.String(mimeType), Caption: proto.String(m.Caption), ContextInfo: ctxInfo,
		}
		return &waE2E.Message{ImageMessage: img}, img
	case "video":
		vid := &waE2E.VideoMessage{
			URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
			Mimetype: proto.String(mimeType), Caption: proto.String(m.Caption), ContextInfo: ctxInfo,
		}
		return &waE2E.Message{VideoMessage: vid}, vid
	case "audio":
		aud := &waE2E.AudioMessage{
			URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
			Mimetype: proto.String(mimeType), ContextInfo: ctxInfo,
		}
		return &waE2E.Message{AudioMessage: aud}, aud
	default:
		doc := &waE2E.DocumentMessage{
			URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
			FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
			Mimetype: proto.String(mimeType), FileName: proto.String(attachmentFilename(m)),
			Caption: proto.String(m.Caption), ContextInfo: ctxInfo,
		}
		return &waE2E.Message{DocumentMessage: doc}, doc
	}
}

// voiceMimetype is what WhatsApp expects for a push-to-talk voice note; a
// bare "audio/ogg" renders as a regular file attachment instead of a
// playable voice bubble.
const voiceMimetype = "audio/ogg; codecs=opus"

// SendVoice uploads oggOpus and sends it as a PTT voice note (no reply
// support — voice notes aren't quoted in practice). Optimistic echo mirrors
// SendMedia's audio path but skips pushEvent to avoid a double render.
func (w *Whatsmeow) SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error) {
	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	if dur < 0 {
		dur = 0
	}

	resp, err := w.wa.Upload(ctx, oggOpus, whatsmeow.MediaAudio)
	if err != nil {
		return "", fmt.Errorf("chatot/client: send voice: upload: %w", err)
	}

	aud := &waE2E.AudioMessage{
		URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
		Mimetype: proto.String(voiceMimetype), PTT: proto.Bool(true), Seconds: proto.Uint32(uint32(dur)),
	}

	id := w.wa.GenerateMessageID()
	if _, err := w.wa.SendMessage(ctx, to, &waE2E.Message{AudioMessage: aud}, whatsmeow.SendRequestExtra{ID: id}); err != nil {
		return "", fmt.Errorf("chatot/client: send voice: %w", err)
	}

	localPath, cacheErr := w.writeMediaFile(jid, id, voiceMimetype, oggOpus)
	if cacheErr != nil {
		w.log.Warnf("chatot/client: cache outbound voice note failed: %v", cacheErr)
	}
	out := Message{
		ID: id, ChatJID: jid, FromJID: w.ownJID(), FromMe: true, TS: time.Now().Unix(),
		Attachment: &Attachment{Kind: "audio", MimeType: voiceMimetype, LocalPath: localPath, ProtoBlob: marshalMedia(aud)},
	}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of sent voice note failed: %v", err)
	}
	return id, nil
}

// stickerMimetype is what a proper WhatsApp sticker carries on the wire.
// Sending anything else still goes through (best-effort — no image->webp
// conversion here) but may not render as a sticker on other clients.
const stickerMimetype = "image/webp"

// SendSticker uploads the file at path and sends it as a sticker message.
// Stickers upload through the same MediaImage bucket as regular images
// (whatsmeow's classToMediaType maps StickerMessage -> MediaImage too); only
// the message type and mimetype mark it as a sticker on the wire. Optimistic
// echo mirrors SendMedia.
func (w *Whatsmeow) SendSticker(ctx context.Context, jid, path string) (string, error) {
	to, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("chatot/client: send sticker: read file: %w", err)
	}

	mimeType := stickerMimetype
	if sniffed := http.DetectContentType(data); sniffed != "image/webp" && sniffed != "application/octet-stream" {
		mimeType = sniffed
	}

	resp, err := w.wa.Upload(ctx, data, whatsmeow.MediaImage)
	if err != nil {
		return "", fmt.Errorf("chatot/client: send sticker: upload: %w", err)
	}

	sticker := &waE2E.StickerMessage{
		URL: &resp.URL, DirectPath: &resp.DirectPath, MediaKey: resp.MediaKey,
		FileEncSHA256: resp.FileEncSHA256, FileSHA256: resp.FileSHA256, FileLength: &resp.FileLength,
		Mimetype: proto.String(mimeType),
	}

	id := w.wa.GenerateMessageID()
	if _, err := w.wa.SendMessage(ctx, to, &waE2E.Message{StickerMessage: sticker}, whatsmeow.SendRequestExtra{ID: id}); err != nil {
		return "", fmt.Errorf("chatot/client: send sticker: %w", err)
	}

	localPath, cacheErr := w.writeMediaFile(jid, id, mimeType, data)
	if cacheErr != nil {
		w.log.Warnf("chatot/client: cache outbound sticker failed: %v", cacheErr)
	}
	out := Message{
		ID: id, ChatJID: jid, FromJID: w.ownJID(), FromMe: true, TS: time.Now().Unix(),
		Attachment: &Attachment{Kind: "sticker", MimeType: mimeType, LocalPath: localPath, ProtoBlob: marshalMedia(sticker)},
	}
	if err := w.ingestMessage(&out); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of sent sticker failed: %v", err)
	}
	return id, nil
}

// React sets ("" clears) a reaction on a message. sender in BuildReaction
// must be the *target* message's original sender (empty/self for our own
// messages), not the reactor — whatsmeow needs it to build the message key
// whatsapp uses to identify which message is being reacted to in a group.
func (w *Whatsmeow) React(ctx context.Context, jid, msgID, emoji string) error {
	chat, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	var sender types.JID
	target, ok, lookupErr := w.store.MessageByID(jid, msgID)
	switch {
	case lookupErr == nil && ok && !target.FromMe && target.FromJID != "":
		if sender, err = types.ParseJID(target.FromJID); err != nil {
			sender = types.EmptyJID
		}
	case chat.Server == types.GroupServer && (lookupErr != nil || !ok):
		// A zero sender makes BuildMessageKey mark the key FromMe=true, which
		// mis-identifies the target for a group message we didn't author.
		// Refuse rather than send a corrupt reaction.
		return fmt.Errorf("chatot/client: react: target message %s not found for group reaction key", msgID)
	}

	reaction := w.wa.BuildReaction(chat, sender, msgID, emoji)
	if _, err := w.wa.SendMessage(ctx, chat, reaction); err != nil {
		return fmt.Errorf("chatot/client: send reaction: %w", err)
	}

	if err := w.store.UpsertReaction(store.ReactionRow{
		ChatJID: jid, MsgID: msgID, ReactorJID: w.ownJID(), Emoji: emoji, TS: time.Now().Unix(),
	}); err != nil {
		w.log.Warnf("chatot/client: optimistic upsert of sent reaction failed: %v", err)
	}
	return nil
}

// MarkRead sends a single read receipt covering msgIDs. All of them must
// share the same original sender (whatsmeow's constraint); for 1:1 chats
// that's always the chat JID itself, which is what we pass as sender here.
// Group chats with mixed senders would need one call per sender — not
// needed yet since the UI only marks read on 1:1/aggregate chat open.
func (w *Whatsmeow) MarkRead(ctx context.Context, jid string, msgIDs []string) error {
	if len(msgIDs) == 0 {
		return nil
	}
	chat, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	sender := chat
	if chat.Server == types.GroupServer {
		sender = types.EmptyJID // best-effort: see doc comment above
	}
	ids := make([]types.MessageID, len(msgIDs))
	copy(ids, msgIDs)
	if err := w.wa.MarkRead(ctx, ids, time.Now(), chat, sender); err != nil {
		return fmt.Errorf("chatot/client: mark read: %w", err)
	}
	return w.store.MarkChatRead(jid)
}

// PinChat pins/unpins jid via app-state, then optimistically updates the
// local store and notifies the UI to refresh.
func (w *Whatsmeow) PinChat(ctx context.Context, jid string, pin bool) error {
	target, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	if err := w.wa.SendAppState(ctx, appstate.BuildPin(target, pin)); err != nil {
		return fmt.Errorf("chatot/client: send pin app-state: %w", err)
	}
	if err := w.store.SetChatPinned(jid, pin); err != nil {
		w.log.Warnf("chatot/client: optimistic pin update failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// MuteChat mutes/unmutes jid indefinitely via app-state.
func (w *Whatsmeow) MuteChat(ctx context.Context, jid string, mute bool) error {
	target, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	if err := w.wa.SendAppState(ctx, appstate.BuildMute(target, mute, 0)); err != nil {
		return fmt.Errorf("chatot/client: send mute app-state: %w", err)
	}
	if err := w.store.SetChatMuted(jid, mute); err != nil {
		w.log.Warnf("chatot/client: optimistic mute update failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// ArchiveChat archives/unarchives jid via app-state, using the chat's last
// known activity timestamp (the exact last-message key isn't tracked, but
// whatsmeow/WhatsApp accept a nil one).
func (w *Whatsmeow) ArchiveChat(ctx context.Context, jid string, archive bool) error {
	target, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	ts, err := w.store.ChatLastMessageTS(jid)
	if err != nil {
		w.log.Warnf("chatot/client: lookup last message ts for archive: %v", err)
	}
	lastMessageTS := time.Now()
	if ts > 0 {
		lastMessageTS = time.Unix(ts, 0)
	}
	if err := w.wa.SendAppState(ctx, appstate.BuildArchive(target, archive, lastMessageTS, nil)); err != nil {
		return fmt.Errorf("chatot/client: send archive app-state: %w", err)
	}
	if err := w.store.SetChatArchived(jid, archive); err != nil {
		w.log.Warnf("chatot/client: optimistic archive update failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// MarkChatUnread marks jid unread (unread=false in the underlying app-state
// patch marks it read, so an "unread" toggle is BuildMarkChatAsRead(jid,
// !unread, ...)).
func (w *Whatsmeow) MarkChatUnread(ctx context.Context, jid string, unread bool) error {
	target, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	ts, err := w.store.ChatLastMessageTS(jid)
	if err != nil {
		w.log.Warnf("chatot/client: lookup last message ts for mark-unread: %v", err)
	}
	lastMessageTS := time.Now()
	if ts > 0 {
		lastMessageTS = time.Unix(ts, 0)
	}
	if err := w.wa.SendAppState(ctx, appstate.BuildMarkChatAsRead(target, !unread, lastMessageTS, nil)); err != nil {
		return fmt.Errorf("chatot/client: send mark-unread app-state: %w", err)
	}
	if err := w.store.SetChatUnread(jid, unread); err != nil {
		w.log.Warnf("chatot/client: optimistic unread update failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// StarMessage stars/unstars msgID via app-state. Like React, BuildStar needs
// the target message's original sender to build its key: for our own
// message that's our own JID, for a group message the sender within the
// group, and for a 1:1 peer's message the chat JID itself.
func (w *Whatsmeow) StarMessage(ctx context.Context, jid, msgID string, starred bool) error {
	chat, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}

	target, ok, err := w.store.MessageByID(jid, msgID)
	if err != nil {
		return fmt.Errorf("chatot/client: lookup message %s: %w", msgID, err)
	}
	if !ok {
		return fmt.Errorf("chatot/client: message %s not found in chat %s", msgID, jid)
	}

	var sender types.JID
	switch {
	case target.FromMe:
		if sender, err = types.ParseJID(w.ownJID()); err != nil {
			return fmt.Errorf("chatot/client: parse own jid: %w", err)
		}
	case chat.Server == types.GroupServer:
		if sender, err = types.ParseJID(target.FromJID); err != nil {
			return fmt.Errorf("chatot/client: parse sender jid %q: %w", target.FromJID, err)
		}
	default:
		sender = chat
	}

	patch := appstate.BuildStar(chat, sender, types.MessageID(msgID), target.FromMe, starred)
	if err := w.wa.SendAppState(ctx, patch); err != nil {
		return fmt.Errorf("chatot/client: send star app-state: %w", err)
	}

	if err := w.store.SetMessageStarred(jid, msgID, starred); err != nil {
		w.log.Warnf("chatot/client: optimistic star update failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventReaction, Reaction: &Reaction{ChatJID: jid, MsgID: msgID}})
	return nil
}

// StarredMessages reads starred messages across every chat from the store.
func (w *Whatsmeow) StarredMessages(limit int) ([]Message, error) {
	rows, err := w.store.StarredMessages(limit)
	if err != nil {
		return nil, err
	}
	selfJID := w.ownJID()
	out := make([]Message, len(rows))
	for i, m := range rows {
		out[i] = messageFromStore(m, selfJID)
	}
	return out, nil
}

// Statuses reads recent status ("stories") updates from the store's
// status@broadcast chat, newest first.
func (w *Whatsmeow) Statuses(limit int) ([]Message, error) {
	rows, err := w.store.Statuses(limit)
	if err != nil {
		return nil, err
	}
	selfJID := w.ownJID()
	out := make([]Message, len(rows))
	for i, m := range rows {
		out[i] = messageFromStore(m, selfJID)
	}
	return out, nil
}

// PostStatus posts a text status update to the status broadcast.
//
// Live-unverifiable risk: whatsmeow may need the broadcast recipient list
// addressed for a status to actually reach contacts; we make the plain
// SendMessage call and surface whatever error it returns rather than faking
// success. Untested against a live account (no linked device here).
func (w *Whatsmeow) PostStatus(ctx context.Context, text string) error {
	msg := &waE2E.Message{
		ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String(text)},
	}
	if _, err := w.wa.SendMessage(ctx, types.StatusBroadcastJID, msg); err != nil {
		return fmt.Errorf("chatot/client: post status: %w", err)
	}
	return nil
}

// applyBlocklistEvent updates the cached blocked set from an inbound
// *events.Blocklist. A "modify" action carries no Changes and means the
// whole list must be re-fetched; anything else is a batch of individual
// block/unblock Changes applied in place. Either way it ends by pushing an
// EventChatUpdate so the chat list (and any open block-state UI) refreshes.
func (w *Whatsmeow) applyBlocklistEvent(bl *events.Blocklist) {
	if bl.Action == events.BlocklistActionModify {
		go func() {
			if _, err := w.Blocklist(context.Background()); err != nil {
				w.log.Warnf("chatot/client: refresh blocklist: %v", err)
			}
			w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{}})
		}()
		return
	}
	w.blockMu.Lock()
	for _, ch := range bl.Changes {
		jid := ch.JID.String()
		if ch.Action == events.BlocklistChangeActionBlock {
			w.blocked[jid] = true
		} else {
			delete(w.blocked, jid)
		}
	}
	w.blockMu.Unlock()
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{}})
}

// setBlockedCache replaces the cached blocked set wholesale from a
// types.Blocklist snapshot (returned by GetBlocklist/UpdateBlocklist).
func (w *Whatsmeow) setBlockedCache(list *types.Blocklist) {
	if list == nil {
		return
	}
	set := make(map[string]bool, len(list.JIDs))
	for _, j := range list.JIDs {
		set[j.String()] = true
	}
	w.blockMu.Lock()
	w.blocked = set
	w.blockMu.Unlock()
}

// Blocklist fetches the full blocked-JID list and refreshes the local cache
// IsBlocked reads.
func (w *Whatsmeow) Blocklist(ctx context.Context) ([]string, error) {
	list, err := w.wa.GetBlocklist(ctx)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: get blocklist: %w", err)
	}
	w.setBlockedCache(list)
	out := make([]string, len(list.JIDs))
	for i, j := range list.JIDs {
		out[i] = j.String()
	}
	return out, nil
}

// SetBlocked blocks or unblocks jid, refreshing the cached set from the
// server's authoritative response and notifying the UI to refresh.
func (w *Whatsmeow) SetBlocked(ctx context.Context, jid string, blocked bool) error {
	target, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	action := events.BlocklistChangeActionUnblock
	if blocked {
		action = events.BlocklistChangeActionBlock
	}
	list, err := w.wa.UpdateBlocklist(ctx, target, action)
	if err != nil {
		return fmt.Errorf("chatot/client: update blocklist: %w", err)
	}
	w.setBlockedCache(list)
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// IsBlocked is a cheap synchronous read of the cached blocked set, warmed on
// connect and kept live by SetBlocked and inbound Blocklist events.
func (w *Whatsmeow) IsBlocked(jid string) bool {
	w.blockMu.Lock()
	defer w.blockMu.Unlock()
	return w.blocked[jid]
}

// PrivacySettings reads the account's privacy settings; read-only, no
// setter is offered.
func (w *Whatsmeow) PrivacySettings(ctx context.Context) (map[string]string, error) {
	return privacySettingsToMap(w.wa.GetPrivacySettings(ctx)), nil
}

// SendPresence sets the account's overall online/offline state. Also the
// call whatsmeow needs at least once after connecting before it will
// deliver other users' presence to us; handleRaw fires this on every
// events.Connected.
func (w *Whatsmeow) SendPresence(available bool) error {
	state := types.PresenceUnavailable
	if available {
		state = types.PresenceAvailable
	}
	if err := w.wa.SendPresence(context.Background(), state); err != nil {
		return fmt.Errorf("chatot/client: send presence: %w", err)
	}
	return nil
}

// CheckOnWhatsApp queries whatsmeow's usync for a single phone number.
func (w *Whatsmeow) CheckOnWhatsApp(ctx context.Context, phone string) (string, bool, error) {
	resp, err := w.wa.IsOnWhatsApp(ctx, []string{phone})
	if err != nil {
		return "", false, fmt.Errorf("chatot/client: is on whatsapp: %w", err)
	}
	if len(resp) == 0 || !resp[0].IsIn {
		return "", false, nil
	}
	return resp[0].JID.String(), true, nil
}

// SendTyping sends a per-chat composing/paused indicator as plain text media.
func (w *Whatsmeow) SendTyping(jid string, typing bool) error {
	to, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	state := types.ChatPresencePaused
	if typing {
		state = types.ChatPresenceComposing
	}
	if err := w.wa.SendChatPresence(context.Background(), to, state, types.ChatPresenceMediaText); err != nil {
		return fmt.Errorf("chatot/client: send chat presence: %w", err)
	}
	return nil
}

// SendRecording sends the "recording a voice note" chat-presence (composing
// + audio media) when recording is true, else the plain paused state.
func (w *Whatsmeow) SendRecording(jid string, recording bool) error {
	to, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	if !recording {
		if err := w.wa.SendChatPresence(context.Background(), to, types.ChatPresencePaused, types.ChatPresenceMediaText); err != nil {
			return fmt.Errorf("chatot/client: send chat presence: %w", err)
		}
		return nil
	}
	if err := w.wa.SendChatPresence(context.Background(), to, types.ChatPresenceComposing, types.ChatPresenceMediaAudio); err != nil {
		return fmt.Errorf("chatot/client: send chat presence: %w", err)
	}
	return nil
}

// RejectCall declines an incoming call offer. chatot never places or
// answers calls (whatsmeow can't); this is the only call action supported.
func (w *Whatsmeow) RejectCall(ctx context.Context, callJID, callID string) error {
	from, err := types.ParseJID(callJID)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", callJID, err)
	}
	if err := w.wa.RejectCall(ctx, from, callID); err != nil {
		return fmt.Errorf("chatot/client: reject call: %w", err)
	}
	return nil
}

// DownloadMedia fetches and caches an attachment's bytes to disk, decrypting
// via whatsmeow using the descriptor stashed at ingest time (see
// decodeDownloadable). Already-downloaded media (local_path set and the
// file still present) is returned without hitting the network. On success,
// triggers cache eviction so the cache stays under maxMediaCacheBytes.
func (w *Whatsmeow) DownloadMedia(ctx context.Context, msgID string) (string, error) {
	row, ok, err := w.store.MediaByMsgID(msgID)
	if err != nil {
		return "", fmt.Errorf("chatot/client: download media: lookup: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("chatot/client: download media: no media for message %s", msgID)
	}
	if row.LocalPath != "" {
		if _, statErr := os.Stat(row.LocalPath); statErr == nil {
			return row.LocalPath, nil
		}
	}

	downloadable, err := decodeDownloadable(row.Kind, row.ProtoBlob)
	if err != nil {
		return "", fmt.Errorf("chatot/client: download media: %w", err)
	}
	data, err := w.wa.Download(ctx, downloadable)
	if err != nil {
		go w.requestMediaRetry(row, downloadable)
		return "", fmt.Errorf("chatot/client: download media: %w", err)
	}

	path, err := w.writeMediaFile(row.ChatJID, row.MsgID, row.MimeType, data)
	if err != nil {
		return "", fmt.Errorf("chatot/client: download media: write: %w", err)
	}
	if err := w.store.SetMediaLocalPath(row.ChatJID, row.MsgID, path); err != nil {
		w.log.Warnf("chatot/client: set media local path: %v", err)
	}
	w.evictMediaCache()
	return path, nil
}

// decodeDownloadable reconstructs the concrete waE2E media message proto
// from its stashed bytes. It must be the concrete type (not a hand-rolled
// struct satisfying whatsmeow.DownloadableMessage): whatsmeow.Client.Download
// resolves the media type via protoreflect on the concrete type, so a
// lookalike struct would fail with ErrUnknownMediaType.
func decodeDownloadable(kind string, blob []byte) (whatsmeow.DownloadableMessage, error) {
	if len(blob) == 0 {
		return nil, fmt.Errorf("no download descriptor stored for kind %q", kind)
	}
	var msg proto.Message
	switch kind {
	case "image":
		msg = &waE2E.ImageMessage{}
	case "video":
		msg = &waE2E.VideoMessage{}
	case "audio":
		msg = &waE2E.AudioMessage{}
	case "document":
		msg = &waE2E.DocumentMessage{}
	case "sticker":
		msg = &waE2E.StickerMessage{}
	default:
		return nil, fmt.Errorf("unsupported media kind %q", kind)
	}
	if err := proto.Unmarshal(blob, msg); err != nil {
		return nil, fmt.Errorf("decode media descriptor: %w", err)
	}
	return msg.(whatsmeow.DownloadableMessage), nil
}

// requestMediaRetry asks the sender's device to re-upload media whose direct
// path expired (the common cause of a DownloadMedia failure on old
// history). Best-effort: every failure here is logged and swallowed, never
// surfaced to the caller who's already got the original download error. A
// successful request's response arrives later as an *events.MediaRetry,
// decrypted in handleRaw.
func (w *Whatsmeow) requestMediaRetry(row store.MediaRow, downloadable whatsmeow.DownloadableMessage) {
	mediaKey := downloadable.GetMediaKey()
	if len(mediaKey) == 0 {
		return
	}
	msg, ok, err := w.store.MessageByID(row.ChatJID, row.MsgID)
	if err != nil || !ok {
		return
	}
	chatJID, err := types.ParseJID(row.ChatJID)
	if err != nil {
		return
	}
	sender := chatJID
	if !msg.FromMe {
		if s, err := types.ParseJID(msg.FromJID); err == nil {
			sender = s
		}
	} else if own, err := types.ParseJID(w.ownJID()); err == nil {
		sender = own
	}
	info := &types.MessageInfo{
		ID: types.MessageID(row.MsgID),
		MessageSource: types.MessageSource{
			Chat:     chatJID,
			Sender:   sender,
			IsFromMe: msg.FromMe,
			IsGroup:  chatJID.Server == types.GroupServer,
		},
	}
	if err := w.wa.SendMediaRetryReceipt(context.Background(), info, mediaKey); err != nil {
		w.log.Warnf("chatot/client: send media retry receipt: %v", err)
	}
}

// handleMediaRetry decrypts the phone's response to requestMediaRetry and, on
// success, patches the stored descriptor's direct path so a subsequent
// DownloadMedia can fetch the re-uploaded file. Every failure is logged and
// swallowed: this is a background repair path, not something the UI waits on.
func (w *Whatsmeow) handleMediaRetry(evt *events.MediaRetry) {
	row, ok, err := w.store.MediaByMsgID(string(evt.MessageID))
	if err != nil || !ok {
		return
	}
	downloadable, err := decodeDownloadable(row.Kind, row.ProtoBlob)
	if err != nil {
		w.log.Warnf("chatot/client: media retry: decode stored descriptor: %v", err)
		return
	}
	notif, err := whatsmeow.DecryptMediaRetryNotification(evt, downloadable.GetMediaKey())
	if err != nil {
		w.log.Warnf("chatot/client: media retry: decrypt notification: %v", err)
		return
	}
	if notif.GetResult() != waMmsRetry.MediaRetryNotification_SUCCESS || notif.GetDirectPath() == "" {
		w.log.Warnf("chatot/client: media retry unsuccessful for %s (result=%v)", evt.MessageID, notif.GetResult())
		return
	}
	descriptor := downloadable.(proto.Message)
	setDirectPath(descriptor, notif.GetDirectPath())
	blob, err := proto.Marshal(descriptor)
	if err != nil {
		w.log.Warnf("chatot/client: media retry: re-marshal descriptor: %v", err)
		return
	}
	if err := w.store.SetMediaProtoBlob(row.ChatJID, row.MsgID, blob); err != nil {
		w.log.Warnf("chatot/client: media retry: update proto blob: %v", err)
		return
	}
	w.pushEvent(Event{Kind: EventReaction, Reaction: &Reaction{ChatJID: row.ChatJID, MsgID: row.MsgID}})
}

// setDirectPath updates the direct-path field of a decoded media descriptor
// in place, ahead of re-marshaling it back to store: DownloadableMessage
// doesn't expose a setter, so each concrete waE2E type needs its own case.
func setDirectPath(m proto.Message, path string) {
	switch v := m.(type) {
	case *waE2E.ImageMessage:
		v.DirectPath = proto.String(path)
	case *waE2E.VideoMessage:
		v.DirectPath = proto.String(path)
	case *waE2E.AudioMessage:
		v.DirectPath = proto.String(path)
	case *waE2E.DocumentMessage:
		v.DirectPath = proto.String(path)
	case *waE2E.StickerMessage:
		v.DirectPath = proto.String(path)
	}
}

// writeMediaFile writes decrypted attachment bytes under
// mediaDir/<chatJID>/<msgID><ext>, creating the chat's cache dir as needed.
func (w *Whatsmeow) writeMediaFile(chatJID, msgID, mimeType string, data []byte) (string, error) {
	dir := filepath.Join(w.mediaDir, chatJID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, msgID+extForMime(mimeType))
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// evictMediaCache caps the on-disk cache, NULLing local_path for whatever it
// deletes so the UI re-offers tap-to-load.
func (w *Whatsmeow) evictMediaCache() {
	if err := media.Evict(w.mediaDir, maxMediaCacheBytes, w.store.NullMediaLocalPathByPath); err != nil {
		w.log.Warnf("chatot/client: media cache eviction: %v", err)
	}
}

// extForMime maps the handful of mime types chatot actually sees to a file
// extension; unrecognized types are left extensionless rather than guessed.
func extForMime(mimeType string) string {
	switch strings.SplitN(mimeType, ";", 2)[0] {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/webp":
		return ".webp"
	case "image/gif":
		return ".gif"
	case "video/mp4":
		return ".mp4"
	case "audio/ogg":
		return ".ogg"
	case "audio/mpeg":
		return ".mp3"
	case "application/pdf":
		return ".pdf"
	default:
		return ""
	}
}
