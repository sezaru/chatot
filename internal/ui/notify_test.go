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
	if body != "📷 sunset" {
		t.Fatalf("got body=%q, want glyph + caption", body)
	}
}

func TestMessageNotificationAttachmentPlaceholder(t *testing.T) {
	msg := client.Message{Attachment: &client.Attachment{Kind: "video"}}
	_, body := messageNotification("Ada", msg)
	if body != "🎥 Video" {
		t.Fatalf("got body=%q, want the video placeholder", body)
	}
}

func TestCallNotificationText(t *testing.T) {
	title, body := callNotification("Grace Hopper")
	if title != "Incoming call" || body != "Grace Hopper" {
		t.Fatalf("got title=%q body=%q", title, body)
	}
}

func TestCallActionParamRoundTrip(t *testing.T) {
	cases := []struct{ chatJID, callID string }{
		{"1234567890@s.whatsapp.net", "ABCDEF123"},
		{"", ""},
		{"jid-with-no-call-id", ""},
	}
	for _, tc := range cases {
		param := encodeCallActionParam(tc.chatJID, tc.callID)
		chatJID, callID, ok := DecodeCallActionParam(param)
		if !ok {
			t.Fatalf("decode(%q) ok=false, want true", param)
		}
		if chatJID != tc.chatJID || callID != tc.callID {
			t.Fatalf("round trip %q,%q -> %q -> %q,%q", tc.chatJID, tc.callID, param, chatJID, callID)
		}
	}
}

func TestDecodeCallActionParamMalformed(t *testing.T) {
	if _, _, ok := DecodeCallActionParam("no-separator-here"); ok {
		t.Fatal("expected ok=false for a param missing the separator")
	}
}

func TestAccountPrefixedTitle(t *testing.T) {
	if got := accountPrefixedTitle("Sam Okafor", "Work"); got != "Work · Sam Okafor" {
		t.Errorf("prefixed title = %q, want %q", got, "Work · Sam Okafor")
	}
	if got := accountPrefixedTitle("Sam Okafor", ""); got != "Sam Okafor" {
		t.Errorf("empty label should leave title unchanged, got %q", got)
	}
}

func TestNotifierAccountPrefix(t *testing.T) {
	multi := func() (string, int) { return "Work", 2 }
	single := func() (string, int) { return "Work", 1 }

	cases := []struct {
		name    string
		perAcct bool
		account func() (string, int)
		want    string
	}{
		{"multi account, enabled", true, multi, "Work"},
		{"single account, enabled", true, single, ""},
		{"multi account, disabled", false, multi, ""},
		{"no accessor", true, nil, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prev := NotificationsPerAccount
			NotificationsPerAccount = tc.perAcct
			defer func() { NotificationsPerAccount = prev }()
			n := &Notifier{account: tc.account}
			if got := n.accountPrefix(); got != tc.want {
				t.Errorf("accountPrefix() = %q, want %q", got, tc.want)
			}
		})
	}
}
