package draw

import "image"

func (dst *Image) Line(p0, p1 image.Point, end0, end1, radius int, src *Image, sp image.Point) {
	dst.LineOp(p0, p1, end0, end1, radius, src, sp, SoverD)
}

func (dst *Image) LineOp(p0, p1 image.Point, end0, end1, radius int, src *Image, sp image.Point, op Op) {
	setdrawop(dst.Display, op)
	a := dst.Display.bufimage(1 + 4 + 2*4 + 2*4 + 4 + 4 + 4 + 4 + 2*4)
	a[0] = 'L'
	bplong(a[1:], uint32(dst.ID))
	bplong(a[5:], uint32(p0.X))
	bplong(a[9:], uint32(p0.Y))
	bplong(a[13:], uint32(p1.X))
	bplong(a[17:], uint32(p1.Y))
	bplong(a[21:], uint32(end0))
	bplong(a[25:], uint32(end1))
	bplong(a[29:], uint32(radius))
	bplong(a[33:], uint32(src.ID))
	bplong(a[37:], uint32(sp.X))
	bplong(a[41:], uint32(sp.Y))
}
