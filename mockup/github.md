repo: sezaru/chatot
branch: master

## Last sync
date: 2026-08-31T04:49:00Z
commit: 3fb8a8d508e3

### Updated in this project
- Built `Chatot Interactive.dc.html`, one working app window covering every mocked feature.
- Checked label support against the source: chatot has none today; in whatsmeow labels arrive via app-state sync (`LabelEdit`, `LabelAssociationChat`) and `LabelEditAction` carries `name`, `predefinedID` and `color` as an int palette index.
- Reworked them as WhatsApp Lists: per-account, available on personal accounts, palette index instead of a custom colour picker.

## Chat lists / labels — what the code actually supports
- `sezaru/chatot@master`: no list or label feature; every `label` hit in `internal/ui` is a `gtk.Label` widget.
- `tulir/whatsmeow`: `types/events/appstate.go` emits `LabelEdit`, `LabelAssociationChat`, `LabelAssociationMessage`; `proto/waSyncAction` `LabelEditAction` = name, color (int32 palette index), predefinedID, deleted, orderIndex, isActive, type, isImmutable.
- `LabelEditAction.ListType`: NONE, UNREAD, GROUPS, FAVORITES, PREDEFINED, CUSTOM, COMMUNITY, SERVER_ASSIGNED, DRAFTED, ARCHIVED, LOCKED, … — i.e. the filter bar. UNREAD/GROUPS/FAVORITES are built in; CUSTOM is a user-made list, on personal accounts too.
- Implication for the mockup: lists are per account, mirrored from the phone, and the colour is a palette index, not a picked colour.

- `tulir/whatsmeow` broadcast lists: `errors.go` `ErrBroadcastListUnsupported` — "sending to non-status broadcast lists is not yet supported" (`broadcast.go`), so "New broadcast list" was removed from the ＋ menu. Incoming broadcast messages are still readable (`types.JID.IsBroadcastList`).
- `tulir/whatsmeow` communities: `group.go` `ReqCreateGroup{IsParent: true}` creates a community, `LinkedParentJID` creates a group inside one — so "New community" is a real flow.

## Screen map
| Screen | Repo files |
| --- | --- |
| Interactive app (all features) | internal/ui/chatlist.go, conversation.go, composer.go, media_view.go, linking.go, style.css |
| Main window (light/dark) | cmd/chatot/main.go, internal/ui/chatlist.go, internal/ui/conversation.go, internal/ui/composer.go, internal/ui/style.css |
| Pairing / linking | internal/ui/linking.go |
| Sidebar search | internal/ui/chatlist.go (searchHitVM, buildSearchHitRow) |
| Media + reply + voice record | internal/ui/media_view.go, internal/ui/composer.go |
| Empty state | internal/ui/conversation.go (empty placeholder) |

## Sync history
- 2026-08-30T16:26:05Z — Adwaita GTK4 mockups (light + dark) from `internal/ui/style.css` and the gotk4 widget trees.
