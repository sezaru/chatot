package ui

import (
	"context"
	"fmt"
	"log"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/core/gioutil"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
	"github.com/diamondburned/gotk4/pkg/pango"

	"chatot/internal/client"
)

// chatRowView holds the pure, pre-rendered display fields for one chat row.
// chatRowVM computes it from a client.Chat so it can be unit-tested without
// a display.
type chatRowView struct {
	JID        string
	Initial    string
	Name       string
	Preview    string
	TimeText   string
	UnreadText string
	ShowUnread bool
	Typing     bool // set by ChatList.refresh from live presence, not chatRowVM
	Pinned     bool
	Muted      bool
	Blocked    bool // set by ChatList.refreshChats via c.IsBlocked, not chatRowVM (pure)
	// AccountColor is the avatar-palette class of the account a row belongs
	// to, set only in merged mode; "" draws no stripe.
	AccountColor string
}

// chatRowVM derives the display view-model for a single chat row. now is
// injected so time formatting is deterministic in tests.
func chatRowVM(c client.Chat, now time.Time) chatRowView {
	name := c.Name
	if name == "" {
		name = "Unknown"
	}

	initial := "?"
	for _, r := range name {
		initial = strings.ToUpper(string(r))
		break
	}

	view := chatRowView{
		JID:      c.JID,
		Initial:  initial,
		Name:     name,
		Preview:  oneLine(c.Preview),
		TimeText: formatChatTime(c.LastMessageTS, now),
		Pinned:   c.Pinned,
		Muted:    c.Muted,
	}
	if c.UnreadCount > 0 {
		view.ShowUnread = true
		if c.UnreadCount > 99 {
			view.UnreadText = "99+"
		} else {
			view.UnreadText = strconv.Itoa(c.UnreadCount)
		}
	}
	return view
}

// oneLine collapses a multi-line message into the single preview line the
// row has room for: newlines become spaces, runs of whitespace one space.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// showChatInList is the archived-filter predicate: with the toggle off, only
// non-archived chats show; with it on, only archived ones do.
func showChatInList(c client.Chat, showArchived bool) bool {
	return c.Archived == showArchived
}

// formatChatTime renders ts (unix seconds) relative to now: "15:04" for
// today, weekday name for the last 7 days, "02/01" otherwise.
func formatChatTime(ts int64, now time.Time) string {
	if ts == 0 {
		return ""
	}
	t := time.Unix(ts, 0).In(now.Location())
	daysSince := int(now.Sub(t).Hours() / 24)

	sameDay := t.Year() == now.Year() && t.YearDay() == now.YearDay()
	if sameDay {
		return t.Format("15:04")
	}
	if daysSince >= 0 && daysSince < 7 {
		return t.Format("Monday")
	}
	return t.Format("02/01")
}

// searchResultLimit caps how many hits the sidebar search shows at once.
const searchResultLimit = 30

// ChatList is the sidebar pane: a search entry over a live list of chats
// backed by a client.Client, with a selection callback for the content
// pane. Below the search entry a single ListBox holds either the normal
// chat rows or, while a query is active, search-hit rows; both row kinds
// resolve to a ChatJID in rowJIDs, so activation and onSelect are shared.
type ChatList struct {
	*gtk.Box

	c      client.Client
	events <-chan client.Event
	// list holds what is not a chat row: search hits and the empty state.
	list *gtk.ListBox
	// The chat rows are a GtkListView over chatModel (chatlist_rows.go);
	// listStack shows it or list, and listScroller is the view's scroller.
	chatModel    *gioutil.ListModel[chatRowItem]
	chatSel      *gtk.SingleSelection
	chatView     *gtk.ListView
	listStack    *gtk.Stack
	listScroller *gtk.ScrolledWindow
	// rowWidgets maps a row widget (by native pointer) to its Go side and
	// boundRows the rows currently bound, by key; avatarGens counts avatar
	// changes per jid so a rebind swaps the picture.
	rowWidgets map[uintptr]*chatRowWidget
	boundRows  map[string]*chatRowWidget
	avatarGens map[string]int
	// reselecting marks a selection change made by reselectRow, not the
	// reader.
	reselecting bool
	searchEntry *gtk.SearchEntry
	railSig     string
	// listKind says which mode built the chat rows; see chatlist_rows.go.
	listKind string
	// lastChips is the chip strip as last built, so a refresh that changes
	// no chip leaves the strip (and its scroll position) alone.
	lastChips       []chipSpec
	chipLabels      []client.Label
	chipLabelCounts map[string]int
	// refreshQueued coalesces the generic event-driven refresh: a history
	// backfill publishes hundreds of events in a row and each full rebuild
	// of the rows costs a store query per chat.
	refreshQueued atomic.Bool
	// refreshInFlight, refreshDirty and refreshGen belong to refreshAsync
	// (chatlist_refresh.go).
	refreshInFlight bool
	refreshDirty    bool
	refreshGen      int
	// syncBanner is the "Syncing older messages" strip above the rows.
	syncBanner *gtk.Box
	syncLabel  *gtk.Label
	syncBar    *gtk.ProgressBar
	rowJIDs    []string // row index -> JID, rebuilt alongside the ListBox rows
	// rowAccounts parallels rowJIDs in merged mode: row index -> owning
	// account id. nil in every other mode.
	rowAccounts []string
	onSelect    func(jid string)
	// selectedJID is the chat open in the content pane. Rows are rebuilt from
	// scratch on every refresh, which drops GtkListBox's own highlight, so
	// the rebuild re-selects this JID's row (see reselectRow).
	selectedJID  string
	composing    composers      // who is typing or recording, per chat
	composingGen map[string]int // composing event counter per chat and sender (composingKey); the stale timer checks it
	// names memoizes ContactName lookups for @mentions in previews; dropped
	// on EventChatUpdate (the contact sync fires one when names land).
	names map[string]string
	// rowMenu, when set, supplies the rows of a chat row's right-click
	// menu (main wires it to the conversation's ⋮ menu); nil keeps the
	// design's own row menu.
	rowMenu func(chat client.Chat) []menuItem
	// chipScroller lets the filter chips scroll sideways rather than
	// grow the sidebar when a label chip joins them.
	chipScroller   *gtk.ScrolledWindow
	query          string // current search text; "" shows the normal chat list
	avatarCache    *avatarCache
	window         *gtk.Window // parent for the new-chat dialog; set via SetWindow
	onNewCommunity func()      // "New community" from the ＋ menu; STUBBED until F48
	showArchived   bool        // toggled by the "Archived" button; see showChatInList
	onStarred      func()
	// merged shows every account's chats in one list (the mockup's 🗂 mode).
	merged      bool
	newsletters []client.Newsletter // channels backing the sidebar in channels mode

	// tab is the bottom bar's active tab: "chats", "status", "channels" or
	// "communities". discover is the Channels tab's Find channels page.
	tab      string
	tabBar   *tabBar
	discover bool
	// backAction runs on the "←" of the header's back page (the archived
	// list, Find channels); archivedTitle is that page's title.
	backAction  func()
	plusPopover *gtk.Popover
	onShowPane  func(name string)
	onForward   func(msg client.Message)
	toasts      *adw.ToastOverlay
	emptyPane   *gtk.Box

	statusList    *gtk.ListBox
	statusActions []func()
	statusFeed    statusFeed
	statusPane    *StatusPane

	channelsList    *gtk.ListBox
	channelsCaption *gtk.Label
	discoverList    *gtk.ListBox
	discoverCaption *gtk.Label
	discoverChips   *gtk.Box
	discoverChipBar *gtk.ScrolledWindow
	discoverEntry   *gtk.Entry
	discoverQuery   string
	discoverCat     string
	discoverRows    []client.Newsletter
	channelPane     *ChannelPane
	currentChannel  string

	communitiesList  *gtk.ListBox
	communities      []client.Community
	communitiesGen   int // bumps per roster fetch; stale responses are dropped
	communityPane    *CommunityPane
	currentCommunity string

	// modes swaps the chat list for an in-sidebar form (new chat/group/community).
	modes *gtk.Stack

	chipRow *gtk.Box   // fixed + inline-label filter chips, under the search entry
	filter  chatFilter // the active chip; see chatFilter/chatMatchesFilter
	search  *gtk.SearchEntry
	// identityStack swaps the header's account button for the archived
	// title; archivedTitle carries the archived count.
	identityStack *gtk.Stack
	archivedTitle *gtk.Label

	// Account switcher (F58): the header identity is a MenuButton whose popover
	// lists accounts; switcher is nil until SetAccountSwitcher wires the manager.
	switcher           AccountSwitcher
	rail               *gtk.Box          // vertical account strip left of the list (mockup rail)
	accountBtn         *gtk.MenuButton   // header switcher button; dev-hook popup target
	plusBtn            *gtk.MenuButton   // header ＋ menu; dev-hook popup target
	appMenuBtn         *gtk.MenuButton   // header ⋮ app menu; dev-hook popup target
	archiveT           *gtk.ToggleButton // "Archived" mode toggle; dev-hook target
	overflowBtn        *gtk.Button       // chip-row "…" label overflow; dev-hook target
	overflowPop        *gtk.Popover      // the … popover while it is open, else nil
	accountAvatar      *gtk.Label
	accountAvatarClass string // current avatar palette class, swapped on active change
	// rebuildAppMenu repaints the ⋮ application menu, whose bottom row
	// depends on whether the account is still linked. Set during
	// construction, called from refreshAccountHeader.
	rebuildAppMenu func()
	// appMenuLinked is the link state the ⋮ menu was last built for, so the
	// popover is only rebuilt when it actually changes; rebuiltAppMenu marks
	// that it has been built at all, since false is a meaningful state.
	appMenuLinked  bool
	rebuiltAppMenu bool

	// shotSel/shotName/shotCreate expose the open group-name step to the
	// dev hooks; nil outside one.
	shotSel          *participantSelection
	shotName         *gtk.Entry
	shotCreate       *gtk.Button
	shotPickFirst    func()       // people step: pick the first contact
	shotPhoto        func([]byte) // name step: apply a picked JPEG
	accountName      *gtk.Label
	accountStatus    *gtk.Label
	onAddAccount     func() // "Add account…"; STUBBED until F59
	onManageAccounts func() // "Manage accounts…"; STUBBED until F59
}

// SetWindow supplies the parent window the new-chat dialog needs; call once
// after the top-level window exists.
func (cl *ChatList) SetWindow(w *gtk.Window) { cl.window = w }

// NewChatList builds a ChatList for c and populates it with the current
// chats. It also subscribes to c.Events() to keep the list live.
func NewChatList(c client.Client) *ChatList {
	root := gtk.NewBox(gtk.OrientationVertical, 0)

	// Row 1: account header, laid out like the mockup's slim top bar: the
	// identity button (avatar + name over phone + chevron) fills the left,
	// then ＋ and ⋮ in that order. A WindowHandle keeps the strip draggable
	// like the header bar it replaces; window controls live on the
	// conversation side (right pane), except the start side of the
	// desktop's gtk-decoration-layout, which lands here: empty under the
	// usual "…:minimize,maximize,close", populated for a left-hand layout.
	headerBox := gtk.NewBox(gtk.OrientationHorizontal, 6)
	headerBox.AddCSSClass("chatot-account-row")
	startControls := newWindowControls(gtk.PackStart)
	headerBox.Append(startControls)

	accountAvatar := gtk.NewLabel("S")
	accountAvatar.AddCSSClass("chatot-avatar")
	accountAvatar.AddCSSClass("chatot-account-avatar")
	accountAvatar.SetSizeRequest(28, 28)
	// Centred, or the label fills the 38px button height and the circle
	// renders as a tall pill.
	accountAvatar.SetVAlign(gtk.AlignCenter)
	centreGlyph(accountAvatar, 28)

	accountText := gtk.NewBox(gtk.OrientationVertical, 0)
	accountText.SetHExpand(true)
	accountText.SetVAlign(gtk.AlignCenter)
	accountName := gtk.NewLabel("Account 1")
	accountName.SetXAlign(0)
	accountName.SetHExpand(true)
	accountName.SetEllipsize(pango.EllipsizeEnd)
	accountName.SetMaxWidthChars(1)
	accountName.AddCSSClass("chatot-account-name")
	accountText.Append(accountName)
	accountStatus := gtk.NewLabel("")
	accountStatus.SetXAlign(0)
	accountStatus.SetEllipsize(pango.EllipsizeEnd)
	accountStatus.SetMaxWidthChars(1)
	accountStatus.AddCSSClass("chatot-account-phone")
	accountText.Append(accountStatus)

	accountBtnBox := gtk.NewBox(gtk.OrientationHorizontal, 9)
	accountBtnBox.SetHExpand(true)
	accountBtnBox.Append(accountAvatar)
	accountBtnBox.Append(accountText)
	// A text glyph, not pan-down-symbolic: the mockup's caret is a tiny 9px
	// ▾ and the symbolic icon refuses to render that small.
	chevron := gtk.NewLabel("▾")
	chevron.AddCSSClass("chatot-account-caret")
	accountBtnBox.Append(chevron)

	accountBtn := gtk.NewMenuButton()
	accountBtn.AddCSSClass("flat")
	accountBtn.AddCSSClass("chatot-account-btn")
	accountBtn.SetHExpand(true)
	accountBtn.SetChild(accountBtnBox)
	accountBtn.SetTooltipText("Switch account")

	// The identity slot swaps for the mockup's "← Archived · N" while the
	// archived list shows, keeping ＋ and ⋮ where they are.
	identity := gtk.NewStack()
	identity.SetHExpand(true)
	identity.AddNamed(accountBtn, "account")
	archivedHead := gtk.NewBox(gtk.OrientationHorizontal, 8)
	archivedHead.AddCSSClass("chatot-archived-head")
	archivedBack := gtk.NewButtonWithLabel("←")
	archivedBack.AddCSSClass("flat")
	archivedBack.RemoveCSSClass("text-button")
	archivedBack.AddCSSClass("chatot-pane-back")
	archivedBack.SetVAlign(gtk.AlignCenter)
	archivedBack.SetTooltipText("Back to chats")
	archivedHead.Append(archivedBack)
	archivedTitle := gtk.NewLabel(archivedTitleText(0))
	archivedTitle.SetXAlign(0)
	archivedTitle.SetHExpand(true)
	archivedTitle.AddCSSClass("chatot-sidebar-formtitle")
	archivedHead.Append(archivedTitle)
	identity.AddNamed(archivedHead, "archived")
	headerBox.Append(identity)

	plusBtn := gtk.NewMenuButton()
	// A glyph, not list-add-symbolic: the mockup's ＋ is a 15px text plus and
	// symbolic icons render at the icon theme's own sizes. SetChild rather
	// than SetLabel: a MenuButton's own label sits in a box that reserves
	// room for a dropdown arrow, which shoved the glyph to the left.
	plusBtn.SetChild(gtk.NewLabel("＋"))
	plusBtn.AddCSSClass("flat")
	plusBtn.AddCSSClass("chatot-hdr-icon")
	plusBtn.SetVAlign(gtk.AlignCenter)
	plusBtn.SetTooltipText("New chat, group, community, or invite")
	headerBox.Append(plusBtn)

	// The mockup's menu affordance is a vertical ⋮; a text glyph renders that
	// way under every icon theme (view-more-symbolic varies by theme).
	appMenuBtn := gtk.NewMenuButton()
	appMenuBtn.SetChild(gtk.NewLabel("⋮"))
	appMenuBtn.AddCSSClass("flat")
	appMenuBtn.AddCSSClass("chatot-hdr-icon")
	appMenuBtn.SetVAlign(gtk.AlignCenter)
	appMenuBtn.SetTooltipText("Menu")
	headerBox.Append(appMenuBtn)

	accountRow := gtk.NewWindowHandle()
	accountRow.SetChild(headerBox)
	root.Append(accountRow)

	// Below the header the sidebar splits into the mockup's two columns: the
	// vertical account rail on the left, then the search/chips/list column.
	// The rail is populated by refreshAccountRail once the switcher is wired.
	contentCols := gtk.NewBox(gtk.OrientationHorizontal, 0)
	contentCols.SetVExpand(true)
	rail := gtk.NewBox(gtk.OrientationVertical, 10)
	rail.AddCSSClass("chatot-account-rail")
	// The tiles sit at the top but the strip itself (its fill and its
	// right-hand hairline) runs the full height of the sidebar.
	rail.SetVAlign(gtk.AlignFill)
	contentCols.Append(rail)
	listCol := gtk.NewBox(gtk.OrientationVertical, 0)
	listCol.SetHExpand(true)
	// The mockup renders New chat / New group / New community in the sidebar,
	// not as separate windows, so the list column is a stack those forms swap
	// into (see showSidebarForm).
	modes := gtk.NewStack()
	modes.SetHExpand(true)
	modes.SetVExpand(true)
	modes.AddNamed(listCol, "chats")
	contentCols.Append(modes)
	root.Append(contentCols)

	// Filled below, once cl exists to own the handlers: the menu's rows come
	// from plusMenuItems, which needs the ChatList's dialog methods.
	plusPopover := newMenuPopover(nil)
	plusBtn.SetPopover(plusPopover)

	// Row 2: full-width search. Drives the same query/filter logic as
	// before via ConnectSearchChanged below.
	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search")
	search.AddCSSClass("chatot-search-entry")
	search.SetHExpand(true)
	listCol.Append(search)
	// Input-method diagnostics (CHATOT_TRACE=1), the single-line control
	// for the composer's traces: same keys, same process, different widget.
	searchKeys := gtk.NewEventControllerKey()
	searchKeys.SetPropagationPhase(gtk.PhaseCapture)
	searchKeys.ConnectKeyPressed(func(keyval, _ uint, state gdk.ModifierType) bool {
		trace(1, "search key 0x%x (%s) mods=%d", keyval, gdk.KeyvalName(keyval), state)
		return false
	})
	search.AddController(searchKeys)
	search.ConnectChanged(func() { trace(1, "search text %q", search.Text()) })

	// The old header's filter icons are gone from the bar; they live in the
	// ⋮ app menu (built below) but stay real ToggleButtons so all the
	// existing active-state cross-clearing keeps working unchanged.
	archiveToggle := gtk.NewToggleButton()

	chipRow := gtk.NewBox(gtk.OrientationHorizontal, 6)
	chipRow.AddCSSClass("chatot-chip-row")
	// The chips sit in a sideways scroller with no visible bar: a selected
	// label adds a chip, and the row must not grow the sidebar for it.
	chipScroller := gtk.NewScrolledWindow()
	chipScroller.AddCSSClass("chatot-chip-scroller")
	// No GTK scrollbar (an overlay one steals clicks along the chips'
	// bottom edge); a slim slider of our own sits under the strip instead.
	chipScroller.SetPolicy(gtk.PolicyExternal, gtk.PolicyNever)
	chipScroller.SetPropagateNaturalWidth(false)
	chipScroller.SetHExpand(true)
	chipScroller.SetChild(chipRow)
	// A wheel over the chips scrolls them sideways (there is no vertical
	// travel to give the gesture to). A touchpad brackets its events in
	// begin/end and reports surface pixels, which move the chips as-is; a
	// wheel reports notches, scaled. (gotk4's CurrentEvent is not usable
	// here: it wraps the event as a GObject, which GdkEvent is not.)
	chipWheel := gtk.NewEventControllerScroll(gtk.EventControllerScrollVertical | gtk.EventControllerScrollHorizontal)
	inSequence := false
	chipWheel.ConnectScrollBegin(func() { inSequence = true })
	chipWheel.ConnectScrollEnd(func() { inSequence = false })
	chipWheel.ConnectScroll(func(dx, dy float64) bool {
		h := chipScroller.HAdjustment()
		pixels := inSequence || scrollsInPixels(chipWheel.CurrentEventDevice())
		h.SetValue(h.Value() + chipScrollDelta(pixels, dx, dy))
		return true
	})
	chipScroller.AddController(chipWheel)
	listCol.Append(chipScroller)
	listCol.Append(newChipSlider(chipScroller.HAdjustment()))

	// Post-link backfill banner: hidden until "full" history chunks stream
	// behind the usable chat list (see SetSyncProgress).
	syncBanner := gtk.NewBox(gtk.OrientationVertical, 4)
	syncBanner.AddCSSClass("chatot-sync-banner")
	syncLabel := gtk.NewLabel("")
	syncLabel.SetXAlign(0)
	syncLabel.AddCSSClass("chatot-sync-banner-text")
	syncBar := gtk.NewProgressBar()
	syncBar.AddCSSClass("chatot-sync-banner-bar")
	syncBanner.Append(syncLabel)
	syncBanner.Append(syncBar)
	syncBanner.SetVisible(false)
	listCol.Append(syncBanner)

	// list holds what is not a chat row: search hits and the empty state.
	list := gtk.NewListBox()
	list.AddCSSClass("navigation-sidebar")
	list.SetVExpand(true)

	// The chat rows are a GtkListView over chatModel (chatlist_rows.go);
	// listStack shows it or list.
	chatModel := chatRowModelType.New()
	chatSel := gtk.NewSingleSelection(chatModel)
	chatSel.SetAutoselect(false)
	chatSel.SetCanUnselect(true)

	// Both lists MUST live in a height-constrained scroller. GtkListBox is
	// NOT virtualized, so its minimum height is the sum of EVERY row; with
	// a real account that minimum (~3500px) propagates up and forces the
	// whole window taller than the screen — shoving the composer off the
	// bottom and leaving nothing for either pane to scroll. Wrapping in a
	// ScrolledWindow is necessary but NOT sufficient: MinContentHeight(0)
	// does not override a child's propagated minimum here, but an explicit
	// SetSizeRequest minimum does. VExpand then grows it to fill the pane
	// and it scrolls internally. hscrollbar Never so a long chat name can't
	// widen the sidebar. A wheel notch glides rather than jumps, as in the
	// thread.
	newListScroller := func() *gtk.ScrolledWindow {
		s := gtk.NewScrolledWindow()
		s.SetVExpand(true)
		s.SetPropagateNaturalHeight(false)
		s.SetMinContentHeight(0)
		s.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
		s.SetSizeRequest(-1, 80)
		smoothWheel(s)
		return s
	}
	listScroller := newListScroller()
	otherScroller := newListScroller()
	otherScroller.SetChild(list)
	listStack := gtk.NewStack()
	listStack.SetVExpand(true)
	listStack.AddNamed(listScroller, "chats")
	listStack.AddNamed(otherScroller, "other")
	listCol.Append(listStack)

	cl := &ChatList{
		searchEntry: search,
		Box:         root, c: c, events: c.Events(), list: list, listScroller: listScroller, listStack: listStack,
		chatModel: chatModel, chatSel: chatSel,
		rowWidgets: map[uintptr]*chatRowWidget{}, boundRows: map[string]*chatRowWidget{}, avatarGens: map[string]int{},
		syncBanner: syncBanner, syncLabel: syncLabel, syncBar: syncBar,
		composing: make(composers), composingGen: make(map[string]int), names: make(map[string]string), avatarCache: newAvatarCache(),
		chipRow: chipRow, chipScroller: chipScroller, rail: rail,
		modes:         modes,
		identityStack: identity, archivedTitle: archivedTitle,
		search:        search,
		accountAvatar: accountAvatar, accountAvatarClass: "chatot-account-avatar",
		accountName: accountName, accountStatus: accountStatus, accountBtn: accountBtn,
		plusBtn: plusBtn, appMenuBtn: appMenuBtn, archiveT: archiveToggle,
		plusPopover: plusPopover,
		tab:         "chats",
		discoverCat: "All",
	}
	cl.chatView = cl.newChatView()
	listScroller.SetChild(cl.chatView)

	// The other tabs' sidebar pages and content panes. The pages join the
	// same stack the in-sidebar forms use; the panes are handed to main.go
	// through Panes().
	cl.statusPane = newStatusPane(cl)
	cl.channelPane = newChannelPane(cl)
	cl.communityPane = newCommunityPane(cl)
	cl.emptyPane = gtk.NewBox(gtk.OrientationVertical, 0)
	cl.emptyPane.SetVExpand(true)
	cl.emptyPane.SetHExpand(true)
	modes.AddNamed(cl.buildStatusPage(), "status")
	modes.AddNamed(cl.buildChannelsPage(), "channels")
	modes.AddNamed(cl.buildDiscoverPage(), "discover")
	modes.AddNamed(cl.buildCommunitiesPage(), "communities")

	// The mockup's bottom navigation spans the whole sidebar, rail included,
	// so it sits under the two columns rather than inside the list one.
	cl.tabBar = newTabBar(cl.selectTab)
	root.Append(cl.tabBar)

	accountBtn.SetCreatePopupFunc(func(mb *gtk.MenuButton) {
		if cl.switcher == nil {
			return
		}
		mb.SetPopover(cl.buildAccountPopover())
	})

	// activate opens the chat behind row idx of rowJIDs, whichever list
	// (chat rows, search hits) it came from.
	activate := func(idx int) {
		if idx < 0 || idx >= len(cl.rowJIDs) {
			return
		}
		// Resolve the row before anything rebuilds the list: the account
		// switch below refreshes rowJIDs, after which idx means nothing.
		jid := cl.rowJIDs[idx]
		account := ""
		if idx < len(cl.rowAccounts) {
			account = cl.rowAccounts[idx]
		}

		// In merged mode a row may belong to another account. Opening it has
		// to switch the active account first: every downstream read (the
		// conversation, the composer) goes through the manager's active
		// account, so without this the pane would come up empty.
		if account != "" {
			if err := cl.switcher.SetActive(account); err != nil {
				log.Printf("chatot: switch to account %q failed: %v", account, err)
				return
			}
			cl.merged = false
			cl.refreshAccountHeader()
			cl.selectedJID = jid
			cl.refresh()
		}
		cl.selectedJID = jid
		if cl.onSelect != nil {
			cl.onSelect(jid)
		}
	}
	list.ConnectRowActivated(func(row *gtk.ListBoxRow) { activate(row.Index()) })
	// A click on a chat row selects it, and a selection the reader made
	// opens the chat; one made by reselectRow is the list catching up with
	// a chat opened elsewhere. Enter on a focused row activates it too.
	chatSel.ConnectSelectionChanged(func(_, _ uint) {
		if cl.reselecting {
			return
		}
		if pos := chatSel.Selected(); pos != gtk.InvalidListPosition {
			activate(int(pos))
		}
	})
	cl.chatView.ConnectActivate(func(pos uint) { activate(int(pos)) })

	search.ConnectSearchChanged(func() {
		cl.query = strings.TrimSpace(search.Text())
		if cl.query != "" {
			cl.filter = chatFilter{}
		}

		cl.refresh()
	})

	// The ＋ menu's rows depend on the active tab (see tabPlusMenuItems).
	cl.refreshPlusMenu()

	archiveToggle.ConnectToggled(func() {
		cl.showArchived = archiveToggle.Active()
		if cl.showArchived {
			cl.showBackHead(archivedTitleText(0), func() { archiveToggle.SetActive(false) })
		} else {
			cl.showAccountHead()
		}
		cl.refresh()
	})
	archivedBack.ConnectClicked(func() {
		if cl.backAction != nil {
			cl.backAction()
		}
	})

	// The mockup's rows plus Blocked contacts (see appMenuItems). Status,
	// channels and "set status" live on the bottom mode bar; privacy and
	// shortcuts are Preferences pages.
	appActions := appMenuActions{
		Archived: func() {
			if cl.tab != "chats" {
				cl.selectTab("chats")
			}
			archiveToggle.SetActive(!archiveToggle.Active())
		},
		Starred:       func() { fire(cl.onStarred) },
		Blocked:       func() { showBlockedDialog(cl.window, cl.c) },
		LinkedDevices: func() { showLinkedDevicesDialog(cl.window, cl.c, cl.unlinkDevice) },
		Preferences:   func() { appMenuBtn.ActivateAction("app.preferences", nil) },
		About:         func() { showAboutDialog(cl.window) },
		Unlink:        func() { cl.unlinkDevice() },
		Relink:        func() { cl.relinkDevice() },
		Quit:          func() { appMenuBtn.ActivateAction("app.quit", nil) },
	}
	// The menu's last row follows the link state, so the popover is rebuilt
	// whenever that moves rather than being frozen at construction time —
	// which is how a signed-out account kept on offering "Unlink this
	// device", an action with nothing left to unlink.
	cl.rebuildAppMenu = func() {
		linked := cl.c.LoggedIn() || cl.c.Paired()
		if !appMenuNeedsRebuild(cl.rebuiltAppMenu, cl.appMenuLinked, linked) {
			return
		}
		cl.rebuiltAppMenu = true
		cl.appMenuLinked = linked
		appMenuBtn.SetPopover(newMenuPopover(appMenuItems(linked, appActions)))
	}
	cl.rebuildAppMenu()

	cl.refresh()
	go cl.watchEvents()

	return cl
}

// showAboutDialog presents a minimal AboutDialog for the ⋮ menu's "About".
func showAboutDialog(parent *gtk.Window) {
	dialog := newCardDialog()
	dialog.SetTitle("About chatot")
	dialog.SetTransientFor(parent)
	dialog.SetDefaultSize(360, -1)

	// Hand-built rather than AdwAboutDialog: the mockup's card is an 84px
	// rounded app mark over the name, a mono version line, a tagline, and two
	// pill buttons — none of which AdwAboutDialog's fixed layout produces.
	box := gtk.NewBox(gtk.OrientationVertical, 10)
	box.AddCSSClass("chatot-about")
	box.SetHAlign(gtk.AlignCenter)

	box.Append(newAppMark(84))

	name := gtk.NewLabel("chatot")
	name.AddCSSClass("chatot-about-name")
	box.Append(name)

	version := gtk.NewLabel(aboutVersionLine())
	version.AddCSSClass("chatot-about-version")
	box.Append(version)

	tagline := gtk.NewLabel("A native WhatsApp client for GNOME. Unofficial, not affiliated with WhatsApp or Meta.")
	tagline.AddCSSClass("chatot-about-tagline")
	tagline.SetWrap(true)
	tagline.SetJustify(gtk.JustifyCenter)
	tagline.SetMaxWidthChars(38)
	box.Append(tagline)

	buttons := gtk.NewBox(gtk.OrientationHorizontal, 8)
	buttons.SetHAlign(gtk.AlignCenter)
	buttons.SetMarginTop(6)
	website := gtk.NewButtonWithLabel("Website")
	website.AddCSSClass("chatot-chip-btn")
	website.ConnectClicked(func() { openURI(aboutHomepage) })
	buttons.Append(website)
	report := gtk.NewButtonWithLabel("Report an issue")
	report.AddCSSClass("chatot-chip-btn")
	report.ConnectClicked(func() { openURI(aboutIssues) })
	buttons.Append(report)
	box.Append(buttons)

	dialog.SetChild(box)
	dialog.Present()
}

// aboutVersion / aboutHomepage / aboutIssues are the About card's facts. The
// version is the single place it is declared and carries the beta tag while
// chatot is one; the mockup shows it beside the toolkit chatot is built on.
const (
	aboutVersion  = "0.8.0-beta"
	aboutHomepage = "https://github.com/sezaru/chatot"
	aboutIssues   = "https://github.com/sezaru/chatot/issues"
)

// aboutVersionLine is the mono line under the app name.
func aboutVersionLine() string {
	return aboutVersion + " · GTK4 · libadwaita"
}

// OnChatSelected registers f to be called with the JID of the activated row.
func (cl *ChatList) OnChatSelected(f func(jid string)) {
	cl.onSelect = f
}

// OnNewCommunityRequested registers f to be called when the user picks "New
// community" from the ＋ menu; STUBBED until F48 implements communities.
func (cl *ChatList) OnNewCommunityRequested(f func()) { cl.onNewCommunity = f }

// SetAccountSwitcher wires the multi-account manager behind the header's
// switcher popover and refreshes the header to the active account. The rest of
// the app keeps holding a plain client.Client; only this seam sees the manager.
func (cl *ChatList) SetAccountSwitcher(sw AccountSwitcher) {
	cl.switcher = sw
	cl.refreshAccountHeader()
}

// OnAddAccountRequested registers f for the switcher's "Add account…" item;
// STUBBED until F59 builds the add-account flow.
// PopupAccountSwitcher opens the account switcher popover — a dev/screenshot hook.
func (cl *ChatList) PopupAccountSwitcher() {
	if cl.accountBtn != nil {
		cl.accountBtn.Popup()
	}
}

func (cl *ChatList) OnAddAccountRequested(f func()) { cl.onAddAccount = f }

// RefreshAccounts repaints the header identity from the switcher; call after
// an account is added or removed so the header reflects the new roster (the
// switcher popover rebuilds itself on each open).
func (cl *ChatList) RefreshAccounts() {
	cl.refreshAccountHeader()
	cl.refreshAppMenu()
}

// OnManageAccountsRequested registers f for the switcher's "Manage accounts…"
// item; STUBBED until F59 builds the manage-accounts flow.
func (cl *ChatList) OnManageAccountsRequested(f func()) { cl.onManageAccounts = f }

// refreshAccountHeader repaints the header identity (avatar initial + palette
// colour, name, status line) from the switcher's active account. Called at
// wire time and after each successful switch; SetActive itself publishes a
// refresh event that reloads the chat list separately.
func (cl *ChatList) refreshAccountHeader() {
	if cl.switcher == nil {
		return
	}
	metas := cl.switcher.Accounts()
	if len(metas) == 0 {
		return
	}
	activeID := cl.switcher.ActiveID()
	active := metas[0]
	for _, m := range metas {
		if m.ID == activeID {
			active = m
			break
		}
	}
	if cl.merged {
		name, sub := mergedHeader(len(metas))
		cl.accountName.SetText(name)
		cl.accountStatus.SetText(sub)
		cl.accountStatus.RemoveCSSClass("chatot-mono")
		cl.accountAvatar.SetText("🗂")
		// The design's merged identity is a brand-green disc, not the active
		// account's colour, and the rail drops its active ring.
		cl.swapAvatarClass("chatot-account-avatar-merged")
		cl.refreshAccountRail()
		return
	}

	cl.accountName.SetText(active.Name)
	text, mono, bad := accountHeaderSubline(active)
	cl.accountStatus.SetText(text)
	setCSSClass(cl.accountStatus, "chatot-mono", mono)
	setCSSClass(cl.accountStatus, "chatot-status-bad", bad)
	cl.accountAvatar.SetText(initialFor(active.Name))
	cl.swapAvatarClass(avatarColorClass(active.ID))
	cl.refreshAccountRail()
}

// accountHeaderSubline is the line under the account name in the sidebar
// header: the mockup's monospace phone number, falling back to the status
// when there is no number to show — which is exactly the signed-out case.
//
// bad paints it red. It follows NeedsRelink and not "not connected", so an
// account merely between sockets stays neutral; only one that has to be
// scanned again is flagged. The header was otherwise the one place the
// signed-out state never registered.
func accountHeaderSubline(meta client.AccountMeta) (text string, mono, bad bool) {
	if meta.Phone != "" {
		return meta.Phone, true, meta.NeedsRelink
	}
	return meta.Status, false, meta.NeedsRelink
}

// appMenuNeedsRebuild reports whether the ⋮ popover has to be rebuilt: the
// first time, and after that only when the link state its last row depends
// on actually moves. Rebuilding on every event would drop the popover out
// from under someone who has it open.
func appMenuNeedsRebuild(built, was, now bool) bool {
	return !built || was != now
}

// refreshAppMenu repaints the ⋮ menu if the account's link state moved.
func (cl *ChatList) refreshAppMenu() {
	if cl.rebuildAppMenu != nil {
		cl.rebuildAppMenu()
	}
}

// swapAvatarClass moves the header avatar onto a new palette class.
func (cl *ChatList) swapAvatarClass(newClass string) {
	if cl.accountAvatarClass == newClass {
		return
	}
	cl.accountAvatar.RemoveCSSClass(cl.accountAvatarClass)
	cl.accountAvatar.AddCSSClass(newClass)
	cl.accountAvatarClass = newClass
}

// OpenGlobalSearch switches the sidebar into search mode with query
// pre-filled, clearing any active starred/status/channels filter — the
// "Search all chats" link in the in-chat search bar funnels here.
func (cl *ChatList) OpenGlobalSearch(query string) {
	cl.search.SetText(query)
	cl.search.GrabFocus()
}

// refreshChats rebuilds the chat rows from d. Must run on the GTK main
// loop.
func (cl *ChatList) refreshChats(d *sidebarData) {
	if cl.merged {
		cl.refreshMergedChats(d)
		return
	}
	chats := pinnedFirst(d.chats)
	cl.archivedTitle.SetText(archivedTitleText(countArchived(chats)))

	now := time.Now()
	cl.rowJIDs = make([]string, 0, len(chats))
	cl.rowAccounts = nil
	want := make([]wantRow, 0, len(chats))
	for _, chat := range chats {
		if !showChatInList(chat, cl.showArchived) {
			continue
		}
		if !chatMatchesFilter(chat, d.chatLabels[chat.JID], cl.filter) {
			continue
		}
		vm := chatRowVM(chat, now)
		vm.Preview = resolveMentionsPlain(vm.Preview, cl.mentionName)
		if kind := cl.composing.kind(chat.JID); kind != "" {
			vm.Preview = composingText(kind, cl.composerNames(chat.JID))
			vm.Typing = true
		}
		vm.Blocked = d.blocked[chat.JID]
		want = append(want, wantRow{key: vm.JID, vm: vm, chat: chat, client: cl.c})
		cl.rowJIDs = append(cl.rowJIDs, vm.JID)
	}

	cl.reconcileRows(listChats, want)
	cl.reselectRow()
}

// refreshMergedChats rebuilds the list from every account at once (the
// mockup's "All accounts" mode): each row carries a 3px stripe in its
// account's colour and its preview is prefixed with the account name.
//
// Clicking a row still opens it through the normal path, which reads the
// ACTIVE account — so a row from another account switches to that account
// first, otherwise the conversation pane would come up empty.
func (cl *ChatList) refreshMergedChats(d *sidebarData) {
	if cl.mergedSource() == nil {
		// Nothing to merge (single-account build): fall back rather than
		// leaving the list blank.
		cl.merged = false
		cl.refreshChats(d)
		return
	}

	now := time.Now()
	cl.rowJIDs = nil
	cl.rowAccounts = nil
	var want []wantRow
	for _, r := range d.merged {
		mc := r.mc
		if !showChatInList(mc.Chat, cl.showArchived) {
			continue
		}
		if !chatMatchesFilter(mc.Chat, r.labels, cl.filter) {
			continue
		}
		vm := chatRowVM(mc.Chat, now)
		vm.Preview = mergedPreview(mc.AccountName, vm.Preview)
		vm.AccountColor = avatarColorClass(mc.AccountID)
		vm.Blocked = r.blocked
		want = append(want, wantRow{key: mc.AccountID + "|" + vm.JID, vm: vm, chat: mc.Chat, client: r.client})
		cl.rowJIDs = append(cl.rowJIDs, vm.JID)
		cl.rowAccounts = append(cl.rowAccounts, mc.AccountID)
	}
	cl.reconcileRows(listMerged, want)
	cl.reselectRow()
}

// mergedPreview prefixes a row's preview with the account it belongs to,
// which is the only thing distinguishing two chats with the same contact
// across accounts.
func mergedPreview(account, preview string) string {
	if account == "" {
		return preview
	}
	// The mockup drops a parenthesised qualifier from the account label here
	// ("Sezar (personal)" reads as "Sezar") so the prefix stays short.
	if i := strings.Index(account, " ("); i > 0 {
		account = account[:i]
	}
	if preview == "" {
		return account
	}
	return account + " · " + preview
}

// refreshSearch queries Search and rebuilds the list as result rows.
// Clicking a result opens its chat via the same onSelect path as a normal
// chat row; jumping to the exact matched message is left for later
// (ConversationView.Load already lands the reader at the newest messages).
func (cl *ChatList) refreshSearch() {
	hits, err := cl.c.Search(cl.query, searchResultLimit)
	if err != nil {
		hits = nil
	}

	cl.resetList()

	if len(hits) == 0 {
		cl.rowJIDs = nil
		cl.list.Append(cl.newListEmptyState())
		return
	}

	now := time.Now()
	cl.rowJIDs = make([]string, 0, len(hits))
	for _, h := range hits {
		cl.list.Append(buildSearchHitRow(cl.c, cl.avatarCache, searchHitVM(h, now)))
		cl.rowJIDs = append(cl.rowJIDs, h.ChatJID)
	}
	cl.reselectRow()
}

// reselectRow highlights the row for selectedJID after a rebuild, so the
// list keeps pointing at the chat that is actually open. Nothing is selected
// when that chat is filtered out of the current rows.
func (cl *ChatList) reselectRow() {
	cl.reselecting = true
	defer func() { cl.reselecting = false }()
	cl.list.UnselectAll()
	if cl.chatSel.Selected() != gtk.InvalidListPosition {
		cl.chatSel.SetSelected(gtk.InvalidListPosition)
	}
	if cl.selectedJID == "" {
		return
	}
	for i, jid := range cl.rowJIDs {
		if jid != cl.selectedJID {
			continue
		}
		if cl.listKind == listOther {
			if row := cl.list.RowAtIndex(i); row != nil {
				cl.list.SelectRow(row)
			}
		} else if i < cl.chatModel.Len() {
			cl.chatSel.SetSelected(uint(i))
		}
		return
	}
}

// SetSelected marks jid as the open chat and highlights its row. openChat
// calls it so a chat opened from a notification or a search hit is
// highlighted like one opened by a click.
func (cl *ChatList) SetSelected(jid string) {
	if cl.selectedJID == jid {
		return
	}
	cl.selectedJID = jid
	cl.reselectRow()
}

// listEmptyState is the mockup's empty chat list: a line naming why nothing
// is showing, and a pill that undoes the reason. Kept pure so the copy is
// unit-testable.
type listEmptyState struct {
	Text   string
	Action string
}

// listEmptyStateFor picks the message and action for the current sidebar
// state, in the mockup's precedence: a search query first, then a filter
// chip, then a genuinely empty account.
func listEmptyStateFor(query string, filtered, archived bool) listEmptyState {
	switch {
	case query != "":
		return listEmptyState{Text: "No chats match “" + query + "”", Action: "Clear search"}
	case filtered:
		return listEmptyState{Text: "No chats in this filter", Action: "Show all chats"}
	case archived:
		return listEmptyState{Text: "No archived chats", Action: "Back to chats"}
	}
	return listEmptyState{Text: "No chats yet", Action: "Start a chat"}
}

// newListEmptyState builds the empty card and wires its action to whichever
// state produced it.
func (cl *ChatList) newListEmptyState() gtk.Widgetter {
	state := listEmptyStateFor(cl.query, cl.filter != chatFilter{}, cl.showArchived)

	box := gtk.NewBox(gtk.OrientationVertical, 8)
	box.AddCSSClass("chatot-list-empty")
	box.SetHAlign(gtk.AlignCenter)

	text := gtk.NewLabel(state.Text)
	text.AddCSSClass("chatot-list-empty-text")
	text.SetWrap(true)
	text.SetJustify(gtk.JustifyCenter)
	text.SetMaxWidthChars(28)
	box.Append(text)

	action := gtk.NewButtonWithLabel(state.Action)
	action.AddCSSClass("chatot-list-empty-action")
	action.SetHAlign(gtk.AlignCenter)
	action.ConnectClicked(func() {
		switch {
		case cl.query != "":
			cl.search.SetText("")
		case cl.filter != chatFilter{}:
			cl.filter = chatFilter{}
			cl.refresh()
		case cl.showArchived:
			cl.archiveT.SetActive(false)
		default:
			cl.showNewChatDialog()
		}
	})
	box.Append(action)
	return box
}

// mergedHeader is the account button's label and subline while the merged
// list is showing: the mockup replaces the account identity with a 🗂 and a
// count.
func mergedHeader(accounts int) (name, sub string) {
	if accounts == 1 {
		return "All accounts", "1 account · merged list"
	}
	return "All accounts", fmt.Sprintf("%d accounts · merged list", accounts)
}

// fire calls f when it is set. The design routes Starred to the content
// pane, which the chat list does not own, so the entry point is a callback
// the window wires rather than a sidebar mode.
func fire(f func()) {
	if f != nil {
		f()
	}
}

// OnStarredRequested sets the handler for the ⋮ menu's "Starred messages".
func (cl *ChatList) OnStarredRequested(f func()) { cl.onStarred = f }

// posterName resolves a status poster's JID to a display name: a known chat
// name if we have one, else a "+number" derived from the JID, else the raw
// JID.
func posterName(jid string, names map[string]string) string {
	if n := names[jid]; n != "" {
		return n
	}
	at := strings.IndexByte(jid, '@')
	if at > 0 {
		user := jid[:at]
		allDigits := true
		for _, r := range user {
			if r < '0' || r > '9' {
				allDigits = false
				break
			}
		}
		if allDigits {
			return "+" + user
		}
	}
	return jid
}

// statusSnippet renders a status update's preview line: its text, or a media
// placeholder ("📷 Photo" / "🎥 Video" / …) for a media status.
func statusSnippet(m client.Message) string {
	if m.Text != "" {
		return m.Text
	}
	if m.Attachment != nil {
		switch m.Attachment.Kind {
		case "image":
			return "📷 Photo"
		case "video":
			return "🎥 Video"
		case "audio":
			return "🎤 Audio"
		case "sticker":
			return "🎨 Sticker"
		default:
			return "📎 " + m.Attachment.Kind
		}
	}
	return ""
}

// composerNames names who is composing in a group, for its row's
// "Ana is typing…" preview; a direct chat's peer goes unnamed.
func (cl *ChatList) composerNames(chat string) []string {
	if !strings.HasSuffix(chat, "@g.us") {
		return nil
	}
	var names []string
	for _, jid := range cl.composing.senders(chat) {
		names = append(names, cl.personName(jid))
	}
	return names
}

// personName resolves a group member's JID for a preview line: "You" for
// the account itself, the contact's name, else the bare number so the
// line is never empty. Shares the @mention cache.
func (cl *ChatList) personName(jid string) string {
	if isOwnJID(jid, cl.c.OwnJID()) {
		return "You"
	}
	jid = nonADJID(jid)
	if n, ok := cl.names[jid]; ok && n != "" {
		return n
	}
	n := cl.c.ContactName(jid)
	cl.names[jid] = n
	if n == "" {
		return bareJIDUser(jid)
	}
	return n
}

// messageSnippet renders a message's one-line stand-in wherever it is
// summarised (a starred row, the quote on a reply), falling back to a kind
// label for non-text bodies like the chat-list preview does.
func messageSnippet(m client.Message) string {
	switch {
	case m.Deleted:
		return tombstoneText
	case m.Text != "":
		return m.Text
	case m.Attachment != nil:
		return mediaChip(*m.Attachment)
	case m.Location != nil:
		return "📍 Location"
	case m.Contact != nil:
		return "👤 Contact"
	case m.Poll != nil:
		return "📊 " + m.Poll.Name
	case m.EventInvite != nil:
		return "📅 " + m.EventInvite.Name
	case m.CallLog != nil:
		if m.CallLog.Video {
			return "🎥 Video call"
		}
		return "📞 Voice call"
	default:
		return ""
	}
}

// watchEvents listens for client events and schedules a list rebuild on the
// GTK main loop via glib.IdleAdd. Runs on its own goroutine for the
// lifetime of the process; the fake/whatsmeow Events() channel is never
// explicitly closed today, so this goroutine simply exits if it is.
// EventChatPresence updates composing (composing sets "typing" or
// "recording" for the sender, anything else clears it) instead of falling through to the
// generic full refresh, since it needs the event's JID+state before
// rebuilding rows. EventChatUpdate (pin/mute/archive/unread changes) needs no
// special handling: it falls through to the generic refresh below like most
// other event kinds.
func (cl *ChatList) watchEvents() {
	for ev := range cl.events {
		// The header's identity (name, number, status) is read off the
		// client, so it must be re-read when the connection state moves:
		// a fresh link takes it from "scan to relink" to the number.
		switch ev.Kind {
		case client.EventConnection, client.EventPairSuccess, client.EventLoggedOut:
			glib.IdleAdd(func() {
				cl.refreshAccountHeader()
				cl.refreshAppMenu()
			})
		}
		// Avatar fetches that failed while disconnected are completed in
		// place once the socket is up, rather than by rebuilding rows.
		if ev.Kind == client.EventConnection && ev.Connection != nil && ev.Connection.Connected {
			glib.IdleAdd(func() { cl.avatarCache.retryFailed(cl.c) })
		}
		if ev.Kind == client.EventChatPresence && ev.ChatPresence != nil {
			chat, sender := ev.ChatPresence.ChatJID, ev.ChatPresence.JID
			kind := composingKind(ev.ChatPresence.State, ev.ChatPresence.Media)
			glib.IdleAdd(func() {
				cl.composing.set(chat, sender, kind)
				key := composingKey(chat, sender)
				cl.composingGen[key]++
				if kind != "" {
					// No "paused" may ever follow (see composingStaleSecs).
					gen := cl.composingGen[key]
					glib.TimeoutSecondsAdd(composingStaleSecs, func() bool {
						if cl.composingGen[key] == gen {
							cl.composing.set(chat, sender, "")
							cl.refresh()
						}
						return false
					})
				}
				cl.queueRefresh()
			})
			continue
		}
		if ev.Kind == client.EventMessage && ev.Message != nil && !ev.Message.FromMe {
			// The peer's message ends the typing bursts there; the
			// generic refresh below rebuilds the rows.
			jid := ev.Message.ChatJID
			glib.IdleAdd(func() { cl.forgetComposing(jid) })
		}
		if ev.Kind == client.EventChatUpdate {
			glib.IdleAdd(func() { cl.names = make(map[string]string) })
		}
		if ev.Kind == client.EventAvatar && ev.Avatar != nil {
			jid := ev.Avatar.JID
			glib.IdleAdd(func() {
				cl.avatarCache.invalidate(jid)
				cl.invalidateRow(jid)
				cl.queueRefresh()
			})
			continue
		}
		// A channel got a post or a live count update: the open pane
		// re-fetches its posts and the Channels list refreshes.
		if ev.Kind == client.EventNewsletterUpdate && ev.Newsletter != nil {
			jid := ev.Newsletter.JID
			glib.IdleAdd(func() {
				cl.channelPane.Reload(jid)
				if cl.tab == "channels" {
					cl.refresh()
				}
			})
			continue
		}
		if sidebarRefreshEvent(ev.Kind) {
			cl.queueRefresh()
		}
	}
}

// sidebarRefreshEvent reports whether an event of kind can change what the
// sidebar shows (rows, chips, badges, header). Presence, reactions, votes
// and calls can't, and they are the bulk of a live account's traffic; each
// used to cost a full rebuild.
func sidebarRefreshEvent(kind client.EventKind) bool {
	switch kind {
	case client.EventMessage, client.EventReceipt, client.EventRevoke,
		client.EventChatUpdate, client.EventHistorySync, client.EventLabelUpdate,
		client.EventConnection, client.EventPairSuccess, client.EventLoggedOut:
		return true
	}
	return false
}

// refreshDebounceMS is how long queueRefresh waits for more events before
// refreshing: a history page or a busy group lands dozens in a row.
const refreshDebounceMS = 100

// queueRefresh schedules one refresh on the main loop for however many
// events arrive within refreshDebounceMS; the refresh reads the store at
// run time, so nothing published in between is missed.
func (cl *ChatList) queueRefresh() {
	if !cl.refreshQueued.CompareAndSwap(false, true) {
		return
	}
	glib.TimeoutAdd(refreshDebounceMS, func() bool {
		cl.refreshQueued.Store(false)
		cl.refreshAsync()
		return false
	})
}

// SetSyncProgress shows the backfill banner with text and a bar at fraction
// (negative pulses an indeterminate bar). Must run on the GTK main loop.
func (cl *ChatList) SetSyncProgress(text string, fraction float64) {
	cl.syncLabel.SetText(text)
	if fraction < 0 {
		cl.syncBar.Pulse()
	} else {
		cl.syncBar.SetFraction(fraction)
	}
	cl.syncBanner.SetVisible(true)
}

// HideSyncProgress retires the backfill banner.
func (cl *ChatList) HideSyncProgress() { cl.syncBanner.SetVisible(false) }

// rowMenuItems is what a chat row's right-click menu shows right now.
func (cl *ChatList) rowMenuItems(chat client.Chat) []menuItem {
	if cl.rowMenu != nil {
		return cl.rowMenu(chat)
	}
	return chatRowMenuItemsFor(cl.c, chat, cl.window)
}

// SetRowMenu supplies the rows of every chat row's right-click menu.
func (cl *ChatList) SetRowMenu(f func(chat client.Chat) []menuItem) { cl.rowMenu = f }

// forgetComposing drops jid's typing/recording preview override, every
// sender's, and disarms their stale timers.
func (cl *ChatList) forgetComposing(jid string) {
	for _, sender := range cl.composing.senders(jid) {
		cl.composingGen[composingKey(jid, sender)]++
	}
	cl.composing.clear(jid)
}

// mentionName resolves the numeric user part of an @mention for a preview
// line ("" when unknown).
func (cl *ChatList) mentionName(user string) string {
	own := cl.c.OwnJID()
	for _, jid := range []string{user + "@s.whatsapp.net", user + "@lid"} {
		if isOwnJID(jid, own) {
			return "You"
		}
		if n, ok := cl.names[jid]; ok {
			if n != "" {
				return n
			}
			continue
		}
		n := cl.c.ContactName(jid)
		cl.names[jid] = n
		if n != "" {
			return n
		}
	}
	return ""
}

// chipScrollStep is how far one wheel notch moves the filter chips.
const chipScrollStep = 24.0

// chipScrollDelta turns a scroll event's deltas into chip pixels: surface
// pixels (a touchpad) pass through, a wheel's notches are scaled. Sideways
// travel wins when the gesture has any.
func chipScrollDelta(pixels bool, dx, dy float64) float64 {
	d := dy
	if dx != 0 {
		d = dx
	}
	if pixels {
		return d
	}
	return d * chipScrollStep
}

// scrollsInPixels reports whether dev's scroll deltas are surface pixels
// rather than wheel notches.
func scrollsInPixels(dev gdk.Devicer) bool {
	if dev == nil {
		return false
	}
	switch gdk.BaseDevice(dev).Source() {
	case gdk.SourceTouchpad, gdk.SourceTouchscreen:
		return true
	}
	return false
}

// showChatContextMenu builds and pops a Popover of action buttons anchored at
// (x, y) within row. The popover is parented to host, the list's own box,
// rather than the row: every event rebuilds the rows, and a popover hung
// off one would close with it a moment after opening.
func showChatContextMenu(host gtk.Widgetter, row *gtk.Box, items []menuItem, x, y float64) {
	pop := newMenuPopover(items)
	px, py := int(x), int(y)
	if b, ok := row.ComputeBounds(host); ok {
		px += int(b.X())
		py += int(b.Y())
	}
	rect := gdk.NewRectangle(px, py, 1, 1)
	pop.SetParent(host)
	pop.ConnectClosed(func() { pop.Unparent() })
	pop.SetPointingTo(&rect)
	pop.Popup()
}

// chatRowMenuItemsFor assembles the row menu's rows and their handlers. The
// mockup lists the account's labels inline under a "Lists" caption rather
// than behind a "Labels ▸" submenu, so the checklist is built here.
//
// "Delete chat" has no client method behind it yet, so it renders insensitive.
// Blocking is not in this menu: the mockup puts it in the conversation
// header's ⋮ menu, which now offers it.
func chatRowMenuItemsFor(c client.Client, chat client.Chat, window *gtk.Window) []menuItem {
	run := func(name string, do func(ctx context.Context) error) func() {
		return func() {
			go func() {
				if err := do(context.Background()); err != nil {
					log.Printf("chatot: chat action %q failed: %v", name, err)
				}
			}()
		}
	}

	return chatRowMenuItems(chat, chatLabelStates(c, chat), chatRowMenuActions{
		Pin:  run("pin", func(ctx context.Context) error { return c.PinChat(ctx, chat.JID, !chat.Pinned) }),
		Mute: run("mute", func(ctx context.Context) error { return c.MuteChat(ctx, chat.JID, !chat.Muted) }),
		Unread: run("unread", func(ctx context.Context) error {
			return c.MarkChatUnread(ctx, chat.JID, chat.UnreadCount == 0)
		}),
		Archive: run("archive", func(ctx context.Context) error {
			return c.ArchiveChat(ctx, chat.JID, !chat.Archived)
		}),
		NewList: func() { showCreateLabelDialog(window, c, nil) },
		ToggleLabel: func(id string) {
			applied := chatHasLabel(c, chat.JID, id)
			run("label", func(ctx context.Context) error {
				return c.SetChatLabeled(ctx, id, chat.JID, !applied)
			})()
		},
	})
}

// chatLabelStates lists every label with whether chat currently carries it,
// for the row menu's inline checklist.
//
// This queries the store on every right-click, where the old "Labels ▸"
// submenu only queried when that submenu was opened. The mockup shows the
// checklist inline, so the query is inherent to the design; it is one
// user-initiated read of a small table, on the same order as the chat-list
// refresh the sidebar already does.
func chatLabelStates(c client.Client, chat client.Chat) []chatLabelState {
	labels, err := c.Labels()
	if err != nil {
		log.Printf("chatot: list labels failed: %v", err)
		return nil
	}
	onChat, _ := c.LabelsForChat(chat.JID)
	applied := make(map[string]bool, len(onChat))
	for _, id := range onChat {
		applied[id] = true
	}

	states := make([]chatLabelState, 0, len(labels))
	for _, l := range labels {
		states = append(states, chatLabelState{
			ID:      l.ID,
			Name:    labelDisplayName(l),
			Applied: applied[l.ID],
		})
	}
	return states
}

// chatHasLabel reports whether the chat currently carries labelID.
func chatHasLabel(c client.Client, jid, labelID string) bool {
	onChat, err := c.LabelsForChat(jid)
	if err != nil {
		return false
	}
	for _, id := range onChat {
		if id == labelID {
			return true
		}
	}
	return false
}

// searchHitView holds the pure, pre-rendered display fields for one search
// result row. searchHitVM computes it from a client.SearchHit so it can be
// unit-tested without a display.
type searchHitView struct {
	ChatJID  string
	ChatName string
	Snippet  string
	TimeText string
	Initial  string
}

// searchHitVM derives the display view-model for a single search hit. now
// is injected so time formatting is deterministic in tests.
func searchHitVM(h client.SearchHit, now time.Time) searchHitView {
	name := h.ChatName
	if name == "" {
		name = h.ChatJID
	}
	initial := "?"
	for _, r := range name {
		initial = strings.ToUpper(string(r))
		break
	}
	return searchHitView{
		ChatJID:  h.ChatJID,
		ChatName: name,
		Snippet:  h.Snippet,
		TimeText: formatChatTime(h.TS, now),
		Initial:  initial,
	}
}

// buildSearchHitRow constructs the GTK widget tree for a single search
// result row from its pre-computed view-model.
func buildSearchHitRow(c client.Client, cache *avatarCache, vm searchHitView) *gtk.Box {
	row := gtk.NewBox(gtk.OrientationHorizontal, 8)
	row.SetMarginTop(6)
	row.SetMarginBottom(6)
	row.SetMarginStart(8)
	row.SetMarginEnd(8)

	// The chat's own avatar (photo, or its palette initial), as in the list.
	avatar := buildAvatar(c, cache, vm.ChatJID, vm.Initial, chatRowAvatarSize)
	avatar.SetVAlign(gtk.AlignStart)
	row.Append(avatar)

	textCol := gtk.NewBox(gtk.OrientationVertical, 2)
	textCol.SetHExpand(true)

	nameLabel := gtk.NewLabel(vm.ChatName)
	nameLabel.SetXAlign(0)
	nameLabel.AddCSSClass("chatot-chat-name")
	textCol.Append(nameLabel)

	snippetLabel := gtk.NewLabel(vm.Snippet)
	snippetLabel.SetXAlign(0)
	snippetLabel.SetWrap(true)
	snippetLabel.SetLines(2)
	snippetLabel.SetEllipsize(pango.EllipsizeEnd)
	snippetLabel.AddCSSClass("chatot-search-snippet")
	textCol.Append(snippetLabel)

	row.Append(textCol)

	timeLabel := gtk.NewLabel(vm.TimeText)
	timeLabel.AddCSSClass("chatot-chat-time")
	timeLabel.SetVAlign(gtk.AlignStart)
	row.Append(timeLabel)

	return row
}

// normalizePhone strips spaces/dashes/parens from input and checks the
// result looks like an E.164 number: a leading "+" followed by 7-15 digits.
func normalizePhone(input string) (string, bool) {
	var b strings.Builder
	for _, r := range input {
		switch {
		case unicode.IsSpace(r) || unicode.Is(unicode.Pd, r) || r == '(' || r == ')' || r == '.':
			continue
		case r == '+' && b.Len() == 0:
			b.WriteRune(r)
		case unicode.IsDigit(r):
			b.WriteRune(r)
		default:
			return "", false
		}
	}
	s := b.String()
	if !strings.HasPrefix(s, "+") {
		return "", false
	}
	digits := s[1:]
	if len(digits) < 7 || len(digits) > 15 {
		return "", false
	}
	return s, true
}

// newChatContacts filters chats to 1:1 (non-group) contacts and sorts them
// case-insensitively by name, for the new-chat view's CONTACTS list.
func newChatContacts(chats []client.Chat) []client.Chat {
	out := make([]client.Chat, 0, len(chats))
	for _, c := range chats {
		if !c.IsGroup {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// showNewChatDialog opens the "New chat" view: a phone-number entry at top
// that live-checks WhatsApp membership via c.CheckOnWhatsApp as the user
// types (validated by normalizePhone) and surfaces an "On WhatsApp · start a
// chat" row with a Message button, plus a CONTACTS list of known 1:1 chats
// below. Both paths open the conversation through the same onSelect seam a
// chat-list row activation uses.
func (cl *ChatList) showNewChatDialog() {
	body := gtk.NewBox(gtk.OrientationVertical, 0)

	head := gtk.NewBox(gtk.OrientationVertical, 6)
	head.AddCSSClass("chatot-sidebar-formbody")

	// One field for both jobs, as the mockup labels it: it filters the contact
	// list and, when what you typed is a phone number, offers to start a chat
	// with it.
	entry := sidebarSearchEntry("Search name or number")
	head.Append(entry)

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	status.AddCSSClass("chatot-newchat-status")
	status.SetVisible(false)
	head.Append(status)

	resultRow := gtk.NewBox(gtk.OrientationHorizontal, 8)
	resultRow.SetVisible(false)
	resultLabel := gtk.NewLabel("On WhatsApp · start a chat")
	resultLabel.SetXAlign(0)
	resultLabel.SetHExpand(true)
	resultRow.Append(resultLabel)
	messageBtn := gtk.NewButtonWithLabel("Message")
	messageBtn.AddCSSClass("suggested-action")
	resultRow.Append(messageBtn)
	head.Append(resultRow)
	body.Append(head)

	list := gtk.NewBox(gtk.OrientationVertical, 0)
	list.AddCSSClass("chatot-people-list")
	body.Append(sidebarScroller(list))

	open := func(jid string) {
		cl.closeSidebarForm()
		if cl.onSelect != nil {
			cl.onSelect(jid)
		}
	}

	var resultJID string
	openResult := func() {
		if resultJID != "" {
			open(resultJID)
		}
	}
	messageBtn.ConnectClicked(openResult)
	entry.ConnectActivate(openResult)

	all := newChatContacts(chatsOrEmpty(cl.c))
	fill := func(query string) {
		removeAllChildren(list)
		for _, ct := range matchContacts(all, query) {
			jid := ct.JID
			list.Append(peopleRow(cl.c, cl.avatarCache, ct, contactStatusLine(ct), false, func() { open(jid) }))
		}
	}
	fill("")

	// generation guards against a stale CheckOnWhatsApp response landing
	// after the user has kept typing past it.
	generation := 0
	entry.ConnectSearchChanged(func() {
		text := entry.Text()
		fill(text)
		resultRow.SetVisible(false)
		resultJID = ""
		generation++ // any edit invalidates an in-flight check, even editing to an invalid number
		phone, ok := normalizePhone(text)
		if !ok {
			status.SetVisible(false)
			return
		}
		status.SetVisible(true)
		status.SetText("Checking…")
		gen := generation
		go func() {
			jid, onWhatsApp, err := cl.c.CheckOnWhatsApp(context.Background(), phone)
			glib.IdleAdd(func() {
				if gen != generation {
					return
				}
				switch {
				case err != nil:
					status.SetText("Couldn't check that number, try again")
				case !onWhatsApp:
					status.SetText("This number isn't on WhatsApp")
				default:
					status.SetVisible(false)
					resultJID = jid
					resultRow.SetVisible(true)
				}
			})
		}()
	})

	cl.showSidebarForm("New chat", body, nil)
	entry.GrabFocus()
}

// matchContacts filters contacts by a case-insensitive name substring; an
// empty or phone-number-looking query returns everything, since the number
// path has its own result row.
func matchContacts(contacts []client.Chat, query string) []client.Chat {
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return contacts
	}
	out := make([]client.Chat, 0, len(contacts))
	for _, ct := range contacts {
		if strings.Contains(strings.ToLower(ct.Name), query) {
			out = append(out, ct)
		}
	}
	return out
}

// contactStatusLine is the dim second line of a people row: the contact's
// last message preview, which is the only per-contact status chatot stores.
func contactStatusLine(ct client.Chat) string {
	return ct.Preview
}

// chatsOrEmpty returns c.Chats(0), or nil if it errors, so callers that only
// need a best-effort list don't have to handle the error themselves.
func chatsOrEmpty(c client.Client) []client.Chat {
	chats, err := c.Chats(0)
	if err != nil {
		return nil
	}
	return chats
}

// showNewGroupDialog opens the two-step "New group" flow: page 1 picks
// participants from the 1:1 contact list (with search + a live selected
// count + removable chips), page 2 sets the group's name, disappearing
// timer and "only admins can post" mode. On success it opens the new
// group's chat through the same onSelect seam a chat-row activation uses.
func (cl *ChatList) showNewGroupDialog() {
	sel := newParticipantSelection()
	cl.showGroupPeopleStep(sel)
}

// showGroupPeopleStep is the mockup's "← New group" sidebar step: a search
// pill, a wrapping row of picked chips, the contact list with ✓ marks, and a
// full-width "Next · name the group" button.
func (cl *ChatList) showGroupPeopleStep(sel *participantSelection) {
	body := gtk.NewBox(gtk.OrientationVertical, 0)

	head := gtk.NewBox(gtk.OrientationVertical, 6)
	head.AddCSSClass("chatot-sidebar-formbody")
	search := sidebarSearchEntry("Search name or number")
	head.Append(search)

	chips := gtk.NewFlowBox()
	chips.SetSelectionMode(gtk.SelectionNone)
	chips.SetMaxChildrenPerLine(4)
	chips.SetRowSpacing(6)
	chips.SetColumnSpacing(6)
	chips.AddCSSClass("chatot-picked-row")
	head.Append(chips)
	body.Append(head)

	list := gtk.NewBox(gtk.OrientationVertical, 0)
	list.AddCSSClass("chatot-people-list")
	body.Append(sidebarScroller(list))

	footer, nextBtn := sidebarPrimaryButton("Next · name the group", func() {
		cl.showGroupNameStep(sel, false)
	})

	contacts := newChatContacts(chatsOrEmpty(cl.c))
	var refresh func()
	refresh = func() {
		nextBtn.SetSensitive(sel.Count() > 0)

		removeAllChildren2(chips)
		if sel.Count() == 0 {
			hint := gtk.NewLabel("Pick people to add")
			hint.AddCSSClass("chatot-picked-hint")
			hint.SetXAlign(0)
			chips.Insert(hint, -1)
		}
		for _, chip := range sel.Chips() {
			jid, name := chip.JID, chip.Name
			chips.Insert(pickedChip(name, contactInitial(name), jid, func() {
				sel.Remove(jid)
				refresh()
			}), -1)
		}

		removeAllChildren(list)
		for _, ct := range matchContacts(contacts, search.Text()) {
			jid, name := ct.JID, ct.Name
			list.Append(peopleRow(cl.c, cl.avatarCache, ct, contactStatusLine(ct), sel.Contains(jid), func() {
				if sel.Contains(jid) {
					sel.Remove(jid)
				} else {
					sel.Add(jid, name)
				}
				refresh()
			}))
		}
	}
	search.ConnectSearchChanged(refresh)
	cl.shotPickFirst = func() {
		if len(contacts) > 0 {
			sel.Add(contacts[0].JID, contacts[0].Name)
			refresh()
		}
	}
	refresh()

	cl.showSidebarForm("New group", body, footer)
	search.GrabFocus()
}

// showGroupNameStep is the mockup's second sidebar step: a round photo
// placeholder, the name field, the picked participants as chips, and the
// green create button. community switches the copy and the backend call.
func (cl *ChatList) showGroupNameStep(sel *participantSelection, community bool) {
	title, placeholder, createLabel := "New group", "Group name", "Create group"
	if community {
		title, placeholder, createLabel = "New community", "Community name", "Create community"
	}

	body := gtk.NewBox(gtk.OrientationVertical, 14)
	body.AddCSSClass("chatot-sidebar-groupbody")
	body.SetVExpand(true)

	// The design's 72px disc with a 👥 glyph and "Add a group photo" under
	// it. Picking a file shows it in the disc and, once the group exists,
	// uploads it as the group picture.
	photoCol := gtk.NewBox(gtk.OrientationVertical, 10)
	photoCol.SetHAlign(gtk.AlignCenter)
	photo := gtk.NewButton()
	photo.AddCSSClass("chatot-group-photo")
	photo.SetSizeRequest(72, 72)
	// Centred, or the button fills the column, which is as wide as the
	// caption under it, and the disc renders as a pill.
	photo.SetHAlign(gtk.AlignCenter)
	photo.SetChild(gtk.NewLabel("👥"))
	// The design's disc is a pointer-cursor button; without the cursor
	// (and the hover tint in CSS) nothing says it is one.
	photo.SetCursorFromName("pointer")
	photoCol.Append(photo)
	photoLabel := gtk.NewLabel("Add a group photo")
	photoLabel.AddCSSClass("chatot-group-photolabel")
	photoCol.Append(photoLabel)
	body.Append(photoCol)
	var photoJPEG []byte
	applyPhoto := func(jpeg []byte) {
		photoJPEG = jpeg
		photo.SetChild(groupPhotoPreview(jpeg))
		photoLabel.SetText("Change photo")
	}
	photo.ConnectClicked(func() { pickGroupPhoto(cl.window, applyPhoto) })
	cl.shotPhoto = applyPhoto

	nameEntry := gtk.NewEntry()
	nameEntry.SetPlaceholderText(placeholder)
	nameEntry.AddCSSClass("chatot-group-name")
	body.Append(nameEntry)

	if community {
		blurb := gtk.NewLabel("A community is a parent group. Members join through the groups you link to it; an announcement group is created with it.")
		blurb.SetXAlign(0)
		blurb.SetWrap(true)
		blurb.AddCSSClass("chatot-group-blurb")
		body.Append(blurb)
	}

	if !community {
		// The design shows the count even before anyone is picked.
		captionCol := gtk.NewBox(gtk.OrientationVertical, 6)
		caption := gtk.NewLabel(participantsCaption(sel.Count()))
		caption.SetXAlign(0)
		caption.AddCSSClass("chatot-group-caption")
		captionCol.Append(caption)
		if sel.Count() > 0 {
			chips := gtk.NewFlowBox()
			chips.SetSelectionMode(gtk.SelectionNone)
			chips.SetMaxChildrenPerLine(4)
			chips.SetRowSpacing(6)
			chips.SetColumnSpacing(6)
			for _, chip := range sel.Chips() {
				// Static chips: this step confirms the pick, the previous one
				// edits it, so no ✕ here.
				chips.Insert(participantChip(chip.Name, contactInitial(chip.Name), chip.JID), -1)
			}
			captionCol.Append(chips)
		}
		body.Append(captionCol)
	}

	// Settings the design's step has no room for but chatot can actually
	// apply. They sit in the same captioned card every other settings surface
	// uses, under the participants, instead of loose rows.
	settings := newSettingsCard()
	disappearing := gtk.NewDropDownFromStrings(disappearingOptions)
	disappearing.AddCSSClass("chatot-card-dropdown")
	settings.Add(dropdownRow("Disappearing messages", disappearing))
	announceRow, announceSwitch := newSwitchRow("Only admins can post", "", false, nil)
	settings.Add(announceRow)
	body.Append(newSettingsGroup("Group settings", settings))

	status := gtk.NewLabel("")
	status.SetXAlign(0)
	status.AddCSSClass("chatot-newchat-status")
	body.Append(status)

	spacer := gtk.NewBox(gtk.OrientationVertical, 0)
	spacer.SetVExpand(true)
	body.Append(spacer)

	footer, createBtn := sidebarPrimaryButton(createLabel, nil)
	createBtn.SetSensitive(false)
	nameEntry.ConnectChanged(func() {
		createBtn.SetSensitive(strings.TrimSpace(nameEntry.Text()) != "")
	})
	createBtn.ConnectClicked(func() {
		name := strings.TrimSpace(nameEntry.Text())
		if name == "" {
			return
		}
		parts := sel.JIDs()
		announce := announceSwitch.Active()
		seconds := disappearingSecondsForIndex(int(disappearing.Selected()))
		jpeg := photoJPEG
		createBtn.SetSensitive(false)
		status.SetText("Creating…")
		go func() {
			jid, err := cl.createGroupOrCommunity(name, parts, community, announce, seconds, jpeg)
			glib.IdleAdd(func() {
				createBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't create, try again")
					return
				}
				cl.closeSidebarForm()
				if cl.onSelect != nil && jid != "" {
					cl.onSelect(jid)
				}
			})
		}()
	})

	cl.shotName, cl.shotCreate = nameEntry, createBtn
	cl.showSidebarForm(title, body, footer)
	nameEntry.GrabFocus()
}

// participantChip is the group-name step's read-only person chip: a small
// initial avatar and the name.
func participantChip(name, initial, jid string) gtk.Widgetter {
	row := gtk.NewBox(gtk.OrientationHorizontal, 5)
	row.AddCSSClass("chatot-picked-chip")
	avatar := newAvatarInitial(jid, initial, 18)
	avatar.AddCSSClass("chatot-picked-avatar")
	avatar.SetVAlign(gtk.AlignCenter)
	row.Append(avatar)
	label := gtk.NewLabel(name)
	label.AddCSSClass("chatot-picked-name")
	row.Append(label)
	return row
}

// participantsCaption is the upper-case caption over the picked chips.
func participantsCaption(n int) string {
	if n == 1 {
		return "1 PARTICIPANT"
	}
	return fmt.Sprintf("%d PARTICIPANTS", n)
}

// createGroupOrCommunity performs the create and its two follow-up settings,
// which only apply to a group (a community's announcement group is created
// by the server, and neither setting is addressable on the parent).
func (cl *ChatList) createGroupOrCommunity(name string, parts []string, community, announce bool, seconds int64, photo []byte) (string, error) {
	ctx := context.Background()
	if community {
		jid, err := cl.c.CreateCommunity(ctx, name, "")
		if err == nil && len(photo) > 0 {
			if perr := cl.c.SetGroupPhoto(ctx, jid, photo); perr != nil {
				log.Printf("chatot/ui: set community photo: %v", perr)
			}
		}
		return jid, err
	}
	jid, err := cl.c.CreateGroup(ctx, name, parts)
	if err != nil {
		return "", err
	}
	if len(photo) > 0 {
		if perr := cl.c.SetGroupPhoto(ctx, jid, photo); perr != nil {
			log.Printf("chatot/ui: set group photo: %v", perr)
		}
	}
	if announce {
		if aerr := cl.c.SetGroupAnnounce(ctx, jid, true); aerr != nil {
			log.Printf("chatot/ui: set group announce: %v", aerr)
		}
	}
	if seconds > 0 {
		if derr := cl.c.SetGroupDisappearingTimer(ctx, jid, seconds); derr != nil {
			log.Printf("chatot/ui: set group disappearing timer: %v", derr)
		}
	}
	return jid, nil
}

// removeAllChildren2 empties a GtkFlowBox, which has no Remove-by-child that
// matches removeAllChildren's GtkBox signature.
func removeAllChildren2(flow *gtk.FlowBox) {
	for child := flow.FirstChild(); child != nil; {
		w := gtk.BaseWidget(child)
		next := w.NextSibling()
		flow.Remove(w)
		child = next
	}
}

// contactInitial returns the upper-cased first rune of name, or "?" if name
// is empty, for a contact-row avatar placeholder.
func contactInitial(name string) string {
	for _, r := range name {
		return strings.ToUpper(string(r))
	}
	return "?"
}

// showJoinGroupDialog opens a modal to join a group by pasting an invite link
// or bare code, then opens the joined group's chat via onSelect.
func (cl *ChatList) showJoinGroupDialog() {
	dialog := newCardDialog()
	dialog.SetTitle("Join group")
	if cl.window != nil {
		dialog.SetTransientFor(cl.window)
	}
	dialog.SetModal(true)

	dialog.SetDefaultSize(360, -1)

	// Not a design surface (the mockup's ＋ menu has no join row), so it
	// borrows the Add-account card's shape: an explanatory line, a bordered
	// card holding the field, a status line and the full-width green action.
	box := dialogBody(12)

	intro := gtk.NewLabel("Paste the invite link someone shared with you, or just its code.")
	intro.SetWrap(true)
	intro.SetJustify(gtk.JustifyCenter)
	intro.SetMaxWidthChars(40)
	intro.AddCSSClass("chatot-card-sub")
	box.Append(intro)

	card := newSettingsCard()
	fieldRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	fieldRow.AddCSSClass("chatot-card-row")
	fieldRow.Append(settingsRowBody("Invite link", "chat.whatsapp.com/…"))
	entry := gtk.NewEntry()
	entry.SetPlaceholderText("Link or code")
	entry.SetVAlign(gtk.AlignCenter)
	entry.SetSizeRequest(150, -1)
	entry.AddCSSClass("chatot-card-entry")
	fieldRow.Append(entry)
	card.Add(fieldRow)
	box.Append(card)

	status := gtk.NewLabel("")
	status.SetWrap(true)
	status.SetJustify(gtk.JustifyCenter)
	status.AddCSSClass("chatot-linking-status")
	box.Append(status)

	joinBtn := gtk.NewButtonWithLabel("Join group")
	joinBtn.AddCSSClass("chatot-primary-btn")
	joinBtn.SetHExpand(true)
	box.Append(joinBtn)

	join := func() {
		code := strings.TrimSpace(entry.Text())
		if code == "" {
			status.SetText("Paste an invite link or code")
			return
		}
		if !isValidInviteInput(code) {
			status.SetText("That doesn't look like a chat.whatsapp.com invite")
			return
		}
		joinBtn.SetSensitive(false)
		status.SetText("Joining…")
		go func() {
			jid, err := cl.c.JoinGroupWithLink(context.Background(), code)
			glib.IdleAdd(func() {
				joinBtn.SetSensitive(true)
				if err != nil {
					status.SetText("Couldn't join, check the link")
					return
				}
				dialog.Close()
				if cl.onSelect != nil {
					cl.onSelect(jid)
				}
			})
		}()
	}
	joinBtn.ConnectClicked(join)
	entry.ConnectActivate(join)

	dialog.SetChild(box)
	dialog.SetDefaultWidget(joinBtn)
	dialog.Present()
}

// isValidInviteInput reports whether s looks like a usable WhatsApp group
// invite: either a chat.whatsapp.com link (with or without scheme) carrying a
// non-empty code, or a bare invite code with no whitespace or slashes.
func isValidInviteInput(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	lower := strings.ToLower(s)
	if strings.Contains(lower, "whatsapp.com") {
		const marker = "chat.whatsapp.com/"
		idx := strings.Index(lower, marker)
		if idx < 0 {
			return false
		}
		code := strings.Trim(s[idx+len(marker):], "/ \t")
		return code != "" && !strings.ContainsAny(code, " \t")
	}
	if strings.ContainsAny(s, " \t/") {
		return false
	}
	return len(s) >= 6
}

// showNewCommunityDialog opens the "← New community" sidebar step, which is
// the group name step with community copy: a community is, from the client's
// perspective, a parent group and renders as one in the list.
func (cl *ChatList) showNewCommunityDialog() {
	// No participant step: WhatsApp populates a community from the groups
	// linked to it, not from a member pick.
	cl.showGroupNameStep(newParticipantSelection(), true)
}

// privacySettingRow is one name/value line in the privacy dialog.
type privacySettingRow struct {
	Name  string
	Value string
}

// privacySettingsRows sorts a privacy-settings map into a deterministic
// display order.
func privacySettingsRows(settings map[string]string) []privacySettingRow {
	rows := make([]privacySettingRow, 0, len(settings))
	for name, value := range settings {
		rows = append(rows, privacySettingRow{Name: name, Value: value})
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Name < rows[j].Name })
	return rows
}

// unlinkErrorGrace is how long unlinkDevice waits before explaining a failed
// unlink, so a removal whose confirmation was lost with the socket settles
// into a plain logged-out session instead of an alarming dialog.
const unlinkErrorGrace = 4 * time.Second

// unlinkDevice logs this device out of WhatsApp. The client emits
// EventLoggedOut on success, which is what swaps the window back to the
// pairing screen — nothing here touches the stack directly.
//
// A failure is shown rather than only logged: an unlink that silently does
// nothing looks exactly like one that worked, and the user is left thinking
// a device is gone that the phone still lists.
func (cl *ChatList) unlinkDevice() {
	c := cl.c
	win := cl.window
	go func() {
		err := c.Logout(context.Background())
		if err == nil {
			return
		}
		log.Printf("chatot: unlink device failed: %v", err)
		// WhatsApp often tears the socket down the moment it processes the
		// removal, so the reply is lost even though the device is gone from
		// the phone. Give whatsmeow's own device-removed handling a moment:
		// if the session ends anyway the unlink worked, and claiming it
		// failed would be worse than saying nothing.
		time.Sleep(unlinkErrorGrace)
		if !c.Paired() {
			return
		}
		glib.IdleAdd(func() {
			alert := adw.NewAlertDialog("Couldn't unlink this device",
				"WhatsApp didn't confirm the unlink: "+err.Error()+
					"\n\nThe device is still linked. Check the connection and try again, or remove chatot from Linked devices on your phone.")
			alert.AddResponse("ok", "OK")
			alert.Present(win)
		})
	}()
}

// relinkDevice starts a fresh pairing round for the account in view, for
// when it is signed out and the window is sitting on the linking screen with
// no code. Normally the logout starts one on its own; this is the manual
// way back when that failed.
func (cl *ChatList) relinkDevice() {
	c := cl.c
	win := cl.window
	go func() {
		err := c.Relink()
		if err == nil {
			return
		}
		log.Printf("chatot: relink device failed: %v", err)
		glib.IdleAdd(func() {
			alert := adw.NewAlertDialog("Couldn't start pairing", err.Error())
			alert.AddResponse("ok", "OK")
			alert.Present(win)
		})
	}()
}

// pinnedFirst orders pinned chats ahead of the rest, keeping each group's
// own (newest-first) order, as the mockup's list does. Pinning is otherwise
// invisible beyond a 📌 glyph.
func pinnedFirst(chats []client.Chat) []client.Chat {
	out := make([]client.Chat, len(chats))
	copy(out, chats)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Pinned && !out[j].Pinned })
	return out
}

// archivedTitleText is the sidebar header while the archived list shows:
// the mockup's "Archived · N", or just "Archived" when there is nothing.
func archivedTitleText(n int) string {
	if n <= 0 {
		return "Archived"
	}
	return "Archived · " + strconv.Itoa(n)
}

// countArchived counts the archived chats in a list.
func countArchived(chats []client.Chat) int {
	n := 0
	for _, c := range chats {
		if c.Archived {
			n++
		}
	}
	return n
}
