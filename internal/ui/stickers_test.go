package ui

import (
	"testing"

	"chatot/internal/client"
)

func TestStickerMenuNamesWhatRemovalMeans(t *testing.T) {
	local := stickerMenuItems(client.Sticker{Key: "file:x"}, func() {})
	if got := menuItemLabels(local); len(got) != 1 || got[0] != "Remove sticker" {
		t.Errorf("local sticker menu = %v", got)
	}
	fav := stickerMenuItems(client.Sticker{Key: "wa:x", FromWhatsApp: true}, func() {})
	if got := menuItemLabels(fav); len(got) != 1 || got[0] != "Remove from this device" {
		t.Errorf("favourite sticker menu = %v", got)
	}
}
