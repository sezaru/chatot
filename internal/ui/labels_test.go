package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestChatHasLabel(t *testing.T) {
	c := client.NewFake() // seeds label "1" onto 1234567890@s.whatsapp.net

	if !chatHasLabel(c, "1112223333@s.whatsapp.net", "") {
		t.Fatal(`empty labelID ("All") should match every chat`)
	}
	if !chatHasLabel(c, "1234567890@s.whatsapp.net", "1") {
		t.Fatal("chat carrying label 1 should match label 1")
	}
	if chatHasLabel(c, "1112223333@s.whatsapp.net", "1") {
		t.Fatal("chat without label 1 should not match label 1")
	}
}
