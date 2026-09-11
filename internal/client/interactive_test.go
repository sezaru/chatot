package client

import (
	"reflect"
	"testing"

	"chatot/internal/store"
	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

func TestExtractInteractiveTemplateWithNativeFlowButtons(t *testing.T) {
	// A Cloud API utility template with quick-reply buttons reaches linked
	// devices as a TemplateMessage whose only format is the interactive
	// variant; the body is inside it, the labels inside JSON blobs.
	var msg Message
	extractText(&waProto.Message{TemplateMessage: &waProto.TemplateMessage{
		Format: &waProto.TemplateMessage_InteractiveMessageTemplate{InteractiveMessageTemplate: &waProto.InteractiveMessage{
			Body:   &waProto.InteractiveMessage_Body{Text: proto.String("Pix enviado com sucesso.")},
			Footer: &waProto.InteractiveMessage_Footer{Text: proto.String("BTG Pactual")},
			InteractiveMessage: &waProto.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton{
					{Name: proto.String("quick_reply"), ButtonParamsJSON: proto.String(`{"display_text":"Ver comprovante","id":"receipt"}`)},
					{Name: proto.String("cta_url"), ButtonParamsJSON: proto.String(`{"display_text":"Abrir app","url":"https://example"}`)},
					{Name: proto.String("cta_copy"), ButtonParamsJSON: proto.String(`{"display_text":"Copiar código","copy_code":"1234"}`)},
					{Name: proto.String("mystery_flow"), ButtonParamsJSON: proto.String(`{}`)},
					{Name: proto.String("galaxy_message"), ButtonParamsJSON: proto.String(`{"flow_message_version":"3","flow_token":"t","flow_id":"1","flow_cta":"Informar","flow_action":"navigate"}`)},
					{Name: proto.String("open_webview"), ButtonParamsJSON: proto.String(`{"title":"Ver fatura","link":{"in_app_webview":true,"url":"https://example/fatura"}}`)},
				},
			}},
		}},
	}}, &msg)
	if want := "Pix enviado com sucesso.\nBTG Pactual"; msg.Text != want {
		t.Fatalf("interactive template text:\n got %q\nwant %q", msg.Text, want)
	}
	want := &Choices{Source: "interactive", Buttons: []ChoiceButton{
		{ID: "receipt", Label: "Ver comprovante", Kind: "reply", Index: 0},
		{Label: "Abrir app", Kind: "url", URL: "https://example", Index: 1},
		{Label: "Copiar código", Kind: "copy", CopyText: "1234", Index: 2},
		{Label: "mystery_flow", Kind: "reply", Index: 3},
		{Label: "Informar", Kind: "flow", Index: 4},
		{Label: "Ver fatura", Kind: "url", URL: "https://example/fatura", Index: 5},
	}}
	if !reflect.DeepEqual(msg.Choices, want) {
		t.Fatalf("interactive template choices:\n got %+v\nwant %+v", msg.Choices, want)
	}
}

func TestExtractInteractiveListPickerRows(t *testing.T) {
	var msg Message
	extractText(&waProto.Message{ViewOnceMessage: &waProto.FutureProofMessage{Message: &waProto.Message{
		InteractiveMessage: &waProto.InteractiveMessage{
			Header: &waProto.InteractiveMessage_Header{Title: proto.String("Menu")},
			InteractiveMessage: &waProto.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
					Name:             proto.String("single_select"),
					ButtonParamsJSON: proto.String(`{"title":"Escolher","sections":[{"title":"Pix","rows":[{"title":"Fazer Pix","id":"1","description":"Enviar"},{"title":"Extrato","id":"2"}]}]}`),
				}},
			}},
		},
	}}}, &msg)
	if msg.Text != "Menu" {
		t.Fatalf("list picker text %q", msg.Text)
	}
	want := &Choices{Source: "interactive", List: &ChoiceList{Title: "Escolher", Sections: []ChoiceSection{
		{Title: "Pix", Rows: []ChoiceRow{{ID: "1", Title: "Fazer Pix", Description: "Enviar"}, {ID: "2", Title: "Extrato"}}},
	}}}
	if !reflect.DeepEqual(msg.Choices, want) {
		t.Fatalf("list picker choices:\n got %+v\nwant %+v", msg.Choices, want)
	}
}

func TestExtractHydratedTemplateAndLegacyShapes(t *testing.T) {
	var msg Message
	extractText(&waProto.Message{TemplateMessage: &waProto.TemplateMessage{
		HydratedTemplate: &waProto.TemplateMessage_HydratedFourRowTemplate{
			HydratedContentText: proto.String("Seu código é 1234"),
			HydratedButtons: []*waProto.HydratedTemplateButton{
				{HydratedButton: &waProto.HydratedTemplateButton_QuickReplyButton{QuickReplyButton: &waProto.HydratedTemplateButton_HydratedQuickReplyButton{DisplayText: proto.String("Copiar"), ID: proto.String("copy")}}},
				{Index: proto.Uint32(1), HydratedButton: &waProto.HydratedTemplateButton_CallButton{CallButton: &waProto.HydratedTemplateButton_HydratedCallButton{DisplayText: proto.String("Ligar"), PhoneNumber: proto.String("+5511999")}}},
			},
		},
	}}, &msg)
	if msg.Text != "Seu código é 1234" {
		t.Fatalf("hydrated template text %q", msg.Text)
	}
	want := &Choices{Source: "template", Buttons: []ChoiceButton{
		{ID: "copy", Label: "Copiar", Kind: "reply"},
		{Label: "Ligar", Kind: "call", Phone: "+5511999", Index: 1},
	}}
	if !reflect.DeepEqual(msg.Choices, want) {
		t.Fatalf("hydrated template choices:\n got %+v\nwant %+v", msg.Choices, want)
	}

	msg = Message{}
	extractText(&waProto.Message{ButtonsMessage: &waProto.ButtonsMessage{
		ContentText: proto.String("Confirma?"),
		Buttons: []*waProto.ButtonsMessage_Button{
			{ButtonID: proto.String("y"), ButtonText: &waProto.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Sim")}},
			{ButtonID: proto.String("n"), ButtonText: &waProto.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Não")}},
		},
	}}, &msg)
	if msg.Text != "Confirma?" {
		t.Fatalf("buttons text %q", msg.Text)
	}
	want = &Choices{Source: "buttons", Buttons: []ChoiceButton{
		{ID: "y", Label: "Sim", Kind: "reply"},
		{ID: "n", Label: "Não", Kind: "reply", Index: 1},
	}}
	if !reflect.DeepEqual(msg.Choices, want) {
		t.Fatalf("buttons choices:\n got %+v\nwant %+v", msg.Choices, want)
	}

	msg = Message{}
	extractText(&waProto.Message{ListMessage: &waProto.ListMessage{
		Description: proto.String("Escolha uma opção"),
		ButtonText:  proto.String("Ver opções"),
		Sections:    []*waProto.ListMessage_Section{{Rows: []*waProto.ListMessage_Row{{Title: proto.String("Saldo"), RowID: proto.String("s")}, {Title: proto.String("Extrato")}}}},
	}}, &msg)
	if msg.Text != "Escolha uma opção" {
		t.Fatalf("list text %q", msg.Text)
	}
	want = &Choices{Source: "list", List: &ChoiceList{Title: "Ver opções", Sections: []ChoiceSection{
		{Rows: []ChoiceRow{{ID: "s", Title: "Saldo"}, {Title: "Extrato"}}},
	}}}
	if !reflect.DeepEqual(msg.Choices, want) {
		t.Fatalf("list choices:\n got %+v\nwant %+v", msg.Choices, want)
	}
}

func TestExtractInteractiveCarouselStaysText(t *testing.T) {
	var msg Message
	card := func(body, label string) *waProto.InteractiveMessage {
		return &waProto.InteractiveMessage{
			Body: &waProto.InteractiveMessage_Body{Text: proto.String(body)},
			InteractiveMessage: &waProto.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{Name: proto.String("quick_reply"), ButtonParamsJSON: proto.String(`{"display_text":"` + label + `"}`)}},
			}},
		}
	}
	extractText(&waProto.Message{InteractiveMessage: &waProto.InteractiveMessage{
		Body: &waProto.InteractiveMessage_Body{Text: proto.String("Ofertas")},
		InteractiveMessage: &waProto.InteractiveMessage_CarouselMessage_{CarouselMessage: &waProto.InteractiveMessage_CarouselMessage{
			Cards: []*waProto.InteractiveMessage{card("Cartão", "Pedir"), card("Seguro", "Contratar")},
		}},
	}}, &msg)
	if want := "Ofertas\nCartão\n▸ Pedir\n\nSeguro\n▸ Contratar"; msg.Text != want {
		t.Fatalf("carousel text %q, want %q", msg.Text, want)
	}
	if msg.Choices != nil {
		t.Fatalf("carousel choices %+v, want none", msg.Choices)
	}
	msg = Message{}
	extractText(&waProto.Message{InteractiveMessage: &waProto.InteractiveMessage{}}, &msg)
	if msg.Text != unsupportedText || msg.Choices != nil {
		t.Fatalf("empty interactive: got %q %+v, want the unsupported placeholder", msg.Text, msg.Choices)
	}
}

func TestChoicesStoreRoundTrip(t *testing.T) {
	in := Message{ID: "m", ChatJID: "c@s.whatsapp.net", Text: "Menu", Choices: &Choices{
		Source:  "interactive",
		Buttons: []ChoiceButton{{ID: "a", Label: "A", Kind: "reply"}, {Label: "Site", Kind: "url", URL: "https://x", Index: 1}},
		List:    &ChoiceList{Title: "Pick", Sections: []ChoiceSection{{Title: "S", Rows: []ChoiceRow{{ID: "r", Title: "R", Description: "d"}}}}},
	}}
	row := storeMessageRow(&in)
	if row.Kind != "choices" || row.Payload == "" {
		t.Fatalf("row kind %q payload %q", row.Kind, row.Payload)
	}
	out := messageFromStore(store.Message{ID: row.MsgID, ChatJID: row.ChatJID, Text: row.Text, Kind: row.Kind, Payload: row.Payload}, "")
	if !reflect.DeepEqual(out.Choices, in.Choices) {
		t.Fatalf("round trip:\n got %+v\nwant %+v", out.Choices, in.Choices)
	}
	if out.Text != "Menu" {
		t.Fatalf("round trip text %q", out.Text)
	}
}

func TestChoiceReplyShapes(t *testing.T) {
	ctx := &waProto.ContextInfo{StanzaID: proto.String("offer")}
	btn := ChoiceSelection{ID: "y", Label: "Sim", Index: 1}
	row := ChoiceSelection{ID: "r", Label: "Saldo", Description: "d", IsRow: true}

	m := choiceReply("buttons", btn, ctx)
	if r := m.GetButtonsResponseMessage(); r.GetSelectedButtonID() != "y" || r.GetSelectedDisplayText() != "Sim" || r.GetType() != waProto.ButtonsResponseMessage_DISPLAY_TEXT || r.GetContextInfo() != ctx {
		t.Fatalf("buttons reply %+v", m)
	}
	m = choiceReply("list", row, ctx)
	if r := m.GetListResponseMessage(); r.GetTitle() != "Saldo" || r.GetSingleSelectReply().GetSelectedRowID() != "r" || r.GetDescription() != "d" || r.GetListType() != waProto.ListResponseMessage_SINGLE_SELECT {
		t.Fatalf("list reply %+v", m)
	}
	m = choiceReply("template", btn, ctx)
	if r := m.GetTemplateButtonReplyMessage(); r.GetSelectedID() != "y" || r.GetSelectedDisplayText() != "Sim" || r.GetSelectedIndex() != 1 {
		t.Fatalf("template reply %+v", m)
	}
	m = choiceReply("interactive", btn, ctx)
	r := m.GetInteractiveResponseMessage()
	if r.GetBody().GetText() != "Sim" || r.GetNativeFlowResponseMessage().GetName() != "quick_reply" || r.GetNativeFlowResponseMessage().GetVersion() != 1 {
		t.Fatalf("interactive button reply %+v", m)
	}
	if got := r.GetNativeFlowResponseMessage().GetParamsJSON(); got != `{"display_text":"Sim","id":"y"}` {
		t.Fatalf("interactive button params %s", got)
	}
	m = choiceReply("interactive", row, ctx)
	r = m.GetInteractiveResponseMessage()
	if r.GetNativeFlowResponseMessage().GetName() != "menu_options" {
		t.Fatalf("interactive row reply %+v", m)
	}
	if got := r.GetNativeFlowResponseMessage().GetParamsJSON(); got != `{"description":"d","id":"r","title":"Saldo"}` {
		t.Fatalf("interactive row params %s", got)
	}
}
