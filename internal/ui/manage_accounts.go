package ui

import (
	"log"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/client"
)

// manageAccountsAvatarSize matches the switcher's row avatar.
const manageAccountsAvatarSize = 40

// ShowManageAccountsDialog opens the "Accounts" manager: a per-account row
// (avatar + name + status + a ⋮ menu of Relink/Remove) and an Add… button that
// chains into the add-account dialog. The list rebuilds after every add/remove
// and calls onChanged so the switcher/header stay in sync. Room is intentionally
// left below the list for F60's per-account toggles.
func ShowManageAccountsDialog(parent *gtk.Window, am *client.AccountManager, onChanged func()) {
	dialog := gtk.NewWindow()
	dialog.SetTitle("Accounts")
	if parent != nil {
		dialog.SetTransientFor(parent)
	}
	dialog.SetModal(true)
	dialog.SetDefaultSize(400, 480)

	box := gtk.NewBox(gtk.OrientationVertical, 12)
	box.SetMarginTop(16)
	box.SetMarginBottom(16)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)

	header := gtk.NewBox(gtk.OrientationHorizontal, 8)
	title := gtk.NewLabel("Accounts")
	title.AddCSSClass("title-4")
	title.SetXAlign(0)
	title.SetHExpand(true)
	addBtn := gtk.NewButtonWithLabel("Add…")
	addBtn.AddCSSClass("suggested-action")
	header.Append(title)
	header.Append(addBtn)
	box.Append(header)

	list := gtk.NewListBox()
	list.SetSelectionMode(gtk.SelectionNone)
	list.AddCSSClass("boxed-list")

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetChild(list)
	box.Append(scroller)

	var rebuild func()
	changed := func() {
		rebuild()
		if onChanged != nil {
			onChanged()
		}
	}
	rebuild = func() {
		for child := list.FirstChild(); child != nil; {
			next := gtk.BaseWidget(child).NextSibling()
			list.Remove(child)
			child = next
		}
		for _, meta := range am.Accounts() {
			list.Append(buildManageAccountRow(dialog, am, meta, changed))
		}
	}
	rebuild()

	addBtn.ConnectClicked(func() {
		ShowAddAccountDialog(dialog, am, changed)
	})

	dialog.SetChild(box)
	dialog.Present()
}

// buildManageAccountRow renders one account row: avatar, name over status, and
// a ⋮ menu with Relink and Remove.
func buildManageAccountRow(dialog *gtk.Window, am *client.AccountManager, meta client.AccountMeta, onChanged func()) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.SetMarginTop(6)
	row.SetMarginBottom(6)
	row.SetMarginStart(8)
	row.SetMarginEnd(8)

	row.Append(newAvatarInitial(meta.ID, initialFor(meta.Name), manageAccountsAvatarSize))

	textCol := gtk.NewBox(gtk.OrientationVertical, 0)
	textCol.SetHExpand(true)
	textCol.SetVAlign(gtk.AlignCenter)

	name := gtk.NewLabel(meta.Name)
	name.SetXAlign(0)
	name.AddCSSClass("chatot-chat-name")
	textCol.Append(name)

	status := gtk.NewLabel(meta.Status)
	status.SetXAlign(0)
	status.AddCSSClass("chatot-account-status")
	textCol.Append(status)

	row.Append(textCol)

	menuBtn := gtk.NewMenuButton()
	menuBtn.SetIconName("view-more-symbolic")
	menuBtn.AddCSSClass("flat")
	menuBtn.SetVAlign(gtk.AlignCenter)

	pop := gtk.NewPopover()
	menuBox := gtk.NewBox(gtk.OrientationVertical, 0)

	relink := gtk.NewButtonWithLabel("Relink")
	relink.AddCSSClass("flat")
	relink.SetHAlign(gtk.AlignFill)
	relink.Child().(*gtk.Label).SetXAlign(0)
	relink.ConnectClicked(func() {
		pop.Popdown()
		showRelinkDialog(dialog, am, meta.ID, onChanged)
	})
	menuBox.Append(relink)

	remove := gtk.NewButtonWithLabel("Remove")
	remove.AddCSSClass("flat")
	remove.SetHAlign(gtk.AlignFill)
	remove.Child().(*gtk.Label).SetXAlign(0)
	remove.ConnectClicked(func() {
		pop.Popdown()
		confirmRemoveAccount(dialog, am, meta, onChanged)
	})
	menuBox.Append(remove)

	pop.SetChild(menuBox)
	menuBtn.SetPopover(pop)
	row.Append(menuBtn)

	return row
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
// user can re-link a logged-out (or to-be-re-paired) account. It subscribes to
// that account's own QR/pair streams; on link it closes and calls onChanged.
func showRelinkDialog(parent *gtk.Window, am *client.AccountManager, id string, onChanged func()) {
	acct := am.Find(id)
	if acct == nil {
		return
	}

	dialog := gtk.NewWindow()
	dialog.SetTitle("Relink account")
	dialog.SetTransientFor(parent)
	dialog.SetModal(true)
	dialog.SetDefaultSize(320, 400)

	box := gtk.NewBox(gtk.OrientationVertical, 12)
	box.SetMarginTop(16)
	box.SetMarginBottom(16)
	box.SetMarginStart(16)
	box.SetMarginEnd(16)

	intro := gtk.NewLabel("On your phone: Linked Devices → Link a device, then scan this code.")
	intro.SetWrap(true)
	intro.SetJustify(gtk.JustifyCenter)
	intro.SetMaxWidthChars(36)
	box.Append(intro)

	qrPic := gtk.NewPicture()
	qrPic.SetSizeRequest(256, 256)
	qrPic.SetCanShrink(false)
	box.Append(qrPic)

	status := gtk.NewLabel("Waiting for you to scan…")
	status.SetWrap(true)
	status.SetJustify(gtk.JustifyCenter)
	box.Append(status)

	done := make(chan struct{})
	closed := false
	closeOnce := func() {
		if !closed {
			closed = true
			close(done)
		}
	}
	dialog.ConnectCloseRequest(func() bool {
		closeOnce()
		return false
	})

	go func() {
		qr := acct.QRCodes()
		for {
			select {
			case <-done:
				return
			case code, ok := <-qr:
				if !ok {
					return
				}
				glib.IdleAdd(func() { setQRPicture(qrPic, status, code) })
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
