package ui

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// participantBadge returns the role label to show next to a group
// participant: the group's owner outranks admin/super-admin flags.
func participantBadge(p client.GroupParticipant, ownerJID string) string {
	switch {
	case ownerJID != "" && p.JID == ownerJID:
		return "Owner"
	case p.IsSuperAdmin, p.IsAdmin:
		return "Admin"
	default:
		return ""
	}
}

// orderParticipants sorts a group's participants for display: owner first,
// then admins, then everyone else, ties broken by JID.
func orderParticipants(parts []client.GroupParticipant, ownerJID string) []client.GroupParticipant {
	out := make([]client.GroupParticipant, len(parts))
	copy(out, parts)
	rank := func(p client.GroupParticipant) int {
		switch {
		case ownerJID != "" && p.JID == ownerJID:
			return 0
		case p.IsAdmin, p.IsSuperAdmin:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		ri, rj := rank(out[i]), rank(out[j])
		if ri != rj {
			return ri < rj
		}
		return out[i].JID < out[j].JID
	})
	return out
}

// isSelfAdmin reports whether ownJID is the group owner or an admin, gating
// the participant-mutation controls.
func isSelfAdmin(info client.GroupInfo, ownJID string) bool {
	if ownJID == "" {
		return false
	}
	if info.OwnerJID == ownJID {
		return true
	}
	for _, p := range info.Participants {
		if p.JID == ownJID {
			return p.IsAdmin || p.IsSuperAdmin
		}
	}
	return false
}

// promoteDemoteLabel is the pure label for a participant's role-toggle
// button and the action string it maps to.
func promoteDemoteLabel(p client.GroupParticipant) (label, action string) {
	if p.IsAdmin || p.IsSuperAdmin {
		return "Demote", "demote"
	}
	return "Promote", "promote"
}

// normalizeParticipant turns a single entered token into a user JID: an
// explicit "user@server" is kept, otherwise the token's digits become a
// "<digits>@s.whatsapp.net" JID. Returns "" for a token with no usable value.
func normalizeParticipant(tok string) string {
	s := strings.TrimSpace(tok)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "@") {
		return s
	}
	digits := strings.Map(func(r rune) rune {
		if unicode.IsDigit(r) {
			return r
		}
		return -1
	}, s)
	if digits == "" {
		return ""
	}
	return digits + "@s.whatsapp.net"
}

// parseParticipantList splits a comma-separated entry of numbers/JIDs into
// normalized user JIDs, dropping empties.
func parseParticipantList(input string) []string {
	var out []string
	for _, tok := range strings.Split(input, ",") {
		if j := normalizeParticipant(tok); j != "" {
			out = append(out, j)
		}
	}
	return out
}

// showGroupInfoDialog opens an actionable modal for jid: name/topic editing,
// announce/locked toggles, per-participant admin actions, add-participant,
// invite link and leave — the admin-only controls gated by c.OwnJID(). The
// content is rebuilt from a fresh c.GroupInfo after every mutation.
func showGroupInfoDialog(parent *gtk.Window, c client.Client, jid string) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Group info")
	dialog.SetDefaultSize(380, 520)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	content := gtk.NewBox(gtk.OrientationVertical, 6)
	content.SetMarginTop(12)
	content.SetMarginBottom(12)
	content.SetMarginStart(12)
	content.SetMarginEnd(12)
	scroller.SetChild(content)
	dialog.SetChild(scroller)

	var reload func()
	reload = func() {
		clearBox(content)
		content.Append(gtk.NewLabel("Loading…"))
		go func() {
			info, err := c.GroupInfo(context.Background(), jid)
			glib.IdleAdd(func() {
				clearBox(content)
				if err != nil || info == nil {
					content.Append(gtk.NewLabel("Couldn't load group info"))
					return
				}
				populateGroupInfo(content, dialog, c, *info, reload)
			})
		}()
	}
	reload()
	dialog.Present()
}

// clearBox removes every child from box.
func clearBox(box *gtk.Box) {
	for child := box.FirstChild(); child != nil; child = box.FirstChild() {
		box.Remove(child)
	}
}

// runGroupAction runs a mutating group action off the main loop and, on
// success, reloads the dialog; failures are logged and surfaced on status.
func runGroupAction(status *gtk.Label, reload func(), label string, do func(ctx context.Context) error) {
	status.SetText("Working…")
	go func() {
		err := do(context.Background())
		glib.IdleAdd(func() {
			if err != nil {
				log.Printf("chatot: group action %q failed: %v", label, err)
				status.SetText("Action failed, try again")
				return
			}
			reload()
		})
	}()
}

// populateGroupInfo fills box with the actionable group panel. admin gates the
// mutation controls; reload rebuilds the panel after a successful action.
func populateGroupInfo(box *gtk.Box, dialog *gtk.Window, c client.Client, info client.GroupInfo, reload func()) {
	admin := isSelfAdmin(info, c.OwnJID())

	status := gtk.NewLabel("")
	status.SetXAlign(0)

	// Name editor.
	nameEntry := gtk.NewEntry()
	nameEntry.SetText(info.Name)
	nameEntry.SetHExpand(true)
	nameSave := gtk.NewButtonWithLabel("Save name")
	nameSave.ConnectClicked(func() {
		runGroupAction(status, reload, "set name", func(ctx context.Context) error {
			return c.SetGroupName(ctx, info.JID, nameEntry.Text())
		})
	})
	box.Append(labeledRow("Name", nameEntry, nameSave))

	// Topic editor.
	topicEntry := gtk.NewEntry()
	topicEntry.SetText(info.Topic)
	topicEntry.SetHExpand(true)
	topicSave := gtk.NewButtonWithLabel("Save topic")
	topicSave.ConnectClicked(func() {
		runGroupAction(status, reload, "set topic", func(ctx context.Context) error {
			return c.SetGroupTopic(ctx, info.JID, topicEntry.Text())
		})
	})
	box.Append(labeledRow("Topic", topicEntry, topicSave))

	if admin {
		announce := gtk.NewCheckButtonWithLabel("Only admins can send")
		announce.SetActive(info.Announce)
		announce.ConnectToggled(func() {
			runGroupAction(status, reload, "set announce", func(ctx context.Context) error {
				return c.SetGroupAnnounce(ctx, info.JID, announce.Active())
			})
		})
		box.Append(announce)

		locked := gtk.NewCheckButtonWithLabel("Only admins can edit info")
		locked.SetActive(info.Locked)
		locked.ConnectToggled(func() {
			runGroupAction(status, reload, "set locked", func(ctx context.Context) error {
				return c.SetGroupLocked(ctx, info.JID, locked.Active())
			})
		})
		box.Append(locked)
	}

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	count := gtk.NewLabel(participantsCountLabel(len(info.Participants)))
	count.SetXAlign(0)
	box.Append(count)

	for _, p := range orderParticipants(info.Participants, info.OwnerJID) {
		box.Append(participantRow(c, info, p, admin, status, reload))
	}

	if admin {
		addEntry := gtk.NewEntry()
		addEntry.SetPlaceholderText("Phone or JID to add")
		addEntry.SetHExpand(true)
		addBtn := gtk.NewButtonWithLabel("Add")
		addBtn.ConnectClicked(func() {
			jids := parseParticipantList(addEntry.Text())
			if len(jids) == 0 {
				status.SetText("Enter a phone number or JID")
				return
			}
			runGroupAction(status, reload, "add participant", func(ctx context.Context) error {
				return c.UpdateGroupParticipants(ctx, info.JID, jids, "add")
			})
		})
		box.Append(labeledRow("Add", addEntry, addBtn))
	}

	box.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	inviteBtn := gtk.NewButtonWithLabel("Invite link")
	inviteBtn.ConnectClicked(func() {
		showInviteLinkDialog(dialog, c, info.JID)
	})
	box.Append(inviteBtn)

	leaveBtn := gtk.NewButtonWithLabel("Leave group")
	leaveBtn.AddCSSClass("destructive-action")
	leaveBtn.ConnectClicked(func() {
		runGroupAction(status, func() { dialog.Close() }, "leave group", func(ctx context.Context) error {
			return c.LeaveGroup(ctx, info.JID)
		})
	})
	box.Append(leaveBtn)

	box.Append(status)
}

// participantRow renders one participant with a role badge and, for an admin
// viewer acting on someone other than themselves, Remove and Promote/Demote
// buttons.
func participantRow(c client.Client, info client.GroupInfo, p client.GroupParticipant, admin bool, status *gtk.Label, reload func()) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)

	name := gtk.NewLabel(participantRowText(p, info.OwnerJID))
	name.SetXAlign(0)
	name.SetHExpand(true)
	name.SetSelectable(true)
	row.Append(name)

	if admin && p.JID != c.OwnJID() {
		label, action := promoteDemoteLabel(p)
		roleBtn := gtk.NewButtonWithLabel(label)
		roleBtn.AddCSSClass("flat")
		roleBtn.ConnectClicked(func() {
			runGroupAction(status, reload, "participant "+action, func(ctx context.Context) error {
				return c.UpdateGroupParticipants(ctx, info.JID, []string{p.JID}, action)
			})
		})
		row.Append(roleBtn)

		removeBtn := gtk.NewButtonWithLabel("Remove")
		removeBtn.AddCSSClass("flat")
		removeBtn.ConnectClicked(func() {
			runGroupAction(status, reload, "remove participant", func(ctx context.Context) error {
				return c.UpdateGroupParticipants(ctx, info.JID, []string{p.JID}, "remove")
			})
		})
		row.Append(removeBtn)
	}
	return row
}

// labeledRow packs a caption, an expanding widget and a trailing button into
// one horizontal row.
func labeledRow(caption string, mid, trailing gtk.Widgetter) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 6)
	lbl := gtk.NewLabel(caption)
	lbl.SetXAlign(0)
	lbl.SetSizeRequest(48, -1)
	row.Append(lbl)
	row.Append(mid)
	row.Append(trailing)
	return row
}

// showInviteLinkDialog fetches jid's invite link into a copyable label, with a
// Reset button that regenerates it.
func showInviteLinkDialog(parent *gtk.Window, c client.Client, jid string) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Invite link")
	dialog.SetDefaultSize(360, 140)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	link := gtk.NewLabel("Loading…")
	link.SetXAlign(0)
	link.SetWrap(true)
	link.SetSelectable(true)
	box.Append(link)

	fetch := func(reset bool) {
		link.SetText("Loading…")
		go func() {
			url, err := c.GroupInviteLink(context.Background(), jid, reset)
			glib.IdleAdd(func() {
				if err != nil {
					link.SetText("Couldn't fetch invite link")
					return
				}
				link.SetText(url)
			})
		}()
	}

	resetBtn := gtk.NewButtonWithLabel("Reset link")
	resetBtn.ConnectClicked(func() { fetch(true) })
	box.Append(resetBtn)

	dialog.SetChild(box)
	dialog.Present()
	fetch(false)
}

func participantsCountLabel(n int) string {
	if n == 1 {
		return "1 participant"
	}
	return fmt.Sprintf("%d participants", n)
}

// participantRowText renders one participant's line: jid, plus a role badge
// in parens when one applies.
func participantRowText(p client.GroupParticipant, ownerJID string) string {
	badge := participantBadge(p, ownerJID)
	if badge == "" {
		return p.JID
	}
	return p.JID + " (" + badge + ")"
}
