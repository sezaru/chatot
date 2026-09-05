package ui

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"log"
	"os"
	"path/filepath"

	"github.com/diamondburned/gotk4/pkg/glib/v2"
	"github.com/diamondburned/gotk4/pkg/gtk/v4"

	"chatot/internal/gif"
)

// GIFService and GIFAPIKey come from Preferences › Network: which search
// service ("giphy" or "tenor") and its key. An empty key leaves the GIF
// tab explaining where to get one.
var (
	GIFService string
	GIFAPIKey  string
)

// gifResultLimit is how many GIFs one search shows.
const gifResultLimit = 24

// gifTileHeight is the mockup's tile height in the 3-column GIF grid.
const gifTileHeight = 74

// GIFResult is one search hit from a GIFProvider: a preview for the tile
// and the mp4 to send, with its pixel size.
type GIFResult struct {
	PreviewURL string
	SendURL    string
	Width      int
	Height     int
}

// GIFProvider searches a GIF service; the picker only knows this seam.
type GIFProvider interface {
	Search(ctx context.Context, query string) ([]GIFResult, error)
}

// errGIFProviderUnconfigured means no API key is set; the GIF tab shows
// how to add one rather than a failure.
var errGIFProviderUnconfigured = errors.New("chatot: gif provider not configured")

// settingsProvider searches the service GIFService names with GIFAPIKey,
// both read on every search so a key typed into Preferences works at once.
type settingsProvider struct{}

// gifService is the search client for the current settings.
func gifService() gif.Provider {
	if GIFService == "tenor" {
		return gif.NewTenor(GIFAPIKey)
	}
	return gif.NewGiphy(GIFAPIKey)
}

func (settingsProvider) Search(ctx context.Context, query string) ([]GIFResult, error) {
	if GIFAPIKey == "" {
		return nil, errGIFProviderUnconfigured
	}
	found, err := gifService().Search(ctx, query, gifResultLimit)
	if err != nil {
		return nil, err
	}
	out := make([]GIFResult, len(found))
	for i, r := range found {
		out[i] = GIFResult{PreviewURL: r.PreviewURL, SendURL: r.MP4URL, Width: r.Width, Height: r.Height}
	}
	return out, nil
}

// gifCachePath is where the file at url is kept: under the cache dir,
// named by a hash of the url.
func gifCachePath(url, ext string) string {
	sum := sha1.Sum([]byte(url))
	return filepath.Join(cacheDir(), "gif", hex.EncodeToString(sum[:])+ext)
}

// fetchGIFFile returns a local copy of url, downloading it the first time.
func fetchGIFFile(ctx context.Context, url, ext string) (string, error) {
	path := gifCachePath(url, ext)
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	data, err := gif.Fetch(ctx, url)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// newGIFTab builds the GIF tab: a search box over a 3-column grid of
// results (Tenor's featured GIFs until the user types). onGIFChosen fires
// with the activated result.
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
	flow.AddCSSClass("chatot-picker-grid")
	flow.SetSelectionMode(gtk.SelectionNone)
	flow.SetMinChildrenPerLine(3)
	flow.SetMaxChildrenPerLine(3)
	flow.SetHomogeneous(true)
	flow.SetRowSpacing(6)
	flow.SetColumnSpacing(6)
	flow.SetActivateOnSingleClick(true)
	flow.SetVAlign(gtk.AlignStart)

	scroller := gtk.NewScrolledWindow()
	scroller.SetVExpand(true)
	scroller.SetPolicy(gtk.PolicyNever, gtk.PolicyAutomatic)
	scroller.SetChild(flow)
	scroller.SetVisible(false)

	empty := gtk.NewLabel("")
	empty.AddCSSClass("dim-label")
	empty.SetWrap(true)
	empty.SetJustify(gtk.JustifyCenter)
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

	showEmpty := func(text string) {
		scroller.SetVisible(false)
		empty.SetText(text)
		empty.SetVisible(true)
	}

	// A search that comes back after a newer one started is dropped.
	generation := 0
	runSearch := func(query string) {
		generation++
		mine := generation
		go func() {
			found, err := provider.Search(context.Background(), query)
			glib.IdleAdd(func() {
				if mine != generation {
					return
				}
				clearFlowBox(flow)
				results = found
				switch {
				case errors.Is(err, errGIFProviderUnconfigured):
					showEmpty("Search GIFs with a Giphy API key\nAdd one under Preferences › Network")
				case err != nil:
					log.Printf("chatot: gif search failed: %v", err)
					showEmpty("Couldn’t search GIFs right now")
				case len(found) == 0:
					showEmpty("No GIFs for “" + query + "”")
				default:
					empty.SetVisible(false)
					scroller.SetVisible(true)
					for _, r := range found {
						flow.Append(newGIFTile(r))
					}
				}
			})
		}()
	}

	search.ConnectSearchChanged(func() { runSearch(search.Text()) })
	runSearch("")

	return root
}

// newGIFTile is one result: its preview, cropped to the tile, fetched in
// the background.
func newGIFTile(r GIFResult) gtk.Widgetter {
	box := gtk.NewBox(gtk.OrientationVertical, 0)
	box.AddCSSClass("chatot-gif-tile")
	box.SetOverflow(gtk.OverflowHidden)
	box.SetSizeRequest(-1, gifTileHeight)

	pic := gtk.NewPicture()
	pic.SetContentFit(gtk.ContentFitCover)
	pic.SetCanShrink(true)
	pic.SetHExpand(true)
	pic.SetVExpand(true)
	box.Append(pic)

	go func() {
		path, err := fetchGIFFile(context.Background(), r.PreviewURL, ".gif")
		if err != nil {
			log.Printf("chatot: gif preview: %v", err)
			return
		}
		glib.IdleAdd(func() { loadPictureAsync(path, 220, pic.SetPaintable) })
	}()
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
