package ui

import (
	"context"
	"errors"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gdk/v4"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/geo"
)

// locationResult is what the Send location sheet hands back on Send.
type locationResult struct {
	Lat, Lon     float64
	Name         string // the typed name, else the picked place's name, else ""
	Address      string // the picked place's address, else ""
	Live         bool
	DurationSecs int
	// Thumbnail is a small JPEG of the map around the point, for the bubble
	// (ours and the recipient's). nil if the tiles couldn't be fetched.
	Thumbnail []byte
}

// locMode is which tab the sheet is on.
type locMode int

const (
	locGPS    locMode = iota // "Current location": the system's fix
	locManual                // "Pick on map": a pin placed by hand
)

// locStatus is the positioning state on the GPS tab.
type locStatus int

const (
	locLocating locStatus = iota
	locFix
	locDenied
)

// liveDurations are the live-share choices, in the mockup's order.
var liveDurations = []struct {
	Label   string
	Seconds int
}{
	{"15 minutes", 15 * 60},
	{"1 hour", 60 * 60},
	{"8 hours", 8 * 60 * 60},
}

// locStageW/H is the sheet's map stage, per the mockup (600px wide sheet).
const (
	locStageW = 600
	locStageH = 264
)

// locThumbW/H is the map preview a sent location carries: the bubble's
// tile size, at a zoom that shows the surrounding streets.
const (
	locThumbW    = 270
	locThumbH    = 86
	locThumbZoom = 16
)

// sharedSearcher is the process-wide Nominatim client (rate limit and
// cache are per process, as the usage policy asks).
var sharedSearcher = geo.NewSearcher()

// locPicker is the sheet's state and widgets.
type locPicker struct {
	dialog *cardDialog
	parent *gtk.Window
	onSend func(locationResult)

	mode   locMode
	status locStatus
	// fix is the system's position; pin the hand-placed point.
	fixLat, fixLon, fixAcc float64
	haveFix                bool
	pinned                 bool
	pinLat, pinLon         float64
	place                  *geo.Place
	live                   bool
	session                *geo.Session
	deniedNote             string
	searchGen              int
	searchTimer            glib.SourceHandle

	// widgets
	modeStack   *gtk.Stack
	mapView     *mapView
	fixChip     *gtk.Box
	fixChipText *gtk.Label
	searchBox   *gtk.Box
	searchEntry *gtk.Entry
	searchClear *gtk.Button
	results     *gtk.Box
	locating    *gtk.Box
	denied      *gtk.Box
	zoomIn      *gtk.Button
	zoomOut     *gtk.Button
	nameEntry   *gtk.Entry
	placeLabel  *gtk.Label
	coordsLabel *gtk.Label
	copyBtn     *gtk.Button
	liveSwitch  *gtk.Switch
	liveSub     *gtk.Label
	durStack    *gtk.Stack
	durPill     gtk.Widgetter
	sourceNote  *gtk.Label
	sendBtn     *gtk.Button
}

// showLocationPicker opens the mockup's Send location sheet over parent.
// allowSystem is the Preferences → Privacy → Location access switch: off,
// the "Current location" tab shows the access-off card and only the map
// picker works. onSend receives the point (and live-share choice) when the
// user confirms; the sheet closes itself.
func showLocationPicker(parent *gtk.Window, allowSystem bool, onSend func(locationResult)) *locPicker {
	p := &locPicker{parent: parent, onSend: onSend, mode: locGPS, status: locLocating}
	p.dialog = newCardDialog()
	p.dialog.SetTitle("Send location")
	p.dialog.SetTransientFor(parent)
	p.dialog.SetModal(true)
	p.dialog.SetDefaultSize(locStageW, -1)

	root := gtk.NewBox(gtk.OrientationVertical, 0)
	root.Append(p.buildTabs())
	root.Append(p.buildStage())
	root.Append(p.buildForm())
	root.Append(p.buildFooter())
	p.dialog.SetChild(root)
	p.dialog.ConnectClosed(func() {
		p.stopSession()
		if p.searchTimer != 0 {
			glib.SourceRemove(p.searchTimer)
			p.searchTimer = 0
		}
	})

	if !allowSystem {
		p.status = locDenied
		p.deniedNote = "Location access off"
	}
	p.render()
	p.dialog.Present()
	if allowSystem {
		p.startLocating()
	}
	return p
}

// buildTabs is the Current location | Pick on map pill.
func (p *locPicker) buildTabs() gtk.Widgetter {
	p.modeStack = gtk.NewStack()
	p.modeStack.AddNamed(gtk.NewBox(gtk.OrientationVertical, 0), "gps")
	p.modeStack.AddNamed(gtk.NewBox(gtk.OrientationVertical, 0), "manual")
	p.modeStack.SetVisible(false)
	pill := newSegmentedSwitcher(p.modeStack, []segmentedPage{{"gps", "Current location"}, {"manual", "Pick on map"}}, true)
	p.modeStack.NotifyProperty("visible-child-name", func() {
		if p.modeStack.VisibleChildName() == "manual" {
			p.setMode(locManual)
		} else {
			p.setMode(locGPS)
		}
	})
	wrap := gtk.NewBox(gtk.OrientationVertical, 0)
	wrap.AddCSSClass("chatot-loc-tabs")
	wrap.Append(pill)
	wrap.Append(p.modeStack)
	return wrap
}

// buildStage is the map with its overlays: the fix chip, the search field
// and results, the locating and access-off states, the zoom column and
// the attribution.
func (p *locPicker) buildStage() gtk.Widgetter {
	stage := gtk.NewOverlay()
	stage.AddCSSClass("chatot-loc-stage")
	stage.SetSizeRequest(locStageW, locStageH)

	p.mapView = newMapView(sharedTiles)
	p.mapView.SetSizeRequest(locStageW, locStageH)
	// Before any fix or pin the map shows a wide view the user can zoom
	// into; the first fix recentres it.
	p.mapView.SetZoom(3)
	p.mapView.SetCentre(20, 0)
	p.mapView.onClick = p.dropPin
	p.mapView.onViewportChanged = p.renderZoom
	stage.SetChild(p.mapView)

	// Fix chip (top-left).
	p.fixChip = gtk.NewBox(gtk.OrientationHorizontal, 7)
	p.fixChip.AddCSSClass("chatot-loc-chip")
	p.fixChip.SetHAlign(gtk.AlignStart)
	p.fixChip.SetVAlign(gtk.AlignStart)
	p.fixChip.SetMarginStart(12)
	p.fixChip.SetMarginTop(12)
	dot := gtk.NewBox(gtk.OrientationVertical, 0)
	dot.AddCSSClass("chatot-loc-chip-dot")
	dot.SetSizeRequest(7, 7)
	dot.SetVAlign(gtk.AlignCenter)
	p.fixChip.Append(dot)
	p.fixChipText = gtk.NewLabel("")
	p.fixChip.Append(p.fixChipText)
	stage.AddOverlay(p.fixChip)

	// Search (top, manual tab).
	p.searchBox = gtk.NewBox(gtk.OrientationVertical, 6)
	p.searchBox.SetHAlign(gtk.AlignFill)
	p.searchBox.SetVAlign(gtk.AlignStart)
	p.searchBox.SetMarginStart(12)
	p.searchBox.SetMarginEnd(56)
	p.searchBox.SetMarginTop(12)
	field := gtk.NewBox(gtk.OrientationHorizontal, 9)
	field.AddCSSClass("chatot-loc-search")
	glass := gtk.NewLabel("🔍")
	glass.AddCSSClass("chatot-loc-search-glyph")
	field.Append(glass)
	p.searchEntry = gtk.NewEntry()
	p.searchEntry.SetPlaceholderText("Search a place or address")
	p.searchEntry.AddCSSClass("chatot-loc-search-entry")
	p.searchEntry.SetHExpand(true)
	p.searchEntry.ConnectChanged(p.queueSearch)
	field.Append(p.searchEntry)
	p.searchClear = gtk.NewButtonWithLabel("✕")
	p.searchClear.AddCSSClass("flat")
	p.searchClear.AddCSSClass("chatot-loc-search-clear")
	p.searchClear.ConnectClicked(func() { p.searchEntry.SetText("") })
	field.Append(p.searchClear)
	p.searchBox.Append(field)
	p.results = gtk.NewBox(gtk.OrientationVertical, 0)
	p.results.AddCSSClass("chatot-loc-results")
	p.results.SetVisible(false)
	p.searchBox.Append(p.results)
	stage.AddOverlay(p.searchBox)

	// Locating scrim.
	p.locating = gtk.NewBox(gtk.OrientationVertical, 11)
	p.locating.AddCSSClass("chatot-loc-scrim")
	p.locating.SetHAlign(gtk.AlignFill)
	p.locating.SetVAlign(gtk.AlignFill)
	inner := gtk.NewBox(gtk.OrientationVertical, 11)
	inner.SetHAlign(gtk.AlignCenter)
	inner.SetVAlign(gtk.AlignCenter)
	inner.SetVExpand(true)
	spinner := gtk.NewSpinner()
	spinner.SetSizeRequest(28, 28)
	spinner.SetSpinning(true)
	inner.Append(spinner)
	finding := gtk.NewLabel("Finding your location…")
	finding.AddCSSClass("chatot-loc-scrim-title")
	inner.Append(finding)
	via := gtk.NewLabel("org.freedesktop.portal.Location")
	via.AddCSSClass("chatot-loc-mono")
	inner.Append(via)
	p.locating.Append(inner)
	stage.AddOverlay(p.locating)

	// Access-off card.
	p.denied = gtk.NewBox(gtk.OrientationVertical, 0)
	p.denied.AddCSSClass("chatot-loc-scrim")
	p.denied.SetHAlign(gtk.AlignFill)
	p.denied.SetVAlign(gtk.AlignFill)
	card := gtk.NewBox(gtk.OrientationVertical, 9)
	card.AddCSSClass("chatot-loc-denied")
	card.SetHAlign(gtk.AlignCenter)
	card.SetVAlign(gtk.AlignCenter)
	card.SetVExpand(true)
	card.SetSizeRequest(372, -1)
	pinDisc := gtk.NewLabel("📍")
	pinDisc.AddCSSClass("chatot-loc-denied-disc")
	pinDisc.SetSizeRequest(36, 36)
	pinDisc.SetHAlign(gtk.AlignCenter)
	card.Append(pinDisc)
	deniedTitle := gtk.NewLabel("Location access is off")
	deniedTitle.AddCSSClass("chatot-loc-denied-title")
	card.Append(deniedTitle)
	deniedText := gtk.NewLabel("chatot asks the system for a position only while this sheet is open, and never in the background. Turn it on in Settings, or place the point yourself.")
	deniedText.AddCSSClass("chatot-loc-denied-text")
	deniedText.SetWrap(true)
	deniedText.SetJustify(gtk.JustifyCenter)
	deniedText.SetMaxWidthChars(46)
	card.Append(deniedText)
	deniedBtns := gtk.NewBox(gtk.OrientationHorizontal, 8)
	deniedBtns.SetHAlign(gtk.AlignCenter)
	deniedBtns.SetMarginTop(3)
	openSettings := gtk.NewButtonWithLabel("Open Settings")
	openSettings.AddCSSClass("chatot-chip-btn")
	openSettings.ConnectClicked(func() {
		PreferencesInitialPage = "privacy"
		if app := p.parent.Application(); app != nil {
			app.ActivateAction("preferences", nil)
		}
	})
	deniedBtns.Append(openSettings)
	pickManual := gtk.NewButtonWithLabel("Pick on map")
	pickManual.AddCSSClass("chatot-loc-outline-btn")
	pickManual.ConnectClicked(func() { p.modeStack.SetVisibleChildName("manual") })
	deniedBtns.Append(pickManual)
	card.Append(deniedBtns)
	p.denied.Append(card)
	stage.AddOverlay(p.denied)

	// Zoom column + recentre (bottom-right).
	controls := gtk.NewBox(gtk.OrientationVertical, 6)
	controls.SetHAlign(gtk.AlignEnd)
	controls.SetVAlign(gtk.AlignEnd)
	controls.SetMarginEnd(12)
	controls.SetMarginBottom(12)
	zoomCol := gtk.NewBox(gtk.OrientationVertical, 0)
	zoomCol.AddCSSClass("chatot-loc-zoom")
	p.zoomIn = gtk.NewButtonWithLabel("＋")
	p.zoomIn.AddCSSClass("flat")
	p.zoomIn.AddCSSClass("chatot-loc-zoom-btn")
	p.zoomIn.ConnectClicked(func() { p.mapView.ZoomBy(1) })
	zoomCol.Append(p.zoomIn)
	zoomCol.Append(gtk.NewSeparator(gtk.OrientationHorizontal))
	p.zoomOut = gtk.NewButtonWithLabel("−")
	p.zoomOut.AddCSSClass("flat")
	p.zoomOut.AddCSSClass("chatot-loc-zoom-btn")
	p.zoomOut.ConnectClicked(func() { p.mapView.ZoomBy(-1) })
	zoomCol.Append(p.zoomOut)
	controls.Append(zoomCol)
	recentre := gtk.NewButtonWithLabel("⌖")
	recentre.AddCSSClass("chatot-loc-recentre")
	recentre.SetTooltipText("Recentre")
	recentre.ConnectClicked(p.recentre)
	controls.Append(recentre)
	stage.AddOverlay(controls)

	attribution := gtk.NewLabel(geo.Attribution)
	attribution.AddCSSClass("chatot-loc-attribution")
	attribution.SetHAlign(gtk.AlignStart)
	attribution.SetVAlign(gtk.AlignEnd)
	attribution.SetMarginStart(10)
	attribution.SetMarginBottom(9)
	attribution.SetCanTarget(false)
	stage.AddOverlay(attribution)

	return stage
}

// buildForm is the part under the map: the name entry, the place readout
// with Copy, and the live-share switch with its durations.
func (p *locPicker) buildForm() gtk.Widgetter {
	form := gtk.NewBox(gtk.OrientationVertical, 10)
	form.AddCSSClass("chatot-loc-form")

	p.nameEntry = gtk.NewEntry()
	p.nameEntry.SetPlaceholderText("Add a name (optional)")
	p.nameEntry.AddCSSClass("chatot-dialog-entry")
	form.Append(p.nameEntry)

	placeRow := gtk.NewBox(gtk.OrientationHorizontal, 10)
	placeCol := gtk.NewBox(gtk.OrientationVertical, 3)
	placeCol.SetHExpand(true)
	p.placeLabel = gtk.NewLabel("")
	p.placeLabel.SetXAlign(0)
	p.placeLabel.SetEllipsize(3) // pango.EllipsizeEnd
	p.placeLabel.AddCSSClass("chatot-loc-place")
	placeCol.Append(p.placeLabel)
	p.coordsLabel = gtk.NewLabel("")
	p.coordsLabel.SetXAlign(0)
	p.coordsLabel.AddCSSClass("chatot-loc-coords")
	placeCol.Append(p.coordsLabel)
	placeRow.Append(placeCol)
	p.copyBtn = gtk.NewButtonWithLabel("Copy")
	p.copyBtn.AddCSSClass("chatot-chip-btn")
	p.copyBtn.AddCSSClass("chatot-loc-copy")
	p.copyBtn.SetVAlign(gtk.AlignCenter)
	p.copyBtn.ConnectClicked(func() {
		lat, lon, ok := p.point()
		if !ok {
			return
		}
		gdk.DisplayGetDefault().Clipboard().SetText(geo.FormatCoord(lat) + ", " + geo.FormatCoord(lon))
	})
	placeRow.Append(p.copyBtn)
	form.Append(placeRow)

	form.Append(gtk.NewSeparator(gtk.OrientationHorizontal))

	liveRow := gtk.NewBox(gtk.OrientationHorizontal, 12)
	liveCol := gtk.NewBox(gtk.OrientationVertical, 2)
	liveCol.SetHExpand(true)
	liveTitle := gtk.NewLabel("Share as live location")
	liveTitle.SetXAlign(0)
	liveTitle.AddCSSClass("chatot-loc-live-title")
	liveCol.Append(liveTitle)
	p.liveSub = gtk.NewLabel("")
	p.liveSub.SetXAlign(0)
	p.liveSub.SetWrap(true)
	p.liveSub.AddCSSClass("chatot-loc-live-sub")
	liveCol.Append(p.liveSub)
	liveRow.Append(liveCol)
	p.liveSwitch = gtk.NewSwitch()
	p.liveSwitch.SetVAlign(gtk.AlignCenter)
	p.liveSwitch.ConnectStateSet(func(on bool) bool {
		p.live = on
		p.render()
		return false
	})
	liveRow.Append(p.liveSwitch)
	form.Append(liveRow)

	p.durStack = gtk.NewStack()
	pages := make([]segmentedPage, len(liveDurations))
	for i, d := range liveDurations {
		p.durStack.AddNamed(gtk.NewBox(gtk.OrientationVertical, 0), d.Label)
		pages[i] = segmentedPage{d.Label, d.Label}
	}
	p.durStack.SetVisible(false)
	p.durStack.SetVisibleChildName(liveDurations[1].Label)
	p.durPill = newSegmentedSwitcher(p.durStack, pages, true)
	form.Append(p.durPill)
	form.Append(p.durStack)

	return form
}

// buildFooter is the source note plus Cancel and the Send button.
func (p *locPicker) buildFooter() gtk.Widgetter {
	footer := gtk.NewBox(gtk.OrientationHorizontal, 10)
	footer.AddCSSClass("chatot-loc-footer")
	p.sourceNote = gtk.NewLabel("")
	p.sourceNote.SetXAlign(0)
	p.sourceNote.SetHExpand(true)
	p.sourceNote.SetEllipsize(3)
	p.sourceNote.AddCSSClass("chatot-loc-source")
	footer.Append(p.sourceNote)
	cancel := gtk.NewButtonWithLabel("Cancel")
	cancel.AddCSSClass("chatot-chip-btn")
	cancel.ConnectClicked(func() { p.dialog.Close() })
	footer.Append(cancel)
	p.sendBtn = gtk.NewButtonWithLabel("Send location")
	p.sendBtn.AddCSSClass("chatot-primary-btn")
	p.sendBtn.ConnectClicked(p.send)
	footer.Append(p.sendBtn)
	return footer
}

// --- state ---

func (p *locPicker) setMode(m locMode) {
	if p.mode == m {
		return
	}
	p.mode = m
	if m == locGPS {
		p.place = nil
		p.searchEntry.SetText("")
		if p.status == locLocating {
			p.startLocating()
		}
		if p.haveFix {
			p.mapView.SetCentre(p.fixLat, p.fixLon)
		}
	}
	p.render()
}

// startLocating opens a portal session (idempotent while one is running).
func (p *locPicker) startLocating() {
	if p.session != nil || p.status == locFix {
		return
	}
	p.status = locLocating
	p.render()
	session, err := geo.Start(context.Background(),
		func(fix geo.Fix) { glib.IdleAdd(func() { p.gotFix(fix) }) },
		func(err error) { glib.IdleAdd(func() { p.locatingFailed(err) }) })
	if err != nil {
		p.locatingFailed(err)
		return
	}
	p.session = session
}

func (p *locPicker) stopSession() {
	if p.session != nil {
		p.session.Close()
		p.session = nil
	}
}

func (p *locPicker) gotFix(fix geo.Fix) {
	first := !p.haveFix
	p.haveFix = true
	p.fixLat, p.fixLon, p.fixAcc = fix.Lat, fix.Lon, fix.Accuracy
	p.status = locFix
	if first {
		p.mapView.SetZoom(17)
		p.mapView.SetCentre(fix.Lat, fix.Lon)
	}
	p.render()
}

func (p *locPicker) locatingFailed(err error) {
	p.stopSession()
	if p.haveFix {
		return // a late error after a fix changes nothing
	}
	p.status = locDenied
	switch {
	case errors.Is(err, geo.ErrDenied):
		p.deniedNote = "Location access denied"
	case errors.Is(err, geo.ErrUnavailable):
		p.deniedNote = "No positioning service"
	default:
		p.deniedNote = "Positioning failed"
		log.Printf("chatot: location: %v", err)
	}
	p.render()
}

// dropPin is a click on the map: the point moves there and the sheet is
// on the manual tab.
func (p *locPicker) dropPin(lat, lon float64) {
	if p.mode == locGPS && p.status == locLocating {
		return
	}
	p.pinned = true
	p.pinLat, p.pinLon = lat, lon
	p.place = nil
	p.searchEntry.SetText("")
	if p.mode != locManual {
		p.modeStack.SetVisibleChildName("manual")
		return // setMode renders
	}
	p.render()
}

// selectPlace is a click on a search result.
func (p *locPicker) selectPlace(place geo.Place) {
	p.pinned = true
	p.pinLat, p.pinLon = place.Lat, place.Lon
	pl := place
	p.place = &pl
	p.nameEntry.SetText(place.Name)
	p.searchEntry.SetText("")
	if p.mapView.Zoom() < 15 {
		p.mapView.SetZoom(17)
	}
	p.mapView.SetCentre(place.Lat, place.Lon)
	p.render()
}

func (p *locPicker) recentre() {
	if lat, lon, ok := p.point(); ok {
		p.mapView.SetCentre(lat, lon)
	}
}

// point is the coordinate the sheet would send, if it has one.
func (p *locPicker) point() (lat, lon float64, ok bool) {
	switch p.mode {
	case locManual:
		if p.pinned {
			return p.pinLat, p.pinLon, true
		}
	case locGPS:
		if p.status == locFix {
			return p.fixLat, p.fixLon, true
		}
	}
	return 0, 0, false
}

// queueSearch debounces the search field (and respects Nominatim's rate).
func (p *locPicker) queueSearch() {
	if p.searchTimer != 0 {
		glib.SourceRemove(p.searchTimer)
		p.searchTimer = 0
	}
	q := strings.TrimSpace(p.searchEntry.Text())
	p.searchClear.SetVisible(q != "")
	if q == "" {
		p.results.SetVisible(false)
		removeAllChildren(p.results)
		return
	}
	p.searchGen++
	gen := p.searchGen
	p.searchTimer = glib.TimeoutAdd(350, func() bool {
		p.searchTimer = 0
		lat, lon := p.mapView.lat, p.mapView.lon
		go func() {
			hits, err := sharedSearcher.Search(context.Background(), q, lat, lon)
			glib.IdleAdd(func() {
				if gen != p.searchGen {
					return
				}
				if err != nil {
					log.Printf("chatot: place search: %v", err)
				}
				p.showResults(hits)
			})
		}()
		return false
	})
}

func (p *locPicker) showResults(hits []geo.Place) {
	removeAllChildren(p.results)
	p.results.SetVisible(true)
	if len(hits) == 0 {
		none := gtk.NewLabel("Nothing found nearby — click the map to drop a pin instead.")
		none.AddCSSClass("chatot-loc-result-none")
		none.SetWrap(true)
		none.SetXAlign(0)
		p.results.Append(none)
		return
	}
	fromLat, fromLon := p.mapView.lat, p.mapView.lon
	if p.haveFix {
		fromLat, fromLon = p.fixLat, p.fixLon
	}
	for i, h := range hits {
		hit := h
		row := gtk.NewBox(gtk.OrientationHorizontal, 10)
		disc := gtk.NewLabel("📍")
		disc.AddCSSClass("chatot-loc-result-disc")
		disc.SetSizeRequest(26, 26)
		disc.SetVAlign(gtk.AlignCenter)
		row.Append(disc)
		col := gtk.NewBox(gtk.OrientationVertical, 1)
		col.SetHExpand(true)
		name := gtk.NewLabel(hit.Name)
		name.SetXAlign(0)
		name.SetEllipsize(3)
		name.AddCSSClass("chatot-loc-result-name")
		col.Append(name)
		addr := gtk.NewLabel(hit.Address)
		addr.SetXAlign(0)
		addr.SetEllipsize(3)
		addr.AddCSSClass("chatot-loc-result-addr")
		col.Append(addr)
		row.Append(col)
		dist := gtk.NewLabel(geo.FormatDistance(geo.Distance(fromLat, fromLon, hit.Lat, hit.Lon)))
		dist.AddCSSClass("chatot-loc-result-dist")
		dist.SetVAlign(gtk.AlignCenter)
		row.Append(dist)
		btn := gtk.NewButton()
		btn.SetChild(row)
		btn.AddCSSClass("flat")
		btn.AddCSSClass("chatot-loc-result")
		if i > 0 {
			btn.AddCSSClass("chatot-loc-result-sep")
		}
		btn.ConnectClicked(func() { p.selectPlace(hit) })
		p.results.Append(btn)
	}
}

// render redraws every state-dependent widget from the picker's state.
func (p *locPicker) render() {
	fixState := p.mode == locGPS && p.status == locFix
	manual := p.mode == locManual
	_, _, canSend := p.point()

	p.fixChip.SetVisible(fixState)
	if fixState {
		p.fixChipText.SetText("Wi-Fi fix · ±" + itoa(int(p.fixAcc+0.5)) + " m")
	}
	p.searchBox.SetVisible(manual)
	p.locating.SetVisible(p.mode == locGPS && p.status == locLocating)
	p.denied.SetVisible(p.mode == locGPS && p.status == locDenied)
	p.mapView.SetInteractive(!(p.mode == locGPS && p.status != locFix))

	switch {
	case manual && p.pinned:
		p.mapView.SetMarker(markerPin, p.pinLat, p.pinLon, 0)
	case fixState:
		p.mapView.SetMarker(markerFix, p.fixLat, p.fixLon, p.fixAcc)
	default:
		p.mapView.SetMarker(markerNone, 0, 0, 0)
	}

	switch {
	case p.place != nil:
		p.placeLabel.SetText(p.place.Address)
	case fixState:
		p.placeLabel.SetText("Approximate position — click the map to place it exactly")
	case !manual:
		p.placeLabel.SetText("No position yet")
	case p.pinned:
		p.placeLabel.SetText("Dropped pin")
	default:
		p.placeLabel.SetText("Click the map to move the pin")
	}
	p.coordsLabel.SetVisible(canSend)
	p.copyBtn.SetVisible(canSend)
	if lat, lon, ok := p.point(); ok {
		coords := geo.FormatCoord(lat) + ", " + geo.FormatCoord(lon)
		if fixState {
			coords += "  ±" + itoa(int(p.fixAcc+0.5)) + " m"
		}
		p.coordsLabel.SetText(coords)
	}

	if p.live {
		p.liveSub.SetText("Recipients see you move until it expires; you can stop from the message.")
		p.sendBtn.SetLabel("Share live location")
	} else {
		p.liveSub.SetText("Sends one fixed point that never updates.")
		p.sendBtn.SetLabel("Send location")
	}
	gtk.BaseWidget(p.durPill).SetVisible(p.live)

	switch {
	case manual:
		p.sourceNote.SetText("Pin · © OpenStreetMap")
	case fixState:
		p.sourceNote.SetText("Wi-Fi fix · GeoClue")
	case p.status == locDenied:
		p.sourceNote.SetText(p.deniedNote)
	default:
		p.sourceNote.SetText("Asking the location portal…")
	}
	p.sendBtn.SetSensitive(canSend)
	p.renderZoom()
}

func (p *locPicker) renderZoom() {
	if p.zoomIn == nil {
		return
	}
	p.zoomIn.SetSensitive(p.mapView.Zoom() < mapZoomMax)
	p.zoomOut.SetSensitive(p.mapView.Zoom() > mapZoomMin)
}

// send builds the result, closes the sheet and hands the result on once
// the map preview has been composed (a few tiles, usually already cached).
func (p *locPicker) send() {
	lat, lon, ok := p.point()
	if !ok {
		return
	}
	res := locationResult{Lat: lat, Lon: lon, Live: p.live, Name: strings.TrimSpace(p.nameEntry.Text())}
	if p.place != nil {
		res.Address = p.place.Address
		if res.Name == "" {
			res.Name = p.place.Name
		}
	}
	if p.live {
		chosen := p.durStack.VisibleChildName()
		for _, d := range liveDurations {
			if d.Label == chosen {
				res.DurationSecs = d.Seconds
			}
		}
		if res.DurationSecs == 0 {
			res.DurationSecs = liveDurations[1].Seconds
		}
	}
	// The positioning session ends with the sheet either way: a live share
	// is a single message (see Composer.sendLiveLocation), not a stream.
	p.stopSession()
	onSend := p.onSend
	p.dialog.Close()
	go func() {
		// The preview is the light map: it is sent to the other side.
		thumb, err := geo.Snapshot(context.Background(), sharedTiles, lat, lon, locThumbZoom, locThumbW, locThumbH)
		if err != nil {
			log.Printf("chatot: location preview: %v", err)
		}
		res.Thumbnail = thumb
		glib.IdleAdd(func() { onSend(res) })
	}()
}

// timeUntil formats "until HH:MM" for a live share of secs from now.
func timeUntil(secs int, now time.Time) string {
	return now.Add(time.Duration(secs) * time.Second).Format("15:04")
}

func itoa(n int) string { return strconv.Itoa(n) }
