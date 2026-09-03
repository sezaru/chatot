package client

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

// seedTabs fills the Fake's Status, Channels and Communities data with the
// interactive mockup's fixtures (STATUS_FEED, CHANNELS, COMMUNITIES in
// mockup/Chatot Interactive.dc.html), so the tabs can be developed and
// screenshotted against the design. Callers hold no lock: this runs from
// NewFake before the Fake is shared.
func (f *Fake) seedTabs(now int64) {
	// Status feed: two contacts with fresh updates, one older poster. No own
	// status, so the "My status" row starts in its "Add to my status" state.
	// Ada's JID doubles as fakeOwnJID, so the feed's posters are Grace and
	// two numbers the chat list doesn't name.
	grace, other, third := "1112223333@s.whatsapp.net", "5551234567@s.whatsapp.net", "1998887777@s.whatsapp.net"
	f.messages[statusBroadcastJID] = []Message{
		{ID: "st-grace-1", ChatJID: statusBroadcastJID, FromJID: grace, TS: now - 3600, Text: "Studio window, late. We ship tomorrow.",
			Attachment: &Attachment{Kind: "image", MimeType: "image/png", Caption: "Studio window, late. We ship tomorrow.", Thumbnail: fakeMapThumbnail()}},
		{ID: "st-grace-2", ChatJID: statusBroadcastJID, FromJID: grace, TS: now - 5000, Text: "Two more takes and the record is done."},
		{ID: "st-third-1", ChatJID: statusBroadcastJID, FromJID: third, TS: now - 7000, Text: "Deploy 2.1 is green. Going to sleep."},
		{ID: "st-other-1", ChatJID: statusBroadcastJID, FromJID: other, TS: now - 40000,
			Attachment: &Attachment{Kind: "image", MimeType: "image/jpeg"}},
	}

	// A third followed channel, then the directory: every channel the
	// mockup's Find channels page lists, followed ones included.
	f.newsletters["333333@newsletter"] = &Newsletter{ID: "333333@newsletter", Name: "Ada's Book Club",
		Description: "One book a month, one thread a week. Run by Ada and three very opinionated moderators.",
		Subscribers: 1904, InviteCode: "adabooks9qt4", Created: now - 500*86400, Following: true, Category: "Entertainment"}
	f.newsletterMsgs["333333@newsletter"] = []NewsletterMessage{
		{ID: "n4", ServerID: 1, Text: "October pick: a short one for once. Discussion thread opens Friday.", TS: now - 2*86400, Views: 1411, Reactions: map[string]int{"📚": 96, "❤️": 40}},
	}
	f.directory = map[string]*Newsletter{}
	for _, n := range f.newsletters {
		d := *n
		f.directory[n.ID] = &d
	}
	for _, n := range []Newsletter{
		{ID: "444444@newsletter", Name: "IPMA Meteorologia", Verified: true, Subscribers: 244000, InviteCode: "ipma6mz1", Created: now - 1200*86400, Category: "News",
			Description: "Official forecasts, sea state and weather warnings for mainland Portugal and the islands."},
		{ID: "555555@newsletter", Name: "Futebol PT", Verified: true, Subscribers: 612000, InviteCode: "futebolpt2wd8", Created: now - 1800*86400, Category: "Sports",
			Description: "Resultados ao minuto, escalações e mercado de transferências da Primeira Liga."},
		{ID: "666666@newsletter", Name: "GNOME Design", Verified: true, Subscribers: 38400, InviteCode: "gnomedesign5pe3", Created: now - 480*86400, Category: "Technology",
			Description: "Mockups, pattern changes and release themes from the GNOME design team."},
		{ID: "777777@newsletter", Name: "Cozinha Rápida", Subscribers: 27100, InviteCode: "cozinharapida7bn0", Created: now - 350*86400, Category: "Food",
			Description: "Uma receita por dia, sempre em menos de trinta minutos. Sem vídeos de dez minutos."},
		{ID: "888888@newsletter", Name: "Vinyl Fridays", Subscribers: 9860, InviteCode: "vinylfridays1xu5", Created: now - 180*86400, Category: "Entertainment",
			Description: "One record every Friday, with a short note on why it still holds up."},
	} {
		d := n
		f.directory[n.ID] = &d
	}
	f.newsletterMsgs["444444@newsletter"] = []NewsletterMessage{
		{ID: "n5", ServerID: 1, Text: "Aviso amarelo para o distrito de Lisboa: chuva forte entre as 03h e as 09h de quinta.", TS: now - 900, Views: 88000, Reactions: map[string]int{"👍": 412}},
		{ID: "n6", ServerID: 2, Text: "Fim de semana estável, máximas de 24 graus no litoral.", TS: now - 3*86400, Views: 71000, Reactions: map[string]int{"☀️": 190}},
	}
	f.newsletterMsgs["555555@newsletter"] = []NewsletterMessage{
		{ID: "n7", ServerID: 1, Text: "Intervalo. 1–0, golo aos 38 minutos de grande penalidade.", TS: now - 5400, Views: 310000, Reactions: map[string]int{"🎉": 2100, "😮": 640}},
	}
	f.newsletterMsgs["666666@newsletter"] = []NewsletterMessage{
		{ID: "n8", ServerID: 1, Text: "Adaptive dialogs landed in libadwaita. Same dialog, three breakpoints.", TS: now - 2*86400, Views: 22000, Reactions: map[string]int{"👍": 980}},
		{ID: "n9", ServerID: 2, Text: "The bottom sheet pattern is now documented in the HIG.", TS: now - 4*86400, Views: 19000, Reactions: map[string]int{"👍": 604}},
	}
	f.newsletterMsgs["777777@newsletter"] = []NewsletterMessage{
		{ID: "n10", ServerID: 1, Text: "Massa com grão e limão — 18 minutos, uma panela só.", TS: now - 3*86400, Views: 14000, Reactions: map[string]int{"😋": 730}},
	}
	f.newsletterMsgs["888888@newsletter"] = []NewsletterMessage{
		{ID: "n11", ServerID: 1, Text: "This week: a live album recorded in a room far too small for it.", TS: now - 6*86400, Views: 7204, Reactions: map[string]int{"❤️": 288}},
	}

	// Communities: the mockup's Bloco B (we administer it) and Escola 4B
	// (we don't). Joined groups exist as chats too, so opening one lands in
	// a real conversation.
	f.addCommunityLocked("blocob@g.us", "Bloco B — Residents",
		"Everything for Bloco B in one place: building notices, works, and the groups neighbours run themselves.",
		grace, true)
	f.addCommunityLocked("escola@g.us", "Escola Primária 4B",
		"Turma do 4.º B — avisos da escola, reuniões de pais e as visitas de estudo do ano lectivo.",
		"4445556666@s.whatsapp.net", false)
	bloco, escola := &f.communities[0], &f.communities[1]
	bloco.Created, bloco.MemberCount = now-540*86400, 128
	bloco.Members = []GroupParticipant{
		{JID: grace, IsAdmin: true, IsSuperAdmin: true}, {JID: fakeOwnJID, IsAdmin: true},
		{JID: "4445556666@s.whatsapp.net"}, {JID: other}, {JID: third},
	}
	bloco.Groups[0].MemberCount = 128
	f.seedCommunityGroup(bloco, "blocob-geral@g.us", "Bloco B — Geral", true, 128, "Priya: the lift is fixed", 12, now-3*3600)
	f.seedCommunityGroup(bloco, "blocob-garagem@g.us", "Garagem & obras", true, 128, "Sam: parking closed Saturday", 0, now-86400)
	f.seedCommunityGroup(bloco, "blocob-piscina@g.us", "Piscina", false, 34, "", 0, 0)
	f.seedCommunityGroup(bloco, "blocob-churrasco@g.us", "Churrasco de verão", false, 19, "", 0, 0)
	f.updateChat(bloco.Groups[0].JID, func(c *Chat) { c.UnreadCount = 3; c.Preview = "Lift maintenance on Thursday" })
	escola.Created, escola.MemberCount = now-60*86400, 61
	escola.Members = []GroupParticipant{
		{JID: "4445556666@s.whatsapp.net", IsAdmin: true, IsSuperAdmin: true}, {JID: other}, {JID: fakeOwnJID}, {JID: "1998887777@s.whatsapp.net"},
	}
	escola.Groups[0].MemberCount = 61
	f.seedCommunityGroup(escola, "escola-pais@g.us", "Pais e mães 4B", true, 61, "Nina: reunião dia 12", 0, now-2*86400)
	f.seedCommunityGroup(escola, "escola-visita@g.us", "Visita de estudo", false, 22, "", 0, 0)
}

// addCommunityLocked registers a community with its announcement group,
// which also exists as a group and a chat. Callers hold f.mu (or own the
// Fake exclusively, as seedTabs does).
func (f *Fake) addCommunityLocked(jid, name, description, creator string, admin bool) {
	now := time.Now().Unix()
	members := []GroupParticipant{{JID: creator, IsAdmin: true, IsSuperAdmin: true}}
	if creator != fakeOwnJID {
		members = append(members, GroupParticipant{JID: fakeOwnJID, IsAdmin: admin})
	}
	f.groups[jid] = &GroupInfo{JID: jid, Name: name, Topic: description, OwnerJID: creator, Participants: members}
	ann := strings.TrimSuffix(jid, "@g.us") + "-ann@g.us"
	f.groups[ann] = &GroupInfo{JID: ann, Name: "Announcements", OwnerJID: creator, Announce: true, Participants: members}
	f.chats = append(f.chats, Chat{JID: ann, Name: name + " · Announcements", IsGroup: true, LastMessageTS: now - 5*86400})
	f.messages[ann] = []Message{{ID: ann + "-m1", ChatJID: ann, FromJID: creator, Text: "Welcome to " + name + ".", TS: now - 5*86400}}
	f.communities = append(f.communities, Community{
		JID: jid, Name: name, Description: description, CreatorJID: creator, Created: now,
		IsAdmin: admin, MemberCount: len(members), Members: members,
		Groups: []CommunityGroup{{JID: ann, Name: "Announcements", Announcement: true, Joined: true, MemberCount: len(members)}},
	})
}

// seedCommunityGroup links one group to c; a joined group also becomes a
// chat (with preview and unread count) and a group.
func (f *Fake) seedCommunityGroup(c *Community, jid, name string, joined bool, members int, preview string, unread int, ts int64) {
	c.Groups = append(c.Groups, CommunityGroup{JID: jid, Name: name, Joined: joined, MemberCount: members, Preview: preview, UnreadCount: unread})
	if !joined {
		return
	}
	f.groups[jid] = &GroupInfo{JID: jid, Name: name, OwnerJID: c.CreatorJID, Participants: c.Members}
	f.chats = append(f.chats, Chat{JID: jid, Name: name, Preview: preview, UnreadCount: unread, IsGroup: true, LastMessageTS: ts})
	f.messages[jid] = []Message{{ID: jid + "-m1", ChatJID: jid, FromJID: c.CreatorJID, Text: preview, TS: ts}}
}

// Communities returns the seeded communities, joined groups' preview,
// unread and mute state read live from the chats so a read or mute shows.
func (f *Fake) Communities(ctx context.Context) ([]Community, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	chats := make(map[string]Chat, len(f.chats))
	for _, c := range f.chats {
		chats[c.JID] = c
	}
	out := make([]Community, len(f.communities))
	for i, c := range f.communities {
		cc := c
		cc.Members = append([]GroupParticipant(nil), c.Members...)
		cc.Groups = append([]CommunityGroup(nil), c.Groups...)
		for j := range cc.Groups {
			g := &cc.Groups[j]
			if !g.Joined {
				continue
			}
			if ch, ok := chats[g.JID]; ok {
				g.Preview, g.UnreadCount = ch.Preview, ch.UnreadCount
				if g.Announcement {
					cc.Muted = ch.Muted
				}
			}
		}
		out[i] = cc
	}
	return out, nil
}

// JoinCommunityGroup marks a linked group joined and gives it a chat.
func (f *Fake) JoinCommunityGroup(ctx context.Context, community, group string) error {
	f.mu.Lock()
	joined := false
	for i := range f.communities {
		c := &f.communities[i]
		if c.JID != community {
			continue
		}
		for j := range c.Groups {
			g := &c.Groups[j]
			if g.JID != group || g.Joined {
				continue
			}
			g.Joined = true
			g.MemberCount++
			f.groups[group] = &GroupInfo{JID: group, Name: g.Name, OwnerJID: c.CreatorJID, Participants: append([]GroupParticipant{{JID: fakeOwnJID}}, c.Members...)}
			f.chats = append(f.chats, Chat{JID: group, Name: g.Name, Preview: "You joined " + g.Name, IsGroup: true, LastMessageTS: time.Now().Unix()})
			f.messages[group] = []Message{{ID: group + "-m1", ChatJID: group, FromJID: c.CreatorJID, Text: "Welcome to " + g.Name + ".", TS: time.Now().Unix()}}
			joined = true
		}
	}
	f.mu.Unlock()
	if !joined {
		return fmt.Errorf("chatot/client: %s is not an unjoined group of %s", group, community)
	}
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: group}})
	return nil
}

// ReactToStatus records our reaction on the status update in the feed.
func (f *Fake) ReactToStatus(ctx context.Context, poster, msgID, emoji string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	msgs := f.messages[statusBroadcastJID]
	for i := range msgs {
		if msgs[i].ID == msgID && msgs[i].FromJID == poster {
			msgs[i].Reactions = withReaction(msgs[i].Reactions, fakeOwnJID, emoji)
			return nil
		}
	}
	return fmt.Errorf("chatot/client: status %q by %s not found", msgID, poster)
}

// DiscoverNewsletters searches the seeded directory by name or description
// (case-insensitive), most followed first, flagging the ones we follow.
func (f *Fake) DiscoverNewsletters(ctx context.Context, query string) ([]Newsletter, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Newsletter, 0, len(f.directory))
	for _, d := range f.directory {
		if q != "" && !strings.Contains(strings.ToLower(d.Name), q) && !strings.Contains(strings.ToLower(d.Description), q) {
			continue
		}
		n := *d
		_, n.Following = f.newsletters[n.ID]
		out = append(out, n)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Subscribers != out[j].Subscribers {
			return out[i].Subscribers > out[j].Subscribers
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

// MarkStatusViewed flags poster's updates as read in the feed; notify has
// nothing to send here.
func (f *Fake) MarkStatusViewed(ctx context.Context, poster string, msgIDs []string, notify bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := map[string]bool{}
	for _, id := range msgIDs {
		want[id] = true
	}
	msgs := f.messages[statusBroadcastJID]
	for i := range msgs {
		if msgs[i].FromJID == poster && want[msgs[i].ID] && msgs[i].Status < MessageStatusRead {
			msgs[i].Status = MessageStatusRead
		}
	}
	return nil
}

// StatusViewers lists the seeded/recorded viewers of our update msgID.
func (f *Fake) StatusViewers(msgID string) ([]StatusViewer, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]StatusViewer(nil), f.statusViewers[msgID]...), nil
}

// addStatusViewer records a view of our update, for tests and seeds.
func (f *Fake) addStatusViewer(msgID, viewer string, ts int64) {
	if f.statusViewers == nil {
		f.statusViewers = map[string][]StatusViewer{}
	}
	f.statusViewers[msgID] = append(f.statusViewers[msgID], StatusViewer{JID: viewer, TS: ts})
}

// MuteStatus files poster under Muted updates and pushes a refresh.
func (f *Fake) MuteStatus(ctx context.Context, poster string, mute bool) error {
	f.mu.Lock()
	if f.statusMuted == nil {
		f.statusMuted = map[string]bool{}
	}
	if mute {
		f.statusMuted[poster] = true
	} else {
		delete(f.statusMuted, poster)
	}
	f.mu.Unlock()
	f.events.Publish(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: poster}})
	return nil
}

// MutedStatusPosters lists the muted posters, sorted for stable output.
func (f *Fake) MutedStatusPosters() ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.statusMuted))
	for jid := range f.statusMuted {
		out = append(out, jid)
	}
	sort.Strings(out)
	return out, nil
}

// HideStatusFrom remembers jid on the exclusion list.
func (f *Fake) HideStatusFrom(ctx context.Context, jid string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.statusHidden == nil {
		f.statusHidden = map[string]bool{}
	}
	f.statusHidden[jid] = true
	return nil
}

// NewsletterMarkViewed bumps the view count of each post once, the way a
// first view does on the server.
func (f *Fake) NewsletterMarkViewed(ctx context.Context, jid string, serverIDs []int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	want := map[int64]bool{}
	for _, id := range serverIDs {
		want[id] = true
	}
	msgs := f.newsletterMsgs[jid]
	for i := range msgs {
		key := jid + "/" + msgs[i].ID
		if want[msgs[i].ServerID] && !f.newsletterViewed[key] {
			if f.newsletterViewed == nil {
				f.newsletterViewed = map[string]bool{}
			}
			f.newsletterViewed[key] = true
			msgs[i].Views++
		}
	}
	return nil
}

// NewsletterSubscribeLive has no live feed to subscribe to.
func (f *Fake) NewsletterSubscribeLive(ctx context.Context, jid string) error { return nil }
