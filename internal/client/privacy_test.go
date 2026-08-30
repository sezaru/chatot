package client

import (
	"testing"

	"go.mau.fi/whatsmeow/types"
)

func TestPrivacySettingsToMap(t *testing.T) {
	s := types.PrivacySettings{
		GroupAdd:     types.PrivacySettingContacts,
		LastSeen:     types.PrivacySettingNone,
		Status:       types.PrivacySettingAll,
		Profile:      types.PrivacySettingContacts,
		ReadReceipts: types.PrivacySettingAll,
		CallAdd:      types.PrivacySettingAll,
		Online:       types.PrivacySettingMatchLastSeen,
		Messages:     types.PrivacySettingAll,
	}
	got := privacySettingsToMap(s)
	want := map[string]string{
		"Group Add":     "contacts",
		"Last Seen":     "none",
		"Status":        "all",
		"Profile Photo": "contacts",
		"Read Receipts": "all",
		"Calls":         "all",
		"Online":        "match_last_seen",
		"Messages":      "all",
		"Defense Mode":  "",
		"Stickers":      "",
	}
	if len(got) != len(want) {
		t.Fatalf("privacySettingsToMap() = %+v, want %+v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("privacySettingsToMap()[%q] = %q, want %q", k, got[k], v)
		}
	}
}
