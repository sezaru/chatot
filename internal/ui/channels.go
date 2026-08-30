package ui

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// newsletterRowView holds the pure display fields for one channel row in the
// sidebar. newsletterRowVM computes it so it can be unit-tested without a
// display.
type newsletterRowView struct {
	Name    string
	Snippet string
	Muted   bool
	Initial string
}

// newsletterRowVM derives the sidebar view-model for one channel: its name,
// a description snippet, whether it's muted, and an avatar initial.
func newsletterRowVM(n client.Newsletter) newsletterRowView {
	name := n.Name
	if name == "" {
		name = "Unknown channel"
	}
	initial := "?"
	for _, r := range name {
		initial = strings.ToUpper(string(r))
		break
	}
	return newsletterRowView{
		Name:    name,
		Snippet: strings.TrimSpace(n.Description),
		Muted:   n.Muted,
		Initial: initial,
	}
}

// newsletterPostView holds the pure display fields for one channel post.
type newsletterPostView struct {
	Text      string
	TimeText  string
	Views     string
	Reactions string
}

// newsletterPostVM derives the read-view model for a single channel post:
// its text (a placeholder when empty), a formatted time, a view count and a
// top-reactions summary. now is injected for deterministic time formatting.
func newsletterPostVM(m client.NewsletterMessage, now time.Time) newsletterPostView {
	text := m.Text
	if strings.TrimSpace(text) == "" {
		text = "(no text)"
	}
	return newsletterPostView{
		Text:      text,
		TimeText:  formatChatTime(m.TS, now),
		Views:     fmt.Sprintf("%d views", m.Views),
		Reactions: reactionSummary(m.Reactions),
	}
}

// reactionSummary renders up to the three most-reacted emoji as "emoji count"
// pairs, sorted by count (desc) then emoji (asc) for deterministic output.
func reactionSummary(counts map[string]int) string {
	if len(counts) == 0 {
		return ""
	}
	type rc struct {
		emoji string
		n     int
	}
	items := make([]rc, 0, len(counts))
	for e, n := range counts {
		items = append(items, rc{e, n})
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].n != items[j].n {
			return items[i].n > items[j].n
		}
		return items[i].emoji < items[j].emoji
	})
	parts := make([]string, 0, len(items))
	for i, it := range items {
		if i >= 3 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", it.emoji, it.n))
	}
	return strings.Join(parts, "   ")
}

// refreshChannels rebuilds the sidebar list from Newsletters. Clicking a row
// opens the channel read dialog (branched on cl.showChannels in the
// row-activated handler). Must run on the GTK main loop.
func (cl *ChatList) refreshChannels() {
	channels, err := cl.c.Newsletters(context.Background())
	if err != nil {
		channels = nil
	}

	cl.list.RemoveAll()
	cl.newsletters = channels

	if len(channels) == 0 {
		cl.rowJIDs = nil
		empty := gtk.NewLabel("No channels")
		empty.AddCSSClass("chatot-search-empty")
		cl.list.Append(empty)
		return
	}

	cl.rowJIDs = make([]string, 0, len(channels))
	for _, n := range channels {
		cl.list.Append(buildNewsletterRow(newsletterRowVM(n)))
		cl.rowJIDs = append(cl.rowJIDs, n.ID)
	}
}

// buildNewsletterRow constructs the widget tree for one channel sidebar row.
func buildNewsletterRow(vm newsletterRowView) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.SetMarginTop(6)
	row.SetMarginBottom(6)
	row.SetMarginStart(8)
	row.SetMarginEnd(8)

	avatar := gtk.NewLabel(vm.Initial)
	avatar.AddCSSClass("chatot-avatar")
	avatar.SetSizeRequest(36, 36)
	row.Append(avatar)

	textCol := gtk.NewBox(gtk.OrientationVertical, 2)
	textCol.SetHExpand(true)

	name := vm.Name
	if vm.Muted {
		name = "🔇 " + name
	}
	nameLabel := gtk.NewLabel(name)
	nameLabel.SetXAlign(0)
	nameLabel.SetEllipsize(pango.EllipsizeEnd)
	nameLabel.SetMaxWidthChars(1)
	nameLabel.SetHExpand(true)
	nameLabel.AddCSSClass("chatot-chat-name")
	textCol.Append(nameLabel)

	if vm.Snippet != "" {
		snippet := gtk.NewLabel(vm.Snippet)
		snippet.SetXAlign(0)
		snippet.SetEllipsize(pango.EllipsizeEnd)
		snippet.SetMaxWidthChars(1)
		snippet.SetHExpand(true)
		snippet.AddCSSClass("chatot-chat-preview")
		textCol.Append(snippet)
	}

	row.Append(textCol)
	return row
}

// openChannel finds the selected channel by JID and opens its read dialog.
func (cl *ChatList) openChannel(jid string) {
	var n client.Newsletter
	for _, c := range cl.newsletters {
		if c.ID == jid {
			n = c
			break
		}
	}
	if n.ID == "" {
		n = client.Newsletter{ID: jid, Name: jid}
	}
	cl.showChannelDialog(n)
}

// showChannelDialog opens a read view of a channel's recent posts plus its
// mute/unfollow actions. Posts are loaded off the main loop.
func (cl *ChatList) showChannelDialog(n client.Newsletter) {
	dialog := gtk.NewWindow()
	dialog.SetTitle(n.Name)
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(420, 520)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	header := gtk.NewLabel(n.Name)
	header.SetXAlign(0)
	header.AddCSSClass("chatot-chat-name")
	box.Append(header)

	if strings.TrimSpace(n.Description) != "" {
		desc := gtk.NewLabel(n.Description)
		desc.SetXAlign(0)
		desc.SetWrap(true)
		desc.AddCSSClass("chatot-chat-preview")
		box.Append(desc)
	}

	actions := gtk.NewBox(gtk.OrientationHorizontal, 6)
	muteBtn := gtk.NewButtonWithLabel(muteActionLabel(n.Muted))
	muteBtn.AddCSSClass("flat")
	muteBtn.ConnectClicked(func() {
		mute := !n.Muted
		muteBtn.SetSensitive(false)
		go func() {
			err := cl.c.NewsletterSetMuted(context.Background(), n.ID, mute)
			glib.IdleAdd(func() {
				muteBtn.SetSensitive(true)
				if err == nil {
					n.Muted = mute
					muteBtn.SetLabel(muteActionLabel(n.Muted))
				}
			})
		}()
	})
	actions.Append(muteBtn)

	unfollowBtn := gtk.NewButtonWithLabel("Unfollow")
	unfollowBtn.AddCSSClass("destructive-action")
	unfollowBtn.ConnectClicked(func() {
		unfollowBtn.SetSensitive(false)
		go func() {
			err := cl.c.UnfollowNewsletter(context.Background(), n.ID)
			glib.IdleAdd(func() {
				if err != nil {
					unfollowBtn.SetSensitive(true)
					return
				}
				dialog.Close()
				cl.refresh()
			})
		}()
	})
	actions.Append(unfollowBtn)
	box.Append(actions)

	postsBox := gtk.NewBox(gtk.OrientationVertical, 8)
	loading := gtk.NewLabel("Loading posts…")
	loading.SetXAlign(0)
	postsBox.Append(loading)

	scroll := gtk.NewScrolledWindow()
	scroll.SetVExpand(true)
	scroll.SetChild(postsBox)
	box.Append(scroll)

	go func() {
		posts, err := cl.c.NewsletterMessages(context.Background(), n.ID, 20)
		glib.IdleAdd(func() {
			postsBox.Remove(loading)
			if err != nil {
				postsBox.Append(gtk.NewLabel("Couldn't load posts"))
				return
			}
			if len(posts) == 0 {
				postsBox.Append(gtk.NewLabel("No posts yet"))
				return
			}
			now := time.Now()
			for _, p := range posts {
				postsBox.Append(cl.buildChannelPost(n.ID, p, newsletterPostVM(p, now)))
			}
		})
	}()

	dialog.SetChild(box)
	dialog.Present()
}

// buildChannelPost renders one channel post with its text, meta line and a
// 👍 react button (NewsletterReact off the main loop).
func (cl *ChatList) buildChannelPost(channelJID string, m client.NewsletterMessage, vm newsletterPostView) *gtk.Box {
	post := gtk.NewBox(gtk.OrientationVertical, 2)
	post.AddCSSClass("chatot-channel-post")

	text := gtk.NewLabel(vm.Text)
	text.SetXAlign(0)
	text.SetWrap(true)
	post.Append(text)

	metaRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	meta := vm.TimeText + " · " + vm.Views
	if vm.Reactions != "" {
		meta += " · " + vm.Reactions
	}
	metaLabel := gtk.NewLabel(meta)
	metaLabel.SetXAlign(0)
	metaLabel.SetHExpand(true)
	metaLabel.AddCSSClass("chatot-chat-time")
	metaRow.Append(metaLabel)

	reactBtn := gtk.NewButtonWithLabel("👍")
	reactBtn.AddCSSClass("flat")
	reactBtn.ConnectClicked(func() {
		go func() {
			_ = cl.c.NewsletterReact(context.Background(), channelJID, m.ID, m.ServerID, "👍")
		}()
	})
	metaRow.Append(reactBtn)

	post.Append(metaRow)
	return post
}

// muteActionLabel is the pure label for a channel's mute toggle, reflecting
// its current muted state.
func muteActionLabel(muted bool) string {
	if muted {
		return "Unmute"
	}
	return "Mute"
}

// showFollowChannelDialog opens a modal to follow a channel by pasting its
// link, then opens the followed channel's read view.
func (cl *ChatList) showFollowChannelDialog() {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Follow channel")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	entry := gtk.NewEntry()
	entry.SetPlaceholderText("whatsapp.com/channel/… or invite key")
	box.Append(entry)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	box.Append(status)

	followBtn := gtk.NewButtonWithLabel("Follow")
	followBtn.AddCSSClass("suggested-action")
	box.Append(followBtn)

	follow := func() {
		link := strings.TrimSpace(entry.Text())
		if link == "" {
			status.SetText("Paste a channel link or key")
			return
		}
		followBtn.SetSensitive(false)
		status.SetText("Following…")
		go func() {
			jid, err := cl.c.FollowNewsletterByLink(context.Background(), link)
			glib.IdleAdd(func() {
				followBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't follow, check the link")
					return
				}
				dialog.Close()
				cl.refresh()
				cl.openChannel(jid)
			})
		}()
	}
	followBtn.ConnectClicked(follow)
	entry.ConnectActivate(follow)

	dialog.SetChild(box)
	dialog.SetDefaultWidget(followBtn)
	dialog.Present()
}
