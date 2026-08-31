package client

import (
	"context"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/types"

	"chatot/internal/store"
)

// GroupInfo fetches jid's current name/topic/membership from WhatsApp and
// persists it to the local store (best-effort — a persist failure doesn't
// fail the call, since the caller already has fresh data to show).
func (w *Whatsmeow) GroupInfo(ctx context.Context, jid string) (*GroupInfo, error) {
	j, err := types.ParseJID(jid)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	info, err := w.wa.GetGroupInfo(ctx, j)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: get group info: %w", err)
	}
	gi := groupInfoFromWhatsmeow(info)
	w.persistGroupInfo(gi)
	return gi, nil
}

func groupInfoFromWhatsmeow(info *types.GroupInfo) *GroupInfo {
	parts := make([]GroupParticipant, len(info.Participants))
	for i, p := range info.Participants {
		parts[i] = GroupParticipant{
			JID:          p.JID.String(),
			IsAdmin:      p.IsAdmin,
			IsSuperAdmin: p.IsSuperAdmin,
		}
	}
	return &GroupInfo{
		JID:               info.JID.String(),
		Name:              info.Name,
		Topic:             info.Topic,
		OwnerJID:          info.OwnerJID.String(),
		Announce:          info.IsAnnounce,
		Locked:            info.IsLocked,
		DisappearingTimer: info.DisappearingTimer,
		Participants:      parts,
	}
}

// OwnJID exposes this device's own user JID for the UI's admin gating.
func (w *Whatsmeow) OwnJID() string { return w.ownJID() }

// CreateGroup creates a group and persists it as a chat so it appears in the
// list, returning the new group's JID.
func (w *Whatsmeow) CreateGroup(ctx context.Context, name string, participantJIDs []string) (string, error) {
	if !validGroupName(name) {
		return "", fmt.Errorf("chatot/client: invalid group name %q (1-25 chars)", name)
	}
	parts, err := parseJIDs(participantJIDs)
	if err != nil {
		return "", err
	}
	info, err := w.wa.CreateGroup(ctx, whatsmeow.ReqCreateGroup{Name: name, Participants: parts})
	if err != nil {
		return "", fmt.Errorf("chatot/client: create group: %w", err)
	}
	gi := groupInfoFromWhatsmeow(info)
	w.persistGroupInfo(gi)
	w.insertGroupChat(gi.JID, gi.Name)
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: gi.JID}})
	return gi.JID, nil
}

// LeaveGroup leaves jid and pushes a refresh so the UI updates.
func (w *Whatsmeow) LeaveGroup(ctx context.Context, jid string) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	if err := w.wa.LeaveGroup(ctx, j); err != nil {
		return fmt.Errorf("chatot/client: leave group: %w", err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return nil
}

// UpdateGroupParticipants adds/removes/promotes/demotes participants, then
// re-fetches the group (refreshGroupInfo) which persists it and pushes a
// refresh event.
func (w *Whatsmeow) UpdateGroupParticipants(ctx context.Context, jid string, participantJIDs []string, action string) error {
	change, ok := mapParticipantAction(action)
	if !ok {
		return fmt.Errorf("chatot/client: unknown participant action %q", action)
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	parts, err := parseJIDs(participantJIDs)
	if err != nil {
		return err
	}
	if _, err := w.wa.UpdateGroupParticipants(ctx, j, parts, change); err != nil {
		return fmt.Errorf("chatot/client: update participants: %w", err)
	}
	w.refreshGroupInfo(jid)
	return nil
}

// SetGroupName renames jid, then refreshes.
func (w *Whatsmeow) SetGroupName(ctx context.Context, jid, name string) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	if err := w.wa.SetGroupName(ctx, j, name); err != nil {
		return fmt.Errorf("chatot/client: set group name: %w", err)
	}
	w.refreshGroupInfo(jid)
	return nil
}

// SetGroupTopic sets jid's topic (empty previous/new IDs let whatsmeow
// generate them), then refreshes.
func (w *Whatsmeow) SetGroupTopic(ctx context.Context, jid, topic string) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	if err := w.wa.SetGroupTopic(ctx, j, "", "", topic); err != nil {
		return fmt.Errorf("chatot/client: set group topic: %w", err)
	}
	w.refreshGroupInfo(jid)
	return nil
}

// SetGroupAnnounce toggles announce mode, then refreshes.
func (w *Whatsmeow) SetGroupAnnounce(ctx context.Context, jid string, announce bool) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	if err := w.wa.SetGroupAnnounce(ctx, j, announce); err != nil {
		return fmt.Errorf("chatot/client: set group announce: %w", err)
	}
	w.refreshGroupInfo(jid)
	return nil
}

// SetGroupLocked toggles locked mode, then refreshes.
func (w *Whatsmeow) SetGroupLocked(ctx context.Context, jid string, locked bool) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	if err := w.wa.SetGroupLocked(ctx, j, locked); err != nil {
		return fmt.Errorf("chatot/client: set group locked: %w", err)
	}
	w.refreshGroupInfo(jid)
	return nil
}

// SetGroupDisappearingTimer sets jid's disappearing-message timer, then
// refreshes.
func (w *Whatsmeow) SetGroupDisappearingTimer(ctx context.Context, jid string, seconds int64) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	if err := w.wa.SetDisappearingTimer(ctx, j, time.Duration(seconds)*time.Second, time.Now()); err != nil {
		return fmt.Errorf("chatot/client: set disappearing timer: %w", err)
	}
	w.refreshGroupInfo(jid)
	return nil
}

// GroupInviteLink returns jid's invite link, optionally resetting it first.
func (w *Whatsmeow) GroupInviteLink(ctx context.Context, jid string, reset bool) (string, error) {
	j, err := types.ParseJID(jid)
	if err != nil {
		return "", fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	link, err := w.wa.GetGroupInviteLink(ctx, j, reset)
	if err != nil {
		return "", fmt.Errorf("chatot/client: get invite link: %w", err)
	}
	return link, nil
}

// JoinGroupWithLink joins via an invite link/code, persists the joined group
// as a chat, and returns its JID.
func (w *Whatsmeow) JoinGroupWithLink(ctx context.Context, code string) (string, error) {
	j, err := w.wa.JoinGroupWithLink(ctx, parseInviteCode(code))
	if err != nil {
		return "", fmt.Errorf("chatot/client: join group: %w", err)
	}
	jid := j.String()
	if gi, err := w.GroupInfo(ctx, jid); err == nil && gi != nil {
		w.insertGroupChat(gi.JID, gi.Name)
	} else {
		w.insertGroupChat(jid, "")
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	return jid, nil
}

// CreateCommunity creates a community (a parent group with GroupParent.IsParent
// set; WhatsApp's server creates the linked announcement group automatically),
// persists it as a chat, sets its description if given, and returns its JID.
func (w *Whatsmeow) CreateCommunity(ctx context.Context, name, description string) (string, error) {
	if !validGroupName(name) {
		return "", fmt.Errorf("chatot/client: invalid community name %q (1-25 chars)", name)
	}
	info, err := w.wa.CreateGroup(ctx, whatsmeow.ReqCreateGroup{
		Name:        name,
		GroupParent: types.GroupParent{IsParent: true},
	})
	if err != nil {
		return "", fmt.Errorf("chatot/client: create community: %w", err)
	}
	gi := groupInfoFromWhatsmeow(info)
	w.persistGroupInfo(gi)
	w.insertGroupChat(gi.JID, gi.Name)
	if description != "" {
		if err := w.SetGroupTopic(ctx, gi.JID, description); err != nil {
			w.log.Warnf("chatot/client: set community description %s: %v", gi.JID, err)
		}
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: gi.JID}})
	return gi.JID, nil
}

// LinkGroupToCommunity links an existing group as a sub-group of community.
func (w *Whatsmeow) LinkGroupToCommunity(ctx context.Context, community, group string) error {
	parent, err := types.ParseJID(community)
	if err != nil {
		return fmt.Errorf("chatot/client: parse community jid: %w", err)
	}
	child, err := types.ParseJID(group)
	if err != nil {
		return fmt.Errorf("chatot/client: parse group jid: %w", err)
	}
	if err := w.wa.LinkGroup(ctx, parent, child); err != nil {
		return fmt.Errorf("chatot/client: link group to community: %w", err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: community}})
	return nil
}

// insertGroupChat ensures a chat row exists for a newly created/joined group
// so it shows up in the chat list.
func (w *Whatsmeow) insertGroupChat(jid, name string) {
	if err := w.store.UpsertChat(store.ChatRow{JID: jid, IsGroup: true, Name: name, LastMessageTS: time.Now().Unix()}); err != nil {
		w.log.Warnf("chatot/client: insert group chat %s: %v", jid, err)
	}
}

// parseJIDs parses a list of JID strings, failing on the first invalid one.
func parseJIDs(jids []string) ([]types.JID, error) {
	out := make([]types.JID, 0, len(jids))
	for _, j := range jids {
		parsed, err := types.ParseJID(j)
		if err != nil {
			return nil, fmt.Errorf("chatot/client: parse participant jid %q: %w", j, err)
		}
		out = append(out, parsed)
	}
	return out, nil
}

// mapParticipantAction maps the interface's action string to whatsmeow's
// ParticipantChange; ok is false for an unrecognized action.
func mapParticipantAction(action string) (whatsmeow.ParticipantChange, bool) {
	switch action {
	case "add":
		return whatsmeow.ParticipantChangeAdd, true
	case "remove":
		return whatsmeow.ParticipantChangeRemove, true
	case "promote":
		return whatsmeow.ParticipantChangePromote, true
	case "demote":
		return whatsmeow.ParticipantChangeDemote, true
	default:
		return "", false
	}
}

// validGroupName reports whether name is a non-empty group name of at most 25
// characters (WhatsApp's limit).
func validGroupName(name string) bool {
	n := strings.TrimSpace(name)
	return n != "" && utf8.RuneCountInString(n) <= 25
}

// parseInviteCode reduces a WhatsApp group invite link to its bare code,
// accepting a full https/http URL, a scheme-less "chat.whatsapp.com/<code>",
// or an already-bare code, trimming surrounding whitespace and slashes.
func parseInviteCode(input string) string {
	s := strings.TrimSpace(input)
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "chat.whatsapp.com/")
	return strings.Trim(s, "/ ")
}

// persistGroupInfo saves gi's metadata and membership to the store,
// warning (not failing) on error.
func (w *Whatsmeow) persistGroupInfo(gi *GroupInfo) {
	if err := w.store.UpsertGroup(store.GroupRow{JID: gi.JID, Name: gi.Name, Topic: gi.Topic}); err != nil {
		w.log.Warnf("chatot/client: persist group %s: %v", gi.JID, err)
	}
	parts := make([]store.GroupParticipant, len(gi.Participants))
	for i, p := range gi.Participants {
		parts[i] = store.GroupParticipant{JID: p.JID, IsAdmin: p.IsAdmin, IsSuperAdmin: p.IsSuperAdmin}
	}
	if err := w.store.SetGroupParticipants(gi.JID, parts); err != nil {
		w.log.Warnf("chatot/client: persist group participants %s: %v", gi.JID, err)
	}
}

// refreshGroupInfo re-fetches jid's group info in the background and pushes
// an EventChatUpdate so any open chat list / group-info panel refreshes.
// Called on any inbound *events.GroupInfo, which fires on membership or
// metadata changes but only carries deltas — a full re-fetch is simpler and
// robust than reassembling state from those deltas.
func (w *Whatsmeow) refreshGroupInfo(jid string) {
	go func() {
		if _, err := w.GroupInfo(context.Background(), jid); err != nil {
			w.log.Warnf("chatot/client: refresh group info for %s: %v", jid, err)
			return
		}
		w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: jid}})
	}()
}
