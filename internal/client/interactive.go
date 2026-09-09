package client

import (
	"encoding/json"
	"strings"

	waProto "go.mau.fi/whatsmeow/binary/proto"
)

// choiceMarker prefixes each button or list row a business message offers,
// so a bubble whose whole point is the choices still tells the reader what
// the phone would have shown ("▸ Ver comprovante").
const choiceMarker = "▸ "

// templateText flattens a business template (Cloud API "template" and
// older "hsm" messages) to the text a person reads: title, body, footer and
// one line per button. WhatsApp delivers the same template in three
// shapes — hydrated, four-row and, for the newer button/carousel
// templates, an interactive message nested inside — and all three land
// here.
func templateText(t *waProto.TemplateMessage) string {
	if im := t.GetInteractiveMessageTemplate(); im != nil {
		return interactiveText(im)
	}
	h := t.GetHydratedTemplate()
	if h == nil {
		h = t.GetHydratedFourRowTemplate()
	}
	if h != nil {
		var labels []string
		for _, b := range h.GetHydratedButtons() {
			labels = append(labels, firstNonEmpty(
				b.GetQuickReplyButton().GetDisplayText(),
				b.GetUrlButton().GetDisplayText(),
				b.GetCallButton().GetDisplayText(),
			))
		}
		return withChoices(joinNonEmpty("\n", h.GetHydratedTitleText(), h.GetHydratedContentText(), h.GetHydratedFooterText()), labels)
	}
	if f := t.GetFourRowTemplate(); f != nil {
		var labels []string
		for _, b := range f.GetButtons() {
			labels = append(labels, firstNonEmpty(
				hsmText(b.GetQuickReplyButton().GetDisplayText()),
				hsmText(b.GetUrlButton().GetDisplayText()),
				hsmText(b.GetCallButton().GetDisplayText()),
			))
		}
		return withChoices(joinNonEmpty("\n", hsmText(f.GetHighlyStructuredMessage()), hsmText(f.GetContent()), hsmText(f.GetFooter())), labels)
	}
	return ""
}

// hsmText returns the readable text of a HighlyStructuredMessage: the
// hydrated body when the phone already resolved the template, otherwise
// the fallback locale string.
func hsmText(h *waProto.HighlyStructuredMessage) string {
	if h == nil {
		return ""
	}
	if inner := h.GetHydratedHsm(); inner != nil {
		return templateText(inner)
	}
	return h.GetFallbackLg()
}

// interactiveText flattens an InteractiveMessage (Cloud API "interactive"
// buttons, lists, CTA links and flows, plus carousels of them) to header,
// body, footer and one line per choice. Native-flow buttons carry their
// label inside a JSON blob, so that is parsed for the display text or, for
// a list picker, the row titles.
func interactiveText(im *waProto.InteractiveMessage) string {
	if c := im.GetCarouselMessage(); c != nil && len(c.GetCards()) > 0 {
		var cards []string
		for _, card := range c.GetCards() {
			if s := interactiveText(card); s != "" {
				cards = append(cards, s)
			}
		}
		return joinNonEmpty("\n", joinNonEmpty("\n", im.GetHeader().GetTitle(), im.GetBody().GetText()), strings.Join(cards, "\n\n"))
	}
	var labels []string
	for _, b := range im.GetNativeFlowMessage().GetButtons() {
		labels = append(labels, nativeFlowLabels(b.GetName(), b.GetButtonParamsJSON())...)
	}
	hdr := im.GetHeader()
	return withChoices(joinNonEmpty("\n", hdr.GetTitle(), hdr.GetSubtitle(), im.GetBody().GetText(), im.GetFooter().GetText()), labels)
}

// nativeFlowLabels extracts what a native-flow button shows: its
// display_text (quick replies, URL/call/copy CTAs), or for a single-select
// list the picker title followed by every row title. The flow name is the
// last resort so an unknown flow still leaves a trace.
func nativeFlowLabels(name, paramsJSON string) []string {
	var p struct {
		DisplayText string `json:"display_text"`
		Title       string `json:"title"`
		Sections    []struct {
			Title string `json:"title"`
			Rows  []struct {
				Title string `json:"title"`
			} `json:"rows"`
		} `json:"sections"`
	}
	_ = json.Unmarshal([]byte(paramsJSON), &p)
	var out []string
	if s := firstNonEmpty(p.DisplayText, p.Title, name); s != "" {
		out = append(out, s)
	}
	for _, sec := range p.Sections {
		for _, r := range sec.Rows {
			if strings.TrimSpace(r.Title) != "" {
				out = append(out, r.Title)
			}
		}
	}
	return out
}

// buttonsText flattens the legacy ButtonsMessage: header text, body,
// footer and one line per button.
func buttonsText(b *waProto.ButtonsMessage) string {
	var labels []string
	for _, btn := range b.GetButtons() {
		labels = append(labels, firstNonEmpty(btn.GetButtonText().GetDisplayText(), btn.GetNativeFlowInfo().GetName()))
	}
	return withChoices(joinNonEmpty("\n", b.GetText(), b.GetContentText(), b.GetFooterText()), labels)
}

// listText flattens a ListMessage: title, description, footer, the picker
// button and one line per row across all sections.
func listText(l *waProto.ListMessage) string {
	var labels []string
	if bt := l.GetButtonText(); strings.TrimSpace(bt) != "" {
		labels = append(labels, bt)
	}
	for _, sec := range l.GetSections() {
		for _, r := range sec.GetRows() {
			labels = append(labels, r.GetTitle())
		}
	}
	return withChoices(joinNonEmpty("\n", l.GetTitle(), l.GetDescription(), l.GetFooterText()), labels)
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

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return p
		}
	}
	return ""
}
