package client

import (
	"fmt"

	"go.mau.fi/whatsmeow/types"
)

// privacySettingsToMap converts whatsmeow's typed privacy struct to a
// display-friendly name -> value map for the read-only privacy dialog.
func privacySettingsToMap(s types.PrivacySettings) map[string]string {
	return map[string]string{
		"Group Add":     string(s.GroupAdd),
		"Last Seen":     string(s.LastSeen),
		"Status":        string(s.Status),
		"Profile Photo": string(s.Profile),
		"Read Receipts": string(s.ReadReceipts),
		"Calls":         string(s.CallAdd),
		"Online":        string(s.Online),
		"Messages":      string(s.Messages),
		"Defense Mode":  string(s.Defense),
		"Stickers":      string(s.Stickers),
	}
}

// privacySettingTypes maps the display keys PrivacySettings reports to
// whatsmeow's setting names and the values WhatsApp accepts for each (the
// lists whatsmeow documents on its PrivacySettingType constants).
var privacySettingTypes = map[string]struct {
	typ    types.PrivacySettingType
	values []string
}{
	"Group Add":     {types.PrivacySettingTypeGroupAdd, []string{"all", "contacts", "contact_blacklist", "none"}},
	"Last Seen":     {types.PrivacySettingTypeLastSeen, []string{"all", "contacts", "contact_blacklist", "none"}},
	"Status":        {types.PrivacySettingTypeStatus, []string{"all", "contacts", "contact_blacklist", "none"}},
	"Profile Photo": {types.PrivacySettingTypeProfile, []string{"all", "contacts", "contact_blacklist", "none"}},
	"Read Receipts": {types.PrivacySettingTypeReadReceipts, []string{"all", "none"}},
	"Calls":         {types.PrivacySettingTypeCallAdd, []string{"all", "known"}},
	"Online":        {types.PrivacySettingTypeOnline, []string{"all", "match_last_seen"}},
	"Messages":      {types.PrivacySettingTypeMessages, []string{"all", "contacts"}},
	"Defense Mode":  {types.PrivacySettingTypeDefense, []string{"on_standard", "off"}},
	"Stickers":      {types.PrivacySettingTypeStickers, []string{"contacts", "contact_allowlist", "none"}},
}

// PrivacySettingOptions lists the values SetPrivacySetting accepts for the
// PrivacySettings key name, nil for a key that can't be changed here.
func PrivacySettingOptions(name string) []string {
	t, ok := privacySettingTypes[name]
	if !ok {
		return nil
	}
	return append([]string(nil), t.values...)
}

// PrivacySettingLabel is the human wording for a privacy value.
func PrivacySettingLabel(value string) string {
	switch value {
	case "all":
		return "Everyone"
	case "contacts":
		return "My contacts"
	case "contact_blacklist":
		return "My contacts except…"
	case "contact_allowlist":
		return "Only some contacts"
	case "none":
		return "Nobody"
	case "match_last_seen":
		return "Same as last seen"
	case "known":
		return "Known numbers"
	case "on_standard":
		return "Standard"
	case "off":
		return "Off"
	case "":
		return "Not set"
	}
	return value
}

// privacySettingType resolves a display key + value to whatsmeow's typed
// pair, refusing values WhatsApp wouldn't accept for that setting.
func privacySettingType(name, value string) (types.PrivacySettingType, types.PrivacySetting, error) {
	t, ok := privacySettingTypes[name]
	if !ok {
		return "", "", fmt.Errorf("chatot/client: unknown privacy setting %q", name)
	}
	for _, v := range t.values {
		if v == value {
			return t.typ, types.PrivacySetting(value), nil
		}
	}
	return "", "", fmt.Errorf("chatot/client: %q is not a valid value for %s", value, name)
}
