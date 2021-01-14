// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import (
	"fmt"

	"9fans.net/go/draw"
)

func Unload(src *Image, r draw.Rectangle, data []uint8) (int, error) {

Top:
	dl := src.Layer
	if dl == nil {
		return unloadmemimage(src, r, data)
	}

	/*
	 * Convert to screen coordinates.
	 */
	lr := r
	r.Min.X += dl.Delta.X
	r.Min.Y += dl.Delta.Y
	r.Max.X += dl.Delta.X
	r.Max.Y += dl.Delta.Y
	dx := dl.Delta.X & (7 / src.Depth)
	if dl.clear && dx == 0 {
		src = dl.Screen.Image
		goto Top
	}

	/*
	 * src is an obscured layer or data is unaligned
	 */
	if dl.save != nil && dx == 0 {
		if dl.Refreshfn != nil {
			return 0, fmt.Errorf("window not Refbackup") /* can't unload window if it's not Refbackup */
		}
		if len(data) > 0 {
			memlhide(src, r)
		}
		return unloadmemimage(dl.save, lr, data)
	}
	tmp, err := AllocImage(lr, src.Pix)
	if err != nil {
		return 0, err
	}
	Draw(tmp, lr, src, lr.Min, nil, lr.Min, draw.S)
	n, err := unloadmemimage(tmp, lr, data)
	Free(tmp)
	return n, err
}
