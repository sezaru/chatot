package ui

import (
	"fmt"

	"github.com/diamondburned/gotk4/pkg/cairo"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// tabDef is one of the sidebar's bottom tabs: its id, label, tooltip and
// the two stroked SVG paths (plus an optional dash pattern on the first)
// that draw its icon. Paths are the mockup's NAVICON set verbatim, so the
// glyphs are the design's rather than an icon theme's approximation.
type tabDef struct {
	ID, Label, Title string
	D1, D2, Dash     string
}

// sidebarTabs is the bottom bar in the mockup's order.
var sidebarTabs = []tabDef{
	{ID: "chats", Label: "Chats", Title: "Chats",
		D1: "M4 11.8C4 7.5 7.6 4 12 4s8 3.5 8 7.8-3.6 7.8-8 7.8c-1.2 0-2.4-.2-3.5-.7L4.2 20l1.1-3.4A7.6 7.6 0 0 1 4 11.8Z"},
	{ID: "status", Label: "Status", Title: "Status updates · 24h",
		D1: "M12 3.6a8.4 8.4 0 1 1 0 16.8 8.4 8.4 0 0 1 0-16.8Z", D2: "M12 9.4a2.6 2.6 0 1 1 0 5.2 2.6 2.6 0 0 1 0-5.2Z", Dash: "14.6 3.1"},
	{ID: "channels", Label: "Channels", Title: "Channels you follow",
		D1: "M4 10v4a1.6 1.6 0 0 0 1.6 1.6h2.1l5.8 3.9V4.5L7.7 8.4H5.6A1.6 1.6 0 0 0 4 10Z", D2: "M16.6 9.3a3.9 3.9 0 0 1 0 5.4M19.2 6.9a7.4 7.4 0 0 1 0 10.2"},
	{ID: "communities", Label: "Communities", Title: "Communities",
		D1: "M3.6 19.6v-1a3.5 3.5 0 0 1 3.5-3.5h2.8a3.5 3.5 0 0 1 3.5 3.5v1M8.5 6.3a3 3 0 1 1 0 6 3 3 0 0 1 0-6Z", D2: "M16.8 19.6v-1a4.4 4.4 0 0 0-1.4-3.2M15.3 6.6a3 3 0 0 1 0 5.4"},
}

// Tab icon colours: the mockup's dim text (55% black on the white bar) and
// its accent-dark for the active tab.
const (
	tabIconDim     = "#737373"
	tabIconDimDark = "#9a9a9a"
	tabIconActive  = "#147a63"
	// tabIconPx is the icon's on-screen size; tabIconScale is what the SVG
	// rendering (tabIconSVG, kept for tests and tooling) is rasterised at.
	tabIconPx    = 21
	tabIconScale = 4
)

// tabIconSVG renders one tab icon as an SVG document in the given stroke
// colour and width.
func tabIconSVG(d tabDef, color string, strokeWidth float64) []byte {
	dash := ""
	if d.Dash != "" {
		dash = fmt.Sprintf(` stroke-dasharray="%s"`, d.Dash)
	}
	px := tabIconPx * tabIconScale
	return []byte(fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="%d" height="%d" viewBox="0 0 24 24" fill="none" stroke="%s" stroke-width="%g" stroke-linecap="round" stroke-linejoin="round"><path d="%s"%s/><path d="%s"/></svg>`,
		px, px, color, strokeWidth, d.D1, dash, d.D2))
}

// tabBadgeText is the count bubble's label: nothing for zero, capped at 99+.
func tabBadgeText(n int) string {
	switch {
	case n <= 0:
		return ""
	case n > 99:
		return "99+"
	default:
		return fmt.Sprint(n)
	}
}

// tabBar is the mockup's bottom navigation: four equal tabs across the whole
// sidebar (rail included), each an icon over a label with an unread bubble,
// the active one marked by a short accent bar at the top.
type tabBar struct {
	*gtk.Box

	active   string
	onSelect func(id string)

	buttons map[string]*gtk.Button
	// icons are cairo-stroked from the mockup's paths (drawTabIcon) rather
	// than decoded SVG textures: a runtime without a gdk-pixbuf SVG loader
	// showed a bar of bare labels.
	icons  map[string]*gtk.DrawingArea
	labels map[string]*gtk.Label
	badges map[string]*gtk.Label
	bars   map[string]*gtk.Box
}

// newTabBar builds the bar with "chats" active. onSelect fires on a click
// on any tab, the active one included (the mockup re-homes the tab then).
func newTabBar(onSelect func(id string)) *tabBar {
	root := gtk.NewBox(gtk.OrientationHorizontal, 0)
	root.AddCSSClass("chatot-tab-bar")
	root.SetHomogeneous(true)
	t := &tabBar{
		Box: root, onSelect: onSelect,
		buttons: map[string]*gtk.Button{}, icons: map[string]*gtk.DrawingArea{}, labels: map[string]*gtk.Label{},
		badges: map[string]*gtk.Label{}, bars: map[string]*gtk.Box{},
	}
	for _, d := range sidebarTabs {
		root.Append(t.buildTab(d))
	}
	t.SetActive("chats")
	return t
}

func (t *tabBar) buildTab(d tabDef) *gtk.Button {
	id := d.ID

	icon := gtk.NewDrawingArea()
	icon.SetSizeRequest(22, 22)
	icon.SetHAlign(gtk.AlignCenter)
	icon.SetVAlign(gtk.AlignCenter)
	icon.SetDrawFunc(func(_ *gtk.DrawingArea, cr *cairo.Context, w, h int) {
		drawTabIcon(cr, d, float64(w), float64(h), t.active == id)
	})
	t.icons[id] = icon

	// The badge overhangs the icon's top-right corner, as in the mockup.
	badge := gtk.NewLabel("")
	badge.AddCSSClass("chatot-tab-badge")
	badge.SetHAlign(gtk.AlignEnd)
	badge.SetVAlign(gtk.AlignStart)
	badge.SetVisible(false)
	t.badges[id] = badge

	iconBox := gtk.NewOverlay()
	iconBox.SetChild(icon)
	iconBox.AddOverlay(badge)
	iconBox.SetHAlign(gtk.AlignCenter)

	label := gtk.NewLabel(d.Label)
	label.AddCSSClass("chatot-tab-label")
	t.labels[id] = label

	col := gtk.NewBox(gtk.OrientationVertical, 3)
	col.SetVAlign(gtk.AlignCenter)
	col.Append(iconBox)
	col.Append(label)

	bar := gtk.NewBox(gtk.OrientationHorizontal, 0)
	bar.AddCSSClass("chatot-tab-mark")
	bar.SetSizeRequest(26, 2)
	bar.SetHAlign(gtk.AlignCenter)
	bar.SetVAlign(gtk.AlignStart)
	t.bars[id] = bar

	body := gtk.NewOverlay()
	body.SetChild(col)
	body.AddOverlay(bar)

	btn := gtk.NewButton()
	btn.SetChild(body)
	btn.AddCSSClass("flat")
	btn.AddCSSClass("chatot-tab-btn")
	btn.SetHExpand(true)
	btn.SetSizeRequest(-1, 50)
	btn.SetTooltipText(d.Title)
	btn.ConnectClicked(func() {
		t.SetActive(id)
		if t.onSelect != nil {
			t.onSelect(id)
		}
	})
	t.buttons[id] = btn
	return btn
}

// drawTabIcon strokes d's paths with cairo, centred at tabIconPx inside
// w×h: the mockup's 24-unit viewBox scaled down, dim 1.6px strokes or the
// accent's 2px when active, the status ring dashed as in the design.
func drawTabIcon(cr *cairo.Context, d tabDef, w, h float64, active bool) {
	color, width := tabIconDim, 1.6
	if isDark() {
		color = tabIconDimDark
	}
	if active {
		color, width = tabIconActive, 2
	}
	r, g, b := hexRGB(color)
	cr.SetSourceRGB(r, g, b)
	cr.SetLineCap(cairo.LineCapRound)
	cr.SetLineJoin(cairo.LineJoinRound)

	cr.Save()
	cr.Translate((w-tabIconPx)/2, (h-tabIconPx)/2)
	scale := float64(tabIconPx) / 24
	cr.Scale(scale, scale)
	// Line width is set after the scale so it is in viewBox units, exactly
	// as the SVG's stroke-width would be.
	cr.SetLineWidth(width)
	if dash := parseDashArray(d.Dash); dash != nil {
		cr.SetDash(dash, 0)
	}
	if err := strokeSVGPath(cr, d.D1); err == nil {
		cr.Stroke()
	}
	cr.SetDash(nil, 0)
	if d.D2 != "" {
		if err := strokeSVGPath(cr, d.D2); err == nil {
			cr.Stroke()
		}
	}
	cr.Restore()
}

// SetActive marks id as the current tab without firing onSelect.
func (t *tabBar) SetActive(id string) {
	t.active = id
	for tid, btn := range t.buttons {
		on := tid == id
		if on {
			btn.AddCSSClass("chatot-tab-on")
		} else {
			btn.RemoveCSSClass("chatot-tab-on")
		}
		t.bars[tid].SetVisible(on)
		t.icons[tid].QueueDraw()
	}
}

// Active returns the current tab id.
func (t *tabBar) Active() string { return t.active }

// SetBadge shows n on id's bubble, or hides it for zero.
func (t *tabBar) SetBadge(id string, n int) {
	badge, ok := t.badges[id]
	if !ok {
		return
	}
	text := tabBadgeText(n)
	badge.SetText(text)
	badge.SetVisible(text != "")
}
