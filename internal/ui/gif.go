package ui

import (
	"context"
	"errors"
	"log"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"
)

// GIFResult is one search hit from a GIFProvider: enough to show a preview
// tile and, on pick, resolve to a sendable URL.
type GIFResult struct {
	PreviewURL string
	SendURL    string
	Width      int
	Height     int
}

// GIFProvider searches an external GIF service (Tenor/Giphy). It's a seam:
// F37 ships only unconfiguredGIFProvider; a real implementation is a later
// feature that swaps in here without touching the picker UI.
type GIFProvider interface {
	Search(ctx context.Context, query string) ([]GIFResult, error)
}

// errGIFProviderUnconfigured is unconfiguredGIFProvider's sentinel error; the
// GIF tab treats it as "not set up yet" (an empty state), not a search failure.
var errGIFProviderUnconfigured = errors.New("chatot: gif provider not configured")

// unconfiguredGIFProvider is the default GIFProvider until a real one is
// wired in — every search fails with errGIFProviderUnconfigured.
type unconfiguredGIFProvider struct{}

func (unconfiguredGIFProvider) Search(context.Context, string) ([]GIFResult, error) {
	return nil, errGIFProviderUnconfigured
}

// newGIFTab builds the GIF tab's content: a search box over a results grid,
// backed by provider. onGIFChosen fires when a result tile is activated;
// today nothing wires a real send (there's no provider to source a real
// SendURL from yet), but the callback exists so that hookup is a one-line
// change once a provider ships.
func newGIFTab(provider GIFProvider, onGIFChosen func(GIFResult)) gtk.Widgetter {
	root := gtk.NewBox(gtk.OrientationVertical, 6)
	root.SetMarginTop(8)
	root.SetMarginBottom(8)
	root.SetMarginStart(8)
	root.SetMarginEnd(8)
	root.SetSizeRequest(280, 260)

	search := gtk.NewSearchEntry()
	search.SetPlaceholderText("Search GIFs")
	root.Append(search)

	flow := gtk.NewFlowBox()
	flow.SetSelectionMode(gtk.SelectionNone)
	flow.SetMaxChildrenPerLine(3)
	flow.SetRowSpacing(4)
	flow.SetColumnSpacing(4)
	flow.SetActivateOnSingleClick(true)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetChild(flow)
	scroller.SetVisible(false)

	empty := gtk.NewLabel("GIF search isn’t set up yet")
	empty.AddCSSClass("dim-label")
	empty.SetVAlign(gtk.AlignCenter)
	empty.SetHAlign(gtk.AlignCenter)
	empty.SetVExpand(true)

	root.Append(scroller)
	root.Append(empty)

	var results []GIFResult
	flow.ConnectChildActivated(func(child *gtk.FlowBoxChild) {
		i := child.Index()
		if i < 0 || i >= len(results) {
			return
		}
		onGIFChosen(results[i])
	})

	runSearch := func(query string) {
		go func() {
			found, err := provider.Search(context.Background(), query)
			glib.IdleAdd(func() {
				clearFlowBox(flow)
				results = found
				if err != nil {
					scroller.SetVisible(false)
					empty.SetVisible(true)
					if !errors.Is(err, errGIFProviderUnconfigured) {
						log.Printf("chatot: gif search failed: %v", err)
					}
					return
				}
				empty.SetVisible(false)
				scroller.SetVisible(true)
				for _, r := range found {
					flow.Append(newGIFTile(r))
				}
			})
		}()
	}

	search.ConnectSearchChanged(func() { runSearch(search.Text()) })
	runSearch("")

	return root
}

// newGIFTile renders one result as a placeholder tile; loading the actual
// PreviewURL image (network fetch + texture decode) is provider follow-up
// work, not part of this UI shell.
func newGIFTile(r GIFResult) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.SetSizeRequest(80, 60)
	box.AddCSSClass("chatot-gif-tile")
	return box
}

// clearFlowBox removes every child of flow, so a re-search starts from an
// empty grid instead of appending onto stale results.
func clearFlowBox(flow *gtk.FlowBox) {
	for child := flow.FirstChild(); child != nil; {
		next := gtk.BaseWidget(child).NextSibling()
		flow.Remove(child)
		child = next
	}
}
