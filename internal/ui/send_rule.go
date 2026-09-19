package ui

import (
	"context"
	"strings"

	"github.com/diamondburned/gotk4/pkg/glib/v2"

	"chatot/internal/client"
)

// A chat can refuse our messages, and the one chatot meets in practice is
// the announcement group: a community's default group, and any group
// switched to "Only admins can send", take messages from their admins
// alone. The server rejects everyone else's, so a composer that still
// offers an entry only lets the reader type into a bubble that will come
// back failed. Nothing in the local store records announce mode, so the
// composer asks the group for it in the background when a chat opens —
// the way the thread's join-request banner does (conversation.go) — and
// remembers the answer per chat so a re-open locks at once.

// sendBlockLine is the line the composer shows in place of its entry for
// info, or "" when the group takes our messages. A group nobody could read
// (nil info) is left open: refusing to compose on a failed lookup would
// lock chats that are perfectly writable.
func sendBlockLine(info *client.GroupInfo, ownJID string) string {
	if info == nil || !info.Announce || isSelfAdmin(*info, ownJID) {
		return ""
	}
	return "Only admins can send messages to this group"
}

// refreshSendRule settles whether jid takes our messages: the last answer
// for that chat at once, then a fresh read of the group behind it. Must run
// on the GTK main loop; the read itself does not.
func (c *Composer) refreshSendRule(jid string) {
	c.blockReq = jid
	c.applySendBlock(c.blockMemo[jid])
	if !strings.HasSuffix(jid, "@g.us") {
		return
	}
	// One read at a time, as the thread's in-place refresh does: a chat
	// update lands for all sorts of reasons (a contact sync alone fires a
	// run of them), and whatever asked while a read was out is served by
	// one more read after it.
	if c.blockFetching {
		c.blockDirty = true
		return
	}
	c.blockFetching = true
	cl, own := c.c, c.c.OwnJID()
	go func() {
		info, err := cl.GroupInfo(context.Background(), jid)
		glib.IdleAdd(func() {
			c.blockFetching = false
			// A group that could not be read (offline, a fetch that
			// failed) leaves the last answer standing: unlocking a chat
			// that refused the reader's last message is the worse guess.
			if err == nil && c.blockReq == jid && c.state.jid == jid {
				line := sendBlockLine(info, own)
				c.blockMemo[jid] = line
				c.applySendBlock(line)
			}
			if c.blockDirty {
				c.blockDirty = false
				c.refreshSendRule(c.state.jid)
			}
		})
	}()
}

// RefreshSendRule re-reads the open chat's rule, for an event that may have
// changed it (a group's announce switch, ours or someone else's). A chat
// other than the one open is ignored. Must run on the GTK main loop.
func (c *Composer) RefreshSendRule(jid string) {
	if jid == "" || jid != c.state.jid {
		return
	}
	c.refreshSendRule(jid)
}

// applySendBlock swaps the strip's entry for line (or back), leaving the
// draft where it is: the chat may well unlock again, and text the reader
// typed is theirs. A reply or an edit in progress does go — its bar sits
// above the strip and would otherwise point at a message nothing here can
// answer.
func (c *Composer) applySendBlock(line string) {
	if line == c.sendBlock {
		return
	}
	c.sendBlock = line
	c.noticeLabel.SetLabel(line)
	if line != "" {
		c.state.CancelReply()
		c.state.CancelEdit()
		c.mentions.Hide()
		c.refreshQuoteBar()
		c.refreshEditBar()
	}
	c.syncStripRows()
}

// sendRefused reports whether the open chat takes no messages from us. The
// strip already hides every way of composing one, so this is the guard on
// the paths that do not go through it: a drop or a paste onto the thread,
// a dialog that sends by itself, a Retry.
func (c *Composer) sendRefused() bool { return c.sendBlock != "" }
