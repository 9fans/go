package draw

import (
	"image"
)

func doellipse(cmd byte, dst *Image, c image.Point, xr, yr, thick int, src *Image, sp image.Point, alpha uint32, phi int, op Op) {
	setdrawop(dst.Display, op)
	a := dst.Display.bufimage(1 + 4 + 4 + 2*4 + 4 + 4 + 4 + 2*4 + 2*4)
	a[0] = cmd
	bplong(a[1:], dst.ID)
	bplong(a[5:], src.ID)
	bplong(a[9:], uint32(c.X))
	bplong(a[13:], uint32(c.Y))
	bplong(a[17:], uint32(xr))
	bplong(a[21:], uint32(yr))
	bplong(a[25:], uint32(thick))
	bplong(a[29:], uint32(sp.X))
	bplong(a[33:], uint32(sp.Y))
	bplong(a[37:], alpha)
	bplong(a[41:], uint32(phi))
}

func (dst *Image) Ellipse(c image.Point, a, b, thick int, src *Image, sp image.Point) {
	doellipse('e', dst, c, a, b, thick, src, sp, 0, 0, SoverD)
}

func (dst *Image) EllipseOp(c image.Point, a, b, thick int, src *Image, sp image.Point, op Op) {
	doellipse('e', dst, c, a, b, thick, src, sp, 0, 0, op)
}

func (dst *Image) FillEllipse(c image.Point, a, b, thick int, src *Image, sp image.Point) {
	doellipse('E', dst, c, a, b, thick, src, sp, 0, 0, SoverD)
}

func (dst *Image) FillEllipseOp(c image.Point, a, b, thick int, src *Image, sp image.Point, op Op) {
	doellipse('E', dst, c, a, b, thick, src, sp, 0, 0, op)
}

func (dst *Image) Arc(c image.Point, a, b, thick int, src *Image, sp image.Point, alpha, phi int) {
	doellipse('e', dst, c, a, b, thick, src, sp, uint32(alpha)|1<<31, phi, SoverD)
}

func (dst *Image) ArcOp(c image.Point, a, b, thick int, src *Image, sp image.Point, alpha, phi int, op Op) {
	doellipse('e', dst, c, a, b, thick, src, sp, uint32(alpha)|1<<31, phi, op)
}

func (dst *Image) FillArc(c image.Point, a, b, thick int, src *Image, sp image.Point, alpha, phi int) {
	doellipse('E', dst, c, a, b, thick, src, sp, uint32(alpha)|1<<31, phi, SoverD)
}

func (dst *Image) FillArcOp(c image.Point, a, b, thick int, src *Image, sp image.Point, alpha, phi int, op Op) {
	doellipse('E', dst, c, a, b, thick, src, sp, uint32(alpha)|1<<31, phi, op)
}
