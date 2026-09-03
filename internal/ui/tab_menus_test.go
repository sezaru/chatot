package ui

import (
	"reflect"
	"testing"

	"chatot/internal/client"
)

func TestTabPlusMenuItems(t *testing.T) {
	cases := map[string][]string{
		"chats":       {"New chat", "New group", "New community", "Join with invite link"},
		"status":      {"Photo status", "Text status", "---", "Status privacy…"},
		"channels":    {"Find channels", "Follow with a link", "---", "Create a channel"},
		"communities": {"New community", "Join with a link"},
	}
	for tab, want := range cases {
		if got := labelsOf(tabPlusMenuItems(tab, tabPlusActions{})); !reflect.DeepEqual(got, want) {
			t.Errorf("tabPlusMenuItems(%q) = %v, want %v", tab, got, want)
		}
	}
}

func TestStatusMenus(t *testing.T) {
	got := labelsOf(statusRowMenuItems("Priya Raman", false, statusRowMenuActions{}))
	want := []string{"View updates", "Reply privately", "---", "Mute Priya", "Hide my status from them"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("statusRowMenuItems = %v, want %v", got, want)
	}
	if items := statusRowMenuItems("Priya Raman", false, statusRowMenuActions{}); !items[4].Destructive {
		t.Errorf("hide row should be destructive")
	}
	if items := statusRowMenuItems("Priya Raman", true, statusRowMenuActions{}); items[3].Label != "Unmute Priya" {
		t.Errorf("muted poster row = %q, want Unmute Priya", items[3].Label)
	}
	if items := statusViewMenuItems(true, statusViewMenuActions{}); items[3].Label != "Unmute this contact" {
		t.Errorf("muted viewer menu = %q", items[3].Label)
	}
	got = labelsOf(myStatusMenuItems(myStatusMenuActions{}))
	want = []string{"Who viewed my status", "Status privacy…", "---", "Delete my status"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("myStatusMenuItems = %v, want %v", got, want)
	}
	got = labelsOf(statusViewMenuItems(false, statusViewMenuActions{}))
	want = []string{"Reply privately", "Forward this update", "---", "Mute this contact", "Report update"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("statusViewMenuItems = %v, want %v", got, want)
	}
}

func TestChannelMenuItems(t *testing.T) {
	row := labelsOf(channelMenuItems(client.Newsletter{}, true, channelMenuActions{}))
	want := []string{"Channel info", "Share channel link", "---", "Mute updates", "Report channel", "Unfollow"}
	if !reflect.DeepEqual(row, want) {
		t.Errorf("row menu = %v, want %v", row, want)
	}
	pane := labelsOf(channelMenuItems(client.Newsletter{Muted: true}, false, channelMenuActions{}))
	want = []string{"Channel info", "Share channel link", "---", "Unmute updates", "Report channel"}
	if !reflect.DeepEqual(pane, want) {
		t.Errorf("pane menu = %v, want %v", pane, want)
	}
	if icons := iconsOf(channelMenuItems(client.Newsletter{Muted: true}, false, channelMenuActions{})); icons[2] != "🔔" {
		t.Errorf("muted channel's mute row icon = %q, want 🔔", icons[2])
	}
}

func TestCommunityMenuItems(t *testing.T) {
	got := labelsOf(communityMenuItems(client.Community{}, communityMenuActions{}))
	want := []string{"Community info", "Invite link", "---", "Mute announcements", "Leave community"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("communityMenuItems = %v, want %v", got, want)
	}
	muted := labelsOf(communityMenuItems(client.Community{Muted: true}, communityMenuActions{}))
	if muted[3] != "Unmute announcements" {
		t.Errorf("muted community row = %q", muted[3])
	}
}

func TestReportReasonsMatchMockup(t *testing.T) {
	if len(reportReasons) != 6 || reportReasons[0] != "Spam or repetitive posts" || reportReasons[5] != "Something else" {
		t.Errorf("reportReasons = %v", reportReasons)
	}
}
