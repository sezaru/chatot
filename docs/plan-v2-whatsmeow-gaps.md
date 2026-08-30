# chatot v2 — whatsmeow feature-gap implementation plan

**Goal:** implement every WhatsApp feature the embedded `go.mau.fi/whatsmeow` lib
supports that chatot's v1 (F1–F13) did not. Continues the v1 numbering at **F14**.

**Execution protocol (identical to v1 — do not deviate):**
- One feature at a time, **in order**.
- Per feature: spawn a **builder** agent, then a **reviewer** agent. Both Sonnet by
  default; if a feature is too complex for Sonnet, **Opus builds + Sonnet reviews**.
- Reviewer scope is **narrow**: read the feature's code, find issues, at most run that
  feature's own tests. NOT the full suite, NOT formatting.
- At each feature's **end**: full `go test ./...` + `gofmt -l .` (both from
  `nix develop -c`), then **commit** (chatot has commit-as-you-go authorization).
- Never stop mid-run to ask. If a feature is truly blocked, pause it, move on, surface
  it at the very end.
- ALL features ship. "Done" per feature = compiles + feature tests green + committed.
  Live-GUI / live-WhatsApp click-through is the user's manual acceptance (agents have
  no display / no linked account) — never claim GUI-verified.

**Architecture seams (v1, for builders):**
- Inbound: whatsmeow event → `client/events.go` `translate()`/`extractText()` → normalized
  `client.Event` → `client/eventbus.go` fan-out AND `client/ingest.go` → `internal/store`.
- Interface: `client.Client` (client.go). Real impl `client/whatsmeow.go`, in-memory
  `client/fake.go` (must stay in sync — tests + `CHATOT_FAKE=1` depend on it).
- Store: `internal/store` is a pure, whatsmeow-agnostic leaf (own value types in
  types.go; `client/convert.go` is the only client↔store boundary). Schema in
  schema.sql; additive columns via `migrateAddColumn`.
- UI: sidebar `ui/chatlist.go`; thread `ui/conversation.go` (GtkListView virtualized,
  bind factory rebuilds a bubble per row via `buildBubble`); composer `ui/composer.go`;
  media `ui/media_view.go`. Pure view-models (`*VM`) are unit-tested without a display.
- Every event-goroutine→GTK hop goes through `glib.IdleAdd`.

## Shared infrastructure (introduced by F14, reused after)

Rich non-text message bodies (location, contact, poll) don't fit the text/media schema.
F14 establishes the seam and later content features reuse it:
- `messages` gains additive columns **`kind TEXT`** (default `''` = plain/text-or-media)
  and **`payload TEXT`** (JSON) via `migrateAddColumn`.
- `client.Message` gains typed optional fields (`Location *Location`, `Contact *Contact`,
  `Poll *Poll`, …) populated in `extractText` and (de)serialized to `payload` in
  `convert.go`. Chat-list preview (`store/chats.go`) learns the new kinds
  (📍 Location / 👤 Contact / 📊 Poll …).
- `buildBubble` dispatches on the new fields to a per-kind renderer.

---

## Phase A — message content types (stop rendering blank bubbles)

### F14 — Location messages (static + live)  [Sonnet]
Parse `LocationMessage` + `LiveLocationMessage` in `extractText`; store via the new
kind/payload seam; render a bubble (name/address if present, lat/long, an "Open in maps"
link via `geo:`/OSM URL). Send a static location from the composer (attach → "Location"
uses `SendMessage` with a `LocationMessage`). Preview: 📍.

### F15 — Contact-card (vCard) messages  [Sonnet]
Parse `ContactMessage` (+ `ContactsArrayMessage`); store; render a card (display name +
number(s), copy affordance). Preview: 👤. (Sending contacts optional; receiving first.)

### F16 — Polls (create, vote, tally)  [Opus builds]
Parse `PollCreationMessage` (question + options); render options with live vote counts.
Vote via `BuildPollVote` + `SendMessage`. Decrypt incoming votes (`DecryptPollVote`) to
update tallies; store per-option counts. Create a poll from the composer. Preview: 📊.

## Phase B — message lifecycle

### F17 — Message edits  [Opus builds]
Inbound: detect `ProtocolMessage`/`EditedMessage` (message edit), update the stored
message text, render an "edited" marker. Outbound: edit an own text message via
`BuildEdit` + `SendMessage`, reflected optimistically. FTS index updates on edit.

### F18 — Delete for everyone / revoke  [Sonnet]
Inbound: handle revoke `ProtocolMessage` (type REVOKE) → mark the message deleted, render
"🚫 This message was deleted". Outbound: revoke an own message via `BuildRevoke` +
`SendMessage`, from a per-bubble action. Add a `deleted` flag column.

## Phase C — status indicators

### F19 — Delivery / read ticks  [Sonnet]
Track per-outgoing-message delivery state from `Receipt` events (delivered vs read).
Store a per-message status; render ✓ / ✓✓ / ✓✓(accent) on own bubbles. (We already send
read receipts opt-in; this is the *display* side for our sent messages.)

## Phase D — identity

### F20 — Avatars / profile pictures  [Sonnet]
`GetProfilePictureInfo` → download + disk-cache avatar images (reuse the media cache dir
+ eviction). Render circular avatars in the chat list + conversation header; fall back to
the initial when absent. Refresh on `Picture` events.

### F21 — Start a new chat  [Sonnet]
A "new chat" entry (compose to a phone number). Validate with `IsOnWhatsApp`; open a
conversation for a JID not yet in the list. Resolve display name via `GetUserInfo`.

## Phase E — chat organization (app-state writes)

### F22 — Pin / mute / archive / mark-unread toggles  [Sonnet]
Per-chat actions writing app-state (`SendAppState` via the app-state builders). Reflect
inbound `Pin`/`Mute`/`Archive`/`MarkChatAsRead` app-state events into the store. Add
`archived` column; sort/filter accordingly.

### F23 — Star messages + starred view  [Sonnet]
Star/unstar a message (app-state `Star`); a starred-messages filter view. Add `starred`
column; reflect inbound `Star` events.

### F24 — Block / unblock + privacy read  [Sonnet]
`GetBlocklist` + `UpdateBlocklist` (block/unblock a contact); reflect `Blocklist`/
`BlocklistChange` events. Read + display `GetPrivacySettings` (read-only surface).

### F25 — Labels  [Opus builds]
Create/edit labels (`LabelEdit`), associate chats/messages (`LabelAssociation*` via
app-state), and filter the chat list by label. New `labels` + `label_chats` tables.

## Phase F — groups

### F26 — Group info & membership display  [Sonnet]
`GetGroupInfo`/`GetJoinedGroups`; store participants; a group-info panel (name, topic,
description, participant list w/ admin badges). Reflect `GroupInfo`/`JoinedGroup` events.

### F27 — Group management actions  [Opus builds]
`CreateGroup`, `LeaveGroup`, `UpdateGroupParticipants` (add/remove/promote/demote),
`SetGroupName`/`Topic`/`Description`/`Photo`/`Announce`/`Locked`, invite links
(`GetGroupInviteLink`, `JoinGroupWithLink`), join-request approvals.

## Phase G — history / media

### F28 — On-demand history sync  [Sonnet]
When scroll-up paging hits the oldest locally-synced message, request more from the phone
via `BuildHistorySyncRequest` + `SendMessage`; ingest the resulting `HistorySync` and
continue paging.

### F29 — Media thumbnails + media retry  [Sonnet]
Show `DownloadThumbnail`/jpeg-thumbnail previews before full download (replace the
tap-to-load chip with a blurred thumb). Handle `MediaRetry`/`MediaRetryError`; re-request
via `SendMediaRetryReceipt` on decrypt failure.

## Phase H — calls / presence extras

### F30 — Reject call + "recording" presence  [Sonnet]
Wire `RejectCall` from the incoming-call notification. Send and display the
`recording` chat-presence state (composer while recording a voice note; header/preview
show "recording audio…").

## Phase I — channels / status

### F31 — Status / Stories  [Opus builds]
View received status updates (`status@broadcast`), and post a text/media status via
`SendMessage` to the status broadcast. A dedicated status view.

### F32 — Channels / Newsletters  [Opus builds]
`GetSubscribedNewsletters`, `GetNewsletterMessages`, follow/unfollow, `NewsletterSendReaction`,
mute. A channels list + read view.

## Phase J — connectivity / misc

### F33 — Phone-number pairing code + proxy  [Sonnet]
`PairPhone` (enter a code on the phone) as an alternative to QR on the linking screen.
Expose `SetProxy` via an env/config option for the connection.

---

**On completion:** run the full suite + gofmt once more, update
`memory/chatot-gtk-app.md`, and report per-feature status + the single manual acceptance
step (`nix develop -c go run ./cmd/chatot`, real QR pairing).
