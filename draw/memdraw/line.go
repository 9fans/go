package memdraw

import (
	"9fans.net/go/draw"
)

const (
	_Arrow1 = 8
	_Arrow2 = 10
	_Arrow3 = 3
)

func lmin(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func lmax(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

// #ifdef NOTUSED
/*
 * Rather than line clip, we run the Bresenham loop over the full line,
 * and clip on each pixel.  This is more expensive but means that
 * lines look the same regardless of how the windowing has tiled them.
 * For speed, we check for clipping outside the loop and make the
 * test easy when possible.
 */

func horline1(dst *Image, p0 draw.Point, p1 draw.Point, srcval uint8, clipr draw.Rectangle) {
	deltax := p1.X - p0.X
	deltay := p1.Y - p0.Y
	dd := int(dst.Width) * 4
	dy := 1
	if deltay < 0 {
		dd = -dd
		deltay = -deltay
		dy = -1
	}
	maxx := lmin(p1.X, clipr.Max.X-1)
	bpp := dst.Depth
	m0 := uint8(0xFF) ^ (0xFF >> bpp)
	m := m0 >> ((p0.X & (7 / dst.Depth)) * bpp)
	easy := p0.In(clipr) && p1.In(clipr)
	e := 2*deltay - deltax
	y := p0.Y
	d := byteaddr(dst, p0)
	deltay *= 2
	deltax = deltay - 2*deltax
	for x := p0.X; x <= maxx; x++ {
		if easy || (clipr.Min.X <= x && clipr.Min.Y <= y && y < clipr.Max.Y) {
			d[0] ^= (d[0] ^ srcval) & m
		}
		if e > 0 {
			y += dy
			d = d[dd:]
			e += deltax
		} else {
			e += deltay
		}
		d = d[1:]
		m >>= bpp
		if m == 0 {
			m = m0
		}
	}
}

func verline1(dst *Image, p0 draw.Point, p1 draw.Point, srcval uint8, clipr draw.Rectangle) {
	deltax := p1.X - p0.X
	deltay := p1.Y - p0.Y
	dd := 1
	if deltax < 0 {
		dd = -1
		deltax = -deltax
	}
	maxy := lmin(p1.Y, clipr.Max.Y-1)
	bpp := dst.Depth
	m0 := uint8(0xFF) ^ (0xFF >> bpp)
	m := m0 >> ((p0.X & (7 / dst.Depth)) * bpp)
	easy := p0.In(clipr) && p1.In(clipr)
	e := 2*deltax - deltay
	x := p0.X
	d := byteaddr(dst, p0)
	deltax *= 2
	deltay = deltax - 2*deltay
	for y := p0.Y; y <= maxy; y++ {
		if easy || (clipr.Min.Y <= y && clipr.Min.X <= x && x < clipr.Max.X) {
			d[0] ^= (d[0] ^ srcval) & m
		}
		if e > 0 {
			x += dd
			d = d[dd:]
			e += deltay
		} else {
			e += deltax
		}
		d = d[dst.Width*4:]
		m >>= bpp
		if m == 0 {
			m = m0
		}
	}
}

func horliner(dst *Image, p0 draw.Point, p1 draw.Point, src *Image, dsrc draw.Point, clipr draw.Rectangle) {
	deltax := p1.X - p0.X
	deltay := p1.Y - p0.Y
	sx := draw.ReplXY(src.R.Min.X, src.R.Max.X, p0.X+dsrc.X)
	minx := lmax(p0.X, clipr.Min.X)
	maxx := lmin(p1.X, clipr.Max.X-1)
	bpp := dst.Depth
	m0 := uint8(0xFF) ^ (0xFF >> bpp)
	m := m0 >> ((minx & (7 / dst.Depth)) * bpp)
	for x := minx; x <= maxx; x++ {
		y := p0.Y + (deltay*(x-p0.X)+deltax/2)/deltax
		if clipr.Min.Y <= y && y < clipr.Max.Y {
			d := byteaddr(dst, draw.Pt(x, y))
			sy := draw.ReplXY(src.R.Min.Y, src.R.Max.Y, y+dsrc.Y)
			s := byteaddr(src, draw.Pt(sx, sy))
			d[0] ^= (d[0] ^ s[0]) & m
		}
		sx++
		if sx >= src.R.Max.X {
			sx = src.R.Min.X
		}
		m >>= bpp
		if m == 0 {
			m = m0
		}
	}
}

func verliner(dst *Image, p0 draw.Point, p1 draw.Point, src *Image, dsrc draw.Point, clipr draw.Rectangle) {
	deltax := p1.X - p0.X
	deltay := p1.Y - p0.Y
	sy := draw.ReplXY(src.R.Min.Y, src.R.Max.Y, p0.Y+dsrc.Y)
	miny := lmax(p0.Y, clipr.Min.Y)
	maxy := lmin(p1.Y, clipr.Max.Y-1)
	bpp := dst.Depth
	m0 := uint8(0xFF) ^ (0xFF >> bpp)
	for y := miny; y <= maxy; y++ {
		var x int
		if deltay == 0 { /* degenerate line */
			x = p0.X
		} else {
			x = p0.X + (deltax*(y-p0.Y)+deltay/2)/deltay
		}
		if clipr.Min.X <= x && x < clipr.Max.X {
			m := m0 >> ((x & (7 / dst.Depth)) * bpp)
			d := byteaddr(dst, draw.Pt(x, y))
			sx := draw.ReplXY(src.R.Min.X, src.R.Max.X, x+dsrc.X)
			s := byteaddr(src, draw.Pt(sx, sy))
			d[0] ^= (d[0] ^ s[0]) & m
		}
		sy++
		if sy >= src.R.Max.Y {
			sy = src.R.Min.Y
		}
	}
}

func horline(dst *Image, p0 draw.Point, p1 draw.Point, src *Image, dsrc draw.Point, clipr draw.Rectangle) {
	deltax := p1.X - p0.X
	deltay := p1.Y - p0.Y
	minx := lmax(p0.X, clipr.Min.X)
	maxx := lmin(p1.X, clipr.Max.X-1)
	bpp := dst.Depth
	m0 := uint8(0xFF) ^ (0xFF >> bpp)
	m := m0 >> ((minx & (7 / dst.Depth)) * bpp)
	for x := minx; x <= maxx; x++ {
		y := p0.Y + (deltay*(x-p0.X)+deltay/2)/deltax
		if clipr.Min.Y <= y && y < clipr.Max.Y {
			d := byteaddr(dst, draw.Pt(x, y))
			s := byteaddr(src, dsrc.Add(draw.Pt(x, y)))
			d[0] ^= (d[0] ^ s[0]) & m
		}
		m >>= bpp
		if m == 0 {
			m = m0
		}
	}
}

func verline(dst *Image, p0 draw.Point, p1 draw.Point, src *Image, dsrc draw.Point, clipr draw.Rectangle) {
	deltax := p1.X - p0.X
	deltay := p1.Y - p0.Y
	miny := lmax(p0.Y, clipr.Min.Y)
	maxy := lmin(p1.Y, clipr.Max.Y-1)
	bpp := dst.Depth
	m0 := uint8(0xFF) ^ (0xFF >> bpp)
	for y := miny; y <= maxy; y++ {
		var x int
		if deltay == 0 { /* degenerate line */
			x = p0.X
		} else {
			x = p0.X + deltax*(y-p0.Y)/deltay
		}
		if clipr.Min.X <= x && x < clipr.Max.X {
			m := m0 >> ((x & (7 / dst.Depth)) * bpp)
			d := byteaddr(dst, draw.Pt(x, y))
			s := byteaddr(src, dsrc.Add(draw.Pt(x, y)))
			d[0] ^= (d[0] ^ s[0]) & m
		}
	}
}

// #endif /* NOTUSED */

var membrush_brush *Image
var membrush_brushradius int

func membrush(radius int) *Image {
	if membrush_brush == nil || membrush_brushradius != radius {
		Free(membrush_brush)
		var err error
		membrush_brush, err = AllocImage(draw.Rect(0, 0, 2*radius+1, 2*radius+1), Opaque.Pix)
		if err == nil {
			FillColor(membrush_brush, draw.Transparent) /* zeros */
			Ellipse(membrush_brush, draw.Pt(radius, radius), radius, radius, -1, Opaque, draw.Pt(radius, radius), draw.S)
		}
		membrush_brushradius = radius
	}
	return membrush_brush
}

func discend(p draw.Point, radius int, dst *Image, src *Image, dsrc draw.Point, op draw.Op) {
	disc := membrush(radius)
	if disc != nil {
		var r draw.Rectangle
		r.Min.X = p.X - radius
		r.Min.Y = p.Y - radius
		r.Max.X = p.X + radius + 1
		r.Max.Y = p.Y + radius + 1
		Draw(dst, r, src, r.Min.Add(dsrc), disc, draw.Pt(0, 0), op)
	}
}

func arrowend(tip draw.Point, pp []draw.Point, end draw.End, sin int, cos int, radius int) {
	var x1 int
	var x2 int
	var x3 int
	/* before rotation */
	if end == draw.EndArrow {
		x1 = _Arrow1
		x2 = _Arrow2
		x3 = _Arrow3
	} else {
		x1 = int(end>>5) & 0x1FF  /* distance along line from end of line to tip */
		x2 = int(end>>14) & 0x1FF /* distance along line from barb to tip */
		x3 = int(end>>23) & 0x1FF /* distance perpendicular from edge of line to barb */
	}

	/* comments follow track of right-facing arrowhead */
	pp[0].X = tip.X + ((2*radius+1)*sin/2 - x1*cos) /* upper side of shaft */
	pp[0].Y = tip.Y - ((2*radius+1)*cos/2 + x1*sin)

	pp[1].X = tip.X + ((2*radius+2*x3+1)*sin/2 - x2*cos) /* upper barb */
	pp[1].Y = tip.Y - ((2*radius+2*x3+1)*cos/2 + x2*sin)

	pp[2].X = tip.X
	pp[2].Y = tip.Y

	pp[3].X = tip.X + (-(2*radius+2*x3+1)*sin/2 - x2*cos) /* lower barb */
	pp[3].Y = tip.Y - (-(2*radius+2*x3+1)*cos/2 + x2*sin)

	pp[4].X = tip.X + (-(2*radius+1)*sin/2 - x1*cos) /* lower side of shaft */
	pp[4].Y = tip.Y + ((2*radius+1)*cos/2 - x1*sin)
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func _memimageline(dst *Image, p0 draw.Point, p1 draw.Point, end0, end1 draw.End, radius int, src *Image, sp draw.Point, clipr draw.Rectangle, op draw.Op) {
	if radius < 0 {
		return
	}
	if !draw.RectClip(&clipr, dst.R) {
		return
	}
	if !draw.RectClip(&clipr, dst.Clipr) {
		return
	}
	d := sp.Sub(p0)
	if !draw.RectClip(&clipr, src.Clipr.Sub(d)) {
		return
	}
	if src.Flags&Frepl == 0 && !draw.RectClip(&clipr, src.R.Sub(d)) {
		return
	}
	/* this means that only verline() handles degenerate lines (p0==p1) */
	hor := abs(p1.X-p0.X) > abs(p1.Y-p0.Y)
	var q draw.Point
	/*
	 * Clipping is a little peculiar.  We can't use Sutherland-Cohen
	 * clipping because lines are wide.  But this is probably just fine:
	 * we do all math with the original p0 and p1, but clip when deciding
	 * what pixels to draw.  This means the layer code can call this routine,
	 * using clipr to define the region being written, and get the same set
	 * of pixels regardless of the dicing.
	 */
	if (hor && p0.X > p1.X) || (!hor && p0.Y > p1.Y) {
		q = p0
		p0 = p1
		p1 = q
		t := end0
		end0 = end1
		end1 = t
	}
	var oclipr draw.Rectangle

	if (p0.X == p1.X || p0.Y == p1.Y) && end0&0x1F == draw.EndSquare && end1&0x1F == draw.EndSquare {
		var r draw.Rectangle
		r.Min = p0
		r.Max = p1
		if p0.X == p1.X {
			r.Min.X -= radius
			r.Max.X += radius + 1
		} else {
			r.Min.Y -= radius
			r.Max.Y += radius + 1
		}
		oclipr = dst.Clipr
		dst.Clipr = clipr
		dst.Draw(r, src, sp, Opaque, sp, op)
		dst.Clipr = oclipr
		return
	}

	/*    Hard: */
	/* draw thick line using polygon fill */
	cos, sin := draw.IntCosSin2(p1.X-p0.X, p1.Y-p0.Y)
	dx := (sin * (2*radius + 1)) / 2
	dy := (cos * (2*radius + 1)) / 2
	var pts [10]draw.Point
	pp := pts[:]
	oclipr = dst.Clipr
	dst.Clipr = clipr
	q.X = draw.ICOSSCALE*p0.X + draw.ICOSSCALE/2 - cos/2
	q.Y = draw.ICOSSCALE*p0.Y + draw.ICOSSCALE/2 - sin/2
	switch end0 & 0x1F {
	case draw.EndDisc:
		discend(p0, radius, dst, src, d, op)
		fallthrough
	/* fall through */
	case draw.EndSquare:
		fallthrough
	default:
		pp[0].X = q.X - dx
		pp[0].Y = q.Y + dy
		pp[1].X = q.X + dx
		pp[1].Y = q.Y - dy
		pp = pp[2:]
	case draw.EndArrow:
		arrowend(q, pp, end0, -sin, -cos, radius)
		_memfillpolysc(dst, pts[:5], ^0, src, pts[0].Add(d.Mul(draw.ICOSSCALE)), 1, 10, 1, op)
		pp[1] = pp[4]
		pp = pp[2:]
	}
	q.X = draw.ICOSSCALE*p1.X + draw.ICOSSCALE/2 + cos/2
	q.Y = draw.ICOSSCALE*p1.Y + draw.ICOSSCALE/2 + sin/2
	switch end1 & 0x1F {
	case draw.EndDisc:
		discend(p1, radius, dst, src, d, op)
		fallthrough
	/* fall through */
	case draw.EndSquare:
		fallthrough
	default:
		pp[0].X = q.X + dx
		pp[0].Y = q.Y - dy
		pp[1].X = q.X - dx
		pp[1].Y = q.Y + dy
		pp = pp[2:]
	case draw.EndArrow:
		arrowend(q, pp, end1, sin, cos, radius)
		_memfillpolysc(dst, pp[:5], ^0, src, pts[0].Add(d.Mul(draw.ICOSSCALE)), 1, 10, 1, op)
		pp[1] = pp[4]
		pp = pp[2:]
	}
	_memfillpolysc(dst, pts[:len(pts)-len(pp)], ^0, src, pts[0].Add(d.Mul(draw.ICOSSCALE)), 0, 10, 1, op)
	dst.Clipr = oclipr
	return
}

func memimageline(dst *Image, p0 draw.Point, p1 draw.Point, end0, end1 draw.End, radius int, src *Image, sp draw.Point, op draw.Op) {
	_memimageline(dst, p0, p1, end0, end1, radius, src, sp, dst.Clipr, op)
}

/*
 * Simple-minded conservative code to compute bounding box of line.
 * Result is probably a little larger than it needs to be.
 */
func addbbox(r *draw.Rectangle, p draw.Point) {
	if r.Min.X > p.X {
		r.Min.X = p.X
	}
	if r.Min.Y > p.Y {
		r.Min.Y = p.Y
	}
	if r.Max.X < p.X+1 {
		r.Max.X = p.X + 1
	}
	if r.Max.Y < p.Y+1 {
		r.Max.Y = p.Y + 1
	}
}

func LineEndSize(end draw.End) int {
	if end&0x3F != draw.EndArrow {
		return 0
	}
	var x3 int
	if end == draw.EndArrow {
		x3 = _Arrow3
	} else {
		x3 = int(end>>23) & 0x1FF
	}
	return x3
}

func LineBBox(p0 draw.Point, p1 draw.Point, end0, end1 draw.End, radius int) draw.Rectangle {
	var r draw.Rectangle
	r.Min.X = 10000000
	r.Min.Y = 10000000
	r.Max.X = -10000000
	r.Max.Y = -10000000
	extra := lmax(LineEndSize(end0), LineEndSize(end1))
	r1 := draw.Rpt(p0, p1).Canon().Inset(-(radius + extra))
	addbbox(&r, r1.Min)
	addbbox(&r, r1.Max)
	return r
}
