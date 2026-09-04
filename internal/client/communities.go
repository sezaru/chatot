package client

import (
	"context"
	"fmt"
	"sort"

	"go.mau.fi/whatsmeow/types"
)

// Communities lists the communities this account is in: the joined groups
// flagged as parents, each with the sub-groups the server links to it.
// Membership of a sub-group is read off the joined-groups list, and the
// store's chat rows supply a joined group's preview, unread count and the
// announcement group's mute state.
func (w *Whatsmeow) Communities(ctx context.Context) ([]Community, error) {
	groups, err := w.wa.GetJoinedGroups(ctx)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: get joined groups: %w", err)
	}
	joined := make(map[string]*types.GroupInfo, len(groups))
	for _, g := range groups {
		if g != nil {
			joined[g.JID.String()] = g
		}
	}
	chats, err := w.Chats(0)
	if err != nil {
		w.log.Warnf("chatot/client: communities: chats: %v", err)
	}
	byJID := make(map[string]Chat, len(chats))
	for _, c := range chats {
		byJID[c.JID] = c
	}

	var out []Community
	for _, g := range groups {
		if g == nil || !g.IsParent {
			continue
		}
		comm := communityFromGroup(g, w.ownJID())
		subs, err := w.wa.GetSubGroups(ctx, g.JID)
		if err != nil {
			w.log.Warnf("chatot/client: sub-groups of %s: %v", g.JID, err)
		}
		for _, s := range subs {
			if s == nil {
				continue
			}
			jid := s.JID.String()
			cg := CommunityGroup{JID: jid, Name: s.Name, Announcement: s.IsDefaultSubGroup}
			if jg, ok := joined[jid]; ok {
				cg.Joined = true
				cg.MemberCount = groupMemberCount(jg)
				if cg.Name == "" {
					cg.Name = jg.Name
				}
				if c, ok := byJID[jid]; ok {
					cg.Preview = c.Preview
					cg.UnreadCount = c.UnreadCount
					if cg.Name == "" {
						cg.Name = c.Name
					}
					if cg.Announcement {
						comm.Muted = c.Muted
					}
				}
			}
			// The sub-group listing may come without subjects; the
			// announcement group carries its community's name.
			if cg.Name == "" && cg.Announcement {
				cg.Name = comm.Name
			}
			comm.Groups = append(comm.Groups, cg)
		}
		sortCommunityGroups(comm.Groups)
		out = append(out, comm)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// communityFromGroup maps a parent group's metadata to a Community (without
// its sub-groups). own is this account's JID, for the admin flag.
func communityFromGroup(g *types.GroupInfo, own string) Community {
	ownUser := ""
	if oj, err := types.ParseJID(own); err == nil {
		ownUser = oj.User
	}
	members := make([]GroupParticipant, 0, len(g.Participants))
	isAdmin := false
	for _, p := range g.Participants {
		members = append(members, GroupParticipant{JID: p.JID.String(), IsAdmin: p.IsAdmin, IsSuperAdmin: p.IsSuperAdmin})
		if ownUser != "" && p.JID.User == ownUser && (p.IsAdmin || p.IsSuperAdmin) {
			isAdmin = true
		}
	}
	var created int64
	if !g.GroupCreated.IsZero() {
		created = g.GroupCreated.Unix()
	}
	return Community{
		JID:         g.JID.String(),
		Name:        g.Name,
		Description: g.Topic,
		CreatorJID:  g.OwnerJID.String(),
		Created:     created,
		IsAdmin:     isAdmin,
		MemberCount: groupMemberCount(g),
		Members:     members,
	}
}

// groupMemberCount prefers the server's participant count, which is set
// even when the participant list itself is elided for large groups.
func groupMemberCount(g *types.GroupInfo) int {
	if g.ParticipantCount > 0 {
		return g.ParticipantCount
	}
	return len(g.Participants)
}

// sortCommunityGroups orders a community's groups the way the sidebar shows
// them: the announcement group first, then joined groups, then the rest,
// each block alphabetical.
func sortCommunityGroups(groups []CommunityGroup) {
	rank := func(g CommunityGroup) int {
		switch {
		case g.Announcement:
			return 0
		case g.Joined:
			return 1
		default:
			return 2
		}
	}
	sort.SliceStable(groups, func(i, j int) bool {
		ri, rj := rank(groups[i]), rank(groups[j])
		if ri != rj {
			return ri < rj
		}
		return groups[i].Name < groups[j].Name
	})
}

// JoinCommunityGroup is unsupported: whatsmeow only joins groups through
// invite links or admin adds, and a linked group a member hasn't joined
// exposes neither. The UI says so instead of failing silently.
func (w *Whatsmeow) JoinCommunityGroup(ctx context.Context, community, group string) error {
	return ErrUnsupported
}

// ReactToStatus reacts to poster's status update. WhatsApp routes a status
// reaction as a reaction message sent to the poster, keyed on the
// status@broadcast message, which is what BuildReaction produces when the
// chat is the status broadcast and the sender is the poster.
func (w *Whatsmeow) ReactToStatus(ctx context.Context, poster, msgID, emoji string) error {
	to, err := types.ParseJID(poster)
	if err != nil {
		return fmt.Errorf("chatot/client: parse status poster jid: %w", err)
	}
	msg := w.wa.BuildReaction(types.StatusBroadcastJID, to, types.MessageID(msgID), emoji)
	if _, err := w.wa.SendMessage(ctx, to, msg); err != nil {
		return fmt.Errorf("chatot/client: react to status: %w", err)
	}
	return nil
}
