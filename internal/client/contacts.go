package client

import (
	"context"
	"strings"
	"sync"
	"time"

	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"

	"chatot/internal/store"
)

// namesRefreshDelay coalesces the flood of per-contact app-state events a
// full sync produces into one chat-list refresh.
const namesRefreshDelay = 400 * time.Millisecond

// contactNames is the memo-free part of Whatsmeow that turns whatsmeow's
// contact knowledge (its own sqlite contact store, push names on inbound
// messages, LID↔phone mappings) into chatot's contacts table, which is what
// the chat list, search and the status feed resolve display names from.
type contactNames struct {
	mu    sync.Mutex
	timer *time.Timer
	// syncMu serialises syncContacts: connect, history sync and app-state
	// completion can all ask for one within the same second.
	syncMu sync.Mutex
	// asked is every chat resolveBusinessNames already queried the server
	// about; a business with no verified name is not asked again.
	asked map[string]bool
}

// OwnName returns this account's WhatsApp profile name (the push name other
// people see), "" before pairing.
func (w *Whatsmeow) OwnName() string {
	if w.wa == nil || w.wa.Store == nil {
		return ""
	}
	return w.wa.Store.PushName
}

// syncContacts mirrors whatsmeow's contact store into chatot's contacts
// table. Each contact is written under the JID whatsmeow keyed it by and,
// when the LID store knows the other address, under that too (with PNJID
// pointing at the phone-number JID), so a chat addressed either way resolves
// to the same name. Runs off the dispatch goroutine; ends with one chat-list
// refresh.
func (w *Whatsmeow) syncContacts(ctx context.Context) {
	if w.store == nil || w.wa == nil {
		return
	}
	w.names.syncMu.Lock()
	defer w.names.syncMu.Unlock()
	all, err := w.wa.Store.Contacts.GetAllContacts(ctx)
	if err != nil {
		w.log.Warnf("chatot/client: sync contacts: %v", err)
		return
	}
	var pns []types.JID
	for jid := range all {
		if jid.Server == types.DefaultUserServer {
			pns = append(pns, jid)
		}
	}
	lidFor := map[types.JID]types.JID{}
	if len(pns) > 0 {
		if m, err := w.wa.Store.LIDs.GetManyLIDsForPNs(ctx, pns); err == nil {
			lidFor = m
		} else {
			w.log.Warnf("chatot/client: sync contacts: lid lookup: %v", err)
		}
	}
	for jid, info := range all {
		row := contactRowFrom(jid, info)
		switch jid.Server {
		case types.HiddenUserServer:
			if pn, err := w.wa.Store.LIDs.GetPNForLID(ctx, jid); err == nil && !pn.IsEmpty() {
				row.PNJID = pn.ToNonAD().String()
				twin := row
				twin.JID, twin.PNJID = pn.ToNonAD().String(), ""
				w.upsertContact(twin)
			}
		case types.DefaultUserServer:
			if lid, ok := lidFor[jid]; ok && !lid.IsEmpty() {
				twin := row
				twin.JID, twin.PNJID = lid.ToNonAD().String(), jid.String()
				w.upsertContact(twin)
			}
		}
		w.upsertContact(row)
	}
	w.scheduleNamesRefresh()
	w.resolveBusinessNames(ctx)
}

// verifiedNameBatch is how many users one usync query asks about.
const verifiedNameBatch = 50

// verifiedNameTimeout bounds one such query.
const verifiedNameTimeout = 30 * time.Second

// resolveBusinessNames asks the server for the verified business name of
// every 1:1 chat the list can only show as a number. A business that has
// not messaged this device since the link has no push name here, its
// history-synced messages carry none, and it is in nobody's address book,
// so the contact sync has nothing for it either; the phone shows the name
// on its business profile, which usync returns. Each chat is asked about
// once per process. Runs off the dispatch goroutine.
func (w *Whatsmeow) resolveBusinessNames(ctx context.Context) {
	if w.store == nil || w.wa == nil || !w.wa.IsConnected() {
		return
	}
	chats, err := w.store.UnnamedChats()
	if err != nil {
		w.log.Warnf("chatot/client: unnamed chats: %v", err)
		return
	}
	// The query goes to the phone-number JID where one is known: a LID
	// alone may not resolve, and the name is filed under both.
	targets := map[types.JID][]string{}
	var jids []types.JID
	w.names.mu.Lock()
	if w.names.asked == nil {
		w.names.asked = map[string]bool{}
	}
	for _, chat := range chats {
		if w.names.asked[chat] {
			continue
		}
		w.names.asked[chat] = true
		q := chat
		if strings.HasSuffix(chat, "@lid") {
			if pn, err := w.store.ContactPNJID(chat); err == nil && pn != "" {
				q = pn
			}
		}
		jid, err := types.ParseJID(q)
		if err != nil {
			continue
		}
		if _, seen := targets[jid]; !seen {
			jids = append(jids, jid)
		}
		targets[jid] = append(targets[jid], chat)
	}
	w.names.mu.Unlock()
	found := 0
	for start := 0; start < len(jids); start += verifiedNameBatch {
		end := min(start+verifiedNameBatch, len(jids))
		qctx, cancel := context.WithTimeout(ctx, verifiedNameTimeout)
		infos, err := w.wa.GetUserInfo(qctx, jids[start:end])
		cancel()
		if err != nil {
			w.log.Warnf("chatot/client: verified names: %v", err)
			return
		}
		for jid, info := range infos {
			name := verifiedNameOf(info.VerifiedName)
			if name == "" {
				continue
			}
			for _, chat := range targets[jid.ToNonAD()] {
				w.upsertContact(store.ContactRow{JID: chat, BusinessName: name})
			}
			w.upsertContact(store.ContactRow{JID: jid.ToNonAD().String(), BusinessName: name})
			found++
		}
	}
	if found > 0 {
		w.log.Infof("chatot/client: verified names: %d of %d unnamed chats are businesses", found, len(jids))
		w.scheduleNamesRefresh()
	}
}

// verifiedNameOf is the business name on a usync verified-name
// certificate, "" for a personal account.
func verifiedNameOf(v *types.VerifiedName) string {
	if v == nil || v.Details == nil {
		return ""
	}
	return strings.TrimSpace(v.Details.GetVerifiedName())
}

func contactRowFrom(jid types.JID, info types.ContactInfo) store.ContactRow {
	return store.ContactRow{
		JID:          jid.ToNonAD().String(),
		BusinessName: info.BusinessName,
		FullName:     info.FullName,
		PushName:     info.PushName,
	}
}

func (w *Whatsmeow) upsertContact(row store.ContactRow) {
	if w.store == nil || row.JID == "" {
		return
	}
	if row.BusinessName == "" && row.FullName == "" && row.PushName == "" && row.SystemName == "" && row.PNJID == "" {
		return
	}
	if err := w.store.UpsertContact(row); err != nil {
		w.log.Warnf("chatot/client: upsert contact %s: %v", row.JID, err)
	}
	if row.PNJID != "" {
		w.mergeChatInto(row.JID, row.PNJID)
	}
}

// learnFromMessage records what an inbound message reveals about its
// sender: the push name it carries, and (for a LID-addressed chat) the
// phone number behind the LID, so the chat resolves to "+number" at worst.
func (w *Whatsmeow) learnFromMessage(info *types.MessageInfo) {
	if info == nil {
		return
	}
	if !info.IsFromMe && info.PushName != "" && !info.Sender.IsEmpty() && info.Sender.Server != types.NewsletterServer {
		w.upsertContact(store.ContactRow{JID: info.Sender.ToNonAD().String(), PushName: info.PushName})
		if !info.SenderAlt.IsEmpty() {
			w.upsertContact(store.ContactRow{JID: info.SenderAlt.ToNonAD().String(), PushName: info.PushName})
		}
	}
	alt := info.SenderAlt
	if info.IsFromMe {
		alt = info.RecipientAlt
	}
	if alt.IsEmpty() {
		return
	}
	switch {
	case info.Chat.Server == types.HiddenUserServer && alt.Server == types.DefaultUserServer:
		w.upsertContact(store.ContactRow{JID: info.Chat.ToNonAD().String(), PNJID: alt.ToNonAD().String()})
	case info.Chat.Server == types.DefaultUserServer && alt.Server == types.HiddenUserServer:
		// A number-addressed message names the LID beside it; recording
		// the pair folds any chat opened under that LID into this one.
		w.upsertContact(store.ContactRow{JID: alt.ToNonAD().String(), PNJID: info.Chat.ToNonAD().String()})
	}
}

// handleContactEvent applies the name-bearing app-state / push-name events
// and reports whether evt was one of them.
func (w *Whatsmeow) handleContactEvent(evt interface{}) bool {
	switch v := evt.(type) {
	case *events.PushName:
		w.upsertContact(store.ContactRow{JID: v.JID.ToNonAD().String(), PushName: v.NewPushName})
		if !v.JIDAlt.IsEmpty() {
			w.upsertContact(store.ContactRow{JID: v.JIDAlt.ToNonAD().String(), PushName: v.NewPushName})
		}
	case *events.BusinessName:
		w.upsertContact(store.ContactRow{JID: v.JID.ToNonAD().String(), BusinessName: v.NewBusinessName})
	case *events.Contact:
		w.upsertContact(store.ContactRow{JID: v.JID.ToNonAD().String(), FullName: v.Action.GetFullName()})
	case *events.AppStateSyncComplete:
		// The contact list lives in app state; once a patch set is in,
		// mirror whatsmeow's store (names for people we never messaged).
		go w.syncContacts(context.Background())
		return true
	default:
		return false
	}
	w.scheduleNamesRefresh()
	return true
}

// scheduleNamesRefresh pushes one EventChatUpdate after a short quiet
// period, however many contact rows changed in the meantime.
func (w *Whatsmeow) scheduleNamesRefresh() {
	w.names.mu.Lock()
	defer w.names.mu.Unlock()
	if w.names.timer != nil {
		w.names.timer.Stop()
	}
	w.names.timer = time.AfterFunc(namesRefreshDelay, func() {
		w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{}})
	})
}
