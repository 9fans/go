// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"9fans.net/go/draw"
)

func (i *Image) Draw(r draw.Rectangle, src *Image, sp draw.Point, mask *Image, mp draw.Point, op draw.Op) {
	memimagedraw(i, r, src, sp, mask, mp, op)
}

func memimagedraw(dst *Image, r draw.Rectangle, src *Image, sp draw.Point, mask *Image, mp draw.Point, op draw.Op) {
	par := _memimagedrawsetup(dst, r, src, sp, mask, mp, op)
	if par == nil {
		return
	}
	_memimagedraw(par)
}

func FillColor(m *Image, val draw.Color) {
	_memfillcolor(m, val)
}

func pixelbits(m *Image, p draw.Point) uint32 {
	return _pixelbits(m, p)
}
