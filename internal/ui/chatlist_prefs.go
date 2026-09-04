package ui

// ShowMessagePreviews mirrors settings.ShowMessagePreviews: whether a chat
// row shows its last message under the name.
var ShowMessagePreviews = true

// RebuildRows throws away every built row and refreshes, for a preference
// that changes how rows render (message previews) rather than what they
// hold: the reconciler only rebuilds a row whose view model changed.
func (cl *ChatList) RebuildRows() {
	cl.resetList()
	cl.refresh()
}

// FocusSearch moves keyboard focus into the sidebar's search entry (the
// Ctrl+K accelerator).
func (cl *ChatList) FocusSearch() {
	if cl.searchEntry != nil {
		cl.searchEntry.GrabFocus()
	}
}
