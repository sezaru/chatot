package ui

import (
	_ "embed"
	"log"

	"github.com/diamondburned/gotk4-adwaita/pkg/adw"
	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// appMarkSVG is the design's app icon (the 2c mark: the white chatot with
// its amber beak over the brand green), drawn as a square so the corners
// can be rounded per surface. It is what gets installed for the desktop
// shell, which renders SVG itself.
//
//go:embed assets/chatot-icon.svg
var appMarkSVG []byte

// appMarkPNG is the same mark rasterised at 512px. The app draws this one:
// decoding the SVG goes through gdk-pixbuf's librsvg loader, which is not
// part of every runtime (it is missing here), and a failed decode used to
// leave the 🐦 glyph standing in for the logo.
//
//go:embed assets/chatot-icon-512.png
var appMarkPNG []byte

// AppMarkSVG is the app icon's SVG source, for installing it where the
// desktop looks for it (see cmd/chatot's desktop entry).
func AppMarkSVG() []byte { return appMarkSVG }

// AppMarkPNG is the app icon rasterised at 512px, installed beside the SVG
// for icon lookups that cannot render one.
func AppMarkPNG() []byte { return appMarkPNG }

// newTrayMark renders the tray variant of the mark (the chatot alone, no
// green tile) at size px, following the theme: grey body on light, white
// on dark. The ⋮ menu's About row uses it as its glyph.
func newTrayMark(size int) gtk.Widgetter {
	light, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(trayIconLightPNG))
	if err != nil {
		log.Printf("chatot: decode tray icon: %v", err)
		return gtk.NewLabel("🐦")
	}
	dark, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(trayIconDarkPNG))
	if err != nil {
		log.Printf("chatot: decode tray icon: %v", err)
		return gtk.NewLabel("🐦")
	}
	pick := func() gdk.Paintabler {
		if isDark() {
			return dark
		}
		return light
	}
	img := gtk.NewImageFromPaintable(pick())
	img.SetPixelSize(size)
	img.SetSizeRequest(size, size)
	img.SetHAlign(gtk.AlignCenter)
	img.SetVAlign(gtk.AlignCenter)
	// Menus are built once and outlive theme flips.
	adw.StyleManagerGetDefault().NotifyProperty("dark", func() { img.SetFromPaintable(pick()) })
	return img
}

// newAppMark renders the app icon at size px with the mockup's 20px-radius
// corners. It falls back to the 🐦 glyph if the image cannot be decoded, so
// a broken loader never blanks the About card.
func newAppMark(size int) gtk.Widgetter {
	return newAppMarkStyled(size, "chatot-about-mark")
}

// newAppMarkStyled is newAppMark with the CSS class that sets the corner
// radius for its surface.
func newAppMarkStyled(size int, class string) gtk.Widgetter {
	texture, err := gdk.NewTextureFromBytes(glib.NewBytesWithGo(appMarkPNG))
	if err != nil {
		log.Printf("chatot: decode app icon: %v", err)
		mark := gtk.NewLabel("🐦")
		mark.AddCSSClass(class)
		mark.SetSizeRequest(size, size)
		return mark
	}
	// A GtkImage, not a GtkPicture: a picture asks for its texture's height
	// at the card's width and stretched the About dialog to fit.
	img := gtk.NewImageFromPaintable(texture)
	img.SetPixelSize(size)
	img.SetSizeRequest(size, size)
	img.SetHAlign(gtk.AlignCenter)
	img.SetVAlign(gtk.AlignCenter)
	// Rounded corners come from CSS; the image has to clip to them.
	img.SetOverflow(gtk.OverflowHidden)
	img.AddCSSClass(class)
	return img
}
