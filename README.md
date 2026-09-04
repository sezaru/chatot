# chatot

A native WhatsApp client for GNOME: GTK4 + libadwaita, talking to WhatsApp
through [whatsmeow](https://github.com/tulir/whatsmeow) in-process. Unofficial,
not affiliated with WhatsApp or Meta. Beta.

## Install (Nix)

The flake builds chatot as a desktop app: the binary is wrapped with the
GStreamer plugins, pixbuf loaders, ffmpeg/poppler and fonts it needs, and the
package ships the `.desktop` entry and icons for app id `com.sezdm.chatot`.

```sh
# try it
nix run github:sezdm/chatot

# install into your profile (adds the launcher to your app grid)
nix profile install github:sezdm/chatot

# from a checkout
nix build .#chatot && ./result/bin/chatot
```

On NixOS or Home Manager, add the flake as an input and put
`chatot.packages.${system}.chatot` in your package list.

## Flatpak

`build-aux/flatpak/com.sezdm.chatot.yml` builds chatot against the GNOME 50
runtime with the Go SDK extension, bundling only what the runtime lacks
(poppler for document previews, JetBrains Mono). The build is offline:
`build-aux/flatpak/go.mod.yml` and `modules.txt` pin every Go module. After
changing `go.mod`, regenerate them with
`go run github.com/dennwc/flatpak-go-mod@latest .` and move the two files
back into `build-aux/flatpak/`.

```sh
# build and install into your user Flatpak installation
flatpak run org.flatpak.Builder --force-clean --user --install \
  --install-deps-from=flathub --ccache \
  build-aux/flatpak/build build-aux/flatpak/com.sezdm.chatot.yml
flatpak run com.sezdm.chatot

# what Flathub checks
flatpak run --command=flatpak-builder-lint org.flatpak.Builder manifest build-aux/flatpak/com.sezdm.chatot.yml
flatpak run --command=flatpak-builder-lint org.flatpak.Builder appstream data/com.sezdm.chatot.metainfo.xml
flatpak run --command=flatpak-builder-lint org.flatpak.Builder repo build-aux/flatpak/repo
```

The Flathub submission uses this manifest with the `chatot` module's
`type: dir` source replaced by the tagged release commit
(`type: git`, `url`, `tag`, `commit`). Screenshots referenced by the AppStream
metadata live in `data/screenshots/`.

## Development

`direnv allow` (or `nix develop`) enters the devenv shell with the cgo GTK
stack, Go and the mockup/capture tooling; `go run ./cmd/chatot` starts the
app against your real account, `CHATOT_FAKE=1` against canned data. See
`CLAUDE.md` for the mockup-driven workflow and `docs/mockup-parity-plan.md`
for what has been built.

## Notification sound

Each desktop notification plays a short chime (Preferences › Notifications
turns it off). The built-in one is a generic synthesized tone. To use your
own, either pick a file under Preferences › Notifications › Sound file (MP3
included; it is transcoded before GTK plays it), or drop a file named
`notify.oga`, `notify.ogg`, `notify.opus`, `notify.flac`, `notify.wav`,
`notify.mp3` or `notify.m4a` into `$XDG_CONFIG_HOME/chatot/` (usually
`~/.config/chatot/`; for the Flatpak, `~/.var/app/com.sezdm.chatot/config/chatot/`).

A package can ship its own default sound. With Nix:

```nix
chatot.override { notificationSound = ./my-tone.oga; }
```

That sets `CHATOT_NOTIFY_SOUND` in the wrapper to the file. Precedence, first
wins: the file picked in Preferences, the drop-in in the config dir, the
packaged `CHATOT_NOTIFY_SOUND` file, the built-in chime.
