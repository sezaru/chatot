# Sending a location — design + implementation notes

The flow lives in `Chatot Interactive.dc.html` (attach ⊕ → 📍 Location). This
file records the decisions behind it, and what the GTK4 side needs.

## The sheet

One 600 px dialog, two tabs, four states:

| State | What it shows |
|---|---|
| Locating | Scrimmed map, spinner, `org.freedesktop.portal.Location` — the call actually being made |
| Fix | Accuracy halo drawn at its **true** radius, `Wi-Fi fix · ±38 m` chip, recentre control |
| Access off | Card over the map: what chatot asks for and when, `Open Settings` + `Pick on map` |
| Pick on map | Search field (place → address → distance), click-to-drop pin, live coordinate readout |

Below the map: optional name, the place/coordinate readout with Copy, a
`Share as live location` switch that reveals 15 minutes / 1 hour / 8 hours, and
one primary action whose label follows the switch.

Design consequences worth keeping:

- **A desktop fix is coarse.** GeoClue on a laptop is Wi-Fi/IP trilateration,
  tens of metres at best. The halo is drawn from the real accuracy value and
  the copy says "click the map to place it exactly", so refining by hand is the
  expected path, not an error path.
- **Access off is a fallback, not a dead end.** The manual picker is a peer tab,
  always reachable, so the sheet never ends in a wall.
- **The picker never reports a coordinate it doesn't have.** With no fix and no
  pin, the readout and Copy are absent rather than showing a stale point.
- **One accent per screen.** Accent marks the positioning dot and the primary
  action; the picked pin is the same red as the location bubble's pin.

`Preferences → Privacy → Location access` gates whether the portal is called at
all, and is how the "access off" path is reached in the prototype.

## Positioning: no GNOME dependency needed

Use the **XDG desktop portal**, `org.freedesktop.portal.Location`:
`CreateSession` → `Start` → `LocationUpdated` (latitude, longitude, accuracy,
heading, speed, timestamp). It is plain D-Bus, so `gio.DBusConnection` from the
gotk4 stack already in `internal/` is enough — no libgeoclue, no GNOME Maps
dependency, and it is the API that works both inside and outside Flatpak. The
portal prompts the user itself, which is the permission UI this design assumes.
Fall back to `org.freedesktop.GeoClue2.Manager` directly when no portal is
running. Start the session when the sheet opens, stop it when it closes; a live
location share keeps it open until it expires or is stopped.

## Map: raster OSM tiles, not libshumate

- **libshumate** is the GTK4 map widget behind GNOME Maps and would be the
  richest option, but it is a new C dependency plus a gotk4 binding to generate.
- **Recommended:** fetch OpenStreetMap raster tiles over HTTP and compose them
  in a `GtkDrawingArea`. The tile ↔ WGS84 maths is ~30 lines — the mockup
  implements exactly it (`mapPx` / `mapLL` / `mapMPP` near the top of the
  canvas), so the picker's pixel↔coordinate conversion is already specified.
  This matches the OpenStreetMap deep link `internal/ui/location_view.go`
  already opens, and keeps packaging unchanged.

  Honour the tile usage policy: identifying `User-Agent`, an on-disk tile cache,
  no bulk prefetching, and "© OpenStreetMap contributors" visible on the map —
  the mockup shows that credit in both the picker and the message bubble. Point
  at a self-hosted or commercial tile source if traffic ever justifies it.

- **Search** is Nominatim's `/search` (viewbox-bounded, `User-Agent`, ≤1 req/s,
  cached, debounced). The four places in the prototype are its real answers for
  central Porto, at their real coordinates.

## Wiring on the Go side

- `client.SendLocation` / `client.SendLiveLocation` already exist; the composer
  currently sends `devLocationLat/devLocationLon` placeholders — the
  `// TODO real location source` in `internal/ui/composer.go` is what this
  design replaces, along with the bare lat/long modal in `pickLocation`.
- Outgoing messages should carry a JPEG of the composed tile as the thumbnail;
  `locationView.Thumbnail` already renders inbound previews the same way, so the
  bubble needs no change beyond feeding it our own preview.
- Live location: keep the portal session and a 15 s update timer alive for the
  chosen duration, and expose the bubble's "Stop sharing" control.

## Assets

`map-porto-z16.png`, `map-porto-z17.png`, `map-porto-z18.png` are 700×420 crops
of OpenStreetMap raster tiles (© OpenStreetMap contributors, ODbL) sharing one
centre in central Porto, so the prototype's zoom controls and coordinate
readouts are real rather than illustrative.
