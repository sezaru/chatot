package client

import "go.mau.fi/whatsmeow/types"

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
