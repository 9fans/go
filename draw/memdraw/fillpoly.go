// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"sort"

	"9fans.net/go/draw"
)

type polySeg struct {
	p0    draw.Point
	p1    draw.Point
	num   int
	den   int
	dz    int
	dzrem int
	z     int
	zerr  int
	d     int
}

func fillline(dst *Image, left int, right int, y int, src *Image, p draw.Point, op draw.Op) {
	var r draw.Rectangle
	r.Min.X = left
	r.Min.Y = y
	r.Max.X = right
	r.Max.Y = y + 1
	p.X += left
	p.Y += y
	Draw(dst, r, src, p, Opaque, p, op)
}

func fillpoint(dst *Image, x int, y int, src *Image, p draw.Point, op draw.Op) {
	var r draw.Rectangle
	r.Min.X = x
	r.Min.Y = y
	r.Max.X = x + 1
	r.Max.Y = y + 1
	p.X += x
	p.Y += y
	Draw(dst, r, src, p, Opaque, p, op)
}

func FillPoly(dst *Image, vert []draw.Point, w int, src *Image, sp draw.Point, op draw.Op) {
	_memfillpolysc(dst, vert, w, src, sp, 0, 0, 0, op)
}

func _memfillpolysc(dst *Image, vert []draw.Point, w int, src *Image, sp draw.Point, detail int, fixshift int, clipped int, op draw.Op) {
	if len(vert) == 0 {
		return
	}

	seg := make([]*polySeg, len(vert)+2)
	segtab := make([]polySeg, len(vert)+1)

	sp.X = (sp.X - vert[0].X) >> fixshift
	sp.Y = (sp.Y - vert[0].Y) >> fixshift
	p0 := vert[len(vert)-1]
	if fixshift == 0 {
		p0.X <<= 1
		p0.Y <<= 1
	}
	for i := 0; i < len(vert); i++ {
		segtab[i].p0 = p0
		p0 = vert[i]
		if fixshift == 0 {
			p0.X <<= 1
			p0.Y <<= 1
		}
		segtab[i].p1 = p0
		segtab[i].d = 1
	}
	if fixshift == 0 {
		fixshift = 1
	}

	xscan(dst, seg, segtab, len(vert), w, src, sp, detail, fixshift, clipped, op)
	if detail != 0 {
		yscan(dst, seg, segtab, len(vert), w, src, sp, fixshift, op)
	}
}

func mod(x int, y int) int {
	z := x % y
	if int((int(z))^(int(y))) > 0 || z == 0 {
		return z
	}
	return z + y
}

func sdiv(x int, y int) int {
	if int((int(x))^(int(y))) >= 0 || x == 0 {
		return x / y
	}

	return (x+(y>>30|1))/y - 1
}

func smuldivmod(x int, y int, z int, mod *int) int {
	if x == 0 || y == 0 {
		*mod = 0
		return 0
	}
	vx := x
	vx *= y
	*mod = vx % z
	if *mod < 0 {
		*mod += z /* z is always >0 */
	}
	if (vx < 0) == (z < 0) {
		return vx / z
	}
	return -((-vx) / z)
}

func xscan(dst *Image, seg []*polySeg, segtab []polySeg, nseg int, wind int, src *Image, sp draw.Point, detail int, fixshift int, clipped int, op draw.Op) {
	fill := fillline
	/*
		 * This can only work on 8-bit destinations, since fillcolor is
		 * just using memset on sp.x.
		 *
		 * I'd rather not even enable it then, since then if the general
		 * code is too slow, someone will come up with a better improvement
		 * than this sleazy hack.	-rsc
		 *
			if(clipped && (src->flags&Frepl) && src->depth==8 && Dx(src->r)==1 && Dy(src->r)==1) {
				fill = fillcolor;
				sp.x = membyteval(src);
			}
		 *
	*/

	p := 0
	for i := 0; i < nseg; i++ {
		s := &segtab[i]
		seg[p] = s
		if s.p0.Y == s.p1.Y {
			continue
		}
		if s.p0.Y > s.p1.Y {
			pt := s.p0
			s.p0 = s.p1
			s.p1 = pt
			s.d = -s.d
		}
		s.num = s.p1.X - s.p0.X
		s.den = s.p1.Y - s.p0.Y
		s.dz = sdiv(s.num, s.den) << fixshift
		s.dzrem = mod(s.num, s.den) << fixshift
		s.dz += sdiv(s.dzrem, s.den)
		s.dzrem = mod(s.dzrem, s.den)
		p++
	}
	if p == 0 {
		return
	}
	seg[p] = nil
	sort.Slice(seg[:p], func(i, j int) bool { return seg[i].p0.Y < seg[j].p0.Y })

	onehalf := 0
	if fixshift != 0 {
		onehalf = 1 << (fixshift - 1)
	}

	minx := dst.Clipr.Min.X
	maxx := dst.Clipr.Max.X

	y := seg[0].p0.Y
	if y < dst.Clipr.Min.Y<<fixshift {
		y = dst.Clipr.Min.Y << fixshift
	}
	iy := (y + onehalf) >> fixshift
	y = (iy << fixshift) + onehalf
	maxy := dst.Clipr.Max.Y << fixshift

	next := 0
	ep := next

	for y < maxy {
		p = 0
		var q int
		for q = p; p < ep; p++ {
			s := seg[p]
			if s.p1.Y < y {
				continue
			}
			s.z += s.dz
			s.zerr += s.dzrem
			if s.zerr >= s.den {
				s.z++
				s.zerr -= s.den
				if s.zerr < 0 || s.zerr >= s.den {
					print("bad ratzerr1: %ld den %ld dzrem %ld\n", s.zerr, s.den, s.dzrem)
				}
			}
			seg[q] = s
			q++
		}

		for p = next; seg[p] != nil; p++ {
			s := seg[p]
			if s.p0.Y >= y {
				break
			}
			if s.p1.Y < y {
				continue
			}
			s.z = s.p0.X
			s.z += smuldivmod(y-s.p0.Y, s.num, s.den, &s.zerr)
			if s.zerr < 0 || s.zerr >= s.den {
				print("bad ratzerr2: %ld den %ld ratdzrem %ld\n", s.zerr, s.den, s.dzrem)
			}
			seg[q] = s
			q++
		}
		ep = q
		next = p

		if ep == 0 {
			if seg[next] == nil {
				break
			}
			iy = (seg[next].p0.Y + onehalf) >> fixshift
			y = (iy << fixshift) + onehalf
			continue
		}

		zsort(seg, ep)

		for p = 0; p < ep; p++ {
			s := seg[p]
			cnt := 0
			x := s.z
			xerr := s.zerr
			xden := s.den
			ix := (x + onehalf) >> fixshift
			if ix >= maxx {
				break
			}
			if ix < minx {
				ix = minx
			}
			cnt += s.d
			p++
			s = seg[p]
			for {
				if p == ep {
					print("xscan: fill to infinity")
					return
				}
				cnt += s.d
				if cnt&wind == 0 {
					break
				}
				p++
				s = seg[p]
			}
			x2 := s.z
			ix2 := (x2 + onehalf) >> fixshift
			if ix2 <= minx {
				continue
			}
			if ix2 > maxx {
				ix2 = maxx
			}
			if ix == ix2 && detail != 0 {
				if xerr*s.den+s.zerr*xden > s.den*xden {
					x++
				}
				ix = (x + x2) >> (fixshift + 1)
				ix2 = ix + 1
			}
			fill(dst, ix, ix2, iy, src, sp, op)
		}
		y += (1 << fixshift)
		iy++
	}
}

func yscan(dst *Image, seg []*polySeg, segtab []polySeg, nseg int, wind int, src *Image, sp draw.Point, fixshift int, op draw.Op) {
	p := 0
	for i := 0; i < nseg; i++ {
		s := &segtab[i]
		seg[p] = s
		if s.p0.X == s.p1.X {
			continue
		}
		if s.p0.X > s.p1.X {
			pt := s.p0
			s.p0 = s.p1
			s.p1 = pt
			s.d = -s.d
		}
		s.num = s.p1.Y - s.p0.Y
		s.den = s.p1.X - s.p0.X
		s.dz = sdiv(s.num, s.den) << fixshift
		s.dzrem = mod(s.num, s.den) << fixshift
		s.dz += sdiv(s.dzrem, s.den)
		s.dzrem = mod(s.dzrem, s.den)
		p++
	}
	if p == 0 {
		return
	}
	seg[p] = nil
	sort.Slice(seg[:p], func(i, j int) bool { return seg[i].p0.X < seg[j].p0.X })

	onehalf := 0
	if fixshift != 0 {
		onehalf = 1 << (fixshift - 1)
	}

	miny := dst.Clipr.Min.Y
	maxy := dst.Clipr.Max.Y

	x := seg[0].p0.X
	if x < dst.Clipr.Min.X<<fixshift {
		x = dst.Clipr.Min.X << fixshift
	}
	ix := (x + onehalf) >> fixshift
	x = (ix << fixshift) + onehalf
	maxx := dst.Clipr.Max.X << fixshift

	next := 0
	ep := next

	for x < maxx {
		p = 0
		var q int
		for q = p; p < ep; p++ {
			s := seg[p]
			if s.p1.X < x {
				continue
			}
			s.z += s.dz
			s.zerr += s.dzrem
			if s.zerr >= s.den {
				s.z++
				s.zerr -= s.den
				if s.zerr < 0 || s.zerr >= s.den {
					print("bad ratzerr1: %ld den %ld ratdzrem %ld\n", s.zerr, s.den, s.dzrem)
				}
			}
			seg[q] = s
			q++
		}

		for p = next; seg[p] != nil; p++ {
			s := seg[p]
			if s.p0.X >= x {
				break
			}
			if s.p1.X < x {
				continue
			}
			s.z = s.p0.Y
			s.z += smuldivmod(x-s.p0.X, s.num, s.den, &s.zerr)
			if s.zerr < 0 || s.zerr >= s.den {
				print("bad ratzerr2: %ld den %ld ratdzrem %ld\n", s.zerr, s.den, s.dzrem)
			}
			seg[q] = s
			q++
		}
		ep = q
		next = p

		if ep == 0 {
			if seg[next] == nil {
				break
			}
			ix = (seg[next].p0.X + onehalf) >> fixshift
			x = (ix << fixshift) + onehalf
			continue
		}

		zsort(seg, ep)

		for p = 0; p < ep; p++ {
			cnt := 0
			y := seg[p].z
			yerr := seg[p].zerr
			yden := seg[p].den
			iy := (y + onehalf) >> fixshift
			if iy >= maxy {
				break
			}
			if iy < miny {
				iy = miny
			}
			cnt += seg[p].d
			p++
			for {
				if p == ep {
					print("yscan: fill to infinity")
					return
				}
				cnt += seg[p].d
				if cnt&wind == 0 {
					break
				}
				p++
			}
			y2 := seg[p].z
			iy2 := (y2 + onehalf) >> fixshift
			if iy2 <= miny {
				continue
			}
			if iy2 > maxy {
				iy2 = maxy
			}
			if iy == iy2 {
				if yerr*seg[p].den+seg[p].zerr*yden > seg[p].den*yden {
					y++
				}
				iy = (y + y2) >> (fixshift + 1)
				fillpoint(dst, ix, iy, src, sp, op)
			}
		}
		x += (1 << fixshift)
		ix++
	}
}

func zsort(seg []*polySeg, ep int) {
	if ep < 20 {
		/* bubble sort by z - they should be almost sorted already */
		q := ep
		for {
			done := true
			q--
			for p := 0; p < q; p++ {
				if seg[p].z > seg[p+1].z {
					s := seg[p]
					seg[p] = seg[p+1]
					seg[p+1] = s
					done = false
				}
			}
			if done {
				break
			}
		}
	} else {
		q := ep - 1
		for p := 0; p < q; p++ {
			if seg[p].z > seg[p+1].z {
				sort.Slice(seg[:ep], func(i, j int) bool { return seg[i].z < seg[j].z })
				break
			}
		}
	}
}
