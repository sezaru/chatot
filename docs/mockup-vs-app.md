# Interactive mockup vs. app — visual audit (2026-09-02)

Source of truth: `mockup/Chatot Interactive.dc.html` (1240×840 canvas, light).
App captured with `CHATOT_FAKE=1`, `theme: light`, window floated at 1240×840
on niri (scale 1.2). Shots live in `.playwright-mcp/cmp/` as `mock-NN-*.png`
and `app-NN-*.png` (same NN = same state). Mockup states were driven by
setting the design's React logic state directly; app states via the
`CHATOT_SHOT=<state>` dev hook added in `cmd/chatot/main.go`.

Legend: **[G]** global · **[L]** layout/size · **[S]** style/colour ·
**[C]** content/structure (items, icons, copy) · **[M]** missing surface.

## 0. Global

- **[G] Font.** Cantarell (and JetBrains Mono for the phone line) is not
  installed in the devenv; the app falls back to DejaVu Sans, which is wider
  and taller, so every row/header/bubble reads bigger than the mockup. Adding
  `pkgs.cantarell-fonts` + `pkgs.jetbrains-mono` to `.nix/devenv.nix`
  (fontconfig picks them up via `$XDG_DATA_DIRS/fonts`) fixes most of the
  "everything is slightly too large" impression (verified: `app-80/81-font-*`).
- **[L] Sidebar width.** Mock rail+list = 388 px (54 + 334); app ≈ 403 px
  (`SetSidebarWidthFraction(0.27)` + min 300). Set fraction to 388/1240 ≈ 0.313
  and min/max to 388.
- **[L] Header strip height.** Mock 47 px (46 + 1 px hairline) on both sides;
  app ≈ 60 px (Adwaita HeaderBar min-height + `.chatot-account-row` padding).
- **[C] Window controls.** Mock shows − □ ✕ as 24 px chip-coloured circles;
  app shows only ✕ (GTK default layout under niri). Set the header's
  decoration layout to `:minimize,maximize,close`.
- **[S] Popover/menu chrome.** Mock menus: 230–260 px wide white cards,
  12 px radius, soft shadow, **no arrow**, items left-aligned with a 16 px
  emoji/glyph icon column, 13 px text, mono accel labels right-aligned,
  destructive items in `#c01c28`, 1 px separators. App menus: Adwaita popover
  with arrow, centred bold labels, no icons/accels. Affects ＋ menu, ⋮ app
  menu, chat ⋮ menu, message ⋯ menu, chat-row context menu, labels popover.
- **[S] Dialog chrome.** All secondary dialogs are plain `gtk.NewWindow` →
  on niri (no server-side decorations) they render with **no title bar and
  no close button** (add account, accounts, forward, export, new chat/group/
  community, join, media page, group info, blocked, privacy). Mock dialogs are
  in-window cards: 12 px radius, title row (15 px bold + ✕ at right),
  hairline under the title, 16–18 px padding, dimmed backdrop. Move these to
  `adw.Dialog` (in-window) with an `adw.HeaderBar`/title row.

## 1. Sidebar header (account button, ＋, ⋮)

- **[S] Account avatar colour.** Mock: the account's palette colour (same as
  its rail tile). App: always brand green in the header while the rail tile is
  orange → inconsistent. Use the account colour.
- **[L] Button sizes.** Mock ＋/⋮ are 28×28, 7 px radius, 15 px glyph, dim
  colour; account button 38 px tall with 28 px avatar, name 13 px bold, phone
  9.5 px mono. App buttons are Adwaita 34 px+, header ≈ 60 px.
- **[C] ＋ menu items.** Mock: 💬 New chat · 👥 New group · 🏘 New community.
  App: adds "Join with invite link" and has no icons.
- **[C] ⋮ app menu items.** Mock: 📂 Archived · ⭐ Starred messages | 🖥 Linked
  devices · ⚙ Preferences `Ctrl+,` · 🐦 About chatot | ⏻ Unlink this device
  (red) · ✕ Quit `Ctrl+Q`. App: Archived chats · Starred messages · Status
  updates · Channels | Set status · Privacy settings · Blocked contacts ·
  Keyboard shortcuts · Preferences | About chatot. No icons/accels; extra
  items the mock puts elsewhere (Blocked/Privacy live in Preferences ▸ Privacy).

## 2. Account rail

- **[L] Badge.** Mock unread pill: 17 px tall, 10 px bold, `padding 0 5px`,
  no border, hugging the tile's top-right (overhangs ~4 px right, 3 px up).
  App badge is smaller (~13 px), has a 2 px sidebar-coloured border and sits
  inside the tile bounds.
- **[S] Logged-out account** tile is grey `rgba(120,120,120,.55)` at 70 %
  opacity in the mock; the app has no logged-out state styling (fake has none).
- OK: 38 px tiles, 12 px radius, 10 px gap, dashed ＋, active double ring.

## 3. Search + filter chips

- **[S] Search entry.** Mock: 34 px white pill, `⌕` glyph 12 px dim, 13.5 px
  text, hairline `rgba(0,0,0,.13)`, `padding 0 12px`, 8/8/4 margins. App is
  close but uses the symbolic search icon and a stronger border; the clear
  button in the mock is a plain `✕` glyph.
- **[S] Chips.** Mock: 25 px tall, `padding 4px 11px`, 12 px text, count 10.5 px
  bold accent, active chip `#1b8c72` white bold. App matches closely; the
  overflow button is `…` text in a 28×24 pill in the mock (`⋯`, 13 px) vs the
  app's `…` label chip with chip padding.
- **[C] Label filter.** Mock: choosing a label swaps *Favorites* for a green
  `● Family 2` chip in position 3. App: not verified (fake labels "Work",
  "Family" exist); the overflow popover shows a coloured dot + name + count
  (mock: 10 px round dot, count right-aligned dim, `＋ Manage lists…` after a
  separator) — app shows tall coloured bars and "Work 1" as one label.

## 4. Chat list rows

- **[L] Row geometry.** Mock: 53 px rows, `padding 7px 8px`, 8 px side
  margins, 1 px gap, 8 px radius, 38 px avatar, name 13.5 px bold, preview
  12.5 px dim, time 10.5 px, pin 📌 9 px. App rows ≈ 64 px (`margin 1px 6px;
  padding 6px 8px` + larger font), 40 px avatar.
- **[S] Unread time colour.** Mock colours the timestamp accent green when the
  chat has unread; app keeps it grey.
- **[S] Selected row.** Mock `rgba(0,0,0,.1)` — app matches.
- **[C] Merged "All accounts" mode** (mock 38): 3 px account-colour stripe at
  the row's left, preview prefixed with the account label, header avatar `🗂`.
  App has no merged list mode.
- **[C] Empty state.** Mock: centred "No chats match “zzz”" + a pill
  "Clear search" button. App: italic "No results" only.

## 5. Conversation header

- **[L] Size.** Mock 47 px: 28 px avatar, name 14 px bold, presence 11.5 px
  (green when typing), ⋮ 28×28, window controls 24 px circles. App: 60 px,
  40 px avatar, larger name, no presence line shown for fake contacts.
- **[C] ⋮ chat menu.** Mock (11 items, icons, sections): ℹ Contact info ·
  ⌕ Search in chat `Ctrl+F` · 🖼 Media, links and docs | 🔇 Mute
  notifications… · 📌 Pin chat · ⏱ Disappearing messages… · 📂 Archive chat |
  ⤓ Export chat… · 🗑 Clear chat… (red) · 🚫 Block contact… (red). App: Search
  in chat · Media, links and docs · Export chat… · Clear chat…
- **[M] Contact info dialog** (mock 55: big avatar, name, presence · phone,
  rows Mute / Disappearing / Media / Block). App: none for 1:1 chats.
- **[M] Mute duration dialog** (8 hours / 1 week / Always) and
  **Disappearing-messages dialog** (Off ✓ / 24 h / 7 d / 90 d) — mock 53/54.
  App: no such choosers from the chat menu (disappearing lives in group info).
- **[M] Block confirmation** (mock 52: "Block Alex Rivera?", "Also report this
  contact" switch, Cancel | Block red). App: blocks directly from the row menu.
- **[L] In-chat search** (mock 28): the search replaces the header title area
  — 34 px pill entry, `1/1` counter 10.5 px mono, ▲ ▼ ✕ 28 px buttons, all
  inside the 47 px header. App: a second row under the header with a large
  entry, "1 of 1 matches in this chat" text, ⌃ ⌄, "Search all chats", ✕.

## 6. Message thread

- **[L] Thread padding.** Mock `10px 18px 14px`, bubbles max ≈ 302 px for
  media, 14 px radius, `padding 7px 11px 5px`, text 13.5 px, meta 10 px, 5 px
  between bubbles, day pill 21 px / 10.5 px / `padding 3px 11px` / 8 px top
  margin. App: 20 px side margins, text ≈ 14.7 px, day pill ≈ 12 px.
- **[S] Read ticks.** Mock `#cdece4` (pale mint) bold 10 px on green bubbles;
  app uses WhatsApp blue `#34b7f1`.
- **[S] Timestamp/edited.** Mock: `edited` italic 10 px before the time; app
  appends an edited marker to the time label (fine) — colours OK.
- **[L] Hover actions.** Mock: ONE floating pill above the bubble's top edge
  (`top:-15px`, right/left 8 px): 👍 ❤️ 😂 😮 😢 🙏 (24 px each) · ＋ (chip bg)
  · 1 px divider · ↩ ↪ ⋯ (24 px, dim), white bg, hairline, 999 px radius,
  `padding 2px 4px`, shadow `0 4px 14px rgba(0,0,0,.16)`. App: three 26 px
  square icon buttons (↩ 🙂 ⋯) beside the bubble; the quick-react pill only
  appears after clicking 🙂, and pops below the bubble.
- **[C] ⋯ message menu.** Mock: ↩ Reply · ↪ Forward · ⭐ Star message ·
  📋 Copy text · 📌 Pin in chat | ℹ Message info · 🗑 Delete message (red).
  App: Reply · Forward · Copy text · Edit · Star · Delete for me (centred, no
  icons, no sections).
- **[C] Full reaction picker** (mock 21): "PICK A REACTION" card with an 8-col
  emoji grid, 322×218. App: native GtkEmojiChooser.
- **[S] Reactions row.** Mock: white pills `padding 1px 6px`, 11 px emoji,
  hairline + shadow, overlapping the bubble's bottom edge (`bottom:-11px`),
  right/left 8 px. App: emoji inline inside the bubble under the footer.
- **[L] Image/video tile.** Mock: 280×115 hatch tile, 10 px radius, 38 px
  green ⬇ disc centred, "📷 Photo · 840 KB" 12 px dim **below** the disc,
  caption 13.5 px under the tile. App: ≈ 280×150 tile, "📷 Photo" label at the
  **top**, no size, no caption sample.
- **[L] Location.** Mock static pin: 270×86 hatch map tile with a 13 px red
  dot, "📍 Rua das Flores 210" 12.5 px bold, "Porto · click to open in Maps"
  11 px dim. App: green title, coordinates, underlined "Open in maps" link,
  no map tile.
- **[S] View-once.** Mock: 28 px outlined green circle with "1", title
  "Photo · view once" 12.5 px bold (ink colour), sub 11 px dim. App: 📷 icon,
  green title text.
- **[C] Contact card.** Mock: 38 px avatar, name 13 px bold, phone 11.5 px dim,
  hairline, centred green bold "Message" action. App: 👤 name + phone only.
- **[L] Document row.** Mock: 28 px green ⬇ disc, "📄 lease-2026.pdf" 12.5 px
  bold, "Click to download · PDF · 1.2 MB" 11 px dim; failed state = red
  `↻` disc + "Download failed · click to retry" red. App: similar structure;
  title not bold, no type/size line.
- **[C] Voice message.** Mock: pending = disc + "🎤 Voice message · 0:12" +
  "Click to download · 48 KB"; ready = 28 px green ▶ + 22-bar waveform +
  `0:24` mono. App: not seen in fake data (needs a seed).
- **[C] Poll.** Mock: author 12.5 px green bold, question 13.5 px bold,
  options with 16 px checkbox + label + count, 4 px accent progress bar,
  "Select one · 4 votes" 10.5 px dim. App: 📊 question, "Select one", options
  as centred bold buttons "Pizza (1)", "1 vote(s)".
- **[S] Sticker / emoji-only / GIF.** Mock stickers 120 px bare on a hatch
  tile when pending; GIF = image tile with "GIF" badge. App close; sticker
  pending label "Sticker" sits at the tile top.
- **[C] Group system events.** Mock: centred pills "Priya added Nina Costa",
  "Tomás requested to join · approve in group settings". App: a green banner
  "1 person requested to join · Review" above the thread instead.
- **[C] Typing indicator.** Mock: 54×26 bubble with three 6 px dots. App:
  has a typing model but no bubble seen (fake never types).
- **[C] Deleted message.** Mock: 🚫 + italic "This message was deleted" inside
  a bubble with the author. App: italic text, no icon.

## 7. Composer

- **[L] Bar.** Mock 51 px (`padding 8px 12px`), 📎 and 🙂 32 px circles 7 px
  apart, entry 34 px pill 13.5 px `padding 0 14px`, 🎤 34 px. App ≈ 57 px,
  📎 and 🙂 ≈ 70 px apart (Adwaita button min-width), 📎 rendered dim/blue.
- **[C] Attach menu.** Mock (`attach: true`): vertical list of 7 rows with
  tinted 28 px circle icons — 📷 Photos · 🎬 Video · 📄 Document · 👤 Contact ·
  📍 Location · 📊 Poll · 🎵 Audio file — 230 px wide card above 📎. App: 3×3
  grid of tiles (Photo, Camera, Document, Location, Contact, Poll, Event).
- **[C] Emoji/GIF/Stickers picker.** Mock: one 360 px popover above 🙂 with a
  segmented pill switcher (Emoji | GIF | Stickers); Emoji = 9-col grid of
  24 px cells; GIF = 3×2 colour tiles; Stickers = 4×2 tiles + "Recents ·
  Porto pack · Cats" footer + "Add pack". App: 🙂 opens native
  GtkEmojiChooser; GIF/Stickers are a separate popover from the 📎 button
  with a plain StackSwitcher.
- **[L] Reply bar.** Mock: inside the composer band, 3 px green left bar,
  name 11.5 px green bold + text 12.5 px, ✕ at right, `rgba(0,0,0,.06)` bg.
  App: full-width grey bar above the composer with the quoted text only (no
  author name), ✕ far right.
- **[S] Recording.** Mock: entry replaced by "● Recording… 0:07" (red dot,
  mono timer) + "Cancel" link + red 34 px ■ disc. App: entry greyed out, small
  red stop icon in a flat button.
- **[S] Send button.** Mock: 34 px green disc with ➤; app matches (icon is
  `go-next-symbolic`).
- **[M] Attachment tray** (mock 27): full-pane preview with "Cancel · name ·
  Send 2" header, big preview, caption entry, thumbnail strip with ✕ and ＋.
  App: sends files straight from the file chooser.

## 8. Sidebar modes / panes

- **[L] New chat / Add people / group name** (mock 33–35): rendered **in the
  sidebar** with a `← New chat` header, "Search name or number" pill, contact
  rows (38 px avatar, status line), picked-people chips, a full-width green
  "Next · name the group" / "Create group" button at the bottom, group-photo
  placeholder. App: separate undecorated windows (phone entry, "CONTACTS",
  "0 of 1024 selected", Next).
- **[L] Archived** (mock 36): sidebar title "← Archived · 2" with the archived
  rows in the same list. App: title unchanged, list empty (fake has none),
  chip counts vanish.
- **[L] Starred** (mock 32): a right-pane page "← Starred messages · 1
  starred" with rows (avatar, chat name, text, time, ⭐). App: sidebar swaps to
  "No starred messages" (italic).
- **[L] Media, links and docs** (mock 29–31): right-pane page with `←` title
  and a segmented Media | Links | Docs switcher; Media = 5-col grid of 150 px
  tiles with weekday labels; Links/Docs = 44 px rows with a 36 px icon square,
  bold title, dim sub, date right. App: separate 420 px window, "Media 2 /
  Links 1 / Docs 1" StackSwitcher, "SEPTEMBER 2026" header, tiny icons.
- **[L] Linking screen** (mock 64): header "chatot" + window controls, 22 px
  bold "Link chatot to WhatsApp", 13.5 px dim instructions, 220 px QR card
  with the 🐦 centre, "● Waiting for scan · code refreshes in 42s", green
  "Simulate a successful scan" + grey "Link with phone number". App: 256 px
  raw QR, instruction sentence, "Waiting for you to scan…", flat text button.

## 9. Dialogs

- **[L] Add account** (mock 40): titled card, instructions, 180 px QR card,
  "Waiting for you to scan…", "Label this account / Shown in the account
  button" row with entry, green full-width button. App: text + entry + button,
  no title, no QR.
- **[L] Accounts** (mock 41): title "Accounts" + green "Add…" pill, rows with
  36 px avatar, name 13.5 bold, mono "+351 … · connected" (status coloured),
  ⋮ per row; two switch rows below. App: close in structure, no title bar,
  "…" buttons, no phone/status line styling.
- **[L] Manage lists** (mock 42): rows with 10 px dot, name, "1 chat", 🗑;
  palette swatches + "New list name" entry + "Add"; helper text. App: not
  verified (opens from the labels popover).
- **[L] Preferences** (mock 43–48): 720×445 card, title row with ✕, left
  sidebar nav (🎨 Appearance · 🔔 Notifications · 🔒 Privacy · 🌐 Network ·
  ⌨ Shortcuts · 🛠 Advanced), section captions in 11 px caps, white grouped
  rows with switches/dropdown values. App: AdwPreferencesWindow with a top
  view-switcher (General · Privacy · Network) and only Theme/Notifications on
  General.
- **[L] Forward** (mock 49): "Forward to…" title + ✕, rows with 30 px avatar
  and 18 px round check, "2 selected" + green Send. App: quoted text on top,
  search entry, square checkboxes, "0 chats selected", Cancel/Forward.
- **[L] Export** (mock 50): rows Format / Range (dropdown values right,
  dim) / Include media switch, "SAVE TO" caption + mono path + "Choose…",
  Cancel + green Export. App: same rows but Adwaita buttons, no title.
- **[S] Clear chat / Block** alerts (mock 51/52): Adwaita-style alert with a
  switch row and red confirm — app's Clear matches well (switch inside a
  card, red button); Block alert missing.
- **[L] Linked devices / About / Create poll / Send contact** (mock 56–59):
  Linked devices missing in the app; About = mock shows 80 px icon, "0.4.0 ·
  GTK4 · libadwaita" mono, tagline, Website/Report buttons — app shows name
  twice and a version pill; Create poll = question + option entries, "＋ Add
  option", "Allow multiple answers", Cancel/Send poll — app: not captured
  (opens from attach); Send contact = search + rows with radio circles +
  "0 selected"/Send — app: not captured.
- **[C] Toast.** Mock: black pill bottom-centre "Copied to clipboard  Undo"
  (Undo green). App: Adwaita toast "Message copied to clipboard" + Undo + ✕.

## 10. Dark mode (out of scope for this pass)

Mock dark tokens: bg `#242424`, header `#303030`, sidebar `#2c2c2c`, popover
`#383838`, accent `#6fd3ba` for links/ticks. Not audited here.
