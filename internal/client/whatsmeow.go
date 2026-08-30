package client

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"

	wastore "go.mau.fi/whatsmeow/store"
	"go.mau.fi/whatsmeow/store/sqlstore"

	"chatot/internal/store"
)

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

// SendText sends a plain-text (optionally reply) message.
// TODO(F6): implement compose/send.
func (w *Whatsmeow) SendText(ctx context.Context, jid, text string, replyTo *MsgRef) (string, error) {
	return "", errors.New("not implemented: SendText (F6)")
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

// React sets or clears a reaction on a message.
// TODO(F6): implement compose/react.
func (w *Whatsmeow) React(ctx context.Context, jid, msgID, emoji string) error {
	return errors.New("not implemented: React (F6)")
}

// MarkRead sends read receipts for the given messages.
// TODO(F6): implement compose/mark-read.
func (w *Whatsmeow) MarkRead(ctx context.Context, jid string, msgIDs []string) error {
	return errors.New("not implemented: MarkRead (F6)")
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

// DownloadMedia fetches and caches an attachment's bytes to disk.
// TODO(F7): implement on-demand download + capped cache.
func (w *Whatsmeow) DownloadMedia(ctx context.Context, msgID string) (string, error) {
	return "", errors.New("not implemented: DownloadMedia (F7)")
}
