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

## Development

`direnv allow` (or `nix develop`) enters the devenv shell with the cgo GTK
stack, Go and the mockup/capture tooling; `go run ./cmd/chatot` starts the
app against your real account, `CHATOT_FAKE=1` against canned data. See
`CLAUDE.md` for the mockup-driven workflow and `docs/mockup-parity-plan.md`
for what has been built.
