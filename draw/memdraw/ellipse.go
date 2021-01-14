// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

/*
 * ellipse(dst, c, a, b, t, src, sp)
 *   draws an ellipse centered at c with semiaxes a,b>=0
 *   and semithickness t>=0, or filled if t<0.  point sp
 *   in src maps to c in dst
 *
 *   very thick skinny ellipses are brushed with circles (slow)
 *   others are approximated by filling between 2 ellipses
 *   criterion for very thick when b<a: t/b > 0.5*x/(1-x)
 *   where x = b/a
 */

package memdraw

import (
	"9fans.net/go/draw"
)

type ellipseParam struct {
	dst  *Image
	src  *Image
	c    draw.Point
	t    int
	sp   draw.Point
	disc *Image
	op   draw.Op
}

/*
 * denote residual error by e(x,y) = b^2*x^2 + a^2*y^2 - a^2*b^2
 * e(x,y) = 0 on ellipse, e(x,y) < 0 inside, e(x,y) > 0 outside
 */

type ellipseState struct {
	a    int
	x    int
	a2   int64
	b2   int64
	b2x  int64
	a2y  int64
	c1   int64
	c2   int64
	ee   int64
	dxe  int64
	dye  int64
	d2xe int64
	d2ye int64
}

func newstate(s *ellipseState, a int, b int) *ellipseState {
	s.x = 0
	s.a = a
	s.a2 = int64(a * a)
	s.b2 = int64(b * b)
	s.b2x = int64(0)
	s.a2y = s.a2 * int64(b)
	s.c1 = -((s.a2 >> 2) + int64(a&1) + s.b2)
	s.c2 = -((s.b2 >> 2) + int64(b&1))
	s.ee = -s.a2y
	s.dxe = int64(0)
	s.dye = s.ee << 1
	s.d2xe = s.b2 << 1
	s.d2ye = s.a2 << 1
	return s
}

/*
 * return x coord of rightmost pixel on next scan line
 */
func step(s *ellipseState) int {
	for s.x < s.a {
		if s.ee+s.b2x <= s.c1 || s.ee+s.a2y <= s.c2 { /* e(x+1,y-1/2) <= 0 */ /* e(x+1/2,y) <= 0 (rare) */
			s.dxe += s.d2xe
			s.ee += s.dxe
			s.b2x += s.b2
			s.x++
			continue
		}
		s.dye += s.d2ye
		s.ee += s.dye
		s.a2y -= s.a2
		if s.ee-s.a2y <= s.c2 { /* e(x+1/2,y-1) <= 0 */
			s.dxe += s.d2xe
			s.ee += s.dxe
			s.b2x += s.b2
			tmp21 := s.x
			s.x++
			return tmp21
		}
		break
	}
	return s.x
}

func Ellipse(dst *Image, c draw.Point, a int, b int, t int, src *Image, sp draw.Point, op draw.Op) {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	var p ellipseParam
	p.dst = dst
	p.src = src
	p.c = c
	p.t = t
	p.sp = sp.Sub(c)
	p.disc = nil
	p.op = op

	u := (t << 1) * (a - b)
	var in ellipseState
	if b < a && u > b*b || a < b && -u > a*a {
		/*	if(b<a&&(t<<1)>b*b/a || a<b&&(t<<1)>a*a/b)	# very thick */
		bellipse(b, newstate(&in, a, b), &p)
		return
	}
	var out ellipseState
	var y int
	var inb int

	if t < 0 {
		inb = -1
		y = b
		newstate(&out, a, y)
	} else {
		inb = b - t
		y = b + t
		newstate(&out, a+t, y)
	}
	if t > 0 {
		newstate(&in, a-t, inb)
	}
	inx := 0
	for ; y >= 0; y-- {
		outx := step(&out)
		if y > inb {
			erect(-outx, y, outx, y, &p)
			if y != 0 {
				erect(-outx, -y, outx, -y, &p)
			}
			continue
		}
		if t > 0 {
			inx = step(&in)
			if y == inb {
				inx = 0
			}
		} else if inx > outx {
			inx = outx
		}
		erect(inx, y, outx, y, &p)
		if y != 0 {
			erect(inx, -y, outx, -y, &p)
		}
		erect(-outx, y, -inx, y, &p)
		if y != 0 {
			erect(-outx, -y, -inx, -y, &p)
		}
		inx = outx + 1
	}
}

/*
 * a brushed ellipse
 */
func bellipse(y int, s *ellipseState, p *ellipseParam) {
	t := p.t
	var err error
	p.disc, err = AllocImage(draw.Rect(-t, -t, t+1, t+1), draw.GREY1)
	if err != nil {
		return
	}
	FillColor(p.disc, draw.Transparent)
	Ellipse(p.disc, draw.ZP, t, t, -1, Opaque, draw.ZP, p.op)
	oy := y
	ox := 0
	x := step(s)
	nx := x
	for {
		for nx == x {
			nx = step(s)
		}
		y++
		eline(-x, -oy, -ox, -y, p)
		eline(ox, -oy, x, -y, p)
		eline(-x, y, -ox, oy, p)
		eline(ox, y, x, oy, p)
		ox = x + 1
		x = nx
		y--
		oy = y
		if oy <= 0 {
			break
		}
	}
}

/*
 * a rectangle with closed (not half-open) coordinates expressed
 * relative to the center of the ellipse
 */
func erect(x0 int, y0 int, x1 int, y1 int, p *ellipseParam) {
	/*	print("R %d,%d %d,%d\n", x0, y0, x1, y1); */
	r := draw.Rect(p.c.X+x0, p.c.Y+y0, p.c.X+x1+1, p.c.Y+y1+1)
	Draw(p.dst, r, p.src, p.sp.Add(r.Min), Opaque, draw.ZP, p.op)
}

/*
 * a brushed point similarly specified
 */
func epoint(x int, y int, p *ellipseParam) {
	/*	print("P%d %d,%d\n", p->t, x, y);	*/
	p0 := draw.Pt(p.c.X+x, p.c.Y+y)
	r := draw.Rpt(p0.Add(p.disc.R.Min), p0.Add(p.disc.R.Max))
	Draw(p.dst, r, p.src, p.sp.Add(r.Min), p.disc, p.disc.R.Min, p.op)
}

/*
 * a brushed horizontal or vertical line similarly specified
 */
func eline(x0 int, y0 int, x1 int, y1 int, p *ellipseParam) {
	/*	print("L%d %d,%d %d,%d\n", p->t, x0, y0, x1, y1); */
	if x1 > x0+1 {
		erect(x0+1, y0-p.t, x1-1, y1+p.t, p)
	} else if y1 > y0+1 {
		erect(x0-p.t, y0+1, x1+p.t, y1-1, p)
	}
	epoint(x0, y0, p)
	if x1-x0 != 0 || y1-y0 != 0 {
		epoint(x1, y1, p)
	}
}
