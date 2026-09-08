package ui

import (
	"log"

	"github.com/diamondburned/gotk4/pkg/glib/v2"

	"chatot/internal/client"
)

// A refresh reads the store first and touches widgets after. The reads
// (the chat list, every chat's labels, the blocked set, the status feed
// behind the tab badge, the rail's unread counts) cost 15-25 ms on a
// 550-chat store, and they ran on the main loop for every event a live
// account publishes: a dropped frame or three per receipt while
// scrolling, in both panes. queueRefresh now gathers them in a goroutine
// (loadSidebarData) and applies the result on the main loop
// (applySidebarData); refresh keeps the synchronous path for the callers
// that need the rows before they return (a toggle, a tab switch).

// sidebarParams is the main-loop state a load needs, captured before the
// goroutine starts so it never reads ChatList fields off the main loop.
type sidebarParams struct {
	merged bool
	source *client.AccountManager
	// statusBadge asks for the status feed; the badge is not shown while
	// the Status tab itself is open.
	statusBadge bool
}

// mergedRow is one row of merged mode with its own account's lookups done.
type mergedRow struct {
	mc      client.MergedChat
	client  client.Client
	labels  []string
	blocked bool
}

// sidebarData is what one refresh read from the store.
type sidebarData struct {
	err        error
	chats      []client.Chat
	labels     []client.Label
	chatLabels map[string][]string // jid -> label ids, for every chat
	blocked    map[string]bool
	// statusRecent is the count behind the Status tab's badge, -1 when it
	// was not read.
	statusRecent int
	// merged holds merged mode's rows; nil outside it.
	merged  []mergedRow
	railSig string
}

func (cl *ChatList) sidebarParams() sidebarParams {
	p := sidebarParams{merged: cl.merged, statusBadge: cl.tabBar != nil && cl.tab != "status"}
	if cl.merged {
		p.source = cl.mergedSource()
	}
	return p
}

// loadSidebarData reads everything a refresh needs. Safe off the main
// loop: it goes through the client only.
func (cl *ChatList) loadSidebarData(p sidebarParams) *sidebarData {
	d := &sidebarData{statusRecent: -1}
	d.chats, d.err = cl.c.Chats(0)
	if d.err != nil {
		return d
	}
	d.labels, _ = cl.c.Labels()
	d.chatLabels = make(map[string][]string, len(d.chats))
	d.blocked = make(map[string]bool)
	for _, c := range d.chats {
		if ids, err := cl.c.LabelsForChat(c.JID); err == nil {
			d.chatLabels[c.JID] = ids
		}
		if cl.c.IsBlocked(c.JID) {
			d.blocked[c.JID] = true
		}
	}
	if p.merged && p.source != nil {
		for _, mc := range p.source.MergedChats(0) {
			// Every per-row lookup MUST go through the row's own account:
			// the manager forwards to the ACTIVE one, which for another
			// account's chat would give the wrong avatar, no blocked
			// badge, and a label filter hiding every row not its own.
			rc := p.source.ClientFor(mc.AccountID)
			if rc == nil {
				continue
			}
			row := mergedRow{mc: mc, client: rc, blocked: rc.IsBlocked(mc.Chat.JID)}
			row.labels, _ = rc.LabelsForChat(mc.Chat.JID)
			d.merged = append(d.merged, row)
		}
	}
	if p.statusBadge {
		d.statusRecent = len(cl.loadStatusFeed().Recent)
	}
	d.railSig = cl.railSignature()
	return d
}

// applySidebarData rebuilds the sidebar from d. Must run on the GTK main
// loop.
func (cl *ChatList) applySidebarData(d *sidebarData) {
	if d.err != nil {
		log.Printf("chatot: list chats: %v", d.err)
		return
	}
	cl.updateChipRow(d)
	switch {
	case cl.tab == "communities":
		cl.refreshCommunities()
	case cl.tab == "channels" && cl.discover:
		cl.refreshDiscover()
	case cl.tab == "channels":
		cl.refreshChannels()
	case cl.tab == "status":
		cl.refreshStatus()
	case cl.query != "":
		cl.refreshSearch()
	default:
		cl.refreshChats(d)
	}
	cl.updateTabBadges(d)
	if d.railSig != cl.railSig {
		cl.railSig = d.railSig
		cl.refreshAccountRail()
	}
}

// refresh rebuilds the sidebar from the store at once. Must run on the
// GTK main loop; the store reads run there too, so the event-driven path
// goes through queueRefresh instead.
func (cl *ChatList) refresh() {
	// A result still out from before this belongs to the state it was
	// asked in (another account, merged or not) and is dropped.
	cl.refreshGen++
	cl.applySidebarData(cl.loadSidebarData(cl.sidebarParams()))
}

// refreshAsync reads the store off the main loop and applies the result
// when it lands; one read runs at a time, and a request made while one is
// out is served by one more read after it. Must run on the GTK main loop.
func (cl *ChatList) refreshAsync() {
	if cl.refreshInFlight {
		cl.refreshDirty = true
		return
	}
	cl.refreshInFlight = true
	p := cl.sidebarParams()
	gen := cl.refreshGen
	go func() {
		d := cl.loadSidebarData(p)
		glib.IdleAdd(func() {
			cl.refreshInFlight = false
			if gen == cl.refreshGen {
				cl.applySidebarData(d)
			}
			if cl.refreshDirty {
				cl.refreshDirty = false
				cl.refreshAsync()
			}
		})
	}()
}
