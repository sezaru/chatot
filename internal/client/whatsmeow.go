package client

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
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

// eventBufferSize is generous on purpose: translate() runs synchronously
// inside whatsmeow's own dispatch goroutine, so a full channel must never
// block it. pushEvent drops the event (with a log line) instead of blocking.
const eventBufferSize = 1024

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

	events  chan Event
	qrCodes chan string
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

	w := &Whatsmeow{
		log:       clientLog,
		container: container,
		device:    device,
		wa:        wa,
		store:     msgStore,
		mediaDir:  filepath.Join(stateDir, "media"),
		events:    make(chan Event, eventBufferSize),
		qrCodes:   make(chan string, 8),
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
	if hs, ok := evt.(*events.HistorySync); ok {
		w.applyHistorySync(hs.Data)
	}
	e := translate(evt)
	if e == nil {
		return
	}
	w.ingestEvent(*e)
	w.pushEvent(*e)
}

func (w *Whatsmeow) pushEvent(e Event) {
	select {
	case w.events <- e:
	default:
		w.log.Warnf("event channel full, dropping event kind=%d", e.Kind)
	}
}

// Start connects to WhatsApp. If no device is paired yet, it opens the QR
// channel *before* connecting (whatsmeow requires this ordering) and fans
// QR codes onto QRCodes(); pairing completion arrives as an EventPairSuccess
// on Events() once whatsmeow's own handler processes events.PairSuccess.
func (w *Whatsmeow) Start(ctx context.Context) error {
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

func (w *Whatsmeow) Events() <-chan Event { return w.events }

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

// Messages reads a conversation's messages from the local store.
func (w *Whatsmeow) Messages(jid string, limit int) ([]Message, error) {
	rows, err := w.store.Messages(jid, limit)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: messages: %w", err)
	}
	out := make([]Message, len(rows))
	for i, m := range rows {
		out[i] = messageFromStore(m)
	}
	return out, nil
}

// Search runs an fts5 query over the local store.
// TODO(F11): implement once the store has an fts5 index.
func (w *Whatsmeow) Search(query string, limit int) ([]SearchHit, error) {
	return nil, errors.New("not implemented: Search (F11)")
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

// SendMedia uploads and sends an attachment.
// TODO(F8): implement media send.
func (w *Whatsmeow) SendMedia(ctx context.Context, jid string, m Attachment, replyTo *MsgRef) (string, error) {
	return "", errors.New("not implemented: SendMedia (F8)")
}

// SendVoice uploads and sends a voice note.
// TODO(F9): implement voice notes.
func (w *Whatsmeow) SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error) {
	return "", errors.New("not implemented: SendVoice (F9)")
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

// SendPresence sets the account's overall online/offline state.
// TODO(F10): implement presence.
func (w *Whatsmeow) SendPresence(available bool) error {
	return errors.New("not implemented: SendPresence (F10)")
}

// SendTyping sends a per-chat composing/paused indicator.
// TODO(F10): implement typing indicators.
func (w *Whatsmeow) SendTyping(jid string, typing bool) error {
	return errors.New("not implemented: SendTyping (F10)")
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
