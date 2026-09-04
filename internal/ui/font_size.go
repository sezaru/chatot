package ui

import "github.com/diamondburned/gotk4/pkg/gtk/v4"

// fontSizeClasses maps a settings.FontSize value to the CSS class the main
// window carries; "default" carries none.
var fontSizeClasses = map[string]string{
	"small": "chatot-font-small",
	"large": "chatot-font-large",
}

// fontSizeLabel is the display name for a settings.FontSize value.
func fontSizeLabel(size string) string {
	switch size {
	case "small":
		return "Small"
	case "large":
		return "Large"
	}
	return "Default"
}

// ApplyFontSize sets the main window's text scale class so the chat list
// and message text follow the Font size preference.
func ApplyFontSize(win gtk.Widgetter, size string) {
	w := gtk.BaseWidget(win)
	for _, class := range fontSizeClasses {
		w.RemoveCSSClass(class)
	}
	if class, ok := fontSizeClasses[size]; ok {
		w.AddCSSClass(class)
	}
}
