package client

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var _ Client = (*AccountManager)(nil)

// Account is one logged-in WhatsApp account: a stable slug (ID), a display
// label (Name) and the per-account Client backing it (a *Whatsmeow in prod,
// a *Fake in dev), each with its own state dir.
type Account struct {
	ID   string
	Name string
	c    Client
	// Proxy is this account's optional per-account proxy URL, applied when its
	// client is (re)created; overrides the global proxy for this account.
	Proxy string
	// stop cancels the context this account's client was Started with, so
	// RemoveAccount can disconnect it without touching the whole app. Nil when
	// the account isn't currently running.
	stop context.CancelFunc
}

// QRCodes exposes this account's pairing-QR stream so the add-account dialog
// can render codes for a not-yet-linked account without going through the
// manager's active-account proxy.
func (a *Account) QRCodes() <-chan string { return a.c.QRCodes() }

// Events exposes this account's own event stream so the add-account dialog can
// detect pair success on the new account specifically.
func (a *Account) Events() <-chan Event { return a.c.Events() }

// LoggedIn reports whether this account's client is paired and connected.
func (a *Account) LoggedIn() bool { return a.c.LoggedIn() }

// PairPhone requests a phone-number pairing code for this account.
func (a *Account) PairPhone(ctx context.Context, phone string) (string, error) {
	return a.c.PairPhone(ctx, phone)
}

// AccountMeta is the display-facing view of an account (no live Client), for
// the account switcher UI. Status/Unread are a best-effort snapshot read off
// the account's client at Accounts() time: Status is "Connected" when logged
// in, else the relink prompt (a live "reconnecting" state isn't observable
// through the Client seam); Unread is the number of chats with unread
// messages outside the archive, what the sidebar's Unread chip counts and
// the phone's own badge ignores archived chats for.
type AccountMeta struct {
	ID     string
	Name   string
	Status string
	Phone  string
	Unread int
}

// phoneFromJID extracts the dialable number from a user JID
// ("1234567890@s.whatsapp.net" → "+1234567890"); "" when not logged in.
func phoneFromJID(jid string) string {
	user, _, ok := strings.Cut(jid, "@")
	if !ok || user == "" {
		return ""
	}
	// Drop any device/agent suffix (":<n>" or ".<n>").
	if i := strings.IndexAny(user, ":."); i >= 0 {
		user = user[:i]
	}
	if user == "" {
		return ""
	}
	return "+" + user
}

// accountStatusLine is the pure status subline for an account: a paired
// account whose socket is down is between connections, not signed out.
func accountStatusLine(paired, loggedIn bool) string {
	switch {
	case loggedIn:
		return "Connected"
	case paired:
		return "Reconnecting…"
	}
	return "Logged out · scan to relink"
}

// AccountManager owns an ordered set of accounts, exactly one of them active,
// and is itself a Client: every Client method forwards to the active
// account's client. It exposes its own event/QR streams that proxy the active
// account's streams, so switching the active account is invisible to the rest
// of the app (which only ever holds a Client). Future accounts get their own
// state dir under $XDG_STATE_HOME/chatot/accounts/<id>/; the "default" account
// keeps the legacy $XDG_STATE_HOME/chatot path for back-compat.
type AccountManager struct {
	mu       sync.Mutex
	accounts []*Account
	activeID string
	// stopProxy is closed to tear down the goroutines proxying the current
	// active account's streams; a fresh one is made on every active swap.
	stopProxy chan struct{}

	events  *eventBus
	qrCodes chan string

	// keepInactive keeps non-active accounts connected in the background (the
	// default). When false, only the active account runs; switching accounts
	// starts the newly-active one and stops the previously-active.
	keepInactive bool
	// baseCtx is the context handed to Start, reused to start accounts later
	// (on an account switch or a keep-connected toggle).
	baseCtx context.Context

	// baseDir is $XDG_STATE_HOME/chatot; pairing accounts get baseDir/accounts/
	// <id>/ and the roster is baseDir/accounts.json. Empty in fake/test mode,
	// where AddPairingAccount and roster persistence are disabled.
	baseDir string
}

// NewAccountManager returns an empty manager. Register accounts with
// AddAccount; the first one added becomes active.
func NewAccountManager() *AccountManager {
	return &AccountManager{
		events:       newEventBus(nil),
		qrCodes:      make(chan string, 1),
		keepInactive: true,
	}
}

// AddAccount registers c under id/name. The first account added becomes the
// active one and its streams start being proxied immediately.
func (m *AccountManager) AddAccount(id, name string, c Client) {
	m.mu.Lock()
	defer m.mu.Unlock()
	a := &Account{ID: id, Name: name, c: c}
	m.accounts = append(m.accounts, a)
	if m.activeID == "" {
		m.activeID = id
		m.stopProxy = make(chan struct{})
		m.startProxy(a, m.stopProxy)
	}
}

// startProxy subscribes to a's event/QR streams and re-publishes each onto the
// manager's own streams until stop is closed. It Subscribes synchronously so
// the subscription exists before startProxy returns; only the forwarding runs
// in goroutines.
func (m *AccountManager) startProxy(a *Account, stop chan struct{}) {
	evCh := a.c.Events()
	go func() {
		for {
			select {
			case <-stop:
				return
			case ev, ok := <-evCh:
				if !ok {
					return
				}
				// Re-check stop: select picks randomly when both are ready, so
				// a just-stopped proxy must not forward a straggler event.
				select {
				case <-stop:
					return
				default:
					m.events.Publish(ev)
				}
			}
		}
	}()

	qrCh := a.c.QRCodes()
	go func() {
		for {
			select {
			case <-stop:
				return
			case code, ok := <-qrCh:
				if !ok {
					return
				}
				select {
				case <-stop:
					return
				default:
				}
				select {
				case m.qrCodes <- code:
				default:
				}
			}
		}
	}()
}

// active returns the active account's client. It is never nil once an account
// has been added.
func (m *AccountManager) active() Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.accounts {
		if a.ID == m.activeID {
			return a.c
		}
	}
	return nil
}

// Accounts returns the registered accounts in insertion order.
func (m *AccountManager) Accounts() []AccountMeta {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]AccountMeta, len(m.accounts))
	for i, a := range m.accounts {
		unread := 0
		if chats, err := a.c.Chats(0); err == nil {
			for _, ch := range chats {
				if ch.UnreadCount > 0 && !ch.Archived {
					unread++
				}
			}
		}
		out[i] = AccountMeta{
			ID:     a.ID,
			Name:   a.displayName(i),
			Status: accountStatusLine(a.c.Paired(), a.c.LoggedIn()),
			Phone:  phoneFromJID(a.c.OwnJID()),
			Unread: unread,
		}
	}
	return out
}

// ActiveID returns the active account's id ("" if none registered).
func (m *AccountManager) ActiveID() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.activeID
}

// SetActive switches the active account to id: it stops proxying the old
// account's streams, starts proxying the new one, then publishes a synthetic
// reload event so the chat list (and any open conversation) rebuild from the
// newly active account.
func (m *AccountManager) SetActive(id string) error {
	m.mu.Lock()
	target := m.findLocked(id)
	if target == nil {
		m.mu.Unlock()
		return fmt.Errorf("chatot/client: unknown account %q", id)
	}
	if id == m.activeID {
		m.mu.Unlock()
		return nil
	}
	prev := m.findLocked(m.activeID)
	keep := m.keepInactive
	ctx := m.baseCtx
	if m.stopProxy != nil {
		close(m.stopProxy)
	}
	m.activeID = id
	m.stopProxy = make(chan struct{})
	m.startProxy(target, m.stopProxy)
	m.mu.Unlock()

	// With background accounts disconnected, bring the newly-active one up and
	// take the previously-active one down (start-before-stop avoids a gap).
	if !keep && ctx != nil {
		if err := m.startAccount(ctx, target); err != nil {
			return err
		}
		if prev != nil {
			m.stopAccount(prev)
		}
	}

	m.events.Publish(Event{Kind: EventHistorySync, HistorySync: &HistorySync{Progress: -1}})
	return nil
}

// Start starts the registered accounts and remembers ctx so accounts can be
// (re)started later on a switch or a keep-connected toggle. With keepInactive
// (the default) every account is started so background accounts stay connected;
// with it off only the active account runs. Single-account behavior is
// identical either way.
func (m *AccountManager) Start(ctx context.Context) error {
	m.mu.Lock()
	m.baseCtx = ctx
	keep := m.keepInactive
	active := m.activeID
	accts := make([]*Account, len(m.accounts))
	copy(accts, m.accounts)
	m.mu.Unlock()
	for _, a := range accts {
		if !keep && a.ID != active {
			continue
		}
		if err := m.startAccount(ctx, a); err != nil {
			return err
		}
	}
	return nil
}

// startAccount starts a on a child of ctx if it isn't already running,
// recording its cancel func so it can be stopped later. A no-op when already
// running (stop != nil).
func (m *AccountManager) startAccount(ctx context.Context, a *Account) error {
	m.mu.Lock()
	if a.stop != nil {
		m.mu.Unlock()
		return nil
	}
	actx, cancel := context.WithCancel(ctx)
	a.stop = cancel
	m.mu.Unlock()
	if err := a.c.Start(actx); err != nil {
		cancel()
		m.mu.Lock()
		a.stop = nil
		m.mu.Unlock()
		return err
	}
	return nil
}

// stopAccount cancels a's client context and marks it not running. A no-op when
// already stopped.
func (m *AccountManager) stopAccount(a *Account) {
	m.mu.Lock()
	stop := a.stop
	a.stop = nil
	m.mu.Unlock()
	if stop != nil {
		stop()
	}
}

// ActiveName returns the active account's display label ("" if none).
func (m *AccountManager) ActiveName() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i, a := range m.accounts {
		if a.ID == m.activeID {
			return a.displayName(i)
		}
	}
	return ""
}

// Count returns the number of registered accounts.
func (m *AccountManager) Count() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.accounts)
}

// SetKeepInactiveConnected applies the keep-inactive-connected preference: when
// turned off, every non-active account is disconnected; when turned on, they're
// (re)connected. Idempotent; a no-op before Start (nothing is running yet).
func (m *AccountManager) SetKeepInactiveConnected(keep bool) {
	m.mu.Lock()
	if m.keepInactive == keep {
		m.mu.Unlock()
		return
	}
	m.keepInactive = keep
	ctx := m.baseCtx
	active := m.activeID
	accts := make([]*Account, len(m.accounts))
	copy(accts, m.accounts)
	m.mu.Unlock()

	for _, a := range accts {
		if a.ID == active {
			continue
		}
		if keep {
			if ctx != nil {
				if err := m.startAccount(ctx, a); err != nil {
					return
				}
			}
		} else {
			m.stopAccount(a)
		}
	}
}

// SetAccountProxy records a per-account proxy URL and persists the roster. It
// does not reconnect the account: the proxy is applied when the account's
// client is next (re)created, so a relink or restart is needed to take effect.
func (m *AccountManager) SetAccountProxy(id, proxy string) error {
	m.mu.Lock()
	a := m.findLocked(id)
	if a == nil {
		m.mu.Unlock()
		return fmt.Errorf("chatot/client: unknown account %q", id)
	}
	a.Proxy = proxy
	m.mu.Unlock()
	return m.persistRoster()
}

// AccountProxy returns id's configured per-account proxy URL ("" if none/unknown).
func (m *AccountManager) AccountProxy(id string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a := m.findLocked(id); a != nil {
		return a.Proxy
	}
	return ""
}

// findLocked returns the account with id, or nil. Caller must hold m.mu.
func (m *AccountManager) findLocked(id string) *Account {
	for _, a := range m.accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// SetBaseDir records the manager's base state dir ($XDG_STATE_HOME/chatot),
// enabling pairing-account creation and roster persistence. Call before
// LoadRoster.
func (m *AccountManager) SetBaseDir(dir string) { m.baseDir = dir }

// LoadRoster re-creates every persisted pairing account from
// baseDir/accounts.json, each backed by NewWhatsmeow(baseDir/accounts/<id>/),
// and registers it (inactive — the default account, added first, stays
// active). A missing roster is a no-op, so single-account behavior is
// unchanged. No-op when no base dir is set (fake/test mode).
func (m *AccountManager) LoadRoster() error {
	if m.baseDir == "" {
		return nil
	}
	r, err := loadRoster(filepath.Join(m.baseDir, rosterFile))
	if err != nil {
		return err
	}
	for _, e := range r.Accounts {
		c, err := NewWhatsmeow(m.accountDir(e.ID))
		if err != nil {
			return fmt.Errorf("chatot/client: restore account %q: %w", e.ID, err)
		}
		if e.Proxy != "" {
			c.SetProxy(e.Proxy)
		}
		m.AddAccount(e.ID, e.Label, c)
		if a := m.accountByID(e.ID); a != nil {
			a.Proxy = e.Proxy
		}
	}
	return nil
}

// AddPairingAccount creates a brand-new, not-yet-linked account under label:
// it slugifies label to a unique id, builds a whatsmeow client on a fresh
// per-account state dir, registers it (inactive) and Starts it so its QR/pair
// events flow, then persists the roster. The returned *Account exposes its own
// QRCodes()/Events() so the caller can drive the pairing UI. Fails in fake/test
// mode (no base dir) rather than pretending to pair.
func (m *AccountManager) AddPairingAccount(label string) (*Account, error) {
	if m.baseDir == "" {
		// No state dir means the demo build (CHATOT_FAKE=1): there is no
		// WhatsApp to pair with, so the new account is a Fake that only ever
		// emits demo QR codes. The dialog renders the same as for a real one.
		if _, ok := m.active().(*Fake); !ok {
			return nil, errors.New("chatot/client: adding accounts needs a real WhatsApp connection")
		}
		id := m.uniqueID(label)
		m.AddAccount(id, label, NewPairingFake())
		a := m.accountByID(id)
		if err := m.startAccount(context.Background(), a); err != nil {
			return nil, err
		}
		return a, nil
	}
	id := m.uniqueID(label)
	c, err := NewWhatsmeow(m.accountDir(id))
	if err != nil {
		return nil, fmt.Errorf("chatot/client: create account %q: %w", id, err)
	}
	m.AddAccount(id, label, c)

	a := m.accountByID(id)
	m.mu.Lock()
	ctx := m.baseCtx
	m.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}
	// A pairing account always starts so its QR/pair events flow, regardless of
	// the keep-inactive-connected setting.
	if err := m.startAccount(ctx, a); err != nil {
		return nil, fmt.Errorf("chatot/client: start account %q: %w", id, err)
	}

	// Best-effort: a failed roster write doesn't undo the live account (it just
	// won't survive a restart), so don't fail the add over it.
	_ = m.persistRoster()
	return a, nil
}

// RenameAccount changes the label shown for id in the switcher, rail and
// notifications, and persists it with the roster.
func (m *AccountManager) RenameAccount(id, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("chatot/client: account label cannot be empty")
	}
	m.mu.Lock()
	a := m.findLocked(id)
	if a == nil {
		m.mu.Unlock()
		return fmt.Errorf("chatot/client: no account %q", id)
	}
	a.Name = name
	m.mu.Unlock()
	m.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{}})
	return m.persistRoster()
}

// RemoveAccount drops id from the roster and disconnects its client. It refuses
// to remove the last remaining account; removing the active one first switches
// to another account. The on-disk state dir is left intact (a future "delete
// data" is a separate, destructive action).
func (m *AccountManager) RemoveAccount(id string) error {
	m.mu.Lock()
	if len(m.accounts) <= 1 {
		m.mu.Unlock()
		return errors.New("chatot/client: cannot remove the last account")
	}
	found := false
	var next string
	for _, a := range m.accounts {
		if a.ID == id {
			found = true
		} else if next == "" {
			next = a.ID
		}
	}
	active := id == m.activeID
	m.mu.Unlock()
	if !found {
		return fmt.Errorf("chatot/client: unknown account %q", id)
	}

	if active {
		if err := m.SetActive(next); err != nil {
			return err
		}
	}

	m.mu.Lock()
	var removed *Account
	remaining := make([]*Account, 0, len(m.accounts))
	for _, a := range m.accounts {
		if a.ID == id {
			removed = a
			continue
		}
		remaining = append(remaining, a)
	}
	m.accounts = remaining
	m.mu.Unlock()

	if removed != nil && removed.stop != nil {
		removed.stop()
	}
	return m.persistRoster()
}

// accountDir is the per-account state dir for a pairing account.
func (m *AccountManager) accountDir(id string) string {
	return filepath.Join(m.baseDir, "accounts", id)
}

// Find returns the registered account with id (or nil) for UI that needs its
// live QR/pair streams, e.g. the relink flow.
func (m *AccountManager) Find(id string) *Account { return m.accountByID(id) }

// accountByID returns the registered account with id, or nil.
func (m *AccountManager) accountByID(id string) *Account {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, a := range m.accounts {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// uniqueID slugifies label and disambiguates against the ids already in use
// (including "default") by appending -2, -3, ….
func (m *AccountManager) uniqueID(label string) string {
	base := slugify(label)
	m.mu.Lock()
	taken := make(map[string]bool, len(m.accounts))
	for _, a := range m.accounts {
		taken[a.ID] = true
	}
	m.mu.Unlock()
	id := base
	for n := 2; taken[id]; n++ {
		id = fmt.Sprintf("%s-%d", base, n)
	}
	return id
}

// persistRoster writes the current pairing accounts (everything but the
// implicit "default") to baseDir/accounts.json. No-op without a base dir.
func (m *AccountManager) persistRoster() error {
	if m.baseDir == "" {
		return nil
	}
	m.mu.Lock()
	var r roster
	for _, a := range m.accounts {
		if a.ID == defaultAccountID {
			continue
		}
		r.Accounts = append(r.Accounts, rosterEntry{ID: a.ID, Label: a.Name, Proxy: a.Proxy})
	}
	m.mu.Unlock()
	return saveRoster(filepath.Join(m.baseDir, rosterFile), r)
}

func (m *AccountManager) QRCodes() <-chan string { return m.qrCodes }

func (m *AccountManager) Events() <-chan Event { return m.events.Subscribe() }

func (m *AccountManager) LoggedIn() bool { return m.active().LoggedIn() }
func (m *AccountManager) Paired() bool   { return m.active().Paired() }

func (m *AccountManager) ContactName(jid string) string { return m.active().ContactName(jid) }

func (m *AccountManager) Logout(ctx context.Context) error { return m.active().Logout(ctx) }

func (m *AccountManager) PairPhone(ctx context.Context, phone string) (string, error) {
	return m.active().PairPhone(ctx, phone)
}

func (m *AccountManager) Chats(limit int) ([]Chat, error) { return m.active().Chats(limit) }

func (m *AccountManager) Messages(jid string, limit int) ([]Message, error) {
	return m.active().Messages(jid, limit)
}

func (m *AccountManager) MessagesBefore(jid, beforeMsgID string, limit int) ([]Message, error) {
	return m.active().MessagesBefore(jid, beforeMsgID, limit)
}

func (m *AccountManager) RequestMoreHistory(ctx context.Context, chatJID, oldestMsgID string, count int) error {
	return m.active().RequestMoreHistory(ctx, chatJID, oldestMsgID, count)
}

func (m *AccountManager) Search(query string, limit int) ([]SearchHit, error) {
	return m.active().Search(query, limit)
}

func (m *AccountManager) SearchInChat(chatJID, query string, limit int) ([]SearchHit, error) {
	return m.active().SearchInChat(chatJID, query, limit)
}

func (m *AccountManager) SendText(ctx context.Context, jid, text string, replyTo *MsgRef) (string, error) {
	return m.active().SendText(ctx, jid, text, replyTo)
}

func (m *AccountManager) SendMedia(ctx context.Context, jid string, a Attachment, replyTo *MsgRef) (string, error) {
	return m.active().SendMedia(ctx, jid, a, replyTo)
}

func (m *AccountManager) SendLocation(ctx context.Context, jid string, loc Location, replyTo *MsgRef) (string, error) {
	return m.active().SendLocation(ctx, jid, loc, replyTo)
}

func (m *AccountManager) SendLiveLocation(ctx context.Context, jid string, lat, lon float64, durationSecs int) (string, error) {
	return m.active().SendLiveLocation(ctx, jid, lat, lon, durationSecs)
}

func (m *AccountManager) SendContact(ctx context.Context, jid string, contact Contact, replyTo *MsgRef) (string, error) {
	return m.active().SendContact(ctx, jid, contact, replyTo)
}

func (m *AccountManager) ForwardMessage(ctx context.Context, msg Message, toJID string) (string, error) {
	return m.active().ForwardMessage(ctx, msg, toJID)
}

func (m *AccountManager) ClearChat(ctx context.Context, jid string, alsoMedia bool) error {
	return m.active().ClearChat(ctx, jid, alsoMedia)
}

func (m *AccountManager) SendVoice(ctx context.Context, jid string, oggOpus []byte, dur int) (string, error) {
	return m.active().SendVoice(ctx, jid, oggOpus, dur)
}

func (m *AccountManager) SendSticker(ctx context.Context, jid, path string) (string, error) {
	return m.active().SendSticker(ctx, jid, path)
}

func (m *AccountManager) Stickers() ([]Sticker, error)            { return m.active().Stickers() }
func (m *AccountManager) AddSticker(path string) (Sticker, error) { return m.active().AddSticker(path) }
func (m *AccountManager) RemoveSticker(ctx context.Context, key string) error {
	return m.active().RemoveSticker(ctx, key)
}

func (m *AccountManager) CreatePoll(ctx context.Context, jid, name string, options []string, selectable int) (string, error) {
	return m.active().CreatePoll(ctx, jid, name, options, selectable)
}

func (m *AccountManager) VotePoll(ctx context.Context, chatJID, pollMsgID string, options []string) error {
	return m.active().VotePoll(ctx, chatJID, pollMsgID, options)
}

func (m *AccountManager) EditMessage(ctx context.Context, chatJID, msgID, newText string) error {
	return m.active().EditMessage(ctx, chatJID, msgID, newText)
}

func (m *AccountManager) DeleteMessage(ctx context.Context, chatJID, msgID string) error {
	return m.active().DeleteMessage(ctx, chatJID, msgID)
}

func (m *AccountManager) DeleteMessageForMe(ctx context.Context, chatJID, msgID string) error {
	return m.active().DeleteMessageForMe(ctx, chatJID, msgID)
}

func (m *AccountManager) React(ctx context.Context, jid, msgID, emoji string) error {
	return m.active().React(ctx, jid, msgID, emoji)
}

func (m *AccountManager) MarkRead(ctx context.Context, jid string, msgIDs []string, notifySender bool) error {
	return m.active().MarkRead(ctx, jid, msgIDs, notifySender)
}

func (m *AccountManager) ClearUnread(jid string) error { return m.active().ClearUnread(jid) }

func (m *AccountManager) StopLiveLocation(ctx context.Context, chatJID, msgID string) error {
	return m.active().StopLiveLocation(ctx, chatJID, msgID)
}

func (m *AccountManager) CheckOnWhatsApp(ctx context.Context, phone string) (string, bool, error) {
	return m.active().CheckOnWhatsApp(ctx, phone)
}

func (m *AccountManager) SendPresence(available bool) error {
	return m.active().SendPresence(available)
}

func (m *AccountManager) SendTyping(jid string, typing bool) error {
	return m.active().SendTyping(jid, typing)
}

func (m *AccountManager) SendRecording(jid string, recording bool) error {
	return m.active().SendRecording(jid, recording)
}

func (m *AccountManager) PinChat(ctx context.Context, jid string, pin bool) error {
	return m.active().PinChat(ctx, jid, pin)
}

func (m *AccountManager) MuteChatFor(ctx context.Context, jid string, d time.Duration) error {
	return m.active().MuteChatFor(ctx, jid, d)
}

func (m *AccountManager) MuteChat(ctx context.Context, jid string, mute bool) error {
	return m.active().MuteChat(ctx, jid, mute)
}

func (m *AccountManager) ArchiveChat(ctx context.Context, jid string, archive bool) error {
	return m.active().ArchiveChat(ctx, jid, archive)
}

func (m *AccountManager) MarkChatUnread(ctx context.Context, jid string, unread bool) error {
	return m.active().MarkChatUnread(ctx, jid, unread)
}

func (m *AccountManager) StarMessage(ctx context.Context, chatJID, msgID string, starred bool) error {
	return m.active().StarMessage(ctx, chatJID, msgID, starred)
}

func (m *AccountManager) PinMessage(ctx context.Context, chatJID, msgID string, pin bool) error {
	return m.active().PinMessage(ctx, chatJID, msgID, pin)
}

func (m *AccountManager) StarredMessages(limit int) ([]Message, error) {
	return m.active().StarredMessages(limit)
}

func (m *AccountManager) Statuses(limit int) ([]Message, error) {
	return m.active().Statuses(limit)
}

func (m *AccountManager) ChatMedia(jid string) ([]MediaItem, error) {
	return m.active().ChatMedia(jid)
}

func (m *AccountManager) ChatDocs(jid string) ([]DocItem, error) {
	return m.active().ChatDocs(jid)
}

func (m *AccountManager) ChatLinks(jid string) ([]LinkItem, error) {
	return m.active().ChatLinks(jid)
}

func (m *AccountManager) PostStatus(ctx context.Context, text string) error {
	return m.active().PostStatus(ctx, text)
}

func (m *AccountManager) RejectCall(ctx context.Context, callJID, callID string) error {
	return m.active().RejectCall(ctx, callJID, callID)
}

func (m *AccountManager) Blocklist(ctx context.Context) ([]string, error) {
	return m.active().Blocklist(ctx)
}

func (m *AccountManager) SetBlocked(ctx context.Context, jid string, blocked bool) error {
	return m.active().SetBlocked(ctx, jid, blocked)
}

func (m *AccountManager) IsBlocked(jid string) bool { return m.active().IsBlocked(jid) }

func (m *AccountManager) PrivacySettings(ctx context.Context) (map[string]string, error) {
	return m.active().PrivacySettings(ctx)
}

func (m *AccountManager) Labels() ([]Label, error) { return m.active().Labels() }

func (m *AccountManager) CreateLabel(ctx context.Context, name string, color int) (string, error) {
	return m.active().CreateLabel(ctx, name, color)
}

func (m *AccountManager) EditLabel(ctx context.Context, id, name string, color int) error {
	return m.active().EditLabel(ctx, id, name, color)
}

func (m *AccountManager) DeleteLabel(ctx context.Context, id string) error {
	return m.active().DeleteLabel(ctx, id)
}

func (m *AccountManager) SetChatLabeled(ctx context.Context, labelID, chatJID string, labeled bool) error {
	return m.active().SetChatLabeled(ctx, labelID, chatJID, labeled)
}

func (m *AccountManager) LabelsForChat(chatJID string) ([]string, error) {
	return m.active().LabelsForChat(chatJID)
}

func (m *AccountManager) GroupInfo(ctx context.Context, jid string) (*GroupInfo, error) {
	return m.active().GroupInfo(ctx, jid)
}

func (m *AccountManager) OwnJID() string { return m.active().OwnJID() }

func (m *AccountManager) OwnName() string { return m.active().OwnName() }

// displayName is the label the UI shows for a: the user-given roster label
// when there is one, else the account's WhatsApp profile name, else its
// phone number, else a numbered placeholder.
func (a *Account) displayName(index int) string {
	if a.Name != "" {
		return a.Name
	}
	if n := a.c.OwnName(); n != "" {
		return n
	}
	if p := phoneFromJID(a.c.OwnJID()); p != "" {
		return p
	}
	return fmt.Sprintf("Account %d", index+1)
}

func (m *AccountManager) CreateGroup(ctx context.Context, name string, participantJIDs []string) (string, error) {
	return m.active().CreateGroup(ctx, name, participantJIDs)
}

func (m *AccountManager) LeaveGroup(ctx context.Context, jid string) error {
	return m.active().LeaveGroup(ctx, jid)
}

func (m *AccountManager) UpdateGroupParticipants(ctx context.Context, jid string, participantJIDs []string, action string) error {
	return m.active().UpdateGroupParticipants(ctx, jid, participantJIDs, action)
}

func (m *AccountManager) SetGroupName(ctx context.Context, jid, name string) error {
	return m.active().SetGroupName(ctx, jid, name)
}

func (m *AccountManager) SetGroupTopic(ctx context.Context, jid, topic string) error {
	return m.active().SetGroupTopic(ctx, jid, topic)
}

func (m *AccountManager) SetGroupAnnounce(ctx context.Context, jid string, announce bool) error {
	return m.active().SetGroupAnnounce(ctx, jid, announce)
}

func (m *AccountManager) SetGroupPhoto(ctx context.Context, jid string, jpeg []byte) error {
	return m.active().SetGroupPhoto(ctx, jid, jpeg)
}

func (m *AccountManager) SetGroupLocked(ctx context.Context, jid string, locked bool) error {
	return m.active().SetGroupLocked(ctx, jid, locked)
}

func (m *AccountManager) SetGroupDisappearingTimer(ctx context.Context, jid string, seconds int64) error {
	return m.active().SetGroupDisappearingTimer(ctx, jid, seconds)
}

func (m *AccountManager) GroupInviteLink(ctx context.Context, jid string, reset bool) (string, error) {
	return m.active().GroupInviteLink(ctx, jid, reset)
}

func (m *AccountManager) JoinGroupWithLink(ctx context.Context, code string) (string, error) {
	return m.active().JoinGroupWithLink(ctx, code)
}

func (m *AccountManager) CreateCommunity(ctx context.Context, name, description string) (string, error) {
	return m.active().CreateCommunity(ctx, name, description)
}

func (m *AccountManager) LinkGroupToCommunity(ctx context.Context, community, group string) error {
	return m.active().LinkGroupToCommunity(ctx, community, group)
}

func (m *AccountManager) GroupJoinRequests(ctx context.Context, jid string) ([]JoinRequest, error) {
	return m.active().GroupJoinRequests(ctx, jid)
}

func (m *AccountManager) ResolveGroupJoinRequest(ctx context.Context, groupJID, participantJID string, approve bool) error {
	return m.active().ResolveGroupJoinRequest(ctx, groupJID, participantJID, approve)
}

func (m *AccountManager) Newsletters(ctx context.Context) ([]Newsletter, error) {
	return m.active().Newsletters(ctx)
}

func (m *AccountManager) NewsletterMessages(ctx context.Context, jid string, count int) ([]NewsletterMessage, error) {
	return m.active().NewsletterMessages(ctx, jid, count)
}

func (m *AccountManager) FollowNewsletter(ctx context.Context, jid string) error {
	return m.active().FollowNewsletter(ctx, jid)
}

func (m *AccountManager) UnfollowNewsletter(ctx context.Context, jid string) error {
	return m.active().UnfollowNewsletter(ctx, jid)
}

func (m *AccountManager) NewsletterSetMuted(ctx context.Context, jid string, mute bool) error {
	return m.active().NewsletterSetMuted(ctx, jid, mute)
}

func (m *AccountManager) NewsletterReact(ctx context.Context, jid, messageID string, serverID int64, emoji string) error {
	return m.active().NewsletterReact(ctx, jid, messageID, serverID, emoji)
}

func (m *AccountManager) FollowNewsletterByLink(ctx context.Context, link string) (string, error) {
	return m.active().FollowNewsletterByLink(ctx, link)
}

func (m *AccountManager) DiscoverNewsletters(ctx context.Context, query string) ([]Newsletter, error) {
	return m.active().DiscoverNewsletters(ctx, query)
}

func (m *AccountManager) Communities(ctx context.Context) ([]Community, error) {
	return m.active().Communities(ctx)
}

func (m *AccountManager) JoinCommunityGroup(ctx context.Context, community, group string) error {
	return m.active().JoinCommunityGroup(ctx, community, group)
}

func (m *AccountManager) ReactToStatus(ctx context.Context, poster, msgID, emoji string) error {
	return m.active().ReactToStatus(ctx, poster, msgID, emoji)
}

func (m *AccountManager) DownloadMedia(ctx context.Context, msgID string) (string, error) {
	return m.active().DownloadMedia(ctx, msgID)
}

func (m *AccountManager) MarkViewOnceOpened(ctx context.Context, chatJID, msgID string) error {
	return m.active().MarkViewOnceOpened(ctx, chatJID, msgID)
}

func (m *AccountManager) Avatar(ctx context.Context, jid string) (string, error) {
	return m.active().Avatar(ctx, jid)
}

// MergedChat is one row of the merged "All accounts" list: a chat plus which
// account it belongs to, so the sidebar can stripe and label it.
type MergedChat struct {
	Chat
	AccountID   string
	AccountName string
}

// MergedChats returns every account's chats in one list, newest-first across
// accounts, for the sidebar's merged mode. An account whose Chats read fails
// is skipped rather than failing the whole list — one disconnected account
// shouldn't blank the others.
func (m *AccountManager) MergedChats(limit int) []MergedChat {
	m.mu.Lock()
	accounts := make([]*Account, len(m.accounts))
	copy(accounts, m.accounts)
	m.mu.Unlock()

	var out []MergedChat
	for i, a := range accounts {
		chats, err := a.c.Chats(limit)
		if err != nil {
			continue
		}
		for _, ch := range chats {
			out = append(out, MergedChat{Chat: ch, AccountID: a.ID, AccountName: a.displayName(i)})
		}
	}
	// Pinned chats first (matching each account's own ordering), then newest.
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Pinned != out[j].Pinned {
			return out[i].Pinned
		}
		return out[i].LastMessageTS > out[j].LastMessageTS
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// ClientFor returns the Client backing account id, nil when unknown.
//
// Every AccountManager method forwards to the ACTIVE account, which is right
// for the app's normal single-account-at-a-time view but wrong for the merged
// "all accounts" list: a row from an inactive account has to be looked up
// (avatar, blocked state, labels) against its own client, not the active one.
func (m *AccountManager) ClientFor(id string) Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	if a := m.findLocked(id); a != nil {
		return a.c
	}
	return nil
}

func (m *AccountManager) NewsletterMarkViewed(ctx context.Context, jid string, serverIDs []int64) error {
	return m.active().NewsletterMarkViewed(ctx, jid, serverIDs)
}

func (m *AccountManager) NewsletterSubscribeLive(ctx context.Context, jid string) error {
	return m.active().NewsletterSubscribeLive(ctx, jid)
}

func (m *AccountManager) MarkStatusViewed(ctx context.Context, poster string, msgIDs []string, notify bool) error {
	return m.active().MarkStatusViewed(ctx, poster, msgIDs, notify)
}

func (m *AccountManager) StatusViewers(msgID string) ([]StatusViewer, error) {
	return m.active().StatusViewers(msgID)
}

func (m *AccountManager) MuteStatus(ctx context.Context, poster string, mute bool) error {
	return m.active().MuteStatus(ctx, poster, mute)
}

func (m *AccountManager) MutedStatusPosters() ([]string, error) {
	return m.active().MutedStatusPosters()
}

func (m *AccountManager) HideStatusFrom(ctx context.Context, jid string) error {
	return m.active().HideStatusFrom(ctx, jid)
}

func (m *AccountManager) SetPrivacySetting(ctx context.Context, name, value string) error {
	return m.active().SetPrivacySetting(ctx, name, value)
}

// ActiveClient is the account currently driving the UI, as its own Client
// (dev hooks poke the fake behind it).
func (m *AccountManager) ActiveClient() Client { return m.active() }
