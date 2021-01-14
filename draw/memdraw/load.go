// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"fmt"

	"9fans.net/go/draw"
)

func loadmemimage(i *Image, r draw.Rectangle, data []uint8) (int, error) {
	if !draw.RectInRect(r, i.R) {
		return 0, fmt.Errorf("invalid rectangle")
	}
	l := draw.BytesPerLine(r, i.Depth)
	if len(data) < l*r.Dy() {
		return 0, fmt.Errorf("buffer too small")
	}
	ndata := l * r.Dy()
	q := byteaddr(i, r.Min)
	mx := 7 / i.Depth
	lpart := (r.Min.X & mx) * i.Depth
	rpart := (r.Max.X & mx) * i.Depth
	m := uint8(0xFF) >> lpart
	var y int
	/* may need to do bit insertion on edges */
	if l == 1 { /* all in one byte */
		if rpart != 0 {
			m ^= 0xFF >> rpart
		}
		for y = r.Min.Y; y < r.Max.Y; y++ {
			q[0] ^= (data[0] ^ q[0]) & m
			q = q[i.Width*4:]
			data = data[1:]
		}
		return ndata, nil
	}
	if lpart == 0 && rpart == 0 { /* easy case */
		for y = r.Min.Y; y < r.Max.Y; y++ {
			copy(q[:l], data[:l])
			q = q[i.Width*4:]
			data = data[l:]
		}
		return ndata, nil
	}
	mr := uint8(0xFF) ^ (0xFF >> rpart)
	if lpart != 0 && rpart == 0 {
		for y = r.Min.Y; y < r.Max.Y; y++ {
			q[0] ^= (data[0] ^ q[0]) & m
			if l > 1 {
				copy(q[1:l], data[1:l])
			}
			q = q[i.Width*4:]
			data = data[l:]
		}
		return ndata, nil
	}
	if lpart == 0 && rpart != 0 {
		for y = r.Min.Y; y < r.Max.Y; y++ {
			if l > 1 {
				copy(q[:l-1], data[:l-1])
			}
			q[l-1] ^= (data[l-1] ^ q[l-1]) & mr
			q = q[i.Width*4:]
			data = data[l:]
		}
		return ndata, nil
	}
	for y = r.Min.Y; y < r.Max.Y; y++ {
		q[0] ^= (data[0] ^ q[0]) & m
		if l > 2 {
			copy(q[1:l-1], data[1:l-1])
		}
		q[l-1] ^= (data[l-1] ^ q[l-1]) & mr
		q = q[i.Width*4:]
		data = data[l:]
	}
	return ndata, nil
}
