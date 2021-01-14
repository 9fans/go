// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

/*
 * elarc(dst,c,a,b,t,src,sp,alpha,phi)
 *   draws the part of an ellipse between rays at angles alpha and alpha+phi
 *   measured counterclockwise from the positive x axis. other
 *   arguments are as for ellipse(dst,c,a,b,t,src,sp)
 */

package memdraw

import (
	"9fans.net/go/draw"
)

const (
	_R = iota
	_T
	_L
	_B
)

/* right, top, left, bottom */

var corners = [4]draw.Point{
	draw.Point{1, 1},
	draw.Point{-1, 1},
	draw.Point{-1, -1},
	draw.Point{1, -1},
}

var p00 draw.Point

/*
 * make a "wedge" mask covering the desired angle and contained in
 * a surrounding square; draw a full ellipse; intersect that with the
 * wedge to make a mask through which to copy src to dst.
 */
func Arc(dst *Image, c draw.Point, a int, b int, t int, src *Image, sp draw.Point, alpha int, phi int, op draw.Op) {
	if a < 0 {
		a = -a
	}
	if b < 0 {
		b = -b
	}
	w := t
	if w < 0 {
		w = 0
	}
	alpha = -alpha /* compensate for upside-down coords */
	phi = -phi
	beta := alpha + phi
	if phi < 0 {
		tmp := alpha
		alpha = beta
		beta = tmp
		phi = -phi
	}
	if phi >= 360 {
		Ellipse(dst, c, a, b, t, src, sp, op)
		return
	}
	for alpha < 0 {
		alpha += 360
	}
	for beta < 0 {
		beta += 360
	}
	c1 := alpha / 90 & 3 /* number of nearest corner */
	c2 := beta / 90 & 3
	/*
	 * icossin returns point at radius ICOSSCALE.
	 * multiplying by m1 moves it outside the ellipse
	 */
	rect := draw.Rect(-a-w, -b-w, a+w+1, b+w+1)
	m := rect.Max.X /* inradius of bounding square */
	if m < rect.Max.Y {
		m = rect.Max.Y
	}
	m1 := (m + draw.ICOSSCALE - 1) >> 10
	m = m1 << 10 /* assure m1*cossin is inside */
	i := 0
	var bnd [8]draw.Point
	bnd[i] = draw.Pt(0, 0)
	i++
	var p draw.Point
	p.X, p.Y = draw.IntCosSin(alpha)
	bnd[i] = p.Mul(m1)
	i++
	for {
		bnd[i] = corners[c1].Mul(m)
		i++
		if c1 == c2 && phi < 180 {
			break
		}
		c1 = (c1 + 1) & 3
		phi -= 90
	}
	p.X, p.Y = draw.IntCosSin(beta)
	bnd[i] = p.Mul(m1)
	i++

	var figure, mask *Image
	wedge, err := AllocImage(rect, draw.GREY1)
	if err != nil {
		goto Return
	}
	FillColor(wedge, draw.Transparent)
	FillPoly(wedge, bnd[:i], ^0, Opaque, p00, draw.S)
	figure, err = AllocImage(rect, draw.GREY1)
	if err != nil {
		goto Return
	}
	FillColor(figure, draw.Transparent)
	Ellipse(figure, p00, a, b, t, Opaque, p00, draw.S)
	mask, err = AllocImage(rect, draw.GREY1)
	if err != nil {
		goto Return
	}
	FillColor(mask, draw.Transparent)
	mask.Draw(rect, figure, rect.Min, wedge, rect.Min, draw.S)
	c = c.Sub(dst.R.Min)
	Draw(dst, dst.R, src, sp.Sub(c), mask, p00.Sub(c), op)

Return:
	Free(wedge)
	Free(figure)
	Free(mask)
}
