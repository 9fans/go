// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"fmt"

	"9fans.net/go/draw"
)

func unloadmemimage(i *Image, r draw.Rectangle, data []uint8) (int, error) {
	if !draw.RectInRect(r, i.R) {
		return 0, fmt.Errorf("invalid rectangle")
	}
	l := draw.BytesPerLine(r, i.Depth)
	if len(data) < l*r.Dy() {
		return 0, fmt.Errorf("buffer too small")
	}
	ndata := l * r.Dy()
	q := byteaddr(i, r.Min)
	for y := r.Min.Y; y < r.Max.Y; y++ {
		copy(data[:l], q[:l])
		q = q[i.Width*4:]
		data = data[l:]
	}
	return ndata, nil
}
