package ui

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strings"
	"unicode"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

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

// participantSelection tracks the new-group flow's picked participants,
// preserving insertion order for the chip row while supporting O(1)
// membership checks.
type participantSelection struct {
	order []string
	names map[string]string
}

func newParticipantSelection() *participantSelection {
	return &participantSelection{names: make(map[string]string)}
}

// Add records jid as selected (name is used for its chip label), a no-op if
// already selected.
func (s *participantSelection) Add(jid, name string) {
	if _, ok := s.names[jid]; ok {
		return
	}
	s.order = append(s.order, jid)
	s.names[jid] = name
}

// Remove deselects jid, a no-op if not selected.
func (s *participantSelection) Remove(jid string) {
	if _, ok := s.names[jid]; !ok {
		return
	}
	delete(s.names, jid)
	for i, j := range s.order {
		if j == jid {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

// Contains reports whether jid is currently selected.
func (s *participantSelection) Contains(jid string) bool {
	_, ok := s.names[jid]
	return ok
}

// Count returns the number of selected participants.
func (s *participantSelection) Count() int { return len(s.order) }

// JIDs returns the selected JIDs in the order they were added.
func (s *participantSelection) JIDs() []string {
	out := make([]string, len(s.order))
	copy(out, s.order)
	return out
}

// Chips returns the selected (jid, name) pairs in insertion order, for
// rendering the removable chip row.
func (s *participantSelection) Chips() []struct{ JID, Name string } {
	out := make([]struct{ JID, Name string }, len(s.order))
	for i, jid := range s.order {
		out[i] = struct{ JID, Name string }{JID: jid, Name: s.names[jid]}
	}
	return out
}

// disappearingOptions are the labels for the new-group "Disappearing
// messages" dropdown, index-aligned with disappearingSecondsForIndex.
var disappearingOptions = []string{"Off", "24 hours", "7 days", "90 days"}

var disappearingSecondsByIndex = []int64{0, 24 * 60 * 60, 7 * 24 * 60 * 60, 90 * 24 * 60 * 60}

// disappearingSecondsForIndex maps a disappearingOptions dropdown selection
// to a duration in seconds (0 = off), defaulting to off for an out-of-range
// index.
func disappearingSecondsForIndex(idx int) int64 {
	if idx < 0 || idx >= len(disappearingSecondsByIndex) {
		return 0
	}
	return disappearingSecondsByIndex[idx]
}

// disappearingIndexForSeconds maps a stored disappearing-timer duration back
// to its disappearingOptions dropdown index, the inverse of
// disappearingSecondsForIndex. An unrecognized duration falls back to "Off".
func disappearingIndexForSeconds(seconds uint32) int {
	for i, s := range disappearingSecondsByIndex {
		if int64(seconds) == s {
			return i
		}
	}
	return 0
}

// groupInfoActions are the rows the group card shares with the chat ⋮ menu
// (mute, disappearing, media), plus the toast and forward sinks the invite
// dialog needs. A nil callback renders its row insensitive.
type groupInfoActions struct {
	Muted             bool
	Mute              func()
	DisappearingValue string
	Disappearing      func()
	MediaCount        string
	Media             func()
	Toast             func(string)
	Forward           func(client.Message)
}

// groupInviteBody is the invite dialog's explanation for a plain group.
const groupInviteBody = "Anyone with this link can join the group. Reset it to revoke every link you have shared."

// groupInfoSub is the dim line under a group's name, the mockup's "Alex,
// Priya, Sam, Nina, you": up to five member first names, "you" last when we
// are in the group, and "+N" for anyone past the fifth.
func groupInfoSub(firstNames []string, includesYou bool) string {
	limit := 5
	if includesYou {
		limit = 4
	}
	shown := firstNames
	rest := 0
	if len(shown) > limit {
		rest = len(shown) - limit
		shown = shown[:limit]
	}
	s := strings.Join(shown, ", ")
	if rest > 0 {
		s += fmt.Sprintf(" +%d", rest)
	}
	if includesYou {
		if s != "" {
			s += ", "
		}
		s += "you"
	}
	return s
}

// participantFirstNames resolves a roster to display first names in the
// owner-admins-members order, leaving us out and reporting that separately.
func participantFirstNames(parts []client.GroupParticipant, ownerJID, own string, names map[string]string) (firstNames []string, includesYou bool) {
	for _, p := range orderParticipants(parts, ownerJID) {
		if isOwnJID(p.JID, own) {
			includesYou = true
			continue
		}
		firstNames = append(firstNames, strings.SplitN(posterName(p.JID, names), " ", 2)[0])
	}
	return firstNames, includesYou
}

// memberActionLabel words a participant action for a failure toast.
func memberActionLabel(action string) string {
	switch action {
	case "promote":
		return "make them an admin"
	case "demote":
		return "remove them as admin"
	case "remove":
		return "remove them"
	}
	return action
}

// groupMemberMenuItems is an admin's ⋮ on another member: promote or
// demote, then remove.
func groupMemberMenuItems(p client.GroupParticipant, act func(action string)) []menuItem {
	_, action := promoteDemoteLabel(p)
	label := "Make group admin"
	if action == "demote" {
		label = "Dismiss as admin"
	}
	return []menuItem{
		{Icon: "★", Label: label, OnActivate: func() { act(action) }},
		{Icon: "✕", Label: "Remove from group", Destructive: true, OnActivate: func() { act("remove") }},
	}
}

// presentAlert shows alert inside parent, or over the active window when
// the dialog was opened without one (a typed nil would crash the binding).
func presentAlert(alert *adw.AlertDialog, parent *gtk.Window) {
	if parent == nil {
		alert.Present(nil)
		return
	}
	alert.Present(parent)
}

// showGroupInfoDialog opens the mockup's info card for a group: the big
// avatar over the name and a member line, a card of settings rows, the
// participant list (promote/demote/remove behind a ⋮ for admins) and Leave
// group. The card is rebuilt from a fresh GroupInfo after every change.
func showGroupInfoDialog(parent *gtk.Window, c client.Client, cache *avatarCache, jid string, a groupInfoActions) {
	dialog := newCardDialog()
	dialog.SetTitle("Group info")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetDefaultSize(400, -1)

	scroller := gtk.NewScrolledWindow()
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetMaxContentHeight(640)
	scroller.SetPropagateNaturalHeight(true)
	content := gtk.NewBox(gtk.OrientationVertical, 0)
	scroller.SetChild(content)
	dialog.SetChild(scroller)

	toast := a.Toast
	if toast == nil {
		toast = func(string) {}
	}
	note := func(text string) {
		clearBox(content)
		l := gtk.NewLabel(text)
		l.AddCSSClass("chatot-dialog-hint")
		l.SetMarginTop(24)
		l.SetMarginBottom(24)
		content.Append(l)
	}

	var reload func()
	// run performs a change off the main loop and rebuilds the card on
	// success; a failure keeps the card and says so in a toast.
	run := func(label string, do func(ctx context.Context) error) {
		go func() {
			err := do(context.Background())
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: group %s: %v", label, err)
					toast("Couldn't " + label)
					return
				}
				reload()
			})
		}()
	}
	// The card is presented only once the roster is in: an AdwDialog sizes
	// itself to its first content, and one shown over a "Loading…" line
	// stayed that short after the real card replaced it.
	presented := false
	reload = func() {
		go func() {
			info, err := c.GroupInfo(context.Background(), jid)
			glib.IdleAdd(func() {
				if err != nil || info == nil {
					note("Couldn't load group info")
				} else {
					clearBox(content)
					content.Append(buildGroupInfo(dialog, c, cache, *info, a, toast, run))
				}
				if !presented {
					presented = true
					dialog.Present()
				}
			})
		}()
	}
	reload()
}

// buildGroupInfo lays the card for info. Every row that leads elsewhere
// closes the card first, as the mockup's rows do.
func buildGroupInfo(dialog *cardDialog, c client.Client, cache *avatarCache, info client.GroupInfo, a groupInfoActions, toast func(string), run func(string, func(context.Context) error)) gtk.Widgetter {
	own := c.OwnJID()
	admin := isSelfAdmin(info, own)
	names := chatNames(c)

	box := gtk.NewBox(gtk.OrientationVertical, 0)

	head := gtk.NewBox(gtk.OrientationVertical, 8)
	head.AddCSSClass("chatot-info-head")
	head.SetHAlign(gtk.AlignCenter)
	avatar := buildAvatar(c, cache, info.JID, initialFor(info.Name), 76)
	avatar.SetHAlign(gtk.AlignCenter)
	head.Append(avatar)
	title := gtk.NewLabel(info.Name)
	title.AddCSSClass("chatot-info-name")
	title.SetWrap(true)
	title.SetJustify(gtk.JustifyCenter)
	head.Append(title)
	first, you := participantFirstNames(info.Participants, info.OwnerJID, own, names)
	if sub := groupInfoSub(first, you); sub != "" {
		s := gtk.NewLabel(sub)
		s.AddCSSClass("chatot-info-sub")
		s.SetWrap(true)
		s.SetJustify(gtk.JustifyCenter)
		head.Append(s)
	}
	if info.Topic != "" {
		t := gtk.NewLabel(info.Topic)
		t.AddCSSClass("chatot-info-topic")
		t.SetWrap(true)
		t.SetJustify(gtk.JustifyCenter)
		t.SetMaxWidthChars(40)
		head.Append(t)
	}
	box.Append(head)

	closing := func(f func()) func() {
		if f == nil {
			return nil
		}
		return func() {
			dialog.Close()
			f()
		}
	}

	body := dialogBody(10)
	card := newSettingsCard()
	if admin || !info.Locked {
		card.Add(newIconRow("✎", "Edit name and description", "", false, func() {
			showGroupEditDialog(dialog.Window(), info, func(name, topic string) {
				run("update the group", func(ctx context.Context) error {
					if name != info.Name {
						if err := c.SetGroupName(ctx, info.JID, name); err != nil {
							return err
						}
					}
					if topic != info.Topic {
						return c.SetGroupTopic(ctx, info.JID, topic)
					}
					return nil
				})
			})
		}))
	}
	muteIcon := "🔇"
	if a.Muted {
		muteIcon = "🔔"
	}
	card.Add(newIconRow(muteIcon, muteInfoRowLabel(a.Muted), "", false, closing(a.Mute)))
	card.Add(newIconRow("⏱", "Disappearing messages", a.DisappearingValue, false, closing(a.Disappearing)))
	card.Add(newIconRow("🖼", "Media, links and docs", a.MediaCount, false, closing(a.Media)))
	card.Add(newIconRow("🔗", "Invite link", "", false, func() {
		showInviteLinkDialog(dialog.Window(), c, info.JID, "Invite to "+info.Name, groupInviteBody, a.Toast, a.Forward)
	}))
	if admin {
		announce, _ := newSwitchRow("Only admins can send", "", info.Announce, func(on bool) {
			run("change who can send", func(ctx context.Context) error { return c.SetGroupAnnounce(ctx, info.JID, on) })
		})
		card.Add(announce)
		locked, _ := newSwitchRow("Only admins can edit group info", "", info.Locked, func(on bool) {
			run("change who can edit", func(ctx context.Context) error { return c.SetGroupLocked(ctx, info.JID, on) })
		})
		card.Add(locked)
	}
	body.Append(card)

	members := newSettingsCard()
	for _, p := range orderParticipants(info.Participants, info.OwnerJID) {
		members.Add(groupMemberRow(c, cache, info, p, names, admin, run))
	}
	if admin {
		members.Add(newIconRow("＋", "Add participant", "", false, func() {
			showAddParticipantDialog(dialog.Window(), c, info, run)
		}))
	}
	body.Append(newSettingsGroup(strings.ToUpper(participantsCountLabel(len(info.Participants))), members))

	leave := newSettingsCard()
	leave.Add(newIconRow("⤓", "Leave group", "", true, func() {
		confirmLeaveGroup(dialog, c, info, toast)
	}))
	body.Append(leave)
	box.Append(body)
	return box
}

// groupMemberRow is one participant: avatar, name ("You" for us), the
// owner/admin chip, and for an admin viewer a ⋮ with the member actions.
func groupMemberRow(c client.Client, cache *avatarCache, info client.GroupInfo, p client.GroupParticipant, names map[string]string, admin bool, run func(string, func(context.Context) error)) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	row.AddCSSClass("chatot-card-row")

	name := posterName(p.JID, names)
	initial := initialFor(name)
	self := isOwnJID(p.JID, c.OwnJID())
	if self {
		name = "You"
	}
	row.Append(buildAvatar(c, cache, p.JID, initial, 32))

	l := gtk.NewLabel(name)
	l.SetXAlign(0)
	l.SetHExpand(true)
	l.SetEllipsize(pango.EllipsizeEnd)
	l.AddCSSClass("chatot-card-label")
	row.Append(l)

	if badge := participantBadge(p, info.OwnerJID); badge != "" {
		chip := gtk.NewLabel(strings.ToLower(badge))
		chip.AddCSSClass("chatot-role-chip")
		chip.SetVAlign(gtk.AlignCenter)
		row.Append(chip)
	}

	if admin && !self {
		more := gtk.NewButtonWithLabel("⋮")
		more.RemoveCSSClass("text-button")
		more.AddCSSClass("flat")
		more.AddCSSClass("chatot-member-more")
		more.SetVAlign(gtk.AlignCenter)
		more.SetTooltipText("Member options")
		more.ConnectClicked(func() {
			popupMenuBelow(more, groupMemberMenuItems(p, func(action string) {
				run(memberActionLabel(action), func(ctx context.Context) error {
					return c.UpdateGroupParticipants(ctx, info.JID, []string{p.JID}, action)
				})
			}))
		})
		row.Append(more)
	}
	return row
}

// showGroupEditDialog is the small "Edit group" card: name and description
// fields with Save, which reports both values back.
func showGroupEditDialog(parent *gtk.Window, info client.GroupInfo, save func(name, topic string)) {
	dialog := newCardDialog()
	dialog.SetTitle("Edit group")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetDefaultSize(380, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := dialogBody(10)

	nameCap := gtk.NewLabel("NAME")
	nameCap.SetXAlign(0)
	nameCap.AddCSSClass("chatot-card-caption")
	body.Append(nameCap)
	nameEntry := gtk.NewEntry()
	nameEntry.SetText(info.Name)
	nameEntry.SetPlaceholderText("Group name")
	nameEntry.AddCSSClass("chatot-dialog-entry")
	body.Append(nameEntry)

	topicCap := gtk.NewLabel("DESCRIPTION")
	topicCap.SetXAlign(0)
	topicCap.AddCSSClass("chatot-card-caption")
	body.Append(topicCap)
	topicEntry := gtk.NewEntry()
	topicEntry.SetText(info.Topic)
	topicEntry.SetPlaceholderText("What is this group about?")
	topicEntry.AddCSSClass("chatot-dialog-entry")
	body.Append(topicEntry)
	box.Append(body)

	footer := newDialogFooter()
	footer.Append(newChipButton("Cancel", func() { dialog.Close() }))
	saveBtn := newPrimaryButton("Save", func() {
		name := strings.TrimSpace(nameEntry.Text())
		if name == "" {
			return
		}
		dialog.Close()
		save(name, strings.TrimSpace(topicEntry.Text()))
	})
	footer.Append(saveBtn)
	nameEntry.ConnectChanged(func() {
		saveBtn.SetSensitive(strings.TrimSpace(nameEntry.Text()) != "")
	})
	box.Append(footer)

	dialog.SetChild(box)
	dialog.Present()
}

// showAddParticipantDialog asks for a phone number or JID and adds it.
func showAddParticipantDialog(parent *gtk.Window, c client.Client, info client.GroupInfo, run func(string, func(context.Context) error)) {
	dialog := newCardDialog()
	dialog.SetTitle("Add participant")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetDefaultSize(380, -1)

	box := gtk.NewBox(gtk.OrientationVertical, 0)
	body := dialogBody(10)
	body.Append(newDialogBodyText("Enter a phone number with its country code. Several can be added at once, separated by commas."))
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("+351 912 000 000")
	entry.AddCSSClass("chatot-dialog-entry")
	body.Append(entry)
	hint := gtk.NewLabel("")
	hint.SetXAlign(0)
	hint.AddCSSClass("chatot-dialog-hint")
	hint.SetVisible(false)
	body.Append(hint)
	box.Append(body)

	add := func() {
		jids := parseParticipantList(entry.Text())
		if len(jids) == 0 {
			hint.SetText("Enter a phone number or JID")
			hint.SetVisible(true)
			return
		}
		dialog.Close()
		run("add the participant", func(ctx context.Context) error {
			return c.UpdateGroupParticipants(ctx, info.JID, jids, "add")
		})
	}
	entry.ConnectActivate(add)

	footer := newDialogFooter()
	footer.Append(newChipButton("Cancel", func() { dialog.Close() }))
	footer.Append(newPrimaryButton("Add", add))
	box.Append(footer)

	dialog.SetChild(box)
	dialog.Present()
}

// confirmLeaveGroup asks before leaving, then closes the info card.
func confirmLeaveGroup(dialog *cardDialog, c client.Client, info client.GroupInfo, toast func(string)) {
	alert := adw.NewAlertDialog("Leave "+info.Name+"?", "You'll stop getting this group's messages. Admins are told that you left.")
	alert.AddResponse("cancel", "Cancel")
	alert.AddResponse("leave", "Leave")
	alert.SetResponseAppearance("leave", adw.ResponseDestructive)
	alert.SetCloseResponse("cancel")
	alert.ConnectResponse(func(response string) {
		if response != "leave" {
			return
		}
		go func() {
			err := c.LeaveGroup(context.Background(), info.JID)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: leave group: %v", err)
					toast("Couldn't leave " + info.Name)
					return
				}
				toast("Left " + info.Name)
				dialog.Close()
			})
		}()
	})
	presentAlert(alert, dialog.Window())
}

// clearBox removes every child from box.
func clearBox(box *gtk.Box) {
	for child := box.FirstChild(); child != nil; child = box.FirstChild() {
		box.Remove(child)
	}
}

// participantsCountLabel is the members caption: "1 participant", "5
// participants".
func participantsCountLabel(n int) string {
	if n == 1 {
		return "1 participant"
	}
	return fmt.Sprintf("%d participants", n)
}
