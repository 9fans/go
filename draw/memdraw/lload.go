// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import "9fans.net/go/draw"

func Load(dst *Image, r draw.Rectangle, data []uint8, iscompressed bool) (int, error) {
	loadfn := loadmemimage
	if iscompressed {
		loadfn = cloadmemimage
	}

Top:
	dl := dst.Layer
	if dl == nil {
		return loadfn(dst, r, data)
	}

	/*
	 * Convert to screen coordinates.
	 */
	lr := r
	r.Min.X += dl.Delta.X
	r.Min.Y += dl.Delta.Y
	r.Max.X += dl.Delta.X
	r.Max.Y += dl.Delta.Y
	dx := dl.Delta.X & (7 / dst.Depth)
	if dl.clear && dx == 0 {
		dst = dl.Screen.Image
		goto Top
	}

	/*
	 * dst is an obscured layer or data is unaligned
	 */
	if dl.save != nil && dx == 0 {
		n, err := loadfn(dl.save, lr, data)
		if n > 0 {
			memlexpose(dst, r)
		}
		return n, err
	}
	tmp, err := AllocImage(lr, dst.Pix)
	if err != nil {
		return 0, err
	}
	n, err := loadfn(tmp, lr, data)
	Draw(dst, lr, tmp, lr.Min, nil, lr.Min, draw.S)
	Free(tmp)
	return n, err
}
