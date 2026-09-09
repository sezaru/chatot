package client

import (
	"testing"

	waProto "go.mau.fi/whatsmeow/binary/proto"
	"google.golang.org/protobuf/proto"
)

func TestExtractTextInteractiveTemplateWithNativeFlowButtons(t *testing.T) {
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
					{Name: proto.String("mystery_flow"), ButtonParamsJSON: proto.String(`{}`)},
				},
			}},
		}},
	}}, &msg)
	want := "Pix enviado com sucesso.\nBTG Pactual\n▸ Ver comprovante\n▸ Abrir app\n▸ mystery_flow"
	if msg.Text != want {
		t.Fatalf("interactive template text:\n got %q\nwant %q", msg.Text, want)
	}
}

func TestExtractTextInteractiveListPickerRows(t *testing.T) {
	var msg Message
	extractText(&waProto.Message{ViewOnceMessage: &waProto.FutureProofMessage{Message: &waProto.Message{
		InteractiveMessage: &waProto.InteractiveMessage{
			Header: &waProto.InteractiveMessage_Header{Title: proto.String("Menu")},
			InteractiveMessage: &waProto.InteractiveMessage_NativeFlowMessage_{NativeFlowMessage: &waProto.InteractiveMessage_NativeFlowMessage{
				Buttons: []*waProto.InteractiveMessage_NativeFlowMessage_NativeFlowButton{{
					Name:             proto.String("single_select"),
					ButtonParamsJSON: proto.String(`{"title":"Escolher","sections":[{"title":"Pix","rows":[{"title":"Fazer Pix","id":"1"},{"title":"Extrato","id":"2"}]}]}`),
				}},
			}},
		},
	}}}, &msg)
	want := "Menu\n▸ Escolher\n▸ Fazer Pix\n▸ Extrato"
	if msg.Text != want {
		t.Fatalf("list picker text:\n got %q\nwant %q", msg.Text, want)
	}
}

func TestExtractTextHydratedTemplateAndLegacyButtons(t *testing.T) {
	var msg Message
	extractText(&waProto.Message{TemplateMessage: &waProto.TemplateMessage{
		HydratedTemplate: &waProto.TemplateMessage_HydratedFourRowTemplate{
			HydratedContentText: proto.String("Seu código é 1234"),
			HydratedButtons: []*waProto.HydratedTemplateButton{
				{HydratedButton: &waProto.HydratedTemplateButton_QuickReplyButton{QuickReplyButton: &waProto.HydratedTemplateButton_HydratedQuickReplyButton{DisplayText: proto.String("Copiar")}}},
				{HydratedButton: &waProto.HydratedTemplateButton_CallButton{CallButton: &waProto.HydratedTemplateButton_HydratedCallButton{DisplayText: proto.String("Ligar")}}},
			},
		},
	}}, &msg)
	if want := "Seu código é 1234\n▸ Copiar\n▸ Ligar"; msg.Text != want {
		t.Fatalf("hydrated template text %q, want %q", msg.Text, want)
	}
	msg = Message{}
	extractText(&waProto.Message{ButtonsMessage: &waProto.ButtonsMessage{
		ContentText: proto.String("Confirma?"),
		Buttons: []*waProto.ButtonsMessage_Button{
			{ButtonText: &waProto.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Sim")}},
			{ButtonText: &waProto.ButtonsMessage_Button_ButtonText{DisplayText: proto.String("Não")}},
		},
	}}, &msg)
	if want := "Confirma?\n▸ Sim\n▸ Não"; msg.Text != want {
		t.Fatalf("buttons text %q, want %q", msg.Text, want)
	}
	msg = Message{}
	extractText(&waProto.Message{ListMessage: &waProto.ListMessage{
		Description: proto.String("Escolha uma opção"),
		ButtonText:  proto.String("Ver opções"),
		Sections:    []*waProto.ListMessage_Section{{Rows: []*waProto.ListMessage_Row{{Title: proto.String("Saldo")}, {Title: proto.String("Extrato")}}}},
	}}, &msg)
	if want := "Escolha uma opção\n▸ Ver opções\n▸ Saldo\n▸ Extrato"; msg.Text != want {
		t.Fatalf("list text %q, want %q", msg.Text, want)
	}
}

func TestExtractTextInteractiveCarouselCards(t *testing.T) {
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
	msg = Message{}
	extractText(&waProto.Message{InteractiveMessage: &waProto.InteractiveMessage{}}, &msg)
	if msg.Text != unsupportedText {
		t.Fatalf("empty interactive: got %q, want the unsupported placeholder", msg.Text)
	}
}
