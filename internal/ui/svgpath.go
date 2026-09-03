package ui

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/diamondburned/gotk4/pkg/cairo"
)

// pathSink receives the segments of an SVG path in absolute coordinates.
// Arcs arrive already converted to centre form (SVG spec F.6.5): centre,
// radii, rotation phi and the sweep from theta1 over dtheta, all in radians.
type pathSink interface {
	MoveTo(x, y float64)
	LineTo(x, y float64)
	CurveTo(x1, y1, x2, y2, x, y float64)
	Arc(cx, cy, rx, ry, phi, theta1, dtheta float64)
	Close()
}

// walkSVGPath parses the SVG path data d (the M/L/H/V/C/S/A/Z commands, in
// both cases, with implicit repetition) and replays it on s. It exists so
// the mockup's icon paths can be stroked with cairo without depending on a
// gdk-pixbuf SVG loader, which some runtimes lack.
func walkSVGPath(d string, s pathSink) error {
	toks, err := tokenizePath(d)
	if err != nil {
		return err
	}
	var (
		cx, cy  float64 // current point
		sx, sy  float64 // start of the current subpath
		lx, ly  float64 // last cubic control point, for S
		cmd     byte
		lastCmd byte
		i       int
		nextTok = func() (float64, bool) {
			if i < len(toks) && toks[i].num {
				v := toks[i].val
				i++
				return v, true
			}
			return 0, false
		}
		need = func(n int) ([]float64, error) {
			out := make([]float64, 0, n)
			for k := 0; k < n; k++ {
				v, ok := nextTok()
				if !ok {
					return nil, fmt.Errorf("svg path: command %q wants %d numbers", cmd, n)
				}
				out = append(out, v)
			}
			return out, nil
		}
	)
	for i < len(toks) {
		if !toks[i].num {
			cmd = toks[i].cmd
			i++
		} else if cmd == 0 {
			return fmt.Errorf("svg path: number before any command")
		} else if cmd == 'M' {
			cmd = 'L' // implicit lineto after a moveto
		} else if cmd == 'm' {
			cmd = 'l'
		}
		rel := cmd >= 'a' && cmd <= 'z'
		up := cmd &^ 0x20
		switch up {
		case 'M':
			a, err := need(2)
			if err != nil {
				return err
			}
			if rel {
				a[0] += cx
				a[1] += cy
			}
			cx, cy = a[0], a[1]
			sx, sy = cx, cy
			s.MoveTo(cx, cy)
		case 'L':
			a, err := need(2)
			if err != nil {
				return err
			}
			if rel {
				a[0] += cx
				a[1] += cy
			}
			cx, cy = a[0], a[1]
			s.LineTo(cx, cy)
		case 'H':
			a, err := need(1)
			if err != nil {
				return err
			}
			if rel {
				a[0] += cx
			}
			cx = a[0]
			s.LineTo(cx, cy)
		case 'V':
			a, err := need(1)
			if err != nil {
				return err
			}
			if rel {
				a[0] += cy
			}
			cy = a[0]
			s.LineTo(cx, cy)
		case 'C':
			a, err := need(6)
			if err != nil {
				return err
			}
			if rel {
				for k := 0; k < 6; k += 2 {
					a[k] += cx
					a[k+1] += cy
				}
			}
			s.CurveTo(a[0], a[1], a[2], a[3], a[4], a[5])
			lx, ly = a[2], a[3]
			cx, cy = a[4], a[5]
		case 'S':
			a, err := need(4)
			if err != nil {
				return err
			}
			if rel {
				for k := 0; k < 4; k += 2 {
					a[k] += cx
					a[k+1] += cy
				}
			}
			// The first control point mirrors the previous cubic's second
			// one; after any other command it is the current point.
			x1, y1 := cx, cy
			if lastCmd&^0x20 == 'C' || lastCmd&^0x20 == 'S' {
				x1, y1 = 2*cx-lx, 2*cy-ly
			}
			s.CurveTo(x1, y1, a[0], a[1], a[2], a[3])
			lx, ly = a[0], a[1]
			cx, cy = a[2], a[3]
		case 'A':
			a, err := need(7)
			if err != nil {
				return err
			}
			if rel {
				a[5] += cx
				a[6] += cy
			}
			arcToCenter(s, cx, cy, a[0], a[1], a[2], a[3] != 0, a[4] != 0, a[5], a[6])
			cx, cy = a[5], a[6]
		case 'Z':
			s.Close()
			cx, cy = sx, sy
		default:
			return fmt.Errorf("svg path: unsupported command %q", cmd)
		}
		lastCmd = cmd
	}
	return nil
}

// arcToCenter converts one SVG endpoint arc to centre parameterisation
// (SVG 1.1 §F.6.5) and hands it to s. A zero radius degenerates to a line,
// and radii too small to reach the endpoint are scaled up, as the spec says.
func arcToCenter(s pathSink, x1, y1, rx, ry, rotDeg float64, large, sweep bool, x2, y2 float64) {
	rx, ry = math.Abs(rx), math.Abs(ry)
	if rx == 0 || ry == 0 || (x1 == x2 && y1 == y2) {
		s.LineTo(x2, y2)
		return
	}
	phi := rotDeg * math.Pi / 180
	cosP, sinP := math.Cos(phi), math.Sin(phi)
	dx, dy := (x1-x2)/2, (y1-y2)/2
	x1p := cosP*dx + sinP*dy
	y1p := -sinP*dx + cosP*dy
	if lambda := x1p*x1p/(rx*rx) + y1p*y1p/(ry*ry); lambda > 1 {
		k := math.Sqrt(lambda)
		rx *= k
		ry *= k
	}
	num := rx*rx*ry*ry - rx*rx*y1p*y1p - ry*ry*x1p*x1p
	den := rx*rx*y1p*y1p + ry*ry*x1p*x1p
	coef := 0.0
	if den != 0 && num > 0 {
		coef = math.Sqrt(num / den)
	}
	if large == sweep {
		coef = -coef
	}
	cxp := coef * rx * y1p / ry
	cyp := -coef * ry * x1p / rx
	cx := cosP*cxp - sinP*cyp + (x1+x2)/2
	cy := sinP*cxp + cosP*cyp + (y1+y2)/2

	ux, uy := (x1p-cxp)/rx, (y1p-cyp)/ry
	vx, vy := (-x1p-cxp)/rx, (-y1p-cyp)/ry
	theta1 := vecAngle(1, 0, ux, uy)
	dtheta := vecAngle(ux, uy, vx, vy)
	if !sweep && dtheta > 0 {
		dtheta -= 2 * math.Pi
	} else if sweep && dtheta < 0 {
		dtheta += 2 * math.Pi
	}
	s.Arc(cx, cy, rx, ry, phi, theta1, dtheta)
}

// vecAngle is the signed angle from vector u to vector v.
func vecAngle(ux, uy, vx, vy float64) float64 {
	dot := ux*vx + uy*vy
	l := math.Hypot(ux, uy) * math.Hypot(vx, vy)
	if l == 0 {
		return 0
	}
	c := math.Max(-1, math.Min(1, dot/l))
	a := math.Acos(c)
	if ux*vy-uy*vx < 0 {
		return -a
	}
	return a
}

// pathToken is one command letter or one number of SVG path data.
type pathToken struct {
	num bool
	cmd byte
	val float64
}

// tokenizePath splits path data into commands and numbers, accepting the
// compact forms minifiers emit ("7.8-3.6", ".2-.7", "1 1 0").
func tokenizePath(d string) ([]pathToken, error) {
	var toks []pathToken
	i := 0
	for i < len(d) {
		c := d[i]
		switch {
		case c == ' ' || c == ',' || c == '\n' || c == '\t' || c == '\r':
			i++
		case (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z'):
			toks = append(toks, pathToken{cmd: c})
			i++
		case c == '-' || c == '+' || c == '.' || (c >= '0' && c <= '9'):
			j := i
			if d[j] == '-' || d[j] == '+' {
				j++
			}
			dot := false
			for j < len(d) {
				ch := d[j]
				if ch >= '0' && ch <= '9' {
					j++
				} else if ch == '.' && !dot {
					dot = true
					j++
				} else {
					break
				}
			}
			if j < len(d) && (d[j] == 'e' || d[j] == 'E') {
				k := j + 1
				if k < len(d) && (d[k] == '-' || d[k] == '+') {
					k++
				}
				for k < len(d) && d[k] >= '0' && d[k] <= '9' {
					k++
				}
				j = k
			}
			v, err := strconv.ParseFloat(strings.TrimPrefix(d[i:j], "+"), 64)
			if err != nil {
				return nil, fmt.Errorf("svg path: bad number %q", d[i:j])
			}
			toks = append(toks, pathToken{num: true, val: v})
			i = j
		default:
			return nil, fmt.Errorf("svg path: unexpected %q", c)
		}
	}
	return toks, nil
}

// cairoPath replays a walked SVG path onto a cairo context. Elliptical arcs
// are drawn as unit circles under a temporary translate/rotate/scale, which
// cairo records in device space, so the transform never leaks into the
// stroke that follows.
type cairoPath struct{ cr *cairo.Context }

func (p cairoPath) MoveTo(x, y float64) { p.cr.MoveTo(x, y) }
func (p cairoPath) LineTo(x, y float64) { p.cr.LineTo(x, y) }
func (p cairoPath) CurveTo(x1, y1, x2, y2, x, y float64) {
	p.cr.CurveTo(x1, y1, x2, y2, x, y)
}
func (p cairoPath) Close() { p.cr.ClosePath() }
func (p cairoPath) Arc(cx, cy, rx, ry, phi, theta1, dtheta float64) {
	p.cr.Save()
	p.cr.Translate(cx, cy)
	p.cr.Rotate(phi)
	p.cr.Scale(rx, ry)
	if dtheta >= 0 {
		p.cr.Arc(0, 0, 1, theta1, theta1+dtheta)
	} else {
		p.cr.ArcNegative(0, 0, 1, theta1, theta1+dtheta)
	}
	p.cr.Restore()
}

// strokeSVGPath adds the SVG path d to cr's current path. Parse errors are
// reported, leaving whatever was emitted before them on the path.
func strokeSVGPath(cr *cairo.Context, d string) error {
	return walkSVGPath(d, cairoPath{cr})
}

// parseDashArray turns an SVG stroke-dasharray ("14.6 3.1") into cairo's
// dash lengths; nil for an empty or malformed value (a solid line).
func parseDashArray(s string) []float64 {
	var out []float64
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ' ' || r == ',' }) {
		v, err := strconv.ParseFloat(f, 64)
		if err != nil || v < 0 {
			return nil
		}
		out = append(out, v)
	}
	return out
}

// hexRGB parses "#rrggbb" into unit floats; black for anything else.
func hexRGB(s string) (r, g, b float64) {
	if len(s) != 7 || s[0] != '#' {
		return 0, 0, 0
	}
	v, err := strconv.ParseUint(s[1:], 16, 32)
	if err != nil {
		return 0, 0, 0
	}
	return float64(v>>16&0xff) / 255, float64(v>>8&0xff) / 255, float64(v&0xff) / 255
}
