# chatot v3 — mockup fidelity + multi-account

Source of truth: the Claude Design project **"Chatot Multi-account.dc.html"** (10 design
turns t1→t10). This wave (a) reskins the whole app to the mockup's visual spec and
(b) adds multi-account plus a large set of new surfaces the mockup introduces.

Rendered mockup reference lives at `/tmp/chatot-mockup/shot-t1..t10.png` (re-render from
the design MCP if lost). Turn map:

- **t1** foundation: two-pane app, light+dark, QR link, sidebar search, composer states,
  attachment load states, empty state.
- **t2** message action menu, copy-with-undo, reactions quick-row, full emoji picker,
  attachment menu, forward dialog.
- **t3** label filter row + custom labels + overflow; right-click chat context menu w/ Labels.
- **t4** message content types (polls, static/live location, vCard, edits, revoke, view-once,
  disappearing, receipts/ticks, media retry, group system events, join-approval, recording).
- **t5** contact/header menu (Search in chat / Media,links,docs / Export / Clear); in-chat
  search w/ hit-stepping; Media·Links·Docs page; Clear + Export dialogs.
- **t6** composer emoji/GIF/sticker picker; how stickers & GIFs land in the thread.
- **t7** sidebar ＋ menu (new chat/group/community/invite); new-chat view; ⋮ app menu;
  new-group flow; archived list; **Preferences window** (General/Privacy/Network); tray menu.
- **t8/t9/t10** multi-account: account rail (≥2 accounts), switcher popover, add-account
  dialog, accounts-management dialog, per-account notifications/keep-connected, proxy
  Preferences + per-account proxy override.

## Execution protocol (identical to F14–F33)

One feature at a time, in order. Per feature: **builder** agent → **reviewer** agent
(narrow: read the diff, find real bugs, at most run that feature's own tests) → full
`nix develop -c bash -c 'go build ./... && go vet ./... && go test ./... && gofmt -l .'`
gate → **commit**. Opus builds the architecturally/crypto heavy ones (tagged `[Opus]`),
Sonnet builds + reviews the rest. Never stop mid-run to ask: pause a blocked feature,
surface it at the very end. Everything ships; nothing silently cut. Never claim
GUI/live-verified — the agents have no display / no linked account.

### Visual-fidelity loop (new this wave)

The app runs in the live niri session; `CHATOT_FAKE=1 nix develop -c go run ./cmd/chatot`
pops a real window. After each visually-significant feature: launch it, capture with
`niri msg action screenshot-window` (saved to ~/Pictures/Screenshots), read the PNG, diff
against the matching mockup section, iterate CSS/layout until faithful. NOT a pixel diff —
GTK4 native ≠ HTML; goal is faithful adaptation with every element present and correct.
The Fake client must seed data for every new surface (multiple accounts, labels, media
page, etc.) so each screen is reachable without a live link.

## Architecture notes

### Multi-account (keystone — F35)

Today `main.go buildClient()` returns ONE `client.Client`; every widget is constructed with
it. Target: an `AccountManager` owning `[]*Account`, each Account = its own
`*client.Whatsmeow` with its own state dir/DB (`$XDG_STATE_HOME/chatot/accounts/<id>/`) +
metadata {id, label, colour, connected}. Active-account pointer; switching sets active and
tells the UI to rebind + reload (window and widgets persist — "switching swaps the chat
list, not the window"). Accounts registry persisted as JSON alongside the account dirs.
Keep-inactive-connected toggle governs whether non-active accounts stay Connected (for
background notifications). The `Client` interface is unchanged per-account; the manager is
new and the UI widgets gain a `SetClient`/rebind seam. Fake grows a multi-account seed.

### Theming (F34)

Override libadwaita accent to the mockup green via `AdwStyleManager` accent + a big
`style.css` pass (Cantarell base font, bubble radii/colours, sidebar rows, filter chips,
composer, headers) covering `.light`/`.dark`. Keep using Adwaita named colours where the
mockup matches; hard-set where it doesn't.

## Design tokens (extracted from mockup source)

- Font: **Cantarell** (UI); JetBrains Mono only for the canvas chrome, not the app.
- Accent green **#1b8c72** (buttons, outgoing bubbles) · links **#147a63** · hover **#0f6350** ·
  dark-mode accent **#6fd3ba** · light tint **#cdece4** · teal secondary **#427677**.
- Destructive **#c01c28** (light) / **#ff7b85** (dark).
- App bg: **#fff / #fafafa / #f4f3f2** (light), **#1a1a1a / #1e1e1e / #242424 / #303030** (dark).
- Secondary text via `rgba(0,0,0,.45–.6)` (light) / `rgba(255,255,255,.45)` (dark).
- Muted per-contact avatar palette: #5a7ab5 #b58a4a #9c5b8a #c26b5c #7a8b5a #e8a34a #427677 #6a6a8c.
- Radii: **999px** pills/avatars · **12–14px** bubbles/cards · **7–8px** buttons/inputs/rows.

## Decisions (from user)

- **GIF (F37):** build the picker UI + inline-loop render now; the search *provider* (Tenor/etc.)
  is a later follow-up — stub results with a "provider not configured" state.
- **Stickers (F38):** send + recents + no-bubble render now; pack marketplace explored later.
- **Communities (F48):** implement exactly what whatsmeow supports; no more.
- **Camera (F36):** omit for now (GStreamer + pipewire/portal capture pipeline is a big native
  dep; "Photo or video…" already covers sending images). Revisit as its own feature later.
- **Tray (F55):** add the StatusNotifierItem dep — core to t7d/t8.
- **Event (F52):** build if `EventMessage` exists in the pinned whatsmeow; else pause + surface.
- **Ordering:** multi-account goes **LAST**. The whole single-account app is built to mockup
  fidelity first; the account switch is a **façade** (`client.Client` forwarding to the active
  account, re-emitting its events) so widgets need zero retrofit — a switch looks like a
  reconnect they already handle. Per-account bits (add-account, accounts mgmt, per-account
  notifications/proxy) land with the multi-account wave at the end.

## Features (execution order = numeric order; commits F34→F60)

Phase 1 — theme
- **F34 [Opus]** global theme pass: green accent + Cantarell + light/dark; restyle sidebar rows,
  filter chips, bubbles, composer, conversation header, day separators, ticks to match t1.

Phase 2 — composer input surfaces
- **F35** full emoji picker (search + categories), shared by composer + reaction "+" (t2/t6).
- **F36** attachment menu popover (Photo/Document/Location/Contact/Poll/Event — no Camera) (t2c).
- **F37** GIF picker UI + inline-loop render; provider deferred (t6).
- **F38** stickers: send + recents + no-bubble render (t6).

Phase 3 — message actions
- **F39** message action menu (Reply/Forward/Copy/React/Delete) + copy-with-undo toast (t2a/t2b).
- **F40** forward dialog (multi-select chats) + forwarded marker (t2d).

Phase 4 — chat/contact surfaces
- **F41** label filter row + custom-label chips/counts/overflow + right-click Labels submenu (t3).
- **F42** contact/header menu + in-chat search with hit-stepping + highlighted matches (t5a/t5b).
- **F43** Media·Links·Docs page (3 tabs) (t5c).
- **F44** Export-chat dialog + Clear-chat dialog (t5d).

Phase 5 — new-chat / groups / app menu
- **F45** sidebar ＋ menu (new chat/group/community/invite) + new-chat view redesign (t7a).
- **F46** new-group flow (pick participants → details: name/disappearing/only-admins) (t7c).
- **F47** ⋮ app menu + Archived list + Blocked-contacts page + Keyboard-shortcuts dialog +
  About dialog + Set-status-message (t7b).
- **F48** communities + join-with-invite-link — whatever whatsmeow supports (t7a).

Phase 6 — message content additions
- **F49** view-once messages (chip + open-consumes) (t1/t4).
- **F50** live-location send (start/stop sharing) (t4).
- **F51** disappearing-messages timer set + system notice (t4).
- **F52** Event message type — if `EventMessage` supported, else pause+surface (t2c).
- **F53** group join-approval request banner (t4).

Phase 7 — Preferences + tray (global; account-agnostic)
- **F54** Preferences window shell + General tab (theme, window controls, tray toggles) +
  Privacy tab (read receipts, typing, default disappearing timer, blocked count) (t7d).
- **F55** system tray (StatusNotifierItem) + close-to-tray + start-minimised + tray menu (t7d/t8).
- **F56** Network tab: global proxy UI (type/host/port/auth, also-proxy-media,
  never-without-proxy, test connection, reconnect dialog) (t10a).

Phase 8 — multi-account (LAST)
- **F57 [Opus]** AccountManager façade: N whatsmeow clients, per-account state dirs, active
  switch, add/remove, JSON registry, main.go rewiring via the façade, Fake multi-account seed.
- **F58** account rail (shown ≥2 accounts) + header-as-account-button + switcher popover (t8a/t9).
- **F59** add-account dialog (QR + phone + label) + accounts-management dialog (t8d/t8e).
- **F60** per-account notifications (toast label prefix) + keep-inactive-connected +
  per-account proxy override (t8e/t10b).
