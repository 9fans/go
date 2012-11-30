package draw

import (
	"image"
)

type Op int

const (
	/* Porter-Duff compositing operators */
	Clear Op = 0

	SinD  Op = 8
	DinS  Op = 4
	SoutD Op = 2
	DoutS Op = 1

	S      = SinD | SoutD
	SoverD = SinD | SoutD | DoutS
	SatopD = SinD | DoutS
	SxorD  = SoutD | DoutS

	D      = DinS | DoutS
	DoverS = DinS | DoutS | SoutD
	DatopS = DinS | SoutD
	DxorS  = DoutS | SoutD /* == SxorD */

	Ncomp = 12
)

func setdrawop(d *Display, op Op) {
	if op != SoverD {
		a := d.bufimage(2)
		a[0] = 'O'
		a[1] = byte(op)
	}
}

func draw1(dst *Image, r image.Rectangle, src *Image, p0 image.Point, mask *Image, p1 image.Point, op Op) {
	setdrawop(dst.Display, op)

	a := dst.Display.bufimage(1 + 4 + 4 + 4 + 4*4 + 2*4 + 2*4)
	if src == nil {
		src = dst.Display.Black
	}
	if mask == nil {
		mask = dst.Display.Opaque
	}
	a[0] = 'd'
	bplong(a[1:], dst.ID)
	bplong(a[5:], src.ID)
	bplong(a[9:], mask.ID)
	bplong(a[13:], uint32(r.Min.X))
	bplong(a[17:], uint32(r.Min.Y))
	bplong(a[21:], uint32(r.Max.X))
	bplong(a[25:], uint32(r.Max.Y))
	bplong(a[29:], uint32(p0.X))
	bplong(a[33:], uint32(p0.Y))
	bplong(a[37:], uint32(p1.X))
	bplong(a[41:], uint32(p1.Y))
}

func (dst *Image) Draw(r image.Rectangle, src, mask *Image, p1 image.Point) {
	draw1(dst, r, src, p1, mask, p1, SoverD)
}

func (dst *Image) DrawOp(r image.Rectangle, src, mask *Image, p1 image.Point, op Op) {
	draw1(dst, r, src, p1, mask, p1, op)
}

func (dst *Image) GenDraw(r image.Rectangle, src *Image, p0 image.Point, mask *Image, p1 image.Point) {
	draw1(dst, r, src, p0, mask, p1, SoverD)
}

func GenDrawOp(dst *Image, r image.Rectangle, src *Image, p0 image.Point, mask *Image, p1 image.Point, op Op) {
	draw1(dst, r, src, p0, mask, p1, op)
}
