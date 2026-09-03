package ui

import (
	"log"
	"strings"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
	"chatot/internal/settings"
)

// manageAccountsAvatarSize is the mockup's 34px row avatar.
const manageAccountsAvatarSize = 34

// ShowManageAccountsDialog opens the mockup's "Accounts" card: the title row
// carries the green Add… pill (and no ✕ — the design's card has none), then a
// bordered list of accounts (avatar, label, a mono "phone · state" line, a ⋮
// of Relabel/Relink/Remove) and a second card with the two global toggles.
// The list rebuilds after every change and calls onChanged so the switcher
// and header stay in sync; toggle changes mutate prefs, call onSettingsChanged
// to persist, and (for keep-connected) apply immediately to the manager.
func ShowManageAccountsDialog(parent *gtk.Window, am *client.AccountManager, prefs *settings.Settings, onChanged, onSettingsChanged func()) {
	dialog := newCardDialog()
	dialog.SetTitle("Accounts")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(420, -1)

	addBtn := gtk.NewButtonWithLabel("Add…")
	addBtn.AddCSSClass("chatot-primary-btn")
	addBtn.AddCSSClass("chatot-dialog-headbtn")
	addBtn.SetVAlign(gtk.AlignCenter)
	dialog.PackEnd(addBtn)

	box := dialogBody(10)

	// The card is content-sized: a stretching list left the last row with a
	// slab of empty space under it.
	list := newSettingsCard()
	box.Append(list)

	var rebuild func()
	changed := func() {
		rebuild()
		if onChanged != nil {
			onChanged()
		}
	}
	rebuild = func() {
		removeAllChildren(list.Box)
		list.rows = 0
		for _, meta := range am.Accounts() {
			list.Add(buildManageAccountRow(dialog.Window(), am, meta, changed))
		}
	}
	rebuild()

	addBtn.ConnectClicked(func() {
		ShowAddAccountDialog(dialog.Window(), am, changed)
	})

	toggles := newSettingsCard()
	perAccount, _ := newSwitchRow("Notifications per account",
		"Toast titles are prefixed with the label",
		prefs.NotificationsPerAccount, func(on bool) {
			prefs.NotificationsPerAccount = on
			NotificationsPerAccount = on
			if onSettingsChanged != nil {
				onSettingsChanged()
			}
		})
	toggles.Add(perAccount)
	keepConnected, _ := newSwitchRow("Keep inactive accounts connected",
		"Receive while another account is shown",
		prefs.KeepInactiveConnected, func(on bool) {
			prefs.KeepInactiveConnected = on
			am.SetKeepInactiveConnected(on)
			if onSettingsChanged != nil {
				onSettingsChanged()
			}
		})
	toggles.Add(keepConnected)
	box.Append(toggles)

	dialog.SetChild(box)
	dialog.Present()
}

// accountStatusSubline is the mockup's per-account line: the phone number and
// the connection state, mono and lower-case, e.g. "+351912345678 · connected".
// Falls back to the state alone for an account that isn't linked yet.
func accountStatusSubline(meta client.AccountMeta) string {
	state := strings.ToLower(meta.Status)
	if meta.Phone == "" {
		return state
	}
	return meta.Phone + " · " + state
}

// buildManageAccountRow renders one account row: avatar, name over the mono
// status line (red when the account needs relinking), and a vertical ⋮
// opening the design's Relabel/Relink/Remove menu.
func buildManageAccountRow(dialog *gtk.Window, am *client.AccountManager, meta client.AccountMeta, onChanged func()) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 11)
	row.AddCSSClass("chatot-card-row")

	row.Append(newAvatarInitial(meta.ID, initialFor(meta.Name), manageAccountsAvatarSize))

	textCol := gtk.NewBox(gtk.OrientationVertical, 2)
	textCol.SetHExpand(true)
	textCol.SetVAlign(gtk.AlignCenter)

	name := gtk.NewLabel(meta.Name)
	name.SetXAlign(0)
	name.AddCSSClass("chatot-people-name")
	textCol.Append(name)

	status := gtk.NewLabel(accountStatusSubline(meta))
	status.SetXAlign(0)
	status.SetWrap(true)
	status.AddCSSClass("chatot-account-status")
	if meta.Status != "Connected" {
		status.AddCSSClass("chatot-status-bad")
	}
	textCol.Append(status)

	row.Append(textCol)

	menuBtn := gtk.NewMenuButton()
	// A text ⋮ (vertical, like the header's) via SetChild; the symbolic icon
	// rendered horizontal and a MenuButton's own label sits off-centre.
	menuBtn.SetChild(gtk.NewLabel("⋮"))
	menuBtn.AddCSSClass("flat")
	menuBtn.AddCSSClass("chatot-hdr-icon")
	menuBtn.SetVAlign(gtk.AlignCenter)
	menuBtn.SetTooltipText("Account options")
	pop := newMenuPopover(accountRowMenuItems(accountRowMenuActions{
		Relabel: func() { showRelabelAccountDialog(dialog, am, meta, onChanged) },
		Relink:  func() { showRelinkDialog(dialog, am, meta.ID, onChanged) },
		Remove:  func() { confirmRemoveAccount(dialog, am, meta, onChanged) },
	}))
	menuBtn.SetPopover(pop)
	row.Append(menuBtn)

	return row
}

// accountRowMenuActions are the Accounts card's per-row ⋮ callbacks.
type accountRowMenuActions struct {
	Relabel func()
	Relink  func()
	Remove  func()
}

// accountRowMenuItems is the per-account ⋮ menu. The mockup names the three
// as "Relabel · Reconnect · Log out"; chatot's reconnect is a QR relink and
// its log-out removes the account from this device, so the rows say that.
func accountRowMenuItems(a accountRowMenuActions) []menuItem {
	return []menuItem{
		{Icon: "✎", Label: "Relabel…", OnActivate: a.Relabel},
		{Icon: "🔗", Label: "Relink", OnActivate: a.Relink},
		{Icon: "⏻", Label: "Remove", Destructive: true, OnActivate: a.Remove},
	}
}

// showRelabelAccountDialog renames an account's switcher/rail label.
func showRelabelAccountDialog(parent *gtk.Window, am *client.AccountManager, meta client.AccountMeta, onChanged func()) {
	dialog := newCardDialog()
	dialog.SetTitle("Relabel account")
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(340, -1)

	box := dialogBody(12)
	card := newSettingsCard()
	fieldRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	fieldRow.AddCSSClass("chatot-card-row")
	fieldRow.Append(settingsRowBody("Label", "Shown in the account button"))
	entry := gtk.NewEntry()
	entry.SetText(meta.Name)
	entry.SetVAlign(gtk.AlignCenter)
	entry.SetSizeRequest(140, -1)
	entry.AddCSSClass("chatot-card-entry")
	fieldRow.Append(entry)
	card.Add(fieldRow)
	box.Append(card)

	status := gtk.NewLabel("")
	status.SetWrap(true)
	status.SetJustify(gtk.JustifyCenter)
	status.AddCSSClass("chatot-linking-status")
	status.SetVisible(false)
	box.Append(status)

	saveBtn := gtk.NewButtonWithLabel("Save")
	saveBtn.AddCSSClass("chatot-primary-btn")
	saveBtn.SetHExpand(true)
	save := func() {
		if err := am.RenameAccount(meta.ID, entry.Text()); err != nil {
			log.Printf("chatot: relabel account %q failed: %v", meta.ID, err)
			status.SetText("The label can't be empty")
			status.SetVisible(true)
			return
		}
		dialog.Close()
		if onChanged != nil {
			onChanged()
		}
	}
	saveBtn.ConnectClicked(save)
	entry.ConnectActivate(save)
	box.Append(saveBtn)

	dialog.SetChild(box)
	dialog.Present()
	entry.GrabFocus()
}

// confirmRemoveAccount asks before removing meta, then removes it off the main
// loop and refreshes on success (or shows the guard error, e.g. last account).
func confirmRemoveAccount(parent *gtk.Window, am *client.AccountManager, meta client.AccountMeta, onChanged func()) {
	confirm := adw.NewAlertDialog("Remove "+meta.Name+"?", "This account is unlinked from chatot on this device. Its downloaded data is kept.")
	confirm.AddResponse("cancel", "Cancel")
	confirm.AddResponse("remove", "Remove")
	confirm.SetResponseAppearance("remove", adw.ResponseDestructive)
	confirm.SetDefaultResponse("cancel")
	confirm.SetCloseResponse("cancel")
	confirm.ConnectResponse(func(response string) {
		if response != "remove" {
			return
		}
		go func() {
			err := am.RemoveAccount(meta.ID)
			glib.IdleAdd(func() {
				if err != nil {
					log.Printf("chatot: remove account %q failed: %v", meta.ID, err)
					return
				}
				if onChanged != nil {
					onChanged()
				}
			})
		}()
	})
	confirm.Present(parent)
}

// showRelinkDialog re-presents the pairing QR for an existing account so the
// user can re-link a logged-out account. It is the Add-account card without
// the label field: instruction, the QR card, a status line. It subscribes to
// that account's own QR/pair streams; on link it closes and calls onChanged.
func showRelinkDialog(parent *gtk.Window, am *client.AccountManager, id string, onChanged func()) {
	acct := am.Find(id)
	if acct == nil {
		return
	}

	dialog := newCardDialog()
	dialog.SetTitle("Relink account")
	dialog.SetTransientFor(parent)
	dialog.SetModal(true)
	dialog.SetDefaultSize(360, -1)

	box := dialogBody(12)

	intro := gtk.NewLabel("On your phone: Linked devices → Link a device, then scan this code.")
	intro.SetWrap(true)
	intro.SetJustify(gtk.JustifyCenter)
	intro.SetMaxWidthChars(40)
	intro.AddCSSClass("chatot-card-sub")
	box.Append(intro)

	qr := newQRCard()
	box.Append(qr.card)

	status := gtk.NewLabel("Waiting for a code…")
	status.SetWrap(true)
	status.SetJustify(gtk.JustifyCenter)
	status.AddCSSClass("chatot-linking-status")
	box.Append(status)
	// A linked account never emits a QR: whatsmeow only pairs a fresh session.
	if acct.LoggedIn() {
		status.SetText("This account is already linked. Remove it and add it again to pair a new session.")
	}

	done := make(chan struct{})
	closed := false
	closeOnce := func() {
		if !closed {
			closed = true
			close(done)
		}
	}
	dialog.ConnectClosed(closeOnce)

	go func() {
		codes := acct.QRCodes()
		for {
			select {
			case <-done:
				return
			case code, ok := <-codes:
				if !ok {
					return
				}
				glib.IdleAdd(func() {
					qr.Set(code, status)
					status.SetText("Waiting for you to scan…")
				})
			}
		}
	}()

	go func() {
		ev := acct.Events()
		for {
			select {
			case <-done:
				return
			case e, ok := <-ev:
				if !ok {
					return
				}
				if e.Kind == client.EventPairSuccess ||
					(e.Kind == client.EventConnection && e.Connection != nil && e.Connection.Connected) {
					glib.IdleAdd(func() {
						closeOnce()
						dialog.Close()
						if onChanged != nil {
							onChanged()
						}
					})
					return
				}
			}
		}
	}()

	dialog.SetChild(box)
	dialog.Present()
}
