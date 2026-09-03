package client

import (
	"context"
	"fmt"
	"strconv"

	"go.mau.fi/whatsmeow/appstate"
	"go.mau.fi/whatsmeow/types"

	"chatot/internal/store"
)

// nextLabelID returns the id to assign a new label: one past the largest
// numeric id in existing, or "1" if there are none. Non-numeric ids are
// ignored (WhatsApp labels are numbered "1".."N").
func nextLabelID(existing []string) string {
	max := 0
	for _, id := range existing {
		n, err := strconv.Atoi(id)
		if err != nil {
			continue
		}
		if n > max {
			max = n
		}
	}
	return strconv.Itoa(max + 1)
}

func labelsFromStore(rows []store.Label) []Label {
	out := make([]Label, len(rows))
	for i, l := range rows {
		out[i] = Label{ID: l.ID, Name: l.Name, Color: l.Color}
	}
	return out
}

// Labels returns the non-deleted labels from the store.
func (w *Whatsmeow) Labels() ([]Label, error) {
	rows, err := w.store.Labels()
	if err != nil {
		return nil, err
	}
	return labelsFromStore(rows), nil
}

// CreateLabel allocates the next unused numeric id, sends a LabelEdit
// app-state patch and optimistically stores the label.
func (w *Whatsmeow) CreateLabel(ctx context.Context, name string, color int) (string, error) {
	// Every id ever used counts, deleted and WhatsApp's own lists included:
	// an edit under a taken id would rename that label everywhere.
	ids, err := w.store.LabelIDs()
	if err != nil {
		return "", fmt.Errorf("chatot/client: list labels: %w", err)
	}
	id := nextLabelID(ids)
	if err := w.wa.SendAppState(ctx, appstate.BuildLabelEdit(id, name, int32(color), false)); err != nil {
		return "", fmt.Errorf("chatot/client: send label-edit app-state: %w", err)
	}
	if err := w.store.UpsertLabel(id, name, color, false, false); err != nil {
		w.log.Warnf("chatot/client: optimistic label create failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventLabelUpdate})
	return id, nil
}

// EditLabel renames/recolors an existing label.
func (w *Whatsmeow) EditLabel(ctx context.Context, id, name string, color int) error {
	if err := w.wa.SendAppState(ctx, appstate.BuildLabelEdit(id, name, int32(color), false)); err != nil {
		return fmt.Errorf("chatot/client: send label-edit app-state: %w", err)
	}
	if err := w.store.UpsertLabel(id, name, color, false, false); err != nil {
		w.log.Warnf("chatot/client: optimistic label edit failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventLabelUpdate})
	return nil
}

// DeleteLabel marks a label deleted, preserving its existing name/color in
// the outgoing patch (WhatsApp's LabelEdit carries them even on delete).
func (w *Whatsmeow) DeleteLabel(ctx context.Context, id string) error {
	name, color := "", 0
	if rows, err := w.store.Labels(); err == nil {
		for _, l := range rows {
			if l.ID == id {
				name, color = l.Name, l.Color
				break
			}
		}
	}
	if err := w.wa.SendAppState(ctx, appstate.BuildLabelEdit(id, name, int32(color), true)); err != nil {
		return fmt.Errorf("chatot/client: send label-delete app-state: %w", err)
	}
	if err := w.store.UpsertLabel(id, name, color, true, false); err != nil {
		w.log.Warnf("chatot/client: optimistic label delete failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventLabelUpdate})
	return nil
}

// SetChatLabeled associates/disassociates chatJID with labelID via app-state.
func (w *Whatsmeow) SetChatLabeled(ctx context.Context, labelID, chatJID string, labeled bool) error {
	target, err := types.ParseJID(chatJID)
	if err != nil {
		return fmt.Errorf("chatot/client: parse jid %q: %w", chatJID, err)
	}
	if err := w.wa.SendAppState(ctx, appstate.BuildLabelChat(target, labelID, labeled)); err != nil {
		return fmt.Errorf("chatot/client: send label-chat app-state: %w", err)
	}
	if err := w.store.SetChatLabel(labelID, chatJID, labeled); err != nil {
		w.log.Warnf("chatot/client: optimistic chat-label update failed: %v", err)
	}
	w.pushEvent(Event{Kind: EventLabelUpdate})
	return nil
}

// LabelsForChat returns the ids of labels currently on chatJID.
func (w *Whatsmeow) LabelsForChat(chatJID string) ([]string, error) {
	return w.store.LabelsForChat(chatJID)
}
