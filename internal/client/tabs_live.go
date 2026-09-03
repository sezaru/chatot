package client

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"go.mau.fi/whatsmeow/types"
	"google.golang.org/protobuf/proto"
)

// statusTTL is how long WhatsApp keeps a status update up.
const statusTTL = 24 * time.Hour

// SetPrivacySetting changes one account privacy setting on the server.
func (w *Whatsmeow) SetPrivacySetting(ctx context.Context, name, value string) error {
	typ, val, err := privacySettingType(name, value)
	if err != nil {
		return err
	}
	if _, err := w.wa.SetPrivacySetting(ctx, typ, val); err != nil {
		return fmt.Errorf("chatot/client: set privacy %s: %w", name, err)
	}
	return nil
}

// MarkStatusViewed flags poster's updates as read locally and, when notify
// is set, sends the read receipt WhatsApp counts as a status view (the
// receipt goes to the poster, keyed on the status broadcast).
func (w *Whatsmeow) MarkStatusViewed(ctx context.Context, poster string, msgIDs []string, notify bool) error {
	if len(msgIDs) == 0 {
		return nil
	}
	if err := w.store.SetMessagesStatus(statusBroadcastJID, msgIDs, MessageStatusRead); err != nil {
		w.log.Warnf("chatot/client: mark status viewed locally: %v", err)
	}
	if !notify {
		return nil
	}
	p, err := types.ParseJID(poster)
	if err != nil {
		return fmt.Errorf("chatot/client: parse status poster jid: %w", err)
	}
	ids := make([]types.MessageID, len(msgIDs))
	for i, id := range msgIDs {
		ids[i] = types.MessageID(id)
	}
	if err := w.wa.MarkRead(ctx, ids, time.Now(), types.StatusBroadcastJID, p); err != nil {
		return fmt.Errorf("chatot/client: status view receipt: %w", err)
	}
	return nil
}

// StatusViewers lists who viewed our update msgID, from the read receipts
// their devices sent. WhatsApp offers no query for this, so a linked
// device only knows about views that arrived while it was connected.
func (w *Whatsmeow) StatusViewers(msgID string) ([]StatusViewer, error) {
	rows, err := w.store.ReadReceiptsByMsgID(msgID)
	if err != nil {
		return nil, fmt.Errorf("chatot/client: status viewers: %w", err)
	}
	own := w.ownJID()
	out := make([]StatusViewer, 0, len(rows))
	for _, r := range rows {
		if r.ReaderJID == own {
			continue
		}
		out = append(out, StatusViewer{JID: r.ReaderJID, TS: r.TS})
	}
	return out, nil
}

// MuteStatus files poster under Muted updates locally and pushes the
// userStatusMute app-state patch so the phone and other devices agree.
//
// Live-unverifiable risk: whatsmeow decodes that index into
// events.UserStatusMute but ships no builder for it, so the patch (regular-
// high collection, version 1, a UserStatusMuteAction) is assembled here from
// the decoder's shape. A rejected patch is logged; the local mute stands.
func (w *Whatsmeow) MuteStatus(ctx context.Context, poster string, mute bool) error {
	target, err := types.ParseJID(poster)
	if err != nil {
		return fmt.Errorf("chatot/client: parse status poster jid: %w", err)
	}
	if err := w.store.SetStatusMuted(poster, mute); err != nil {
		return fmt.Errorf("chatot/client: mute status locally: %w", err)
	}
	patch := appstate.PatchInfo{
		Type: appstate.WAPatchRegularHigh,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexUserStatusMute, target.String()},
			Version: 1,
			Value: &waSyncAction.SyncActionValue{
				UserStatusMuteAction: &waSyncAction.UserStatusMuteAction{Muted: proto.Bool(mute)},
			},
		}},
	}
	if err := w.wa.SendAppState(ctx, patch); err != nil {
		w.log.Warnf("chatot/client: sync status mute for %s: %v", poster, err)
	}
	w.pushEvent(Event{Kind: EventChatUpdate, ChatUpdate: &ChatUpdate{JID: poster}})
	return nil
}

// MutedStatusPosters lists the posters muted locally or from another device.
func (w *Whatsmeow) MutedStatusPosters() ([]string, error) {
	return w.store.MutedStatusPosters()
}

// HideStatusFrom adds jid to the status privacy exclusions: on the
// "my contacts" or "contacts except" audience it joins the deny list, on a
// "only share with" audience it leaves the allow list.
//
// Live-unverifiable risk: the status_privacy app-state index has no
// whatsmeow builder either; the StatusPrivacyAction is assembled from the
// proto (mode + user list) and pushed on the regular-high collection.
func (w *Whatsmeow) HideStatusFrom(ctx context.Context, jid string) error {
	target, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", jid, err)
	}
	lists, err := w.wa.GetStatusPrivacy(ctx)
	if err != nil {
		return fmt.Errorf("chatot/client: get status privacy: %w", err)
	}
	mode, users := statusPrivacyAfterHiding(lists, target)
	patch := appstate.PatchInfo{
		Type: appstate.WAPatchRegularHigh,
		Mutations: []appstate.MutationInfo{{
			Index:   []string{appstate.IndexStatusPrivacy},
			Version: 1,
			Value: &waSyncAction.SyncActionValue{
				StatusPrivacy: &waSyncAction.StatusPrivacyAction{Mode: mode.Enum(), UserJID: users},
			},
		}},
	}
	if err := w.wa.SendAppState(ctx, patch); err != nil {
		return fmt.Errorf("chatot/client: set status privacy: %w", err)
	}
	return nil
}

// statusPrivacyAfterHiding works out the audience to push once target is
// excluded: the current default list decides whether target joins a deny
// list or leaves an allow list.
func statusPrivacyAfterHiding(lists []types.StatusPrivacy, target types.JID) (waSyncAction.StatusPrivacyAction_StatusDistributionMode, []string) {
	var current types.StatusPrivacy
	for _, l := range lists {
		if l.IsDefault {
			current = l
			break
		}
	}
	if current.Type == "" && len(lists) > 0 {
		current = lists[0]
	}
	users := make([]string, 0, len(current.List)+1)
	for _, j := range current.List {
		if j.User != target.User {
			users = append(users, j.String())
		}
	}
	if current.Type == types.StatusPrivacyTypeWhitelist {
		return waSyncAction.StatusPrivacyAction_ALLOW_LIST, users
	}
	return waSyncAction.StatusPrivacyAction_DENY_LIST, append(users, target.String())
}

// NewsletterMarkViewed reports the posts with serverIDs as viewed.
func (w *Whatsmeow) NewsletterMarkViewed(ctx context.Context, jid string, serverIDs []int64) error {
	if len(serverIDs) == 0 {
		return nil
	}
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse newsletter jid: %w", err)
	}
	ids := make([]types.MessageServerID, len(serverIDs))
	for i, id := range serverIDs {
		ids[i] = types.MessageServerID(id)
	}
	if err := w.wa.NewsletterMarkViewed(ctx, j, ids); err != nil {
		return fmt.Errorf("chatot/client: newsletter mark viewed: %w", err)
	}
	return nil
}

// NewsletterSubscribeLive subscribes to jid's live view/reaction updates.
func (w *Whatsmeow) NewsletterSubscribeLive(ctx context.Context, jid string) error {
	j, err := types.ParseJID(jid)
	if err != nil {
		return fmt.Errorf("chatot/client: parse newsletter jid: %w", err)
	}
	if _, err := w.wa.NewsletterSubscribeLiveUpdates(ctx, j); err != nil {
		return fmt.Errorf("chatot/client: newsletter live updates: %w", err)
	}
	return nil
}

// mexRecommendedNewsletters is the server's recommended-channels query,
// which whatsmeow names (input: limit + country codes, output:
// xwa2_newsletters_recommended) but doesn't wrap.
const mexRecommendedNewsletters = "7263823273662354"

// DiscoverNewsletters fetches the server's recommended channels for the
// account's country and filters them by query locally (the query takes no
// search text). whatsmeow has no wrapper, so the Mex query goes through its
// exported internals and the reply is parsed tolerantly: whichever array of
// channel metadata objects the response carries.
//
// Live-unverifiable risk: the reply's envelope and the country code format
// aren't confirmed against a live account; a reply without a channel list
// surfaces as an error and the UI falls back to Follow with a link.
func (w *Whatsmeow) DiscoverNewsletters(ctx context.Context, query string) ([]Newsletter, error) {
	input := map[string]any{"limit": 50, "country_codes": countryCodesFor(w.ownJID())}
	raw, err := w.wa.DangerousInternals().SendMexIQ(ctx, mexRecommendedNewsletters, map[string]any{"input": input})
	if err != nil {
		return nil, fmt.Errorf("chatot/client: channel directory: %w", err)
	}
	metas := parseNewsletterList(raw)
	if metas == nil {
		return nil, fmt.Errorf("chatot/client: channel directory: no channel list in reply")
	}
	following := map[string]bool{}
	if subs, err := w.wa.GetSubscribedNewsletters(ctx); err == nil {
		for _, s := range subs {
			if s != nil {
				following[s.ID.String()] = true
			}
		}
	}
	return filterNewsletterDirectory(metas, following, query), nil
}

// filterNewsletterDirectory maps directory metadata to Newsletters, flags
// the followed ones, keeps only those matching query (name or description,
// case-insensitive) and orders them most followed first.
func filterNewsletterDirectory(metas []*types.NewsletterMetadata, following map[string]bool, query string) []Newsletter {
	q := strings.ToLower(strings.TrimSpace(query))
	out := make([]Newsletter, 0, len(metas))
	for _, m := range metas {
		if m == nil {
			continue
		}
		n := newsletterFromMeta(m)
		n.Following = following[n.ID]
		if q != "" && !strings.Contains(strings.ToLower(n.Name), q) && !strings.Contains(strings.ToLower(n.Description), q) {
			continue
		}
		out = append(out, n)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Subscribers != out[j].Subscribers {
			return out[i].Subscribers > out[j].Subscribers
		}
		return out[i].Name < out[j].Name
	})
	return out
}

// parseNewsletterList digs the channel list out of a Mex reply: the first
// array whose elements are channel metadata objects (id + thread_metadata),
// unwrapping GraphQL-style {node: …} edges. Elements that don't decode are
// skipped rather than failing the whole list.
func parseNewsletterList(raw []byte) []*types.NewsletterMetadata {
	var root any
	if err := json.Unmarshal(raw, &root); err != nil {
		return nil
	}
	arr := findNewsletterArray(root)
	if arr == nil {
		return nil
	}
	out := make([]*types.NewsletterMetadata, 0, len(arr))
	for _, el := range arr {
		b, err := json.Marshal(el)
		if err != nil {
			continue
		}
		var m types.NewsletterMetadata
		if err := json.Unmarshal(b, &m); err != nil || m.ID.IsEmpty() {
			continue
		}
		out = append(out, &m)
	}
	return out
}

func isNewsletterObject(v any) bool {
	m, ok := v.(map[string]any)
	if !ok {
		return false
	}
	_, hasID := m["id"]
	_, hasMeta := m["thread_metadata"]
	return hasID && hasMeta
}

func findNewsletterArray(v any) []any {
	switch t := v.(type) {
	case []any:
		if len(t) > 0 {
			if isNewsletterObject(t[0]) {
				return t
			}
			if edge, ok := t[0].(map[string]any); ok && isNewsletterObject(edge["node"]) {
				nodes := make([]any, 0, len(t))
				for _, e := range t {
					if em, ok := e.(map[string]any); ok && isNewsletterObject(em["node"]) {
						nodes = append(nodes, em["node"])
					}
				}
				return nodes
			}
		}
		for _, el := range t {
			if r := findNewsletterArray(el); r != nil {
				return r
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if r := findNewsletterArray(t[k]); r != nil {
				return r
			}
		}
	}
	return nil
}

// phoneCountryCodes maps international dialling prefixes to ISO country
// codes, for the directory query's country filter. Longest prefix wins.
var phoneCountryCodes = map[string]string{
	"1": "US", "7": "RU", "20": "EG", "27": "ZA", "30": "GR", "31": "NL", "32": "BE", "33": "FR",
	"34": "ES", "36": "HU", "39": "IT", "40": "RO", "41": "CH", "43": "AT", "44": "GB", "45": "DK",
	"46": "SE", "47": "NO", "48": "PL", "49": "DE", "51": "PE", "52": "MX", "54": "AR", "55": "BR",
	"56": "CL", "57": "CO", "58": "VE", "60": "MY", "61": "AU", "62": "ID", "63": "PH", "64": "NZ",
	"65": "SG", "66": "TH", "81": "JP", "82": "KR", "84": "VN", "86": "CN", "90": "TR", "91": "IN",
	"92": "PK", "212": "MA", "234": "NG", "244": "AO", "254": "KE", "258": "MZ", "351": "PT",
	"353": "IE", "358": "FI", "380": "UA", "420": "CZ", "966": "SA", "971": "AE", "972": "IL",
}

// countryCodesFor derives the directory's country filter from the account's
// phone number; an unrecognised prefix sends an empty filter.
func countryCodesFor(ownJID string) []string {
	phone := strings.SplitN(ownJID, "@", 2)[0]
	phone = strings.SplitN(phone, ":", 2)[0]
	for n := 3; n >= 1; n-- {
		if len(phone) >= n {
			if cc, ok := phoneCountryCodes[phone[:n]]; ok {
				return []string{cc}
			}
		}
	}
	return []string{}
}
