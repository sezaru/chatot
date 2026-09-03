package geo

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/diamondburned/gotk4/pkg/gio/v2"
	"github.com/diamondburned/gotk4/pkg/glib/v2"
)

// Fix is one position report: WGS84 degrees and the horizontal accuracy in
// metres (0 when the source didn't say).
type Fix struct {
	Lat, Lon float64
	Accuracy float64
}

// ErrDenied reports that the user (or the portal's permission store)
// refused the position request.
var ErrDenied = errors.New("chatot/geo: location access denied")

// ErrUnavailable reports that neither the location portal nor GeoClue is
// reachable on this system.
var ErrUnavailable = errors.New("chatot/geo: no positioning service available")

// Session is a running position request. Fixes arrive on onFix (on the
// GLib main loop) until Close; a failure to start or a later denial comes
// through onErr once.
type Session struct {
	conn    *gio.DBusConnection
	subs    []uint
	closeFn func()
	closed  bool
}

const (
	portalDest  = "org.freedesktop.portal.Desktop"
	portalPath  = "/org/freedesktop/portal/desktop"
	portalIface = "org.freedesktop.portal.Location"
)

// Start asks the XDG desktop portal for the device's position, falling
// back to GeoClue's own D-Bus API when no portal is running. The portal
// prompts the user itself the first time. Fixes keep coming (the portal
// re-reports movement) until Close.
func Start(ctx context.Context, onFix func(Fix), onErr func(error)) (*Session, error) {
	conn, err := gio.BusGetSync(ctx, gio.BusTypeSession)
	if err != nil {
		return nil, fmt.Errorf("chatot/geo: session bus: %w", err)
	}
	s := &Session{conn: conn}
	if err := s.startPortal(ctx, onFix, onErr); err == nil {
		return s, nil
	} else if !errors.Is(err, ErrUnavailable) {
		return nil, err
	}
	sys, err := gio.BusGetSync(ctx, gio.BusTypeSystem)
	if err != nil {
		return nil, ErrUnavailable
	}
	s = &Session{conn: sys}
	if err := s.startGeoClue(ctx, onFix, onErr); err != nil {
		return nil, err
	}
	return s, nil
}

// Close ends the request; the positioning service stops working on our
// behalf. Safe to call twice.
func (s *Session) Close() {
	if s == nil || s.closed {
		return
	}
	s.closed = true
	for _, id := range s.subs {
		s.conn.SignalUnsubscribe(id)
	}
	if s.closeFn != nil {
		s.closeFn()
	}
}

// token is a handle token for portal calls: letters/digits only, unique
// per call.
func token() string {
	return fmt.Sprintf("chatot_%d_%d", time.Now().UnixNano(), rand.IntN(1<<20))
}

// senderPath is the portal's naming of our connection in request/session
// object paths: the unique name without the leading ':' and with '.' → '_'.
func senderPath(uniqueName string) string {
	return strings.ReplaceAll(strings.TrimPrefix(uniqueName, ":"), ".", "_")
}

// asv builds an a{sv} dictionary from string→variant pairs.
func asv(pairs map[string]*glib.Variant) *glib.Variant {
	b := glib.NewVariantBuilder(glib.NewVariantType("a{sv}"))
	for k, v := range pairs {
		b.AddValue(glib.NewVariantDictEntry(glib.NewVariantString(k), glib.NewVariantVariant(v)))
	}
	return b.End()
}

// lookupDouble reads a double out of an a{sv}.
func lookupDouble(dict *glib.Variant, key string) (float64, bool) {
	v := dict.LookupValue(key, glib.NewVariantType("d"))
	if v == nil {
		return 0, false
	}
	return v.Double(), true
}

func (s *Session) startPortal(ctx context.Context, onFix func(Fix), onErr func(error)) error {
	// CreateSession → session handle.
	sessionTok := token()
	reply, err := s.conn.CallSync(ctx, portalDest, portalPath, portalIface, "CreateSession",
		glib.NewVariantTuple([]*glib.Variant{asv(map[string]*glib.Variant{
			"session_handle_token": glib.NewVariantString(sessionTok),
			// ACCURACY_EXACT (5): a desktop fix is coarse anyway; asking
			// for less would only make it worse.
			"accuracy": glib.NewVariantUint32(5),
		})}), glib.NewVariantType("(o)"), gio.DBusCallFlagsNone, 5000)
	if err != nil {
		// No portal on the bus (or one without the Location interface).
		return ErrUnavailable
	}
	session := reply.ChildValue(0).String()
	s.closeFn = func() {
		_, _ = s.conn.CallSync(context.Background(), portalDest, session, "org.freedesktop.portal.Session", "Close",
			nil, nil, gio.DBusCallFlagsNone, 2000)
	}

	// Position updates for this session.
	id := s.conn.SignalSubscribe(portalDest, portalIface, "LocationUpdated", portalPath, session, gio.DBusSignalFlagsNone,
		func(_ *gio.DBusConnection, _, _, _, _ string, params *glib.Variant) {
			if params.NChildren() < 2 {
				return
			}
			dict := params.ChildValue(1)
			lat, ok1 := lookupDouble(dict, "Latitude")
			lon, ok2 := lookupDouble(dict, "Longitude")
			if !ok1 || !ok2 {
				return
			}
			acc, _ := lookupDouble(dict, "Accuracy")
			onFix(Fix{Lat: lat, Lon: lon, Accuracy: acc})
		})
	s.subs = append(s.subs, id)

	// Start → a Request whose Response says whether access was granted.
	reqTok := token()
	reqPath := "/org/freedesktop/portal/desktop/request/" + senderPath(s.conn.UniqueName()) + "/" + reqTok
	rid := s.conn.SignalSubscribe(portalDest, "org.freedesktop.portal.Request", "Response", reqPath, "", gio.DBusSignalFlagsNone,
		func(_ *gio.DBusConnection, _, _, _, _ string, params *glib.Variant) {
			if params.NChildren() < 1 {
				return
			}
			if code := params.ChildValue(0).Uint32(); code != 0 {
				onErr(ErrDenied)
			}
		})
	s.subs = append(s.subs, rid)
	_, err = s.conn.CallSync(ctx, portalDest, session, portalIface, "Start",
		glib.NewVariantTuple([]*glib.Variant{
			glib.NewVariantObjectPath(session),
			glib.NewVariantString(""), // parent window: none (a dialog would need an exported handle)
			asv(map[string]*glib.Variant{"handle_token": glib.NewVariantString(reqTok)}),
		}), glib.NewVariantType("(o)"), gio.DBusCallFlagsNone, 5000)
	if err != nil {
		s.Close()
		return fmt.Errorf("chatot/geo: portal Start: %w", err)
	}
	return nil
}

const (
	geoclueDest  = "org.freedesktop.GeoClue2"
	geoclueMgr   = "/org/freedesktop/GeoClue2/Manager"
	geoclueIface = "org.freedesktop.GeoClue2.Client"
)

// startGeoClue talks to GeoClue directly: GetClient, set DesktopId and the
// requested accuracy, subscribe to LocationUpdated, Start.
func (s *Session) startGeoClue(ctx context.Context, onFix func(Fix), onErr func(error)) error {
	reply, err := s.conn.CallSync(ctx, geoclueDest, geoclueMgr, "org.freedesktop.GeoClue2.Manager", "GetClient",
		nil, glib.NewVariantType("(o)"), gio.DBusCallFlagsNone, 5000)
	if err != nil {
		return ErrUnavailable
	}
	client := reply.ChildValue(0).String()
	setProp := func(name string, v *glib.Variant) error {
		_, err := s.conn.CallSync(ctx, geoclueDest, client, "org.freedesktop.DBus.Properties", "Set",
			glib.NewVariantTuple([]*glib.Variant{
				glib.NewVariantString(geoclueIface), glib.NewVariantString(name), glib.NewVariantVariant(v),
			}), nil, gio.DBusCallFlagsNone, 5000)
		return err
	}
	if err := setProp("DesktopId", glib.NewVariantString("com.sezdm.chatot")); err != nil {
		return fmt.Errorf("chatot/geo: geoclue DesktopId: %w", err)
	}
	_ = setProp("RequestedAccuracyLevel", glib.NewVariantUint32(8)) // EXACT
	s.closeFn = func() {
		_, _ = s.conn.CallSync(context.Background(), geoclueDest, client, geoclueIface, "Stop",
			nil, nil, gio.DBusCallFlagsNone, 2000)
	}
	id := s.conn.SignalSubscribe(geoclueDest, geoclueIface, "LocationUpdated", client, "", gio.DBusSignalFlagsNone,
		func(_ *gio.DBusConnection, _, _, _, _ string, params *glib.Variant) {
			if params.NChildren() < 2 {
				return
			}
			loc := params.ChildValue(1).String()
			get := func(name string) (float64, bool) {
				r, err := s.conn.CallSync(context.Background(), geoclueDest, loc, "org.freedesktop.DBus.Properties", "Get",
					glib.NewVariantTuple([]*glib.Variant{
						glib.NewVariantString("org.freedesktop.GeoClue2.Location"), glib.NewVariantString(name),
					}), glib.NewVariantType("(v)"), gio.DBusCallFlagsNone, 2000)
				if err != nil {
					return 0, false
				}
				return r.ChildValue(0).Variant().Double(), true
			}
			lat, ok1 := get("Latitude")
			lon, ok2 := get("Longitude")
			if !ok1 || !ok2 {
				return
			}
			acc, _ := get("Accuracy")
			onFix(Fix{Lat: lat, Lon: lon, Accuracy: acc})
		})
	s.subs = append(s.subs, id)
	if _, err := s.conn.CallSync(ctx, geoclueDest, client, geoclueIface, "Start", nil, nil, gio.DBusCallFlagsNone, 5000); err != nil {
		s.Close()
		if strings.Contains(err.Error(), "AccessDenied") || strings.Contains(err.Error(), "not allowed") {
			return ErrDenied
		}
		return fmt.Errorf("chatot/geo: geoclue Start: %w", err)
	}
	return nil
}
