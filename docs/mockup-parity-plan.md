# Mockup parity — working plan & handoff notes (2026-09-02)

Companion to `docs/mockup-vs-app.md` (the finding list). This file holds
everything needed to resume the work after a context reset.

## Decisions (from the user)
- Source of truth: `mockup/Chatot Interactive.dc.html` ONLY (light mode).
- Workspace: stay on the current branch (`master`, dirty tree with rail/label
  work in progress) — the user approved the plan ("looks good").
- Dark mode out of scope for this pass.
- Workflow: `/dev-workflow:feature` (feature-development skill): plan approved
  → implement test-first with the fast check → full gate → review-gate.

## Plan (approved) — verify each step by recapturing the same state pair
1. Fonts in devenv (`pkgs.cantarell-fonts`, `pkgs.jetbrains-mono`), sidebar
   width 388, header/composer geometry, window controls (−□✕), tick colour
   `#cdece4`, chat-row geometry (53 px), unread time in accent green.
2. Shared popover-menu builder (icon column · label · accel, red destructive,
   separators, no arrow, 230–260 px) and re-wire ＋ / ⋮ app / chat ⋮ /
   message ⋯ / row context / labels menus to the mock's item sets.
3. Thread: hover pill (6 quick reacts · ＋ · | · ↩ ↪ ⋯ floating above the
   bubble), reactions as overlapping white pills, media tile (label below
   disc, 280×115), location map tile, view-once, contact card, doc row, poll,
   deleted message.
4. Secondary dialogs → in-window `adw.Dialog` cards with a title row; new
   chat / add people / group name into the sidebar; starred + media as
   right-pane pages.
5. Missing dialogs (contact info, mute, disappearing, block confirm, linked
   devices), attach list (7 rows), emoji/GIF/stickers picker popover on 🙂,
   reply bar, recording state, attachment tray.
6. Full gate: `go build ./... && go vet ./... && go test ./... && gofmt -l .`
   then the review-gate skill.

Acceptance: every state pair in the audit reads the same element-for-element
at 1240×840 light, within a few px; no missing/extra items.

## Fast check
`go build ./cmd/chatot && go test ./internal/ui/ -run '<TestNames>'` (cgo GTK
warnings about `free` are noise). No `./.claude/verify` script exists.

## Tooling recipe (all scripts copied to `.playwright-mcp/cmp/tools/`)
- Mockup server: `go build -o srv srv-main.go` serves `./mockup` on
  `127.0.0.1:8765` (Playwright MCP blocks `file:`; no python in shell).
  Open `http://127.0.0.1:8765/Chatot%20Interactive.dc.html` at viewport
  1300×900 → canvas is exactly 1240×840.
- Mockup state: in `browser_evaluate`, walk from the element holding a
  `__reactContainer$` key through fibers until `stateNode.logic.state.chats`
  exists; `window.__dc = stateNode.logic`; then `__dc.setState({...})`.
  Useful keys: `menu:{kind:'app'|'plus'|'chat'|'chatrow'(row)|'msg'(mid),top,left}`,
  `hover:mid`, `reactPick:{mid,top,left}`, `reply:{name,text}`, `attach`,
  `picker`+`pickerTab`, `recording`+`recSecs`, `__dc.openTray('image')`,
  `chatSearch`+`chatQuery`, `pane:'chat'|'media'|'starred'`, `mediaTab`,
  `sidebar:'chats'|'newchat'|'newgroup'|'groupname'|'archived'`, `picked`,
  `accountOpen`, `merged`, `dialog:'addaccount'|'accounts'|'labels'|'prefs'
  (+`prefTab`)|'forward'|'export'|'alert'(+alert:'clear'|'block')|'choice'
  (+choice:'mute'|'timer')|'info'|'devices'|'about'|'poll'|'contacts'`,
  `labelsOpen:{top,left}`, `labelFilter`, `toast`, `query`, `linking`,
  `active:<chat index>`, `dark`. Chat indices: 0 Mom, 1 Alex Rivera (default),
  2 Weekend Trip (group/poll), 3 +1 415…, 4 Marco's Bakery, 5 Dev crew
  (deleted msg), 6 Priya (voice/sticker/emoji), 7 Dad (docs incl. failed).
- Mock geometry dump: `browser_evaluate` walking children of the 1240×840 div
  printing `getBoundingClientRect` + computed font/bg/radius/padding
  (see numbers below).
- App: binary at `/tmp/chatot-cmp/chatot` (rebuild: `go build -o
  /tmp/chatot-cmp/chatot ./cmd/chatot`). Launch via `tools/launch.sh
  [ENV=VAL…]` (sets `CHATOT_FAKE=1`, `XDG_CONFIG_HOME=/tmp/chatot-cmp/config`
  whose `chatot/settings.json` is `{"theme":"light"}`, floats the window at
  1240×840 at (440,100) on niri, writes the id to `/tmp/chatot-cmp/winid`).
  `tools/cap.sh NAME ENV…` = launch + `niri msg action screenshot-window
  --id` + copy to `.playwright-mcp/cmp/app-NAME.png` + kill. `cap2.sh` =
  focused window; `cap3.sh` = newest secondary chatot window (app_id `chatot`,
  title ≠ "chatot": Preferences, Media, Group info). Shots are 1.2× scale with
  a 24 px shadow margin (1536×1056 for the main window).
- App states: `CHATOT_SHOT=<state>` (+ `CHATOT_SHOT_CHAT=<jid>`,
  `CHATOT_SHOT_MSG=<idx, -1 newest>`): plusmenu appmenu chatmenu hover msgmenu
  reactpill reactions reply emoji gif stickers recording draft search starred
  archived newchat newgroup newcommunity joininvite labelspop rowmenu empty
  forward export clear groupinfo about shortcuts blocked privacy linking toast
  switcher addaccount manage. Legacy: `CHATOT_SHOT_ATTACH=1`,
  `CHATOT_SHOT_PREFS=1`, `CHATOT_SHOT_MEDIA=1`. Fake JIDs: Ada
  `1234567890@s.whatsapp.net` (3 texts: in, out-read "Yep!", in-forwarded),
  Grace `1112223333@s.whatsapp.net` (all media types), group
  `weekendtrip@g.us` (no messages, join banner).
- Fonts test: `XDG_DATA_DIRS=/nix/store/25p0pjxxddqwfbily0yirmhjm5wcqij8-cantarell-fonts-0.303.1/share:/nix/store/5318km66a1cnrh6d19wmjbf7whqw1hap-jetbrains-mono-2.304/share:$XDG_DATA_DIRS`.
- DO NOT use ydotool mouse (opens niri overview, steals focus). wtype keys
  go to the focused window only. Never `pkill -f` with a pattern that matches
  the calling shell.

## Mock geometry (light, canvas coords, px) — from computed styles
Header strip: 0–46 + 1 px hairline `rgba(0,0,0,.08)`, bg `#fff`. Sidebar
head 388 wide (`padding 0 8px 0 10px`). Account button 301×38, radius 8,
`padding 4px 8px 4px 4px`: avatar 28 (account colour, 12 px bold white),
name 13 px bold, phone 9.5 px JetBrains Mono `rgba(0,0,0,.55)`, caret ▾ 9 px.
＋/⋮ 28×28 radius 7, 15 px, colour `rgba(0,0,0,.55)`, 6 px apart.
Conversation head: `padding 0 10px 0 14px`; identity button 692×40 radius 8
`padding 2px 6px`: avatar 28, name 14 px bold, presence 11.5 px (`#147a63`
when typing, else `.55`); ⋮ 28×28; controls 3×24 px circles `rgba(0,0,0,.08)`
8 px apart, glyphs − 11 px / □ 9 px / ✕ 10 px.
Rail: 54 wide, bg `#ebebeb`, `padding 10px 0`; tiles 38×38 radius 12, 14 px
bold white, 10 px gap; badge 17 h, 10 px bold, `padding 0 5px`, bg `#1b8c72`,
top-right (x +4 beyond tile, y −3); active ring `0 0 0 2px #ebebeb, 0 0 0 4px
#1b8c72`; ＋ tile 38×38 dashed. List column 334 wide bg `#ebebeb`.
Search: container `padding 8px 8px 4px`; pill 317×34 white, radius 999,
border 1 px `rgba(0,0,0,.13)`, `padding 0 12px`, ⌕ 12 px `.45`, text 13.5.
Chips: row `padding 4px 8px 6px`; chip 25 h `padding 4px 11px` 12 px, bg
`rgba(0,0,0,.08)`, active `#1b8c72` white bold; count 10.5 px bold `#147a63`
(white .85 on active); 6 px gaps; overflow `⋯` 28×24 pill 13 px `.55`.
Rows: 317×53 radius 8 `padding 7px 8px`, x 8 from list edge, 1 px gap;
avatar 38 (15 px bold white); name 13.5 bold; preview 12.5 `.55` (typing:
italic `#147a63`); time 10.5 `.45` (accent `#147a63` if unread); 📌 9 px;
selected bg `rgba(0,0,0,.1)`; unread badge ≈19 px circle `#1b8c72`.
Thread: white; `padding 10px 18px 14px`; bubble radius 14 `padding 7px 11px
5px`, in bg `rgba(0,0,0,.06)`, out `#1b8c72` white; text 13.5; meta 10 px
(`.45` in / `rgba(255,255,255,.75)` out), edited italic; ticks 10 px bold
`#cdece4` when read; 5 px between bubbles; day pill 21 h 10.5 px `padding 3px
11px` bg `.06` margin-top 8. Media tile 280×115 radius 10 margin `-2px 0 4px`,
disc 38 `#1b8c72` ⬇ 15 px, label 12 px `.55` below disc; caption 13.5.
Location tile 270×86 + red dot 13 px, title 12.5 bold, sub 11 `.55`. View-once:
28 px circle outlined `#147a63` "1" 12 px, title 12.5 bold, sub 11. Contact:
avatar 38, name 13 bold, phone 11.5 `.55`, "Message" 12.5 bold `#147a63`
centred `padding-top 6px`. Typing bubble 54×26, dots 6 px. Hover pill:
`top:-15px`, right/left 8 px, bg white, border 1 px `.09`, radius 999,
`padding 2px 4px`, shadow `0 4px 14px rgba(0,0,0,.16)`; buttons 24×24
(quick 13 px; ＋ chip bg 12 px; divider 1×16; ↩ ↪ 12 px, ⋯ 13 px, `.55`).
Reactions: `bottom:-11px`, white pills border 1 px `.08` shadow `0 1px 3px
.12`, `padding 1px 6px`, 11 px, 4 px gap.
Composer: 51 h `padding 8px 12px`; 📎 32×32 14 px, 🙂 32×32 15 px, 7 px apart;
entry 34 h radius 999 white 13.5 `padding 0 14px`; 🎤 34×34 14 px.
Menus: see finding list; item rows ≈ 34 h, icon col 16 px, text 13 px, accel
10.5 px mono `.45`, separators 1 px `.08` with 6 px margins, card radius 12,
shadow `0 12px 34px rgba(0,0,0,.2)` + `0 0 0 1px rgba(0,0,0,.12)`.

## Progress log

### Step 1 — global geometry & fonts (DONE)
- `.nix/devenv.nix`: added `pkgs.cantarell-fonts`, `pkgs.jetbrains-mono`.
  **The devenv shell must be re-entered for these to take effect.** Captures
  work around it via `XDG_DATA_DIRS` in `tools/launch.sh`.
- Sidebar pinned to the design's 388px (`388.0/1240.0` fraction, clamps
  340..420) in `cmd/chatot/main.go`.
- Both header strips are 46px + a 1px hairline; account button 38px; ＋/⋮ are
  28px squares via the new `.chatot-hdr-icon` (the `> button` half of the
  selector is what actually sizes a GtkMenuButton).
- Window controls: AdwHeaderBar's built-in ones stretch to the bar height and
  can't be sized from CSS, so the conversation header packs its own
  `gtk.WindowControls` with `VAlign(Center)` and layout `:minimize,maximize,close`.
- Chat rows 53px (row padding moved onto the inner box), name/preview/time
  13.5/12.5/10.5px, unread timestamps accent green (`chatRowTimeClass`).
- Read ticks `#cdece4`; unread badge 19px; rail badge 17px with the 2px ring.
- Thread padding `10px 18px 14px` (per-row 20px margins removed), bubble
  13.5px text / `7px 11px 5px` padding, meta 10px, day pill 10.5px.
- Composer 8px/12px padding on the white conversation surface with a top
  hairline, 32px 📎/🙂, 34px entry and mic.

### Step 2 — shared popover menu (DONE)
- New `internal/ui/menu.go` (card + row builder, `menuItem`, `newMenuPopover`,
  `setMenuPopoverItems`, `withoutMenuItem`, label-dot rows) and
  `internal/ui/menu_items.go` (one pure item-list function per menu, matching
  the mockup's `menuDef()`), both unit-tested.
- Rewired: sidebar ＋, sidebar ⋮, conversation header ⋮, message ⋯, chat-row
  context menu, chip-row label overflow.
- Removed as dead: `chatActionLabels`/`chatActionLabelsView`,
  `blockActionLabel`, `starMenuLabel`, `showLabelsSubmenu` (+ their tests).
- The header ⋮ menu rebuilds its rows on `show`, not on the button click, so
  every popup path (button, keyboard, screenshot hook) gets current state.
- New `app.quit` action (Ctrl+Q) and `ChatList.unlinkDevice` (client.Logout).

#### Deliberate deviations from the mockup (all visible, none silent)
- App ⋮ menu keeps a trailing section the design has no place for: Status
  updates, Channels, Set status, Privacy settings, Blocked contacts, Keyboard
  shortcuts. The last three move into Preferences in step 5; the first two
  have no mockup home at all and would otherwise become unreachable.
- ＋ menu keeps "Join with invite link"; message ⋯ keeps "Edit message";
  chat-row menu keeps "Mark as read/unread". Each is a real chatot capability
  the mockup never drew.
- Rows with no implementation yet render insensitive rather than missing:
  Linked devices, Contact info (1:1), Disappearing messages…, Pin in chat,
  Message info, Delete chat. All are step-5 work.
- Chat-row menu drops Block: the mockup puts blocking in the conversation
  header's ⋮ menu, which now has it.

### Remaining
Steps 3 (thread bubbles/hover pill), 4 (dialogs → in-window cards), 5 (missing
surfaces), then the review-gate.

### Step 3 — message thread (PARTIAL)
Done:
- Hover affordances are now the mockup's single floating pill (six quick
  reactions · ＋ · divider · ↩ ↪ ⋯) straddling the bubble's top edge, anchored
  to the bubble's outer side. It is an overlay child, so showing it on hover
  costs the thread no space and cannot reflow the list. This also removed the
  reserved gutter that was inflating the gap between bubbles.
- Reactions render as white pills hanging off the bubble's bottom edge
  (`bottom:-11px`), same side as the pill, instead of a row inside the bubble.
- Undownloaded media tiles are the mockup's 280×115 with the download disc
  centred and the label directly beneath it.
- The ⋯ menu is the shared menu card (step 2).

Not done: location tile, view-once card, contact card, document row, poll and
deleted-message bubbles still carry their old geometry.

### Step 4 — dialogs as in-window cards (CORE DONE)
- New `internal/ui/card_dialog.go`: `cardDialog` wraps `adw.Dialog` in an
  `adw.ToolbarView` with the mockup's 47px title row (left-aligned 14.5px bold,
  ✕ at the right, hairline below). Its method set mirrors the `gtk.Window`
  calls the dialogs already made, so each conversion was a constructor swap.
- All 24 `gtk.NewWindow()` dialogs across 13 files now present as in-window
  cards over a dimmed window. This fixes the worst defect in the audit: on a
  compositor with no server-side decorations those windows had **no title bar
  and no close button at all**.
- Two call-site adjustments were needed: `ConnectCloseRequest(func() bool)` →
  `ConnectClosed(func())` (neither caller used the veto), and five places that
  passed the dialog on as a `*gtk.Window` parent now pass `dialog.Window()`.

Not done: new chat / add people / group name as *sidebar* modes, and starred /
media as *right-pane* pages. They are still dialogs, now correctly chromed.

### Step 5 — missing surfaces (PARTIAL)
Done: `internal/ui/choice_dialog.go` adds the mockup's choice card and wires
two rows that were insensitive:
- Disappearing messages (Off / 24 hours / 7 days / 90 days, current ticked),
  group-only because `SetGroupDisappearingTimer` is the only API for it.
  Reuses the existing `disappearingOptions`/`disappearingSecondsForIndex` pair
  rather than duplicating the durations.
- Block confirmation (Cancel · red Block) before `SetBlocked`. Blocking stays
  inert on groups, which have no block target.

Still insensitive, needing backend or UI work: Linked devices, Contact info on
a 1:1, Pin in chat, Message info, Delete chat (no client method). Mute is
wired but toggles directly — WhatsApp's `MuteChat` takes no duration, so the
mockup's 8 hours / 1 week / Always chooser has nothing to call.

Also not done: attachment list (7 rows), the Emoji|GIF|Stickers picker
popover, reply bar, recording state, attachment tray, merged "All accounts"
list, six-page Preferences, and dark mode (explicitly out of scope).

### Tooling note
`.playwright-mcp/cmp/tools/srv-main.go` was renamed to `srv-main.go.txt`: it
sits inside the module, so `go build ./...` and `go test ./...` were picking it
up as a package.

### Review gate (DONE)
A fresh-context `code-reviewer` was run over the diff plus the three new
files. One major finding, fixed:

- **`internal/ui/composer.go`** — the record button is built with the mockup's
  🎤 emoji glyph, but `toggleRecording`/`resetRecordButton` still called
  `SetIconName`, which replaces a GtkButton's child with a stock GtkImage.
  After the first voice note the composer's mic reverted to a symbolic icon
  for the rest of the session. Both now use `SetLabel` (⏹ / 🎤). This was a
  latent bug that predates this pass; the emoji conversion had exposed it.

Minor findings: an empty `TestStarMenuLabel` shell left by the helper removal
(deleted), and two store reads that now happen on popup rather than on a
nested click (the chat-row menu's label checklist and the header menu's chat
lookup). Both are inherent to the mockup showing that state inline; each site
now carries a comment explaining the tradeoff rather than a cache that could
go stale.

The reviewer explicitly cleared: the `cardDialog.Present()` typed-nil handling,
the `ConnectCloseRequest` → `ConnectClosed` conversions (neither caller used
the veto), every `dialog.Window()` call site, and the absence of stale-closure
bugs in the new menu builders.

Final gate: `go build ./...`, `go vet ./...`, `go test ./...` and `gofmt -l`
all clean.

---

## Second pass — the rest of the audit (2026-09-02)

The first pass stopped after steps 1–2 and the front half of 3–5. This pass
closed everything else in `docs/mockup-vs-app.md` except the two items listed
at the very bottom as still open.

### Step 3 (rest) — thread bubbles
- **Attachment size/duration are now real data.** `client.Attachment` grew
  `Size` and `DurationSecs`, populated from the inbound protos
  (`FileLength`/`Seconds`) and persisted via two new `media` columns
  (`file_size`, `duration_secs`) with `migrateAddColumn` migrations. The
  UPSERT coalesces with `MAX(excluded, existing)` so a later copy of the same
  message that omits the size can't clobber one that had it. Without this the
  mockup's "· 1.2 MB" / "· 0:12" sublines had nothing to render.
- Location: hatched 270×86 map tile with a centred red pin, bold "📍 title"
  and a dim "<place> · click to open in Maps" line; the whole card opens the
  point in OpenStreetMap (the design has no separate link row).
- View-once: the design's outlined 28px "1" disc, not a filled ⬇ or a 📷.
  Spent copy is now "No longer available" rather than "Opened".
- Contact card: 38px avatar + name over phone, hairline, centred accent
  "Message" that opens a chat with the card's first dialable number via the
  existing `app.open-chat` action. Inert (not hidden) when there is no number.
- Document/voice rows: 28px disc, "📄 name" / "🎤 Voice message · 0:12" over
  "Click to download · PDF · 1.2 MB". Downloaded documents get the name over a
  "PDF · 1.2 MB" line and an outlined "Open" pill.
- Poll: bare bold question, 16px tickbox rows with the count and a 4px accent
  share bar, hairline footer "Select one · N votes".
- Typing indicator: the design's dotted bubble at the foot of the thread,
  driven by the chat-presence the view already tracked but only surfaced in
  the header subtitle.
- Group join notices became the design's centred neutral pill instead of a
  full-width green banner (the Review action stays — the mockup's pill is
  informational, chatot's can act).
- A voice note was added to the fake seed; it had none, so that bubble had
  never been rendered.

### Step 4 (rest) — sidebar modes and right-pane pages
- New chat / New group / New community are now **sidebar** forms
  (`sidebar_form.go`), each a "← Title" header over a body with an optional
  full-width green action, exactly as the design draws them. New group is the
  design's two steps (pick people → name it).
- Starred messages and Media/links/docs are now **right-pane pages**
  (`starred_page.go`, `media_pane.go`) inside a stack whose default page is the
  conversation. The sidebar's starred mode and the 420px media window are gone.
- The empty chat list is the design's card: a line naming why nothing shows
  and a pill that undoes it (Clear search / Show all chats / Back to chats /
  Start a chat).
- In-chat search moved **into** the 46px header, replacing the identity area:
  34px pill, mono "1/1" counter, ▲ ▼ ✕. "Search all chats" survives as a ⌕
  glyph — the design's header has no room for the label but the capability has
  no other entry point.
- Merged "All accounts" mode: 🗂 header, per-account stripe, account-prefixed
  preview. Opening a row from another account switches to it first.

### Step 5 (rest) — dialogs
A shared `settings_card.go` toolkit (captioned uppercase group + bordered card
+ separator rows, with switch/value/action/icon/device variants) now backs
Preferences, Contact info, Linked devices, Export and Accounts, so they read
identically instead of each inventing its own row.
- **Contact info** and **Linked devices** are new; both menu rows were
  insensitive before.
- **Preferences** is the design's 720×445 card with a six-page icon nav. This
  finally gave Blocked contacts and Account privacy a home, so they left the ⋮
  menu, and Keyboard shortcuts became a page.
- **About** is hand-built to the design (84px mark, mono version, tagline, two
  pill buttons) rather than `AdwAboutDialog`.
- **Forward** lost its quoted preview and Cancel, gaining round ticks and a
  footer with the count beside one green Send.
- **Export**, **Accounts** and **Add account** were rebuilt on the card toolkit;
  Accounts rows now carry the design's mono "+351… · Connected" subline.
- **Manage lists** replaced a bare create-a-label modal: rows with a colour
  dot, name, chat count and 🗑, over a colour palette, a name field and Add.
  The delete/colour APIs already existed and were simply unreachable.
- Composer: attach menu is the design's seven-row list with tinted circles;
  🙂 opens one Emoji|GIF|Stickers card (replacing the native emoji chooser and
  a separate GIF popover); the reply bar is inset with an author line; the
  recording strip replaces the whole composer row; and an attachment **tray**
  now sits between the file chooser and the send.
- **Linking screen**: heading, instruction paragraph, bordered QR card and a
  dotted status line.
- Toast is the design's dark pill with a green action.

### Deliberate deviations (this pass)
- The attach menu **dropped Camera and Event**. Both were stubs that only
  opened a "not implemented" dialog, and the design has no row for either.
- Media/links/docs **lost its month grouping**: the design's grid is flat with
  a per-tile date, so `groupMediaByMonth` went with it.
- **Linked devices lists only this device.** WhatsApp's other-device roster
  needs a server query the client doesn't expose; the note under the card says
  so rather than inventing rows.
- The **linking QR** is rendered at 2× the design's 192px so it stays crisp on
  a scaled display, which makes the card taller than the design's 220px. A
  blurry pairing code is a worse regression than a slightly large card.
- Preferences' colour scheme is a **dropdown**, not the mockup's click-to-cycle
  row: three states are more than a cycling affordance can communicate.

### Review gate (second pass)
A fresh-context `code-reviewer` over the whole diff found two real defects,
both fixed:
- **Major — merged mode read the wrong account.** Every per-row lookup in
  `refreshMergedChats` went through `cl.c`, which is the AccountManager and
  forwards to the ACTIVE account. Rows from other accounts got the wrong
  avatar, never showed a blocked badge, and — worst — vanished entirely under
  a label filter, because `LabelsForChat` errored for a JID absent from the
  active account's store. Fixed by adding `AccountManager.ClientFor(id)` and
  resolving avatar/blocked/labels through the row's own client. Covered by a
  new test.
- **Minor — attachment tray selection desync.** Removing a queued file *before*
  the selected one left the index unmoved, silently switching the preview to a
  different file. Split the index bookkeeping into a pure `removeAt` and tested
  it.

The reviewer explicitly cleared `joinMeta`'s `parts[:0]` reuse, the
goroutine→`glib.IdleAdd` discipline in every new async path, the recording
timer's lifecycle, `showSidebarForm`'s stack-child swap, and the store
migration/UPSERT/scan-order changes.

Separately, a **CSS regression** was caught by screenshot before the review:
`.chatot-toast > widget` also matched the toast overlay's MAIN child, painting
the whole conversation pane with the toast's background. Scoped to
`.chatot-toast toast`.

### Still open
- **Dark mode** — explicitly out of scope for this work.
- **Group system events** (`"Priya added Nina Costa"`) still have no source:
  the client never surfaces membership-change events, so there is nothing to
  render as the design's centred pills. The join-request notice, which does
  exist, now uses that pill styling.

## Third pass — the reported-screenshot batch (2026-09-02)

The user walked the app against the mockup and reported 25 mismatches with
screenshots. Every one was checked against `Chatot Interactive.dc.html`
(computed styles and rendered states, not the markup alone) before being
fixed; where the mockup and the report disagreed, the mockup won unless the
report explicitly asked for a WhatsApp behaviour the mockup lacks.

### Header, rail, sidebar
- Sidebar avatar was a tall pill: the 28px label filled the 38px button
  height. `SetVAlign(Center)`.
- ＋ / ⋮ / 📎 glyphs sat left of centre: a `GtkMenuButton`'s own label lives
  in a box that reserves room for a dropdown arrow. Every glyph MenuButton now
  uses `SetChild(gtk.NewLabel(...))`.
- Conversation ⋮ was a tall rectangle (header bar stretched it). Centred.
- Both header strips are now the same 46px + 1px hairline on `#ffffff`, and
  the sidebar column and rail carry the design's `#ebebeb` with a right-hand
  hairline, so the vertical separator and the bottom hairline line up.
- The rail no longer stops at the last tile: `VAlign(Fill)`. Its ring is the
  mockup's exact `0 0 0 2px #ebebeb, 0 0 0 4px #1b8c72`.
- Merged "All accounts": the rail draws no ring (nothing is *the* active
  account) and the header identity is the design's green 🗂 disc.
- Photo avatars: `GtkPicture` grew to the image's natural size and covered an
  oval; replaced by `AdwAvatar` with a custom image, which is a fixed-size
  disc by construction.
- Chat-row previews: `SingleLineMode` plus a pure `oneLine` collapse, since
  Pango ellipsizes per line and a newline made two.

### Account surfaces
- Switcher popover rebuilt as the mockup's 300px card: 32px avatar, bold
  label, status dot + "Connected · N unread", unread pill or ✓, then
  🗂 / ＋ / ⚙ action rows. Rows are `chatot-menu-item`, so they are no longer
  Adwaita-bold buttons.
- Accounts card: Add… pill in the title row (no ✕, as designed), a
  content-sized list (the stretching scroller left a slab under the last row),
  vertical ⋮, mono lower-case "phone · state" line. The ⋮ menu is the design's
  Relabel · Reconnect · Log out, spelled Relabel… / Relink / Remove. The
  per-account proxy dialog is gone from the UI (the global proxy stays in
  Preferences → Network); `SetAccountProxy` remains in the client.
- Add account: pairing starts on open (`AddPairingAccount`), the QR card is
  there from the first frame with a hatched placeholder, the label is applied
  on link through the new `AccountManager.RenameAccount`, and closing without
  linking removes the provisional account. In `CHATOT_FAKE=1` the manager
  hands out a `NewPairingFake` that emits a demo QR, so the card renders.
- Relink is the same QR card; a linked account says so instead of showing an
  empty square (whatsmeow only pairs a fresh session).

### Group creation
- The Create click crashed: `sidebarPrimaryButton` connected a nil handler
  when the caller wired the click itself. Guarded.
- Name step follows the mockup: 72px 👥 disc, "Add a group photo", the white
  38px name field, "N PARTICIPANTS" over read-only chips (no ✕ — the previous
  step edits the pick). The chatot-only settings (disappearing, admins-only)
  live in a captioned settings card instead of loose rows.
- Add photo works: file chooser → `gdkpixbuf` re-encode to JPEG ≤640px →
  shown in the disc → uploaded via the new `Client.SetGroupPhoto` after
  create.
- Join group is a proper card (intro, bordered field row, primary button).

### Thread
- Location bubble: the 270px tile now sets the bubble's width and the
  subline wraps under it (it used to stretch the bubble and leave dead space
  beside the tile). The tile shows the sender's embedded map preview:
  `Location.Thumbnail` is read from the proto, stored in the payload JSON and
  drawn under the pin overlay; the hatch remains the fallback. The Fake seeds
  a generated map.
- Hover affordances redesigned to WhatsApp's layout at the user's request
  (the mockup's floating pill is replaced): a ⌄ overlay in the bubble corner
  and a 🙂 outside it. 🙂 opens the quick-reaction row; ⌄ opens that row over
  the full menu, below the bubble. Popovers are built per click and
  unparented on close, so virtualized rows stay cheap. Hover states are drawn.
- ＋ opens chatot's own "PICK A REACTION" grid (the mockup's 322px card)
  instead of `GtkEmojiChooser`, which rebuilt its whole table on every open
  and lagged.
- Message menu: Copy text works for locations, contacts, polls and captions
  (`copyableText`); Pin in chat sends a `PinInChatMessage` (new
  `Client.PinMessage`; the sender-key resolution is shared with Star and
  tested — a received 1:1 message keys on the chat, or whatsmeow marks it
  "from me"); Message info opens a card (`messageInfoRows`).
- The copy toast lost its Undo — a clipboard overwrite has nothing to undo,
  and the design's toast is a bare notice.

### Composer
- The reply bar sits above the strip on the chat surface, with the strip's
  hairline beneath it, as designed; the strip is its own box now.
- Recording bar keeps the mockup's chrome but takes WhatsApp's controls at
  the user's request: 🗑 discard, red dot, mono timer, dotted track, ⏸/▶,
  green send. `audio.Recorder` gained Pause/Resume (segment files joined by an
  ffmpeg concat re-encode on Stop; `Elapsed` excludes pauses), and Stop now
  runs off the main loop because that join is as long as the note.

### Review gate
Two findings, both fixed: the pin key for a received direct message was built
with an empty sender (whatsmeow reads that as "from me"), and the paused-note
join ran on the GTK main loop. A third, minor one (blank relabel failed
silently) now shows a status line.

## Fourth pass — second screenshot batch (2026-09-02)

Ten reported items; nine fixed and verified against the interactive mockup,
one traced to the GTK renderer and deliberately left alone.

### Root causes worth knowing
- **Both header strips** (sidebar account row, conversation header) are the
  mockup's single 47px white band. The sidebar strip rendered grey and ~65px
  because the switcher popover's rows reused the strip's `.chatot-account-row`
  class and a later rule made it transparent with `min-height: 0`. The rows
  are `.chatot-switcher-row` now.
- **Three CSS declarations had lost their `@window_fg_color`/`@view_bg_color`
  token** to shell/perl `@`-interpolation (`alpha(, 0.55)`,
  `background-color: ;`). GTK logged "Theme parser error … Expected a valid
  color" and dropped the rule — the group-photo disc was a pill, the group
  name field lost its surface, the switcher status line its dimming. The app
  log is now part of the check (`grep "parser error"`).
- **The rail ring**: GTK paints a later box-shadow *over* an earlier one, the
  reverse of CSS, so `2px gap, 4px green` drew a solid green 4px ring. Green
  is listed first now; the 2px gap is the rail colour (`#ebebeb`), which is
  what the mockup's computed style says (`rgb(235,235,235) 0 0 0 2px,
  rgb(27,140,114) 0 0 0 4px`), not white. Same fix for the theme swatch ring.
- **Hover buttons "did nothing"**: opening a popover moved the pointer onto
  the popover's surface, the row's motion `leave` fired, the buttons hid, and
  the popover's parent going invisible closed it in the same frame. The
  affordance now ignores `leave` while its popover is open and hides on
  close; `motion` re-shows it.
- **Group photo never changed**: `gdkpixbuf.NewPixbufFromFileAtScale` failed
  because this GTK's pixbuf loader set is librsvg only. The picker now goes
  through `gdk.Texture` (GTK's own PNG/JPEG decoders) → `image/png` →
  box-downscale → `image/jpeg`, with unit tests. A `groupphoto` shot hook
  runs the real conversion on a generated PNG.
- **Off-centre S / 🗂**: GtkLabel centres the font's logical box, not the
  glyph's ink. `centreGlyph` measures the Pango ink/logical extents at map
  time and sets xalign/yalign so the ink is centred; applied to the header
  avatar, rail tiles and every initials avatar (`glyphAlign` is unit-tested).
- **Symbolic icons**: `pan-down-symbolic` rendered blank — GTK 4.22's own
  symbolic-SVG parser drops the `<g>` wrapper the icon theme on this system
  uses ("Ignoring element in symbolic icon: <g>"). The hover ⌄ is drawn with
  cairo in the widget's CSS colour instead.

### Per item
1. Darker line under the conversation header (also at the top of the
   reaction popover) — **renderer artefact, ignored by decision**. It is a
   1-device-px dark row at the top clip edge of a rounded node that gets
   partially redrawn while wheel-scrolling. Deterministic repro on squirtle:
   `CHATOT_SHOT=scrollpx CHATOT_SHOT_CHAT=1112223333@s.whatsapp.net
   CHATOT_SHOT_MSG=304`, sample (560,80): ~187 when present, ~244 clean.
   Findings: `GSK_RENDERER=cairo` never shows it; `gl` and `vulkan` always
   do on the Asahi M2 at 1.2× — and still do with Mesa llvmpipe substituted
   for the GPU (`LIBGL_ALWAYS_SOFTWARE=1`, `MESA_VK_DEVICE_SELECT`), so it
   is GTK 4.22's GPU render path rather than the Asahi driver itself. It
   does not appear on charmander (x86_64, Intel Iris Xe, Vulkan, output
   scale 1). Snapping scroll offsets to device pixels changes nothing.
   Not worked around in the app; `GSK_RENDERER=cairo` hides it if needed.
2. Headers: both white, 46px + 1px hairline (see root cause). A second
   cause surfaced after the first fix: the identity GtkMenuButton carried
   min-height + padding on both its outer node and inner button, stacking
   to 54px; the metrics live on the inner button only now. Verified by
   pixel row (both hairlines on the same row), not by eye.
3. Hover: 🙂 and ⌄ both outside the bubble, side by side, 🙂 nearest; ⌄ opens
   reactions + menu below, 🙂 opens reactions above; clicks work.
4–5. Group/community photo: 72px circle, hover tint + pointer cursor, picked
   image replaces the glyph (conversion fixed).
6. Picked chips: Adwaita's 3px `flowboxchild` padding removed; row aligned
   with the search pill (mockup `2px 10px 6px`).
7. Accounts card: ✕ restored (the mockup has none; kept at the user's
   request).
8–9. 🗂 and initial letters ink-centred.
10. Ring: 2px rail-colour gap then 2px green, as the mockup computes.

### Review gate
One major finding, fixed: the bubble popovers were parented on the hover
buttons, and GtkListView rebuilds a row whenever it recycles it or a
receipt/reaction for that message lands (`refreshInPlace`), which would have
torn an open menu down mid-click. They are parented on the conversation's
root box now and only *point at* the button, aligned to the bubble's outer
edge (mockup placement; also keeps an outgoing message's menu inside the
window). Minor ones fixed too: transparent PNGs are flattened onto white
before the JPEG encode, a sub-image origin test was added, and the shot-hook
handles into a sidebar form are cleared when the form closes.

## Fifth pass — third screenshot batch (2026-09-02, evening)

Fourteen screenshots, twenty-five items. Everything below was checked against
the interactive mockup's rendered state (Playwright over the HTTP-served
`.dc.html`) before the GTK change, and against pixel crops of the app after.

### Root causes worth knowing
- **Reactions were modelled as `map[emoji]reactor` ("last wins")**, so a
  second person on the same emoji was invisible and one person could hold
  two reactions. `client.Message.Reactions` (and the store's) is now
  `map[emoji][]reactorJID`; the store already kept one row per reactor, only
  the flattening lost it. The fake replaces our earlier reaction on a new
  pick, like WhatsApp.
- **AdwHeaderBar cannot lay out the mockup's search pill**: it centres its
  title widget at natural width and caps start-packed children at half the
  bar. The conversation header is now a GtkWindowHandle around a plain box
  (like the sidebar strip); pixel rows are unchanged.
- **Media/starred pages replaced the whole right pane**, header included, so
  the window controls vanished. The header is built by the conversation but
  packed by the window above the pane stack (`ConversationView.Header`).
- **`button.text-button`** (any label button) gets 16px side padding from
  Adwaita, outranking a lone class; the composer discs drop the class and
  are centred vertically (they were 32×36). Verified by allocation log
  (`CHATOT_SHOT=composersize`), not by eye.
- **GtkFlowBox never stretches children**; the media grid is a
  column-homogeneous GtkGrid padded to five columns, so tiles share the
  width like the mockup's `repeat(5,1fr)`.
- **A fake `Logout` emitted no `EventLoggedOut`**, so Unlink did nothing.

### Per item
1. Reactions: white pills with a count past one, clickable (ours toggles
   off, another emoji replaces ours). Fake seeds 👍×2 + 😮 on Grace's first
   message and ❤️ on Ada's "Yep!".
2. ⌄ menu no longer carries the quick-reaction row (🙂 does).
3. Reaction picker: the mockup's 32-emoji "Pick a reaction" palette, four
   rows, no scrollbar (it had the composer's full list with a scrollbar).
4. Composer 📎/🙂: 32×32 discs.
5. Contact info card: rows hover/press, every row closes the card, Mute
   toggles (wording follows state), Disappearing opens the timer chooser
   (the client's timer call is not group-specific), Media opens the page,
   values "Off"/count shown like the mockup.
6. In-chat search: one white pill (field · hits · ▲ ▼ ✕) spanning to ⋮.
   The "search all chats" glyph, which the mockup never had, is gone.
7. Media page under the chat header; grids grouped by day with a caption
   (owner's request — the mockup labels tiles only, which is kept too).
8. Mute notifications… asks 8 hours / 1 week / Always (`MuteChatFor`);
   pinned chats sort first in the list.
9. Archived mode swaps the header identity for "← Archived · N".
10. Join requests: card dialog with avatar, name/number, green Approve and
    outlined red Reject (no mockup counterpart; follows its device rows).
11. Starred page under the chat header; rows hover/press.
12. Unlink signs the fake out (EventLoggedOut → pairing screen).
13. About shows the 2c mark (embedded PNG, 84px, 20px corners).
14. ⋮ menu is the mockup's rows plus "Blocked contacts" under Starred.
    Chats · Status · Channels · Communities are a bottom mode bar in the
    sidebar (Set status is the bar's own "Post status" row); Privacy and
    Shortcuts are Preferences pages (account privacy loads into the page;
    the two standalone dialogs are gone).

New hooks: `contactinfo`, `mute`, `joinrequests`, `composersize`.

## Sixth pass — the bottom tabs (Status · Channels · Communities)

The mockup grew a real bottom navigation (`tabs`, `NAVICON`) and three full
tabs with their own sidebar pages, content panes, menus and dialogs
(`sidebarIsStatus/Channels/Discover/Communities`, `paneIsStatus/Channel/
Community/TabEmpty`, the `statusrow`/`mystatus`/`statusview`/`chanrow`/
`chan`/`commrow`/`comm` menus and the `chaninfo`/`chanlink`/`report`/
`comminfo`/`comminvite`/`commaddgroup`/`joinlink` dialogs). Every one of
those is now in the app; the owner-requested mode bar from the fifth pass is
gone in favour of the design's tab bar.

Model additions (both backends): `Newsletter` carries verified / follower
count / invite key / created / following / category; new `Community` and
`CommunityGroup` types with `Communities()` (whatsmeow: joined parent groups
+ `GetSubGroups`, membership off the joined list, previews/unread/mute off
the chat rows); `DiscoverNewsletters(query)` (whatsmeow has no directory:
returns `ErrUnsupported`, the page says so and offers Follow with a link);
`JoinCommunityGroup` (whatsmeow: `ErrUnsupported` — no join-subgroup call;
the fake joins); `ReactToStatus` (a reaction keyed on the status@broadcast
message, sent to the poster). Photo status = `SendMedia` to status@broadcast;
delete status = `DeleteMessage` on it; status reply = `SendText` quoting it.
The fake seeds the mockup's status feed, channel directory and two
communities (joined groups become chats, so the chat list grows by five).

Screens, each captured and compared (`.playwright-mcp/cmp/app-t-*`,
`app-s-*`, `app-c-*`, `app-m-*`):

- Tab bar across rail + list: 50px white strip, the mockup's SVG glyphs
  rendered per state (dim / accent, stroke 1.6 / 2), accent bar on top of
  the active tab, 16px count bubbles (unread chats, unviewed posters,
  unread in community groups; channels expose no unread state).
- Status: "My status" (＋ ring → post; ⋯ menu once posted), RECENT / VIEWED
  sections with segmented rings (cairo arcs, 9° gaps, grey once viewed;
  viewed state is per session), right-click menu, ＋ menu (photo / text /
  privacy). Viewer pane: progress segments that advance every 5 s, poster
  row with ⋮ and ✕, 318×398 card (photo cover-fit or big text on the
  poster's palette colour), caption, reply pill + four quick reactions, or
  "Viewed by 0" for our own (view counts aren't synced to linked devices,
  the toast says so). Prev/next click zones.
- Channels: Find channels pill, FOLLOWING · N, rows with ✓ and 🔇 marks
  hugging the name, row menu (info / share / mute / report / unfollow).
  Directory page under "← Find channels" (tab bar hidden): search pill,
  category chips, Most followed / category / N results caption, outline
  Follow / Following buttons, empty state with "Show all channels". Channel
  pane: header with the Follow pill and ⋮, about chip, posts as rounded
  bubbles with mono meta, ↪ forward, reaction pills (ours highlighted; the
  server only reports counts so "ours" is per session) and the dashed ＋
  opening the Pick a reaction card. Dialogs: info card (updates / created /
  link / notifications), Share (link box + Copy → "Copied", Send to a chat
  = forward dialog), Report (radio reasons, Also unfollow, red Report; the
  report itself is a toast — no API).
- Communities: rows (rounded-square avatar, "N groups · M members", unread),
  New community bar, row/pane menu (info / invite / mute announcements =
  mute the announcement group / leave = leave the parent). Pane: identity
  header (click → info), ANNOUNCEMENT GROUP / GROUPS YOU'RE IN / OTHER
  GROUPS with Join, footer ＋ Add group (admins only) + Invite link. Joined
  groups open their chat on the Chats tab. Dialogs: info card with
  Members (searchable, admin chips) / Groups (state chips) pill, Invite
  (live `GroupInviteLink`, Reset link behind an alert, Send to a chat), Add
  a group (checklist of unlinked group chats → `LinkGroupToCommunity`),
  Join with a link (validated as you type → `JoinGroupWithLink`; a
  community lands on its pane, a plain group in its chat), Join <group>?
  alert.
- Content strip on the non-chat tabs: a plain draggable band with only the
  window controls. Deliberate deviation: the mockup's flex layout parks the
  controls at the LEFT there (nothing else is in the row); ours keep them
  at the right like every other screen.

Hooks (`CHATOT_SHOT=…`, `CHATOT_SHOT_ARG` names the tab / poster / channel
/ community): `tab`, `tabplus`, `statusopen`, `statusrowmenu`,
`statusviewmenu`, `mystatus`, `mystatusopen`, `mystatusmenu`, `textstatus`,
`channelopen`, `chanmenu`, `chanrowmenu`, `chaninfo`, `chanlink`,
`chanreport`, `chanreact`, `discover` (+`CHATOT_SHOT_CAT`), `followlink`,
`communityopen`, `commmenu`, `commrowmenu`, `comminfo` (+`_CAT=groups`),
`comminvite`, `commaddgroup`, `joinlink` (+`CHATOT_SHOT_TEXT`), `joingroup`.

### Font- and loader-free glyphs (2026-09-03)

A runtime without a gdk-pixbuf SVG loader showed the bottom tabs as bare
labels, and the forward dialog's tick was a double-encoded "✓" (hex boxes).
The tab icons are now stroked with cairo from the mockup's paths
(`svgpath.go` walks M/L/H/V/C/S/A/Z; `drawTabIcon`), and every standalone
tick (forward, new-group picker, choice dialogs, polls, the verified mark,
the current-account row) is `newCheckGlyph`, drawn in the widget's CSS
colour. Reaction rows also reserve their pills' hanging space
(`.chatot-row-reacted`), so a pill never overlays the next message, and the
mockup does the same (`reactPad`). Hook: `CHATOT_SHOT=forward` pre-ticks the
JIDs in `CHATOT_SHOT_TEXT` (comma-separated).

### Seventh pass — viewer holds, circles, confirmations, group cards (2026-09-03)

- Status viewer: a cairo-drawn pause/play button before ⋮, a click on the
  card, or typing a reply holds the clock (`StatusPane.startClock`/
  `applyPause`, elapsed time kept across the hold) with a "Paused" chip over
  the stage. No mockup counterpart: the design's viewer has no clock.
- The viewer's ⋮ / ✕ / quick-reaction buttons are `newRoundButton`s (glyph
  overlaid on a fixed square), so they stay circles under any font.
- Unfollowing a channel asks first (`confirmUnfollow`, AdwAlertDialog) from
  the pane button, the discover rows and the row menu; the mockup got the
  same `unfollow` alert.
- Channel reactions are applied optimistically and rolled back on error
  (`applyReactionChange`), keyed per channel; the fake now keeps one
  reaction per post like the server, so repeated clicks no longer pile up.
- Group info is the mockup's info card (`showGroupInfoDialog`): avatar,
  member line, settings rows (edit, mute, disappearing, media, invite link,
  admin switches), the participant card with owner/admin chips and an admin
  ⋮ (make/dismiss admin, remove), Add participant, Leave group behind a
  confirmation. Presented only after the roster loads (an AdwDialog keeps
  its first content's height).
- Group and community invite links share `showInviteLinkDialog`.
- The chat header lost its icon-theme "Group info" button (blank on themes
  without the icon); the ⋮ menu carries it, as in the mockup.
- Hooks: `statuspause`, `chanreactn` (`CHATOT_SHOT_TEXT` emoji ×
  `CHATOT_SHOT_MSG`), `chanunfollow`, `groupinvite`.

## Eighth pass — real wiring, dark scheme, bar heights (2026-09-03)

Everything the fake backend used to stand in for now goes through whatsmeow,
the app renders in the dark scheme, and the two bottom bars share a height.

### Real wiring (client + store + UI)

- **Channel reactions persist.** `newsletter_reactions` table; the real
  `NewsletterReact` records the emoji, `NewsletterMessages` reports it as
  `NewsletterMessage.MyReaction`, the pane seeds its highlight from it.
- **Channel views and live updates.** `NewsletterMarkViewed` (feeds the
  view counts) and `NewsletterSubscribeLive`; `events.NewsletterLiveUpdate`
  and channel posts arriving as messages push `EventNewsletterUpdate`, and
  the open pane re-fetches (`ChannelPane.Reload`). Newsletter JIDs no longer
  surface in the chat list (`Chats` excludes `%@newsletter`).
- **Channel media posts.** `NewsletterMessage.Attachment` (via
  `extractText`); fetched posts are persisted under the channel JID so
  `DownloadMedia` works unchanged; the post shows the thumbnail then the
  download in a 300×200 frame, videos keep a badge, other kinds a chip.
- **Find channels.** whatsmeow's recommended-channels Mex query
  (`7263823273662354`) sent through `DangerousInternals().SendMexIQ`, reply
  parsed tolerantly (`parseNewsletterList`), filtered by the search text
  locally, country filter derived from the phone prefix. **Unverified live**
  — falls back to Follow with a link on any error. Category chips hide when
  the results carry no categories.
- **Status views.** `MarkStatusViewed(poster, ids, notify)`: marks the rows
  read (the `status` column, unused for inbound) and, with read receipts
  on, sends `MarkRead(ids, now, status@broadcast, poster)`. Viewed state
  now derives from the rows, so it survives restarts; the feed drops
  updates older than 24 h (`store.Statuses(since, limit)`).
- **Viewed by.** `read_receipts` table filled from read/played receipts
  (`Receipt.ReaderJID`/`TS`); `StatusViewers(msgID)` feeds the "Viewed by
  N" pill and the new viewers card (`status_viewers.go`, hook
  `statusviewers`). Own text statuses are now persisted on post.
- **Mute a poster.** `MuteStatus` writes `status_mutes` and pushes a
  hand-built `userStatusMute` app-state patch (whatsmeow decodes it but has
  no builder; **unverified live**, a rejected patch only logs);
  `events.UserStatusMute` from other devices lands in the same table. Feed
  gains a MUTED UPDATES section and Unmute menu rows (hook `statusmute`).
- **Hide my status from them.** `HideStatusFrom` reads `GetStatusPrivacy`
  and pushes a `status_privacy` patch (deny list gains the contact, an
  allow list loses them). **Unverified live.**
- **Account privacy is editable.** `SetPrivacySetting(name, value)` with
  `PrivacySettingOptions`/`PrivacySettingLabel`; the Preferences rows open
  a menu of the values WhatsApp accepts.
- **Stays unsupported, said in the UI:** joining a community sub-group
  without a link, reporting a channel or a status (no whatsmeow transport).

### Dark scheme

`style.css` now defines surface tokens (`@chatot_surface`, `@chatot_sidebar`,
`@chatot_hairline[_strong]`, `@chatot_wash`, `@chatot_ring_viewed`) with the
mockup's light values; `style-dark.css` redefines them (Adwaita dark
neutrals) and `ui.InstallStyles` stacks it while `AdwStyleManager` reports
dark. Cairo bits (`drawStatusRing`, `drawTabIcon`) take the dark colour via
`isDark()`. Capture with `CHATOT_CFG=/tmp/chatot-cmp/config-dark` (a config
dir whose settings.json says `"theme":"dark"`); every hook was swept in dark
(`.playwright-mcp/cmp/app-d-*.png`) and reads correctly.

### Bottom bars

GTK's `min-height` is the content box: the composer entry's 34px plus its
1px borders made the strip 53px against the tab bar's 51. The entry is now
32px content (34 with borders, the mockup's pill), so both bars measure 51px
with the same top edge.

## Ninth pass — first live-account fixes (2026-09-03)

Reported against a freshly linked account: empty bubbles, raw JIDs for
names, no avatars, an image that does nothing when clicked, a composer that
looks live with no chat open, and notifications during the initial sync.

- **Content extraction** (`client/content.go`): the history path now gets
  the same unwrapping whatsmeow does live (device-sent, ephemeral, view-once
  v1/v2/ext, document-with-caption, edited, bot, comment, poll v4/option
  image), plus PTV video notes, poll v2–v6, group invites, business
  templates/buttons/lists/interactive (+replies), products, orders,
  invoices, scheduled calls, sticker packs and withheld placeholders. A
  message with nothing renderable (key share, sync notification, pin, album
  header, …) is dropped; an unknown-but-real payload gets an "Unsupported
  message" bubble. History REVOKE stubs become deleted tombstones; other
  stubs are skipped. `store.Open` prunes the blank rows earlier builds kept.
- **Names** (`client/contacts.go`): whatsmeow's contact store is mirrored
  into `contacts` on connect, after history sync and after app-state sync,
  under both the phone-number JID and its LID twin (`contacts.pn_jid` links
  them); push names on inbound messages and `PushName`/`BusinessName`/
  `Contact` events keep it fresh, coalesced into one chat-list refresh. A
  nameless LID chat resolves to the mapped "+number". The account header
  shows the profile (push) name via `Client.OwnName()`; the default account
  no longer carries its JID as a label.
- **Avatars**: transient failures (not connected yet, timeouts) are no
  longer memoised as "no picture" — neither by the client nor the view — so
  the rebuild on Connected fetches them; LID chats ask for the picture on
  the phone-number identity.
- **Photo viewer** (`ui/image_viewer.go`, no mockup counterpart): a
  downloaded image bubble opens a window over the app — fit/actual-size
  toggle, Forward (reuses the bubble's forward flow), Copy (clipboard
  texture), Save as…, Open, Esc closes. Hook: `CHATOT_SHOT=imageviewer`
  with `CHATOT_SHOT_TEXT=<image file>`.
- **Composer**: the whole strip (entry, emoji, attach, mic) is insensitive
  until `SetChat` gets a JID.
- **Quiet sync**: `Event.Synced` marks messages received between
  `Connected` and `OfflineSyncCompleted` (90 s fallback) or stamped more
  than five minutes ago; the notifier skips them.

## Tenth pass — second live-account round (2026-09-03)

Reported against the linked account; each item names the user's report.

- **Startup flash of the QR page.** A paired account now boots on a loading
  page (the app mark on the window background) and the main view crossfades
  in on `Connected`; a failed start or a 12 s timeout also leaves the mark.
- **Archived group in the normal list.** History sync applies the
  conversation's `archived` flag; `EmitAppStateEventsOnFullSync` is on so
  the phone's labels/archive/pin/mute reach the store on a fresh link; an
  existing store does one full app-state resync (meta flag
  `appstate_full_resync_v1`). This is also what loads WhatsApp's own labels.
- **Level dots don't move.** ffmpeg's `astats` prints an RMS reading every
  50 ms; `Recorder.Level` exposes it and `levelTrace` draws the pill's dots
  swelling into bars.
- **No previews for audio / PDF / SVG / video; giant strip thumbnail.**
  `internal/media/preview.go` (ffmpeg poster + probe, pdftoppm page) feeds
  the tray: a player for audio, poster + GtkVideo for video, the rendered
  page for PDF, GdkPixbuf's SVG loader for SVG. Strip tiles are a cairo
  cover crop at 54 px. The same preview is sent as the message's
  `jpegThumbnail` (plus width/height/seconds), so document bubbles show a
  page strip and video bubbles a poster. Videos open in a viewer window
  (⛶ on the bubble; F11 fullscreen). `devenv.nix` adds gst-libav /
  gst-plugins-bad (H.264 for GtkVideo) and poppler-utils.
- **Tray header.** Cancel/Send/title are vertically centred (they were
  filling the 47 px row).
- **Labels … warning.** The popover is unparented before the chip row is
  rebuilt.
- **Auto-scroll.** New messages follow only when the reader is at the foot
  (or the message is our own); a 400 ms sticky window absorbs late row
  heights.
- **Unread badge.** Opening a chat clears it locally regardless of the
  receipt setting (`ClearUnread`); a message landing in the focused open
  chat is read on arrival; `SendReadReceipts` defaults to on, as the
  mockup's Privacy page shows.
- **Selection jumps back.** The chat list remembers the open chat and
  re-selects its row after every rebuild.
- **Dialogs.** Send contact, Create poll and Send location follow the
  mockup (`dlgIsContacts`, `dlgIsPoll`, `dlgIsLoc`). The location sheet
  is backed by `internal/geo`: Web-Mercator maths, an OSM raster tile cache
  (identifying User-Agent, on-disk cache, two fetches at a time), Nominatim
  search (1 req/s, cached), and the XDG location portal with a GeoClue
  fallback. Dark scheme re-renders tiles with inverted lightness (the
  mockup's CSS filter); CARTO's dark basemap was tried and rejected — it
  watermarks unauthenticated tiles. Live shares keep a local timer and a
  Stop sharing button; the bubble shows LIVE / ended states. Continuous
  position updates are not streamed to recipients (whatsmeow has no API
  for them); the share is a single live-location message.
- **Preferences → Privacy → Location access** gates the portal call.
- **MP3 aborts GTK's media backend.** Any MP3 handed to `GtkMediaFile`
  kills the process (`gstdecodebin3.c: assertion failed: (collection)`,
  GStreamer 1.28.5; playbin3 from gst-launch plays the same file). Opus,
  Vorbis, WAV, FLAC and M4A are fine. `inlineAudioSafe` keeps MP3 out of
  the tray transport and the in-chat player; those render an Open row.

## Eleventh pass — third live-account round (2026-09-03)

Reported against the live account with screenshots; every item checked
against `Chatot Interactive.dc.html` (bubble templates at lines 720–930,
viewer pane at 480–720) before implementation.

- **Chat-list previews.** The store's preview now speaks the design's
  vocabulary (`internal/store/preview.go`): `📷 caption|Photo`, `🎥 Video`,
  `🎞 GIF`, `🎤 0:06` (length when known), `📄 name`, `📍 place`,
  `📊 question`, `👤 name`, `🙂 Sticker`; never `[image]`. The same
  wording feeds notifications, reply quotes and starred rows
  (`ui.attachmentPreview`). `@mentions` in previews resolve to names.
- **Own chat reads "You"** (`Whatsmeow.Chats` matches the PN or LID self).
- **Typing.** The chat-list row is accent italic in both schemes (compound
  selector), a composing notice expires after 20 s without a follow-up and
  is dropped when the peer's message lands (`clearComposing` in both the
  list and the thread). The thread's typing bubble is a row of the message
  list (a sentinel model item, `typingSentinelID`) so it sits under the
  last message and scrolls with it; the dots blink with a CSS keyframe.
- **Group bubbles** carry the sender's name (accent bold, `ContactName`
  on the Client interface backed by the contacts table); **@mentions**
  render as bold accent `@Name` (`mentions.go`), "@You" for ourselves.
- **Unread separator.** "N unread messages" pill above the first unseen
  message: the chat's unread count on open, or the first message that
  lands while the window is unfocused; cleared on chat switch.
- **Document bubble** is the design's row (glyph, name, meta, outlined
  **View**) — the page strip is gone — and View opens the viewer.
- **Voice/audio bubble** is the design's 28 px play disc + 4 px track +
  mono length, backed by `mediaPlayer` (`media_player.go`), which also
  drives the viewer transport and the tray. **MP3 is transcoded to FLAC**
  (`media.PlayableAudio`, cached under `$XDG_CACHE_HOME/chatot/playable`)
  instead of being refused, so it plays inline everywhere.
- **Video bubble** is the design's 280×115 poster tile with the play
  pill; playback lives in the viewer (GtkVideo's own controls are gone).
- **Poll:** "Select one or more" footer, 16 px boxes; option buttons no
  longer take focus and `refreshInPlace` restores the scroll offset, which
  is what sent the thread to the top after a vote.
- **Contact card:** numbers formatted (`formatPhoneDisplay`), white
  Message action on an outgoing bubble.
- **Location bubble:** the subline is the place, reverse-geocoded through
  Nominatim (`geo.Reverse`, cached at ~10 m) when the sender gave none;
  the card opens the viewer, not the browser. Live shares remain a single
  message (whatsmeow streams no updates in either direction).
- **Attachment viewer pane** (`viewer_pane.go`, mockup `pane === 'viewer'`):
  header (← glyph title/sub counter ↩ ↪ ☆ ℹ ⋯ primary), dark stage for
  photo/video/GIF, zoom bar (Fit ×1.5 ×2 ×3 ×5, `+ − 0`), PDF pages via
  pdftoppm/pdfinfo with ‹ n / N ›, voice card with waveform, file card
  (ext tile, Save as…/Show in Files), interactive map with zoom, LIVE
  badge and coordinates card, not-downloaded fetch state, caption row,
  52 px filmstrip of every attachment in the chat, details sidebar
  (facts, Show in chat, per-kind actions, keyboard help). Esc/←/→/Space/F.
  The standalone photo/video windows remain only as the fullscreen path.
- **Right-click on a chat row** opens the conversation's ⋮ menu for that
  chat (`SetRowMenu`); rows that act on the open thread open it first.
- **Composer focus** on chat open (`Composer.FocusInput`).
- **Compose key / dead keys.** GTK's Wayland IM context defers composition
  to the compositor's input method; niri has none attached, so `~`+`a`
  typed as `n~ao`. `ensureComposeInput` sets
  `GTK_IM_MODULE=gtk-im-context-simple` unless an IME is configured
  (verified: `n~ao e` → `não é` with the harness).
- **Filter chips** scroll sideways (`PolicyExternal`, wheel-to-scroll);
  the … chip stays pinned outside the scroller.
- **Tray:** the video stage is the shared player + transport (the GtkVideo
  poster overlay had hidden its controls and the MediaStream cast never
  matched a MediaFile); audio gets the same transport with MP3 transcoded.
- **Dev fixtures:** `CHATOT_FAKE_MEDIA=<dir>` seeds downloaded
  photo/clip/voice/mp3/pdf/ods messages; the fixture group has a thread
  with authors and mentions; `CHATOT_SHOT=typing|viewer` hooks.

## Twelfth pass — fourth live-account round (2026-09-03)

- **Poll bubble:** the create-poll dialog's `chatot-poll-option` rule (36px
  rows) was bleeding into the bubble's option labels; renamed to
  `chatot-poll-draft`. The share bar is drawn with cairo (4px pill, accent
  or white fill) instead of a GtkProgressBar. GtkListItems are
  non-focusable so a click inside a row can no longer hand the focus to the
  row; re-binding that row after the tally lands used to make GtkListView
  jump to its first row.
- **Contact card:** the vCard's `waid=` parameter is the number (display
  text may carry U+00A0/U+2011), and `normalizePhone` accepts any Unicode
  dash/space, so "Message" is live.
- **Popovers vs. refresh:** chat-row right-click menus and the label "…"
  popover are parented to the ChatList itself and pointed at the row/button
  via `ComputeBounds`; a popover hung off a row died with the row on the
  next event-driven refresh. `GSK_RENDERER=gl` is set by default (GTK
  4.22's Vulkan renderer logs VK_SUBOPTIMAL_KHR on every popover surface).
- **Fullscreen clip:** `video_viewer.go` is now only the chrome-less
  fullscreen window (stage + transport); Esc/F/F11/⤢ close it and seek the
  viewer pane's player to where it stopped. The old header window (Save
  as… / Open) is gone.
- **Mentions:** `Store.ContactName` returns real names only and resolves
  through the LID↔PN twin in both directions (the "+257…" bug: an unknown
  phone JID answered with a bare "+number" before the LID row was tried).
  `Whatsmeow.SendText` fills `ContextInfo.MentionedJID` for every "@digits"
  in the text (LID when the LID map knows it). The composer has an @
  autocomplete (`mention_picker.go`): people from `GroupInfo` (or the peer
  + You), popover never takes focus, capture-phase keys, "@Name" rewritten
  to "@user" on submit.
- **Group bubbles:** 28px sender avatar beside incoming messages (beyond
  the mockup, which names the sender only).
- **Chips:** wheel notches vs. touchpad pixels via `ScrollEvent.Unit`; the
  "…" chip is inside the strip.
- **Hooks:** `CHATOT_SHOT=vote` (ARG=option, MSG=idx; scrolls to 30% first),
  `mention` (TEXT=fragment), `fullscreen` (MSG=idx).

## Thirteenth pass — fifth live-account round (2026-09-03)

Reports from the live account and the fixes, with root causes:

- **Multi-choice poll only took one option.** A click always sent
  `[option]`. `pollSelection` now toggles the option within the current
  picks on a multi-choice poll, and retracts on a single-choice poll when
  the picked option is clicked again.
- **Opening a chat stalled.** Photo bubbles (and stickers, avatars) went
  through `gtk.NewPictureForFilename`, which decodes the whole file on the
  main loop, once per bubble, again on every row recycle. `picture_async.go`
  decodes off the main loop at the display size and memoises the texture
  per path+side. `chatName` no longer re-runs the whole chat-list query
  (one pass over every chat) for the open chat's name.
- **Built-in lists in the label overflow.** WhatsApp syncs Unread /
  Favorites / Groups / Communities as `LabelEdit` with a `predefinedID`;
  they were stored as plain labels. New `labels.predefined` column, set from
  the event; the one-time migration flags the rows already stored under
  those fixed ids and names. `Labels()` hides them.
- **Chip strip: too fast, no way to scroll without a wheel, assertion
  spam.** gotk4's `EventController.CurrentEvent()` wraps the GdkEvent as a
  GObject (it is not one), so `Unit()` failed with GLib assertions and
  touchpad pixels were multiplied like wheel notches. Touchpads are now
  detected by scroll-begin/end plus the device source. The strip uses an
  overlay scrollbar (appears on hover, no height change) — a deliberate
  departure from the mockup, which hides it (`scrollbar-width: none`).
- **"You" twice in the own-number chat's @ picker.** The DM branch listed
  the peer (the account) and then the account again. Mention rows now carry
  the JID and show the avatar through the composer's own `avatarCache`.
- **Fullscreen video: no playback, position lost, stack overflow.** The
  window built a second player and seeked it from inside a `Watch`
  callback; `SeekTo` notifies watchers, which re-entered the callback
  without end. Fullscreen now paints the pane's own player (a MediaFile
  is a paintable, two pictures can share it), so position and state are
  simply continuous; `videoStage.shared` skips realizing the stream on the
  window's surface (a second realize while it prepares stalls the
  pipeline) and stops its unrealize from pausing. `Play()` on an unprepared GtkMediaStream is a no-op, so
  `Toggle` before `prepared` is remembered (`wantPlay`) and applied on the
  property notify — this is also why a clip could sit at 0:00 after ⤢.
  A click on the stage toggles playback everywhere.
- **Group author as a LID number.** Live messages store the sender as the
  addressed JID (`user:27@lid`), contact rows are keyed bare. `ContactName`
  strips the device part; new messages store `Sender.ToNonAD()`; bubble
  and viewer avatars look up the bare JID.

Harness: `launch.sh` pipes the log through `stdbuf -o0 head`, so `app.log`
is live (it was block-buffered before, useless for debugging); the clip
fixture is now 20 s (a 3 s clip had ended by the time captures landed).

## Fourteenth pass — sixth live-account round (2026-09-03)

- **Chat still slow to open (own-number chat, 1–2 s).** Traced on a copy of
  the real store (`CHATOT_OFFLINE=1`): 40 bubbles took 565 ms, of which
  four voice notes took 560 ms — each built a GStreamer stream on bind
  (411 ms for the first). `mediaPlayer` now keeps the file pending and
  builds the stream on the first play or seek. Settled time 957 → 384 ms.
- **Thread shifts when the pointer enters it.** The hover 🙂 ⌄ were packed
  beside the bubble and shown on enter, so a bubble at full width shrank
  and re-wrapped, growing the row. The buttons now keep their room and hide
  by opacity (and can-target), so a bubble never changes size.
- **Multi-line drafts.** The pill is a text view (`composer_input.go`):
  Enter sends, Shift+Enter breaks the line, the pill grows to six lines and
  scrolls past that (WhatsApp's layout; the mockup only has the single-line
  34px pill, which the idle state still matches). Attach/emoji/mic/send hug
  the strip's bottom as the pill grows. Enter is taken where the view
  inserts its newline (`insert-text`), after the input method has had the
  key, so an Enter that commits a preedit or ends a compose sequence is
  not a send. The voice-note player also remembers a seek, mute or loop
  asked for before its stream exists.
- **Sent group message shown read.** Any member's read receipt moved the
  message to read; WhatsApp shows read only once every other member has
  read it. `groupReadStatus` (pure, tested) matches the stored read
  receipts against the group's current members by person (a LID reader is
  the same person as a phone-number member, through the LID map), the
  account itself excluded. Unknown membership claims nothing: the members
  are fetched and `reconcileGroupReadState` re-derives every message with
  receipts once they land.
- **Chip strip.** The overlay scrollbar stole clicks along the chips'
  bottom edge; replaced by `chip_slider.go`, a 3px slider under the strip
  (drag or click the track), shown only when the chips overflow. The strip
  is rebuilt on every refresh, which reset its scroll to zero — the value
  is put back after the rebuild. Chips no longer take focus on click (the
  ring on "All").

Harness: `CHATOT_OFFLINE=1` (with `CHATOT_FAKE=0` through launch.sh) renders
a copy of the real store; hooks `compose` (TEXT with `\n`) and
`labelfilter` (ARG=label id). Own-number chat: 554888073648@s.whatsapp.net.

## Fifteenth pass — seventh live-account round (2026-09-03)

- **Own number as two chats.** WhatsApp addresses DMs by phone number or
  by LID, and messages arrived filed under whichever the wire used: the
  phone's own-chat messages landed in `<lid>@lid`, chatot's in
  `<number>@s.whatsapp.net` (the real store held 155 LID chats, 12 of them
  twins of a number chat). `canonicalChatJID` (`lidchat.go`) files a LID
  DM under its number whenever the mapping is known — the device record
  for the account itself, then whatsmeow's LID map (written from each
  message's alt address before the event is dispatched), then the
  contacts table's `pn_jid` from history sync — and every event, poll
  vote, app-state chat update and history conversation goes through it.
  `store.MergeChat` folds an existing LID chat into its number chat
  (messages, reactions, votes, labels, media, receipts moved, chat flags
  combined) at startup (`mergeLIDChats`) and whenever a mapping is learned
  (`upsertContact` with a PNJID); a mutex serializes a merge against
  filing a live event, and a merge without a LID chat row is a no-op
  (that call sits on the per-message path). The LID's cached avatar is
  handed to the number so the merged chat keeps its picture offline.
- **Viewer arrows' ring looked pixelated.** A ring GTK draws itself (a
  border, and an inset box-shadow just the same) breaks into four bright
  points at the cardinal positions on a circle at the 1.2× display scale,
  and reads as a thin dotted 1px line whatever width the CSS asks for.
  `navButton` now strokes the ring with cairo in a drawing area under the
  glyph: 2px, at the mockup's alpha (.22 on the dark stage, .15 on the
  light one), redrawn when the stage scheme flips.

Harness: `/tmp/dbq2` reads any copy under `/tmp/dbq` (`DBQ=s.db go run . "SQL"`);
the offline copy's path is in app.log (`offline render from copy of … at …`).
- **Delete asks for me / for everyone.** WhatsApp's prompt (no mockup
  design): `ShowDeleteMessageDialog` offers "Delete for me" on any message
  and "Delete for everyone" on an own one that isn't a tombstone.
  `DeleteMessageForMe` publishes the deleteMessageForMe app-state mutation
  (whatsmeow decodes it but has no builder; index = name, chat, id, fromMe,
  sender-or-0, collection regular_high) and drops the row and its
  attachments (`store.RemoveMessage`); the phone's own deletions arrive as
  `events.DeleteForMe` and are applied the same way.
- **Chats at 99+ unread after the merge.** History sync had written the
  number chats' unread counts as of linking (395, 165, …) and nothing ever
  cleared them: the phone's reads landed on the LID twin (self read
  receipts, markChatAsRead), which is why the LID rows sat at 0. The merge
  now takes the unread count of the row active most recently, a one-time
  `RepairUnreadCounts` caps every count at the inbound messages newer than
  the account's own last message, and the app-state resync runs once more
  (`appstate_full_resync_v2`) with markChatAsRead honouring its message
  range, so an old read cannot clear a newer message. A peer's read
  receipt for our message no longer clears our own badge either, while a
  receipt from one of the account's own devices counts as a self read
  whatever its type. The repair leaves a count of exactly 1 alone (what
  "mark as unread" sets, with no message behind it).
- **Unread pill stayed after reading.** The pill now comes down three
  seconds after its row is on screen in the active window (checked again
  when the countdown ends): `anchorSeen` uses the row's bounds against the
  scroller, re-armed on scroll, on bind and on the window turning active.
- **Pill came down but the badge stayed.** A message that lands while the
  window is unfocused is not read on arrival (correct: the badge counts
  it), and coming back only cleared the pill. `ConversationView.OnUnreadSeen`
  now fires when the pill is taken down and main.go runs the same
  `markReadOnOpen` as a chat open: badge cleared, receipt sent if enabled.
  Known gap: a pill still up when a second message arrives keeps its
  original count until it drops.

- **Logo showed as the 🐦 glyph.** `newAppMark` decoded the embedded SVG
  through gdk-pixbuf, whose librsvg loader is not in this runtime, so the
  About card, the loading screen and the desktop icon all fell back. The
  mark is now `assets/chatot-icon-512.png` rasterised from the 2c SVG
  (`~/downloads/chatot-icon-2c.svg`, copied over `assets/chatot-icon.svg`);
  the tray and the ⋮ menu's About row (11px, the emoji glyphs' weight)
  draw the tray variant: the chatot alone with no tile, one body colour
  per theme (`assets/chatot-icon-tray-{light,dark}.svg` from
  `~/downloads/chatot-icon-2c_tray{light,dark}.svg`, rasterised square at
  128px). Both follow the style manager's `dark` property live. The desktop entry, SVG and a 512px
  PNG are installed into `~/.local/share` as well as `XDG_DATA_HOME` when
  the dev shell redirects the latter, since the shell reads the session's
  data home. About reads `0.1.0-beta`.

- **Dark green text unreadable in dark mode.** Chip counts, unread
  timestamps, the active tab, typing…, links and outline buttons used the
  brand green (#147a63/#1b8c72) as text on the dark surfaces; the mockup's
  own dark mode does the same, so it was no guide. New tokens
  `chatot_accent_text`/`_soft` (brand green on light, mint #46c39a on
  dark, ~6:1 on the sidebar) replace every green text/outline colour in
  style.css; fills keep the brand green. The cairo tab icon and the
  mention accent pick the same mint via `isDark()` at the call site.
- **Packaging.** `.nix/package.nix` + `packages.chatot`/`apps.default` in
  the flake: buildGoModule with the cgo GTK stack, wrapped by
  wrapGAppsHook4 plus ffmpeg/poppler/xdg-utils on PATH, the UI fonts and
  the Adwaita icon theme on XDG_DATA_DIRS, `CHATOT_NO_DESKTOP_ENTRY=1`,
  and the .desktop + SVG/512px icons under share/. Verified from an
  `env -i` launch, which exposed the "Select a chat" glyph naming
  `chat-symbolic` — a Colloid-only icon — now the Chats tab's bubble path
  stroked in cairo (`newEmptyChatGlyph`). README documents the install.

Harness: hook `deletedialog` (MSG idx) opens the delete prompt; hook
`bgarrive` (with `CHATOT_SHOT_CHAT`) has the fake deliver an inbound
message once the window loses focus (`niri msg action focus-window` on
another window), for the arrived-while-away states; `Fake.Receive` backs it.
