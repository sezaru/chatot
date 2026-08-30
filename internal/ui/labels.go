package ui

import (
	"context"
	"log"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// chatHasLabel reports whether chatJID should show under the current label
// filter: labelID == "" (the "All" selection) always matches; otherwise the
// chat's own label set (from the store) must contain labelID.
func chatHasLabel(c client.Client, chatJID, labelID string) bool {
	if labelID == "" {
		return true
	}
	ids, err := c.LabelsForChat(chatJID)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if id == labelID {
			return true
		}
	}
	return false
}

// rebuildLabelFilter repopulates the sidebar's label-filter dropdown from the
// current labels, keeping the previously selected label if it still exists
// (otherwise falling back to "All"). Must run on the GTK main loop.
func (cl *ChatList) rebuildLabelFilter() {
	labels, err := cl.c.Labels()
	if err != nil {
		labels = nil
	}

	names := make([]string, 0, len(labels)+1)
	names = append(names, "All")
	cl.filterLabelIDs = make([]string, 0, len(labels)+1)
	cl.filterLabelIDs = append(cl.filterLabelIDs, "")

	selectedIdx := 0
	for _, l := range labels {
		name := l.Name
		if name == "" {
			name = "Label " + l.ID
		}
		names = append(names, name)
		cl.filterLabelIDs = append(cl.filterLabelIDs, l.ID)
		if l.ID == cl.selectedLabel {
			selectedIdx = len(cl.filterLabelIDs) - 1
		}
	}
	if selectedIdx == 0 {
		cl.selectedLabel = ""
	}

	cl.labelFilter.SetModel(gtk.NewStringList(names))
	cl.labelFilter.SetSelected(uint(selectedIdx))
}

// showLabelsDialog opens a modal listing every label as a checkbox reflecting
// whether chat currently carries it; toggling one calls SetChatLabeled. A
// "New label" row at the bottom creates a label from a name entry.
func showLabelsDialog(parent *gtk.Window, c client.Client, chat client.Chat) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Labels — " + chat.Name)
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.SetMarginTop(12)
	box.SetMarginBottom(12)
	box.SetMarginStart(12)
	box.SetMarginEnd(12)

	listBox := gtk.NewBox(gtk.OrientationVertical, 4)
	box.Append(listBox)

	populate := func() {
		for child := listBox.FirstChild(); child != nil; child = listBox.FirstChild() {
			listBox.Remove(child)
		}

		labels, err := c.Labels()
		if err != nil {
			listBox.Append(gtk.NewLabel("Couldn't load labels"))
			return
		}
		if len(labels) == 0 {
			empty := gtk.NewLabel("No labels yet")
			empty.SetXAlign(0)
			listBox.Append(empty)
		}

		onChat, _ := c.LabelsForChat(chat.JID)
		active := make(map[string]bool, len(onChat))
		for _, id := range onChat {
			active[id] = true
		}

		for _, l := range labels {
			name := l.Name
			if name == "" {
				name = "Label " + l.ID
			}
			check := gtk.NewCheckButtonWithLabel(name)
			check.SetActive(active[l.ID])
			labelID := l.ID
			check.ConnectToggled(func() {
				labeled := check.Active()
				go func() {
					if err := c.SetChatLabeled(context.Background(), labelID, chat.JID, labeled); err != nil {
						log.Printf("chatot: set chat label failed: %v", err)
					}
				}()
			})
			listBox.Append(check)
		}
	}
	populate()

	newRow := gtk.NewBox(gtk.OrientationHorizontal, 4)
	newEntry := gtk.NewEntry()
	newEntry.SetPlaceholderText("New label name")
	newEntry.SetHExpand(true)
	newRow.Append(newEntry)

	addBtn := gtk.NewButtonWithLabel("＋")
	addBtn.SetTooltipText("Create label")
	newRow.Append(addBtn)
	box.Append(newRow)

	createLabel := func() {
		name := newEntry.Text()
		if name == "" {
			return
		}
		newEntry.SetText("")
		go func() {
			if _, err := c.CreateLabel(context.Background(), name, 0); err != nil {
				log.Printf("chatot: create label failed: %v", err)
				return
			}
			glib.IdleAdd(populate)
		}()
	}
	addBtn.ConnectClicked(createLabel)
	newEntry.ConnectActivate(createLabel)

	dialog.SetChild(box)
	dialog.Present()
}
