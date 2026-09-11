package client

import (
	"encoding/json"
	"strings"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

// choiceMarker prefixes each button a carousel card offers, so a bubble
// whose cards cannot be tapped still tells the reader what the phone would
// have shown ("▸ Ver comprovante"). Every other shape keeps its buttons
// structured, in Message.Choices.
const choiceMarker = "▸ "

// templateChoices reads a business template (Cloud API "template" and
// older "hsm" messages): the text a person reads — title, body, footer —
// and the buttons it offers. WhatsApp delivers the same template in three
// shapes — hydrated, four-row and, for the newer button/carousel
// templates, an interactive message nested inside — and all three land
// here.
func templateChoices(t *waProto.TemplateMessage) (string, *Choices) {
	if im := t.GetInteractiveMessageTemplate(); im != nil {
		return interactiveChoices(im)
	}
	h := t.GetHydratedTemplate()
	if h == nil {
		h = t.GetHydratedFourRowTemplate()
	}
	if h != nil {
		ch := &Choices{Source: "template"}
		for i, b := range h.GetHydratedButtons() {
			ch.addButton(hydratedButton(b, i))
		}
		return joinNonEmpty("\n", h.GetHydratedTitleText(), h.GetHydratedContentText(), h.GetHydratedFooterText()), ch.orNil()
	}
	if f := t.GetFourRowTemplate(); f != nil {
		ch := &Choices{Source: "template"}
		for i, b := range f.GetButtons() {
			ch.addButton(templateButton(b, i))
		}
		return joinNonEmpty("\n", hsmText(f.GetHighlyStructuredMessage()), hsmText(f.GetContent()), hsmText(f.GetFooter())), ch.orNil()
	}
	return "", nil
}

// hydratedButton is one button of a template the phone already resolved.
func hydratedButton(b *waProto.HydratedTemplateButton, i int) ChoiceButton {
	index := i
	if b.Index != nil {
		index = int(b.GetIndex())
	}
	switch {
	case b.GetUrlButton() != nil:
		u := b.GetUrlButton()
		return ChoiceButton{Label: u.GetDisplayText(), Kind: "url", URL: u.GetURL(), Index: index}
	case b.GetCallButton() != nil:
		c := b.GetCallButton()
		return ChoiceButton{Label: c.GetDisplayText(), Kind: "call", Phone: c.GetPhoneNumber(), Index: index}
	default:
		q := b.GetQuickReplyButton()
		return ChoiceButton{ID: q.GetID(), Label: q.GetDisplayText(), Kind: "reply", Index: index}
	}
}

// templateButton is one button of an unresolved four-row template, whose
// texts are locale fallbacks.
func templateButton(b *waProto.TemplateButton, i int) ChoiceButton {
	index := i
	if b.Index != nil {
		index = int(b.GetIndex())
	}
	switch {
	case b.GetUrlButton() != nil:
		u := b.GetUrlButton()
		return ChoiceButton{Label: hsmText(u.GetDisplayText()), Kind: "url", URL: hsmText(u.GetURL()), Index: index}
	case b.GetCallButton() != nil:
		c := b.GetCallButton()
		return ChoiceButton{Label: hsmText(c.GetDisplayText()), Kind: "call", Phone: hsmText(c.GetPhoneNumber()), Index: index}
	default:
		q := b.GetQuickReplyButton()
		return ChoiceButton{ID: q.GetID(), Label: hsmText(q.GetDisplayText()), Kind: "reply", Index: index}
	}
}

// hsmText returns the readable text of a HighlyStructuredMessage: the
// hydrated body when the phone already resolved the template, otherwise
// the fallback locale string.
func hsmText(h *waProto.HighlyStructuredMessage) string {
	if h == nil {
		return ""
	}
	if inner := h.GetHydratedHsm(); inner != nil {
		text, _ := templateChoices(inner)
		return text
	}
	return h.GetFallbackLg()
}

// interactiveChoices reads an InteractiveMessage (Cloud API "interactive"
// buttons, lists, CTA links and flows): header, body and footer as text,
// the native-flow buttons as choices. A carousel of them stays text — its
// cards cannot be tapped here — with one marked line per card button.
func interactiveChoices(im *waProto.InteractiveMessage) (string, *Choices) {
	hdr := im.GetHeader()
	if c := im.GetCarouselMessage(); c != nil && len(c.GetCards()) > 0 {
		var cards []string
		for _, card := range c.GetCards() {
			text, ch := interactiveChoices(card)
			if s := withChoices(text, ch.labels()); s != "" {
				cards = append(cards, s)
			}
		}
		return joinNonEmpty("\n", joinNonEmpty("\n", hdr.GetTitle(), im.GetBody().GetText()), strings.Join(cards, "\n\n")), nil
	}
	ch := &Choices{Source: "interactive"}
	for i, b := range im.GetNativeFlowMessage().GetButtons() {
		ch.addNativeFlow(b.GetName(), b.GetButtonParamsJSON(), i)
	}
	return joinNonEmpty("\n", hdr.GetTitle(), hdr.GetSubtitle(), im.GetBody().GetText(), im.GetFooter().GetText()), ch.orNil()
}

// nativeFlowParams is the JSON blob a native-flow button carries its
// content in: the label and target of a quick reply or CTA, or a list
// picker's title and sections.
type nativeFlowParams struct {
	DisplayText string `json:"display_text"`
	ID          string `json:"id"`
	URL         string `json:"url"`
	PhoneNumber string `json:"phone_number"`
	CopyCode    string `json:"copy_code"`
	Title       string `json:"title"`
	// FlowCTA labels a WhatsApp Flow button ("galaxy_message"); Link is
	// an in-app webview button's target ("open_webview").
	FlowCTA string `json:"flow_cta"`
	Link    struct {
		URL string `json:"url"`
	} `json:"link"`
	Sections []struct {
		Title string `json:"title"`
		Rows  []struct {
			Title       string `json:"title"`
			Description string `json:"description"`
			ID          string `json:"id"`
		} `json:"rows"`
	} `json:"sections"`
}

// addNativeFlow files a native-flow button by its flow name: quick replies
// and the URL/call/copy CTAs become buttons, a webview button a URL one, a
// single-select becomes the list picker, a WhatsApp Flow ("galaxy", a form
// only the phone can run) a flow button, and an unknown flow still gets a
// reply button named after it so the reader sees it was there.
func (ch *Choices) addNativeFlow(name, paramsJSON string, i int) {
	var p nativeFlowParams
	_ = json.Unmarshal([]byte(paramsJSON), &p)
	label := firstNonEmpty(p.DisplayText, p.Title, name)
	switch name {
	case "cta_url":
		ch.addButton(ChoiceButton{Label: label, Kind: "url", URL: p.URL, Index: i})
	case "open_webview":
		ch.addButton(ChoiceButton{Label: label, Kind: "url", URL: firstNonEmpty(p.Link.URL, p.URL), Index: i})
	case "galaxy_message":
		ch.addButton(ChoiceButton{Label: firstNonEmpty(p.FlowCTA, p.DisplayText, "Open form"), Kind: "flow", Index: i})
	case "cta_call":
		ch.addButton(ChoiceButton{Label: label, Kind: "call", Phone: p.PhoneNumber, Index: i})
	case "cta_copy":
		ch.addButton(ChoiceButton{Label: label, Kind: "copy", CopyText: p.CopyCode, Index: i})
	case "single_select":
		list := &ChoiceList{Title: firstNonEmpty(p.Title, "Choose")}
		for _, sec := range p.Sections {
			s := ChoiceSection{Title: sec.Title}
			for _, r := range sec.Rows {
				if strings.TrimSpace(r.Title) != "" {
					s.Rows = append(s.Rows, ChoiceRow{ID: r.ID, Title: r.Title, Description: r.Description})
				}
			}
			if len(s.Rows) > 0 {
				list.Sections = append(list.Sections, s)
			}
		}
		if len(list.Sections) > 0 && ch.List == nil {
			ch.List = list
		}
	default:
		ch.addButton(ChoiceButton{ID: p.ID, Label: label, Kind: "reply", Index: i})
	}
}

// buttonsChoices reads the legacy ButtonsMessage: header text, body and
// footer as text, one reply button each.
func buttonsChoices(b *waProto.ButtonsMessage) (string, *Choices) {
	ch := &Choices{Source: "buttons"}
	for i, btn := range b.GetButtons() {
		ch.addButton(ChoiceButton{
			ID:    btn.GetButtonID(),
			Label: firstNonEmpty(btn.GetButtonText().GetDisplayText(), btn.GetNativeFlowInfo().GetName()),
			Kind:  "reply", Index: i,
		})
	}
	return joinNonEmpty("\n", b.GetText(), b.GetContentText(), b.GetFooterText()), ch.orNil()
}

// listChoices reads a ListMessage: title, description and footer as text,
// the picker button and its sections as the list. A product list has no
// rows to pick from and so offers nothing.
func listChoices(l *waProto.ListMessage) (string, *Choices) {
	list := &ChoiceList{Title: firstNonEmpty(l.GetButtonText(), "Choose")}
	for _, sec := range l.GetSections() {
		s := ChoiceSection{Title: sec.GetTitle()}
		for _, r := range sec.GetRows() {
			if strings.TrimSpace(r.GetTitle()) != "" {
				s.Rows = append(s.Rows, ChoiceRow{ID: r.GetRowID(), Title: r.GetTitle(), Description: r.GetDescription()})
			}
		}
		if len(s.Rows) > 0 {
			list.Sections = append(list.Sections, s)
		}
	}
	ch := &Choices{Source: "list"}
	if len(list.Sections) > 0 {
		ch.List = list
	}
	return joinNonEmpty("\n", l.GetTitle(), l.GetDescription(), l.GetFooterText()), ch.orNil()
}

// addButton keeps b unless it has no label to show.
func (ch *Choices) addButton(b ChoiceButton) {
	if strings.TrimSpace(b.Label) == "" {
		return
	}
	ch.Buttons = append(ch.Buttons, b)
}

// orNil is ch, or nil when it offers nothing to tap.
func (ch *Choices) orNil() *Choices {
	if ch == nil || (len(ch.Buttons) == 0 && ch.List == nil) {
		return nil
	}
	return ch
}

// labels lists what the choices show, one line each: the buttons, then the
// list's picker title and its rows.
func (ch *Choices) labels() []string {
	if ch == nil {
		return nil
	}
	var out []string
	for _, b := range ch.Buttons {
		out = append(out, b.Label)
	}
	if ch.List != nil {
		out = append(out, ch.List.Title)
		for _, sec := range ch.List.Sections {
			for _, r := range sec.Rows {
				out = append(out, r.Title)
			}
		}
	}
	return out
}

// withChoices appends one marked line per non-empty label to body.
func withChoices(body string, labels []string) string {
	var lines []string
	for _, l := range labels {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, choiceMarker+l)
		}
	}
	return joinNonEmpty("\n", body, strings.Join(lines, "\n"))
}

// choiceReply builds the message that answers a business message's
// choices with sel, in the shape WhatsApp expects for the shape the offer
// came in (source, see Choices): the legacy buttons and list get their own
// response messages, a hydrated template its button reply, and an
// interactive message a native-flow response whose JSON names the pick.
// ctx quotes the offer, which is how the business ties the answer back.
func choiceReply(source string, sel ChoiceSelection, ctx *waProto.ContextInfo) *waProto.Message {
	switch {
	case source == "list" || (source == "buttons" && sel.IsRow):
		return &waProto.Message{ListResponseMessage: &waProto.ListResponseMessage{
			Title:             proto.String(sel.Label),
			ListType:          waProto.ListResponseMessage_SINGLE_SELECT.Enum(),
			SingleSelectReply: &waProto.ListResponseMessage_SingleSelectReply{SelectedRowID: proto.String(sel.ID)},
			Description:       optString(sel.Description),
			ContextInfo:       ctx,
		}}
	case source == "buttons":
		return &waProto.Message{ButtonsResponseMessage: &waProto.ButtonsResponseMessage{
			SelectedButtonID: proto.String(sel.ID),
			Response:         &waProto.ButtonsResponseMessage_SelectedDisplayText{SelectedDisplayText: sel.Label},
			Type:             waProto.ButtonsResponseMessage_DISPLAY_TEXT.Enum(),
			ContextInfo:      ctx,
		}}
	case source == "template" && !sel.IsRow:
		return &waProto.Message{TemplateButtonReplyMessage: &waProto.TemplateButtonReplyMessage{
			SelectedID:          proto.String(sel.ID),
			SelectedDisplayText: proto.String(sel.Label),
			SelectedIndex:       proto.Uint32(uint32(sel.Index)),
			ContextInfo:         ctx,
		}}
	}
	name, params := "quick_reply", map[string]string{"id": sel.ID, "display_text": sel.Label}
	if sel.IsRow {
		name = "menu_options"
		params = map[string]string{"id": sel.ID, "title": sel.Label, "description": sel.Description}
	}
	b, _ := json.Marshal(params)
	return &waProto.Message{InteractiveResponseMessage: &waProto.InteractiveResponseMessage{
		Body: &waProto.InteractiveResponseMessage_Body{
			Text:   proto.String(sel.Label),
			Format: waProto.InteractiveResponseMessage_Body_DEFAULT.Enum(),
		},
		InteractiveResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage_{
			NativeFlowResponseMessage: &waProto.InteractiveResponseMessage_NativeFlowResponseMessage{
				Name:       proto.String(name),
				ParamsJSON: proto.String(string(b)),
				Version:    proto.Int32(1),
			},
		},
		ContextInfo: ctx,
	}}
}

// optString is s as a proto string, or nil when empty so the field stays
// unset.
func optString(s string) *string {
	if s == "" {
		return nil
	}
	return proto.String(s)
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return p
		}
	}
	return ""
}
