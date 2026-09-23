package ui

import "testing"

func TestDraftBookKeepsTextPerChat(t *testing.T) {
	b := draftBook{}
	b.Put("a@s.whatsapp.net", "half a thought", map[string]string{"Ana": "1"})
	b.Put("b@s.whatsapp.net", "other", nil)

	d := b.Take("a@s.whatsapp.net")
	if d.text != "half a thought" {
		t.Errorf("text = %q, want %q", d.text, "half a thought")
	}
	if d.mentions["Ana"] != "1" {
		t.Errorf("mentions = %v, want Ana→1", d.mentions)
	}
	if got := b.Take("b@s.whatsapp.net").text; got != "other" {
		t.Errorf("b text = %q, want %q", got, "other")
	}
}

func TestDraftBookTakeForgets(t *testing.T) {
	b := draftBook{}
	b.Put("a@s.whatsapp.net", "once", nil)
	b.Take("a@s.whatsapp.net")
	if d := b.Take("a@s.whatsapp.net"); d.text != "" || d.mentions != nil {
		t.Errorf("second Take = %+v, want empty", d)
	}
}

func TestDraftBookBlankDropsChat(t *testing.T) {
	b := draftBook{}
	b.Put("a@s.whatsapp.net", "typed", nil)
	b.Put("a@s.whatsapp.net", "   ", nil)
	if _, ok := b["a@s.whatsapp.net"]; ok {
		t.Error("blank Put kept the chat's draft")
	}
	if d := b.Take("a@s.whatsapp.net"); d.text != "" {
		t.Errorf("Take = %q, want empty", d.text)
	}
}

func TestDraftBookIgnoresNoChat(t *testing.T) {
	b := draftBook{}
	b.Put("", "nowhere", nil)
	if len(b) != 0 {
		t.Errorf("book = %v, want empty", b)
	}
}
