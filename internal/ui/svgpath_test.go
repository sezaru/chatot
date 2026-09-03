package ui

import (
	"math"
	"strings"
	"testing"
)

// recPath records the segments a walk emits, for asserting geometry.
type recPath struct {
	ops []string
	pts [][2]float64
	arc []float64
}

func (r *recPath) MoveTo(x, y float64) {
	r.ops = append(r.ops, "M")
	r.pts = append(r.pts, [2]float64{x, y})
}
func (r *recPath) LineTo(x, y float64) {
	r.ops = append(r.ops, "L")
	r.pts = append(r.pts, [2]float64{x, y})
}
func (r *recPath) CurveTo(x1, y1, x2, y2, x, y float64) {
	r.ops = append(r.ops, "C")
	r.pts = append(r.pts, [2]float64{x, y})
}
func (r *recPath) Arc(cx, cy, rx, ry, phi, theta1, dtheta float64) {
	r.ops = append(r.ops, "A")
	r.arc = append(r.arc, cx, cy, rx, ry, phi, theta1, dtheta)
}
func (r *recPath) Close() { r.ops = append(r.ops, "Z") }

func near(a, b float64) bool { return math.Abs(a-b) < 1e-6 }

func TestWalkSVGPathTabIcons(t *testing.T) {
	// Every path the tab bar strokes must parse, or the bar would show
	// blank icons on a runtime without an SVG loader (the whole point).
	for _, d := range sidebarTabs {
		for _, p := range []string{d.D1, d.D2} {
			if p == "" {
				continue
			}
			var r recPath
			if err := walkSVGPath(p, &r); err != nil {
				t.Errorf("%s: %v", d.ID, err)
			}
			if len(r.ops) == 0 || r.ops[0] != "M" {
				t.Errorf("%s: path %q does not start with a moveto: %v", d.ID, p, r.ops)
			}
		}
	}
}

func TestWalkSVGPathRelativeAndImplicit(t *testing.T) {
	var r recPath
	// h/v/l relative, an implicit lineto after m, and the compact "7.8-3.6"
	// number form all resolve to absolute points.
	if err := walkSVGPath("m1 2 3 4h2v-1l-.5.5Z", &r); err != nil {
		t.Fatal(err)
	}
	want := [][2]float64{{1, 2}, {4, 6}, {6, 6}, {6, 5}, {5.5, 5.5}}
	if strings.Join(r.ops, "") != "MLLLLZ" {
		t.Fatalf("ops = %v", r.ops)
	}
	for i, p := range want {
		if !near(r.pts[i][0], p[0]) || !near(r.pts[i][1], p[1]) {
			t.Errorf("point %d = %v, want %v", i, r.pts[i], p)
		}
	}
}

func TestWalkSVGPathSmoothCubic(t *testing.T) {
	var r recPath
	if err := walkSVGPath("M0 0C1 1 2 1 3 0s2-1 3 0", &r); err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.ops, "") != "MCC" {
		t.Fatalf("ops = %v", r.ops)
	}
	if end := r.pts[2]; !near(end[0], 6) || !near(end[1], 0) {
		t.Errorf("s endpoint = %v, want (6,0)", end)
	}
}

func TestArcToCenterFullCircle(t *testing.T) {
	// The status ring: two half-circle arcs of radius 8.4 around (12,12).
	var r recPath
	if err := walkSVGPath("M12 3.6a8.4 8.4 0 1 1 0 16.8 8.4 8.4 0 0 1 0-16.8Z", &r); err != nil {
		t.Fatal(err)
	}
	if strings.Join(r.ops, "") != "MAAZ" {
		t.Fatalf("ops = %v", r.ops)
	}
	for k := 0; k < 2; k++ {
		a := r.arc[k*7 : k*7+7]
		if !near(a[0], 12) || !near(a[1], 12) || !near(a[2], 8.4) || !near(a[3], 8.4) {
			t.Errorf("arc %d centre/radii = %v", k, a[:4])
		}
		if !near(math.Abs(a[6]), math.Pi) || a[6] < 0 {
			t.Errorf("arc %d sweep = %v, want +π (sweep flag set)", k, a[6])
		}
	}
	// Sweep flag clear goes the other way round.
	var l recPath
	if err := walkSVGPath("M0 0A1 1 0 0 0 2 0", &l); err != nil {
		t.Fatal(err)
	}
	if l.arc[6] >= 0 {
		t.Errorf("counter-clockwise arc has sweep %v, want negative", l.arc[6])
	}
}

func TestWalkSVGPathErrors(t *testing.T) {
	for _, d := range []string{"1 2", "M1", "Q1 2 3 4", "M1 2 3-"} {
		if err := walkSVGPath(d, &recPath{}); err == nil {
			t.Errorf("%q: expected an error", d)
		}
	}
}

func TestDashAndHex(t *testing.T) {
	if got := parseDashArray("14.6 3.1"); len(got) != 2 || !near(got[0], 14.6) || !near(got[1], 3.1) {
		t.Errorf("dash = %v", got)
	}
	if parseDashArray("") != nil || parseDashArray("x") != nil {
		t.Errorf("bad dash arrays should be nil")
	}
	r, g, b := hexRGB("#147a63")
	if !near(r, 0x14/255.0) || !near(g, 0x7a/255.0) || !near(b, 0x63/255.0) {
		t.Errorf("hexRGB = %v %v %v", r, g, b)
	}
	if r, g, b := hexRGB("nope"); r != 0 || g != 0 || b != 0 {
		t.Errorf("bad hex should be black")
	}
}
