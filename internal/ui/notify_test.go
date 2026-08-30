package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestDecideNotifyMessageFromMe(t *testing.T) {
	if decideNotify(notifyInput{Kind: "message", FromMe: true, Enabled: true}) {
		t.Fatal("expected no notification for a from-me message")
	}
}

func TestDecideNotifyDisabled(t *testing.T) {
	if decideNotify(notifyInput{Kind: "message", Enabled: false}) {
		t.Fatal("expected no notification when notifications are disabled")
	}
}

func TestDecideNotifyMutedChat(t *testing.T) {
	if decideNotify(notifyInput{Kind: "message", Enabled: true, Muted: true, ChatJID: "a"}) {
		t.Fatal("expected no notification for a muted chat")
	}
}

func TestDecideNotifySuppressedWhenOpenAndFocused(t *testing.T) {
	in := notifyInput{Kind: "message", Enabled: true, ChatJID: "a", OpenJID: "a", AppFocused: true}
	if decideNotify(in) {
		t.Fatal("expected no notification for the open, focused chat")
	}
}

func TestDecideNotifyOpenButUnfocusedStillNotifies(t *testing.T) {
	in := notifyInput{Kind: "message", Enabled: true, ChatJID: "a", OpenJID: "a", AppFocused: false}
	if !decideNotify(in) {
		t.Fatal("expected a notification when the open chat's window isn't focused")
	}
}

func TestDecideNotifyDifferentChatNotifiesEvenFocused(t *testing.T) {
	in := notifyInput{Kind: "message", Enabled: true, ChatJID: "a", OpenJID: "b", AppFocused: true}
	if !decideNotify(in) {
		t.Fatal("expected a notification for a chat other than the open one")
	}
}

func TestDecideNotifyCallRingsRegardlessOfFocus(t *testing.T) {
	in := notifyInput{Kind: "call", Enabled: true, ChatJID: "a", OpenJID: "a", AppFocused: true}
	if !decideNotify(in) {
		t.Fatal("expected an incoming call to always notify while enabled")
	}
}

func TestDecideNotifyCallFromMe(t *testing.T) {
	if decideNotify(notifyInput{Kind: "call", FromMe: true, Enabled: true}) {
		t.Fatal("expected no notification for a from-me call")
	}
}

func TestDecideNotifyCallDisabled(t *testing.T) {
	if decideNotify(notifyInput{Kind: "call", Enabled: false}) {
		t.Fatal("expected no call notification when notifications are disabled")
	}
}

func TestMessageNotificationText(t *testing.T) {
	title, body := messageNotification("Ada Lovelace", client.Message{Text: "hello there"})
	if title != "Ada Lovelace" || body != "hello there" {
		t.Fatalf("got title=%q body=%q", title, body)
	}
}

func TestMessageNotificationAttachmentCaption(t *testing.T) {
	msg := client.Message{Attachment: &client.Attachment{Kind: "image", Caption: "sunset"}}
	_, body := messageNotification("Ada", msg)
	if body != "sunset" {
		t.Fatalf("got body=%q, want caption", body)
	}
}

func TestMessageNotificationAttachmentPlaceholder(t *testing.T) {
	msg := client.Message{Attachment: &client.Attachment{Kind: "video"}}
	_, body := messageNotification("Ada", msg)
	if body != "[video]" {
		t.Fatalf("got body=%q, want [video] placeholder", body)
	}
}

func TestCallNotificationText(t *testing.T) {
	title, body := callNotification("Grace Hopper")
	if title != "Incoming call" || body != "Grace Hopper" {
		t.Fatalf("got title=%q body=%q", title, body)
	}
}
