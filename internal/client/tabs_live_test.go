package client

import (
	"context"
	"testing"

	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
)

func TestParseNewsletterListFlatAndEdges(t *testing.T) {
	flat := `{"xwa2_newsletters_recommended":[
		{"id":"1@newsletter","state":{"type":"ACTIVE"},"thread_metadata":{"name":{"text":"One"},"description":{"text":"first"},"subscribers_count":"12","invite":"abc","creation_time":"1700000000","verification":"VERIFIED"}},
		{"id":"2@newsletter","state":{"type":"ACTIVE"},"thread_metadata":{"name":{"text":"Two"},"description":{"text":""},"subscribers_count":"7","invite":"def","creation_time":"1700000000","verification":"UNVERIFIED"}}
	]}`
	got := parseNewsletterList([]byte(flat))
	if len(got) != 2 || got[0].ID.String() != "1@newsletter" || got[1].ThreadMeta.Name.Text != "Two" {
		t.Fatalf("flat: got %+v", got)
	}
	edges := `{"data":{"directory":{"edges":[{"node":{"id":"3@newsletter","state":{"type":"ACTIVE"},"thread_metadata":{"name":{"text":"Three"},"description":{"text":""},"subscribers_count":"1","invite":"x","creation_time":"1700000000","verification":"UNVERIFIED"}}}]}}}`
	got = parseNewsletterList([]byte(edges))
	if len(got) != 1 || got[0].ID.String() != "3@newsletter" {
		t.Fatalf("edges: got %+v", got)
	}
	if parseNewsletterList([]byte(`{"data":{"nothing":[1,2]}}`)) != nil {
		t.Fatal("a reply without channels must yield nil")
	}
	if parseNewsletterList([]byte(`not json`)) != nil {
		t.Fatal("garbage must yield nil")
	}
}

func TestFilterNewsletterDirectory(t *testing.T) {
	meta := func(id, name, desc string, subs int) *types.NewsletterMetadata {
		m := &types.NewsletterMetadata{ID: types.NewJID(id, types.NewsletterServer)}
		m.ThreadMeta.Name.Text = name
		m.ThreadMeta.Description.Text = desc
		m.ThreadMeta.SubscriberCount = subs
		return m
	}
	metas := []*types.NewsletterMetadata{
		meta("1", "Weather PT", "forecasts", 10),
		meta("2", "Futebol", "football scores", 500),
		meta("3", "Cooking", "recipes and weather-proof picnics", 50),
	}
	following := map[string]bool{"2@newsletter": true}
	all := filterNewsletterDirectory(metas, following, "")
	if len(all) != 3 || all[0].Name != "Futebol" || !all[0].Following || all[1].Following {
		t.Fatalf("all: got %+v", all)
	}
	hits := filterNewsletterDirectory(metas, following, "WEATHER")
	if len(hits) != 2 || hits[0].Name != "Cooking" || hits[1].Name != "Weather PT" {
		t.Fatalf("query: got %+v", hits)
	}
}

func TestCountryCodesFor(t *testing.T) {
	cases := map[string][]string{
		"351912345678@s.whatsapp.net":     {"PT"},
		"5511999999999:12@s.whatsapp.net": {"BR"},
		"14155550100@s.whatsapp.net":      {"US"},
		"999123@s.whatsapp.net":           {},
		"":                                {},
	}
	for jid, want := range cases {
		got := countryCodesFor(jid)
		if len(got) != len(want) || (len(want) == 1 && got[0] != want[0]) {
			t.Errorf("%s: got %v want %v", jid, got, want)
		}
	}
}

func TestStatusPrivacyAfterHiding(t *testing.T) {
	a, b, target := types.NewJID("1", types.DefaultUserServer), types.NewJID("2", types.DefaultUserServer), types.NewJID("9", types.DefaultUserServer)
	// Contacts audience: target starts a deny list.
	mode, users := statusPrivacyAfterHiding([]types.StatusPrivacy{{Type: types.StatusPrivacyTypeContacts, IsDefault: true}}, target)
	if mode != waSyncAction.StatusPrivacyAction_DENY_LIST || len(users) != 1 || users[0] != target.String() {
		t.Fatalf("contacts: %v %v", mode, users)
	}
	// Existing deny list: target joins it (once).
	mode, users = statusPrivacyAfterHiding([]types.StatusPrivacy{{Type: types.StatusPrivacyTypeBlacklist, List: []types.JID{a, target}, IsDefault: true}}, target)
	if mode != waSyncAction.StatusPrivacyAction_DENY_LIST || len(users) != 2 || users[0] != a.String() || users[1] != target.String() {
		t.Fatalf("blacklist: %v %v", mode, users)
	}
	// Allow list: target leaves it.
	mode, users = statusPrivacyAfterHiding([]types.StatusPrivacy{{Type: types.StatusPrivacyTypeWhitelist, List: []types.JID{a, target, b}, IsDefault: true}}, target)
	if mode != waSyncAction.StatusPrivacyAction_ALLOW_LIST || len(users) != 2 || users[0] != a.String() || users[1] != b.String() {
		t.Fatalf("whitelist: %v %v", mode, users)
	}
}

func TestPrivacySettingOptionsAndLabels(t *testing.T) {
	if got := PrivacySettingOptions("Read Receipts"); len(got) != 2 || got[0] != "all" || got[1] != "none" {
		t.Fatalf("read receipts options: %v", got)
	}
	if PrivacySettingOptions("Nope") != nil {
		t.Fatal("unknown setting must have no options")
	}
	if _, _, err := privacySettingType("Online", "none"); err == nil {
		t.Fatal("none is not valid for Online")
	}
	if typ, val, err := privacySettingType("Status", "contact_blacklist"); err != nil || typ != types.PrivacySettingTypeStatus || val != types.PrivacySettingContactBlacklist {
		t.Fatalf("status: %v %v %v", typ, val, err)
	}
	if PrivacySettingLabel("contact_blacklist") != "My contacts except…" || PrivacySettingLabel("") != "Not set" {
		t.Fatal("labels")
	}
}

func TestTranslateReceiptCarriesReader(t *testing.T) {
	viewer := types.NewJID("111", types.DefaultUserServer)
	e := translate(&events.Receipt{
		MessageSource: types.MessageSource{Chat: viewer, Sender: viewer},
		MessageIDs:    []types.MessageID{"st1"},
		Type:          types.ReceiptTypeRead,
	})
	if e == nil || e.Receipt.ReaderJID != viewer.String() {
		t.Fatalf("read receipt must name the reader: %+v", e)
	}
	self := translate(&events.Receipt{
		MessageSource: types.MessageSource{Chat: viewer, Sender: viewer, IsFromMe: true},
		MessageIDs:    []types.MessageID{"m1"},
		Type:          types.ReceiptTypeReadSelf,
	})
	if self == nil || self.Receipt.ReaderJID != "" {
		t.Fatalf("a self receipt is not a reader: %+v", self)
	}
	delivered := translate(&events.Receipt{MessageSource: types.MessageSource{Chat: viewer, Sender: viewer}, MessageIDs: []types.MessageID{"m1"}, Type: types.ReceiptTypeDelivered})
	if delivered.Receipt.ReaderJID != "" {
		t.Fatal("delivered is not a read")
	}
}

func TestFakeStatusViewedMutedAndViewers(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	grace := "1112223333@s.whatsapp.net"
	before, _ := f.Statuses(50)
	var ids []string
	for _, m := range before {
		if m.FromJID == grace {
			if m.Status >= MessageStatusRead {
				t.Fatalf("seeded status %s already read", m.ID)
			}
			ids = append(ids, m.ID)
		}
	}
	if len(ids) == 0 {
		t.Fatal("no seeded statuses for grace")
	}
	if err := f.MarkStatusViewed(ctx, grace, ids, true); err != nil {
		t.Fatal(err)
	}
	after, _ := f.Statuses(50)
	for _, m := range after {
		if m.FromJID == grace && m.Status < MessageStatusRead {
			t.Fatalf("status %s not marked read", m.ID)
		}
	}
	if err := f.MuteStatus(ctx, grace, true); err != nil {
		t.Fatal(err)
	}
	muted, _ := f.MutedStatusPosters()
	if len(muted) != 1 || muted[0] != grace {
		t.Fatalf("muted: %v", muted)
	}
	_ = f.MuteStatus(ctx, grace, false)
	if muted, _ = f.MutedStatusPosters(); len(muted) != 0 {
		t.Fatalf("unmute: %v", muted)
	}
	f.addStatusViewer("status-x", grace, 5)
	viewers, _ := f.StatusViewers("status-x")
	if len(viewers) != 1 || viewers[0].JID != grace {
		t.Fatalf("viewers: %v", viewers)
	}
	if err := f.SetPrivacySetting(ctx, "Status", "none"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetPrivacySetting(ctx, "Status", "bogus"); err == nil {
		t.Fatal("bogus value accepted")
	}
	p, _ := f.PrivacySettings(ctx)
	if p["Status"] != "none" {
		t.Fatalf("privacy not stored: %v", p)
	}
}

func TestFakeNewsletterMyReactionAndViews(t *testing.T) {
	f := NewFake()
	ctx := context.Background()
	jid := "111111@newsletter"
	post := func() NewsletterMessage {
		msgs, _ := f.NewsletterMessages(ctx, jid, 20)
		for _, m := range msgs {
			if m.ID == "n2" {
				return m
			}
		}
		t.Fatal("post n2 missing")
		return NewsletterMessage{}
	}
	if post().MyReaction != "" {
		t.Fatal("no reaction yet")
	}
	_ = f.NewsletterReact(ctx, jid, "n2", 2, "🔥")
	if post().MyReaction != "🔥" {
		t.Fatalf("my reaction not reported: %q", post().MyReaction)
	}
	views := post().Views
	_ = f.NewsletterMarkViewed(ctx, jid, []int64{2})
	_ = f.NewsletterMarkViewed(ctx, jid, []int64{2})
	if post().Views != views+1 {
		t.Fatalf("views: got %d want %d", post().Views, views+1)
	}
}
