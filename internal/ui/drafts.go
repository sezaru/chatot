package ui

import "strings"

// draft is what a chat's entry held when the user left it: the text as
// typed and the picker's name→user map that turns its @names into wire
// mentions on send.
type draft struct {
	text     string
	mentions map[string]string
}

// draftBook keeps one draft per chat so a switch away and back finds the
// text where it was. Blank text drops the chat's entry rather than keeping
// an empty draft around.
type draftBook map[string]draft

// Put records text as jid's draft, or forgets jid when text is blank.
func (b draftBook) Put(jid, text string, mentions map[string]string) {
	if jid == "" {
		return
	}
	if strings.TrimSpace(text) == "" {
		delete(b, jid)
		return
	}
	b[jid] = draft{text: text, mentions: mentions}
}

// Take hands back jid's draft and forgets it; the composer holds it from
// here until the next switch away stashes it again.
func (b draftBook) Take(jid string) draft {
	d := b[jid]
	delete(b, jid)
	return d
}
