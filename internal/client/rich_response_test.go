package client

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waAICommon"
	"go.mau.fi/whatsmeow/proto/waAICommonDeprecated"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// A Meta AI answer in the unified form: Markdown with a link entity, a code
// block and a table, plus a "thinking" status that is not part of it.
const metaAIAnswer = `{"response_id":"r1","sections":[
 {"view_model":{"__typename":"GenAISingleLayoutViewModel","primitive":{"__typename":"GenAIMarkdownTextUXPrimitive",
  "text":"## Title\n**Bold** and ` + "`code`" + `, see {{l1}}.\n- one\n- two",
  "inline_entities":[{"key":"l1","metadata":{"__typename":"GenAIInlineLinkItem","url":"https://x.org","display_name":"X"}}]}}},
 {"view_model":{"__typename":"GenAISingleLayoutViewModel","primitive":{"__typename":"GenAICodeUXPrimitive","language":"go",
  "code_blocks":[{"content":"fmt.","type":"DEFAULT"},{"content":"Println()\n","type":"METHOD"}]}}},
 {"view_model":{"__typename":"GenAIVStackLayoutViewModel","primitives":[
  {"__typename":"GenATableUXPrimitive","rows":[{"cells":["a","b"],"is_header":true},{"cells":["1","2"]}]},
  {"__typename":"GenAIBotThinkingStatusPrimitive","title":"Thinking"}]}}]}`

func TestMetaAIAnswerReadsAsText(t *testing.T) {
	m := &waE2E.Message{RichResponseMessage: &waE2E.AIRichResponseMessage{
		UnifiedResponse: &waAICommon.AIRichResponseUnifiedResponse{Data: []byte(metaAIAnswer)},
	}}
	var msg Message
	extractText(m, &msg)
	want := "Title\nBold and code, see X (https://x.org).\n• one\n• two\n\nfmt.Println()\n\na | b\n1 | 2"
	if msg.Text != want {
		t.Errorf("answer text =\n%q\nwant\n%q", msg.Text, want)
	}
}

// The older form carries typed submessages instead of JSON.
func TestMetaAIAnswerFromSubmessages(t *testing.T) {
	text := waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_TEXT
	code := waAICommonDeprecated.AIRichResponseSubMessageType_AI_RICH_RESPONSE_CODE
	m := &waE2E.Message{RichResponseMessage: &waE2E.AIRichResponseMessage{
		Submessages: []*waAICommonDeprecated.AIRichResponseSubMessage{
			{MessageType: &text, MessageText: proto.String("**Hi** there")},
			{MessageType: &code, CodeMetadata: &waAICommonDeprecated.AIRichResponseCodeMetadata{
				CodeBlocks: []*waAICommonDeprecated.AIRichResponseCodeMetadata_AIRichResponseCodeBlock{{CodeContent: proto.String("x := 1")}},
			}},
		},
	}}
	var msg Message
	extractText(m, &msg)
	if want := "Hi there\n\nx := 1"; msg.Text != want {
		t.Errorf("answer text = %q, want %q", msg.Text, want)
	}
	// An answer with nothing chatot can show is still a bubble.
	msg = Message{}
	extractText(&waE2E.Message{RichResponseMessage: &waE2E.AIRichResponseMessage{}}, &msg)
	if msg.Text != unsupportedText {
		t.Errorf("empty answer text = %q, want the unsupported line", msg.Text)
	}
}

func TestPlainMarkdown(t *testing.T) {
	cases := []struct{ in, want string }{
		{"# Heading\ntext", "Heading\ntext"},
		{"* item\n  - nested", "• item\n  • nested"},
		{"a **b** c", "a b c"},
		{"[site](https://a.b) and [https://c.d](https://c.d)", "site (https://a.b) and https://c.d"},
		{"```go\n**not bold**\n# not a heading\n```", "**not bold**\n# not a heading"},
		{"above\n---\nbelow", "above\n\nbelow"},
		{"snake_case and 2*3*4", "snake_case and 2*3*4"},
	}
	for _, c := range cases {
		if got := plainMarkdown(c.in); got != c.want {
			t.Errorf("plainMarkdown(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
