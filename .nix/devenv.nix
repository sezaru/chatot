{
  pkgs,
  config,
  inputs,
  ...
}: let
  state = config.env.DEVENV_STATE;

  # The npm @playwright/mcp bundles its own playwright-core whose browser
  # registry expects a "chrome-for-testing" entry, which it can't resolve
  # against the nix-provided $PLAYWRIGHT_BROWSERS_PATH (dir is named
  # chromium-<rev>). Point it straight at the nix chromium binary so it
  # skips registry resolution entirely (mirrors nix-devkit's flutter module).
  playwright-mcp = pkgs.writeShellScriptBin "playwright-mcp" ''
    exec npx @playwright/mcp --executable-path ${pkgs.chromium}/bin/chromium "$@"
  '';
in {
  imports = [
    inputs.devkit.devenvModule
  ];

  # Claude Code CLI + agent-acp (pins CLAUDE_CONFIG_DIR under DEVENV_STATE).
  modules.claude.enable = true;

  # nodejs — the playwright-mcp wrapper's `npx @playwright/mcp` needs it
  # (modules.claude only forces this on when modules.claude.hexdocs.enable is
  # set, which it isn't here).
  modules.node.enable = true;

  # `tidewave` CLI — lets a running app session be inspected/driven directly,
  # an alternative to chromium+playwright for visual mockup confirmation.
  modules.tidewave.enable = true;

  # Open Design daemon + web UI (`od` CLI) — used to customize/update the
  # HTML mockups under ./mockup.
  modules.open-design.enable = true;

  packages = [
    # cgo GTK stack (mirrors the old plain-flake buildInputs).
    pkgs.pkg-config
    pkgs.gcc
    pkgs.go
    pkgs.gtk4
    pkgs.libadwaita
    pkgs.glib
    pkgs.gobject-introspection
    pkgs.gst_all_1.gstreamer
    pkgs.gst_all_1.gst-plugins-base
    pkgs.gst_all_1.gst-plugins-good
    pkgs.ffmpeg
    pkgs.qrencode

    # Browser automation — render the HTML mockup under ./mockup to
    # pixel-exact reference shots. Browsers come from the store, never a
    # runtime download.
    pkgs.playwright-driver.browsers
    pkgs.chromium
    playwright-mcp

    # Drive + capture the real GTK window: grim screenshots (incl. popover
    # surfaces via a fullscreen grab), wtype/ydotool inject keyboard/mouse,
    # slurp picks regions, imagemagick diffs shots vs the mockup.
    pkgs.grim
    pkgs.slurp
    pkgs.wtype
    pkgs.ydotool
    pkgs.wl-clipboard
    pkgs.imagemagick
  ];

  env = {
    # go-sqlite3 + whatsmeow need cgo; the fts5 tag trips missing-braces.
    CGO_ENABLED = "1";
    CGO_CFLAGS = "-Wno-error=missing-braces";
    GOFLAGS = "-tags=sqlite_fts5";

    # Keep the go module/build caches out of $HOME.
    GOPATH = "${state}/go";
    GOCACHE = "${state}/go/build-cache";
    GOMODCACHE = "${state}/go/mod-cache";

    # Keep browser/tool junk out of $HOME. XDG_STATE_HOME is DELIBERATELY not
    # redirected: chatot resolves its real WhatsApp DB from it, so isolating it
    # would make the app-under-test open an empty store (CHATOT_OFFLINE would
    # then have nothing to copy).
    XDG_CACHE_HOME = "${state}/cache";
    XDG_CONFIG_HOME = "${state}/config";
    XDG_DATA_HOME = "${state}/data";

    PLAYWRIGHT_BROWSERS_PATH = "${pkgs.playwright-driver.browsers}";
    PLAYWRIGHT_SKIP_BROWSER_DOWNLOAD = "1";
  };

  # devkit's open-design module parks OD_DATA_DIR in $DEVENV_STATE, which is
  # inside the repo — and od refuses to register a project location that
  # contains its own data dir ("project location cannot overlap daemon data").
  # ./mockup has to be registrable, so the daemon's state lives outside the tree.
  enterShell = ''
    export OD_DATA_DIR="$HOME/.local/state/open-design/chatot"
  '';
}
