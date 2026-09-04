# chatot as an installable desktop app: the cgo GTK4/libadwaita binary,
# wrapped so it finds GStreamer plugins (H.264 clips, voice notes), the
# pixbuf loaders (SVG), ffmpeg/poppler on PATH, and the mockup's UI fonts;
# plus the .desktop entry, AppStream metadata and icons under share/ so
# the shell, dock and notifications show chatot's own mark for app id
# com.sezdm.chatot.
{
  lib,
  buildGoModule,
  pkg-config,
  wrapGAppsHook4,
  gtk4,
  libadwaita,
  glib,
  gobject-introspection, # gotk4's gerror binds girepository at build time
  librsvg,
  adwaita-icon-theme,
  gst_all_1,
  ffmpeg,
  poppler-utils,
  xdg-utils,
  cantarell-fonts,
  jetbrains-mono,
  # Audio file (anything GStreamer or ffmpeg decodes) that becomes the
  # default notification sound, replacing the built-in chime:
  #   chatot.override { notificationSound = ./whatsapp-tone.oga; }
  # The user's own pick in Preferences still wins over it.
  notificationSound ? null,
}: let
  appID = "com.sezdm.chatot";
  version = "0.3.0-beta";
in
  buildGoModule {
    pname = "chatot";
    inherit version;

    # Only what the build reads: the Go tree and the embedded assets. The
    # mockups, docs and capture scratch stay out of the store path.
    src = lib.fileset.toSource {
      root = ../.;
      fileset = lib.fileset.unions [
        ../go.mod
        ../go.sum
        ../cmd
        ../internal
        ../data
      ];
    };

    vendorHash = "sha256-TwfaQR59YWah3LOpwBGor0Sp2yeCppE5/1khtZij9gI=";

    subPackages = ["cmd/chatot"];

    # go-sqlite3 with FTS5 for message search; the fts5 amalgamation trips
    # -Werror=missing-braces under gcc.
    tags = ["sqlite_fts5"];
    env.CGO_CFLAGS = "-Wno-error=missing-braces";

    nativeBuildInputs = [
      pkg-config
      wrapGAppsHook4
    ];

    buildInputs = [
      gtk4
      libadwaita
      glib
      gobject-introspection
      librsvg # gdk-pixbuf's SVG loader
      gst_all_1.gstreamer
      gst_all_1.gst-plugins-base
      gst_all_1.gst-plugins-good
      gst_all_1.gst-plugins-bad
      gst_all_1.gst-libav
    ];

    # The tests need a display for the GTK parts; they run in the devenv.
    doCheck = false;

    # wrapGAppsHook4 already wires GST_PLUGIN_SYSTEM_PATH_1_0, the pixbuf
    # loaders and the GSettings schemas. On top: the tools the app shells
    # out to; the fonts the design specifies and the icon theme behind the
    # UI's -symbolic names, both via XDG_DATA_DIRS (fontconfig reads its
    # fonts/, GTK its icons/), since the hook only adds schema dirs; and no
    # self-installed desktop entry — this package ships its own.
    preFixup = ''
      gappsWrapperArgs+=(
        --prefix PATH : ${lib.makeBinPath [ffmpeg poppler-utils xdg-utils]}
        --prefix XDG_DATA_DIRS : ${cantarell-fonts}/share:${jetbrains-mono}/share:${adwaita-icon-theme}/share
        --set CHATOT_NO_DESKTOP_ENTRY 1
        ${lib.optionalString (notificationSound != null) "--set CHATOT_NOTIFY_SOUND ${notificationSound}"}
      )
    '';

    # The same desktop entry and AppStream metadata the Flatpak ships
    # (data/), plus the icons from the UI assets.
    postInstall = ''
      install -Dm644 internal/ui/assets/chatot-icon.svg \
        $out/share/icons/hicolor/scalable/apps/${appID}.svg
      install -Dm644 internal/ui/assets/chatot-icon-512.png \
        $out/share/icons/hicolor/512x512/apps/${appID}.png
      install -Dm644 data/${appID}.desktop -t $out/share/applications
      install -Dm644 data/${appID}.metainfo.xml -t $out/share/metainfo
    '';

    meta = {
      description = "A native WhatsApp client for GNOME (GTK4 + libadwaita, whatsmeow)";
      homepage = "https://github.com/sezdm/chatot";
      license = lib.licenses.mit;
      mainProgram = "chatot";
      platforms = lib.platforms.linux;
    };
  }
