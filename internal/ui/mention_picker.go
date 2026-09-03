package ui

import (
	"context"
	"log"
	"sort"
	"strings"
	"unicode"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// mentionCandidate is one person the composer can @mention.
type mentionCandidate struct {
	Name string // display name; "You" for the account itself
	JID  string // full JID, for the row's avatar; "" when unknown
	User string // wire user part: phone digits or a LID
}

// mentionPickerMax is how many matches the popover lists.
const mentionPickerMax = 6

// mentionQuery finds the @fragment the cursor (a rune offset) sits in: the
// last '@' at the start of the text or after whitespace, with no whitespace
// between it and the cursor. start is that '@'s rune offset.
func mentionQuery(text string, cursor int) (start int, frag string, ok bool) {
	r := []rune(text)
	if cursor < 0 || cursor > len(r) {
		return 0, "", false
	}
	for i := cursor - 1; i >= 0; i-- {
		switch {
		case unicode.IsSpace(r[i]):
			return 0, "", false
		case r[i] == '@':
			if i > 0 && !unicode.IsSpace(r[i-1]) {
				return 0, "", false // an e-mail address, not a mention
			}
			return i, string(r[i+1 : cursor]), true
		}
	}
	return 0, "", false
}

// filterMentions ranks the candidates matching frag: a name word or the
// number starting with frag first, then any containing it; an empty frag
// lists everyone. Never more than max.
func filterMentions(cands []mentionCandidate, frag string, max int) []mentionCandidate {
	frag = strings.ToLower(strings.TrimSpace(frag))
	type scored struct {
		c    mentionCandidate
		rank int
	}
	var out []scored
	for _, c := range cands {
		name := strings.ToLower(c.Name)
		rank := -1
		switch {
		case frag == "":
			rank = 1
		case strings.HasPrefix(name, frag) || strings.HasPrefix(c.User, frag):
			rank = 0
		case wordPrefix(name, frag):
			rank = 1
		case strings.Contains(name, frag) || strings.Contains(c.User, frag):
			rank = 2
		}
		if rank >= 0 {
			out = append(out, scored{c, rank})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rank < out[j].rank })
	res := make([]mentionCandidate, 0, len(out))
	for _, s := range out {
		if len(res) == max {
			break
		}
		res = append(res, s.c)
	}
	return res
}

func wordPrefix(name, frag string) bool {
	for _, w := range strings.Fields(name) {
		if strings.HasPrefix(w, frag) {
			return true
		}
	}
	return false
}

// applyMention replaces the runes [start, cursor) of text with "@Name "
// and returns the new text and cursor.
func applyMention(text string, start, cursor int, name string) (string, int) {
	r := []rune(text)
	if start < 0 || cursor > len(r) || start > cursor {
		return text, cursor
	}
	ins := []rune("@" + name + " ")
	out := append(append(append([]rune{}, r[:start]...), ins...), r[cursor:]...)
	return string(out), start + len(ins)
}

// wireMentions rewrites every "@Name" the picker inserted to WhatsApp's
// "@user" wire form. Longer names go first so "@Ken Thompson" is not
// half-rewritten by "@Ken".
func wireMentions(text string, names map[string]string) string {
	if len(names) == 0 || !strings.Contains(text, "@") {
		return text
	}
	keys := make([]string, 0, len(names))
	for k := range names {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if len(keys[i]) != len(keys[j]) {
			return len(keys[i]) > len(keys[j])
		}
		return keys[i] < keys[j]
	})
	for _, k := range keys {
		text = strings.ReplaceAll(text, "@"+k, "@"+names[k])
	}
	return text
}

// mentionPeople lists who can be mentioned in chat jid: a group's members
// (named through the contact store) or, in a 1:1 chat, the peer and the
// account itself. Runs network-side calls; not for the main loop.
func mentionPeople(ctx context.Context, c client.Client, jid, chatName string) []mentionCandidate {
	own := c.OwnJID()
	name := func(j string) string {
		if isOwnJID(j, own) {
			return "You"
		}
		if n := c.ContactName(j); n != "" {
			return n
		}
		return "+" + bareJIDUser(j)
	}
	if !strings.HasSuffix(jid, "@g.us") {
		if isOwnJID(jid, own) {
			// The account's own chat: one person in it.
			return []mentionCandidate{{Name: "You", User: bareJIDUser(own), JID: nonADJID(own)}}
		}
		peer := chatName
		if peer == "" {
			peer = name(jid)
		}
		out := []mentionCandidate{{Name: peer, User: bareJIDUser(jid), JID: jid}}
		if own != "" {
			out = append(out, mentionCandidate{Name: "You", User: bareJIDUser(own), JID: nonADJID(own)})
		}
		return out
	}
	info, err := c.GroupInfo(ctx, jid)
	if err != nil || info == nil {
		log.Printf("chatot: mention picker: group info: %v", err)
		return nil
	}
	out := make([]mentionCandidate, 0, len(info.Participants))
	for _, p := range info.Participants {
		out = append(out, mentionCandidate{Name: name(p.JID), User: bareJIDUser(p.JID), JID: nonADJID(p.JID)})
	}
	sort.SliceStable(out, func(i, j int) bool {
		// Others first, alphabetically; You last.
		yi, yj := out[i].Name == "You", out[j].Name == "You"
		if yi != yj {
			return yj
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// ---- the popover ----------------------------------------------------------

// mentionPicker is the composer's @ autocomplete: a card above the entry
// listing the chat's people that match the fragment being typed. It never
// takes the focus; the entry's own key controller steers it.
type mentionPicker struct {
	pop     *gtk.Popover
	box     *gtk.Box
	rows    []mentionCandidate
	btns    []*gtk.Button
	sel     int
	start   int // rune offset of the '@' being completed
	pick    func(mentionCandidate)
	c       client.Client // for the rows' avatars
	avatars *avatarCache
}

func newMentionPicker(anchor gtk.Widgetter, c client.Client, avatars *avatarCache, pick func(mentionCandidate)) *mentionPicker {
	pop := gtk.NewPopover()
	pop.SetHasArrow(false)
	pop.SetAutohide(false)
	pop.SetPosition(gtk.PosTop)
	pop.AddCSSClass("chatot-menu")
	pop.AddCSSClass("chatot-mention-pop")
	pop.SetParent(anchor)
	box := gtk.NewBox(gtk.OrientationVertical, 1)
	box.SetSizeRequest(240, -1)
	pop.SetChild(box)
	return &mentionPicker{pop: pop, box: box, pick: pick, c: c, avatars: avatars}
}

// Show lists rows (hiding on none) with the first selected.
func (m *mentionPicker) Show(rows []mentionCandidate, start int) {
	m.rows = rows
	m.start = start
	m.sel = 0
	removeAllChildren(m.box)
	m.btns = m.btns[:0]
	if len(rows) == 0 {
		m.Hide()
		return
	}
	for i, c := range rows {
		btn := m.row(c)
		cand := c
		btn.ConnectClicked(func() { m.pick(cand) })
		m.box.Append(btn)
		m.btns = append(m.btns, btn)
		if i == 0 {
			btn.AddCSSClass("chatot-mention-row-sel")
		}
	}
	// Left-aligned over the entry, where the @ is, not centred on it.
	rect := gdk.NewRectangle(0, 0, 1, gtk.BaseWidget(m.pop.Parent()).AllocatedHeight())
	m.pop.SetPointingTo(&rect)
	if !m.pop.Visible() {
		m.pop.Popup()
	}
}

func (m *mentionPicker) row(c mentionCandidate) *gtk.Button {
	btn := gtk.NewButton()
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-mention-row")
	btn.SetFocusOnClick(false)
	row := gtk.NewBox(gtk.OrientationHorizontal, 9)
	var avatar gtk.Widgetter
	if c.JID != "" && m.c != nil && m.avatars != nil {
		avatar = buildAvatar(m.c, m.avatars, c.JID, initialFor(c.Name), 26)
	} else {
		avatar = newAvatarInitial(c.User, initialFor(c.Name), 26)
	}
	gtk.BaseWidget(avatar).SetVAlign(gtk.AlignCenter)
	row.Append(avatar)
	col := gtk.NewBox(gtk.OrientationVertical, 0)
	name := gtk.NewLabel(c.Name)
	name.SetXAlign(0)
	name.SetEllipsize(pango.EllipsizeEnd)
	name.AddCSSClass("chatot-mention-name")
	col.Append(name)
	sub := gtk.NewLabel(formatPhoneDisplay("+" + c.User))
	if c.Name == "You" || len(c.User) > 13 {
		// A LID is not a phone number; there is nothing useful to show
		// under the name.
		sub.SetVisible(false)
	}
	sub.SetXAlign(0)
	sub.AddCSSClass("chatot-mention-sub")
	col.Append(sub)
	row.Append(col)
	btn.SetChild(row)
	return btn
}

func (m *mentionPicker) Hide() {
	if m.pop.Visible() {
		m.pop.Popdown()
	}
	m.rows = nil
}

func (m *mentionPicker) Visible() bool { return m.pop.Visible() && len(m.rows) > 0 }

// move steps the selection by delta, wrapping.
func (m *mentionPicker) move(delta int) {
	if len(m.btns) == 0 {
		return
	}
	m.btns[m.sel].RemoveCSSClass("chatot-mention-row-sel")
	m.sel = (m.sel + delta + len(m.btns)) % len(m.btns)
	m.btns[m.sel].AddCSSClass("chatot-mention-row-sel")
}

// Key handles a key press on the entry while the picker is up; true when
// consumed.
func (m *mentionPicker) Key(keyval uint) bool {
	if !m.Visible() {
		return false
	}
	switch keyval {
	case gdk.KEY_Down:
		m.move(1)
	case gdk.KEY_Up:
		m.move(-1)
	case gdk.KEY_Return, gdk.KEY_KP_Enter, gdk.KEY_Tab:
		m.pick(m.rows[m.sel])
	case gdk.KEY_Escape:
		m.Hide()
	default:
		return false
	}
	return true
}

// ---- composer wiring -------------------------------------------------------

// refreshMentionPicker shows or hides the picker for the entry's current
// text and cursor. People are fetched once per chat, off the main loop;
// the picker appears when they land if the cursor is still in an @.
func (c *Composer) refreshMentionPicker() {
	if c.mentions == nil {
		return
	}
	jid := c.state.jid
	start, frag, ok := mentionQuery(c.entry.Text(), c.entry.Position())
	if !ok || jid == "" {
		c.mentions.Hide()
		return
	}
	if c.peopleJID != jid {
		if c.peopleLoading == jid {
			return
		}
		c.peopleLoading = jid
		chatName := c.chatName
		go func() {
			people := mentionPeople(context.Background(), c.c, jid, chatName)
			glib.IdleAdd(func() {
				if c.peopleLoading == jid {
					c.peopleLoading = ""
				}
				c.people, c.peopleJID = people, jid
				if c.state.jid == jid {
					c.refreshMentionPicker()
				}
			})
		}()
		return
	}
	c.mentions.Show(filterMentions(c.people, frag, mentionPickerMax), start)
}

// ShowMentionPicker types "@"+frag into the entry and opens the picker on
// it — a dev/screenshot hook.
func (c *Composer) ShowMentionPicker(frag string) {
	c.entry.GrabFocus()
	c.entry.SetText("@" + frag)
	c.entry.SetPosition(len([]rune(frag)) + 1)
	c.refreshMentionPicker()
}

// pickMention completes the fragment with cand's name and remembers the
// name so submit can put the wire form back.
func (c *Composer) pickMention(cand mentionCandidate) {
	text, cursor := applyMention(c.entry.Text(), c.mentions.start, c.entry.Position(), cand.Name)
	if c.mentionNames == nil {
		c.mentionNames = map[string]string{}
	}
	c.mentionNames[cand.Name] = cand.User
	c.mentions.Hide()
	c.entry.GrabFocus()
	c.entry.SetText(text)
	c.entry.SetPosition(cursor)
}
