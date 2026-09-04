package client

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waSyncAction"
	"google.golang.org/protobuf/proto"
)

func TestNextLabelID(t *testing.T) {
	cases := []struct {
		name     string
		existing []string
		want     string
	}{
		{"empty", nil, "1"},
		{"sequential", []string{"1", "2"}, "3"},
		{"with gaps", []string{"1", "3"}, "4"},
		{"unordered", []string{"5", "2", "3"}, "6"},
		{"non-numeric ignored", []string{"abc", "2"}, "3"},
		{"all non-numeric", []string{"x", "y"}, "1"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := nextLabelID(tc.existing); got != tc.want {
				t.Fatalf("nextLabelID(%v) = %q, want %q", tc.existing, got, tc.want)
			}
		})
	}
}

func TestLabelIsBuiltInAcceptsEveryFlagThePhoneUses(t *testing.T) {
	typed := &waSyncAction.LabelEditAction{Name: proto.String("Unread"), Type: waSyncAction.LabelEditAction_UNREAD.Enum()}
	if !labelIsBuiltIn("7", typed) {
		t.Error("a list-typed edit must be built in even off the fixed ids")
	}
	legacy := &waSyncAction.LabelEditAction{Name: proto.String("Groups"), PredefinedID: proto.Int32(2)}
	if !labelIsBuiltIn("9", legacy) {
		t.Error("a predefinedID edit must be built in")
	}
	bare := &waSyncAction.LabelEditAction{Name: proto.String("Communities")}
	if !labelIsBuiltIn("4", bare) {
		t.Error("stock name in slot 1-4 must be built in when the phone sends no flag")
	}
	custom := &waSyncAction.LabelEditAction{Name: proto.String("Paraguay"), Type: waSyncAction.LabelEditAction_CUSTOM.Enum()}
	if labelIsBuiltIn("5", custom) {
		t.Error("a custom label must not be built in")
	}
	if labelIsBuiltIn("5", &waSyncAction.LabelEditAction{Name: proto.String("Unread")}) {
		t.Error("a user label merely named Unread outside the fixed slots must stay custom")
	}
}
