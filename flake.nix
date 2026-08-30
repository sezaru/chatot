{
  description = "chatot — GTK4 WhatsApp client (whatsmeow, in-process)";

  inputs.nixpkgs.url = "github:NixOS/nixpkgs/nixos-unstable";

  outputs = {
    self,
    nixpkgs,
  }: let
    systems = ["aarch64-linux" "x86_64-linux"];
    forAll = f: nixpkgs.lib.genAttrs systems (system: f nixpkgs.legacyPackages.${system});
  in {
    devShells = forAll (pkgs: {
      default = pkgs.mkShell {
        nativeBuildInputs = [pkgs.pkg-config pkgs.go pkgs.gcc];
        buildInputs = [
          pkgs.gtk4
          pkgs.libadwaita
          pkgs.glib
          pkgs.gobject-introspection
          pkgs.gst_all_1.gstreamer
          pkgs.gst_all_1.gst-plugins-base
          pkgs.gst_all_1.gst-plugins-good
          pkgs.ffmpeg
          pkgs.qrencode
        ];
        # go-sqlite3 + whatsmeow need cgo; go-sqlite3's fts5 tag trips missing-braces.
        CGO_ENABLED = "1";
        CGO_CFLAGS = "-Wno-error=missing-braces";
        GOFLAGS = "-tags=sqlite_fts5";
      };
    });
  };
}
