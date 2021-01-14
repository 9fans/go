// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"9fans.net/go/draw"
)

func atoi(s []byte) int {
	i, _ := strconv.Atoi(strings.TrimSpace(string(s)))
	return i
}

var ldepthToPix = []draw.Pix{
	draw.GREY1,
	draw.GREY2,
	draw.GREY4,
	draw.CMAP8,
}

func creadmemimage(fd io.Reader) (*Image, error) {
	hdr := make([]byte, 5*12)
	if _, err := io.ReadFull(fd, hdr); err != nil {
		return nil, err
	}

	/*
	 * distinguish new channel descriptor from old ldepth.
	 * channel descriptors have letters as well as numbers,
	 * while ldepths are a single digit formatted as %-11d.
	 */
	new := 0
	for m := 0; m < 10; m++ {
		if hdr[m] != ' ' {
			new = 1
			break
		}
	}
	if hdr[11] != ' ' {
		return nil, fmt.Errorf("creadimage: bad format")
	}
	var chan_ draw.Pix
	if new != 0 {
		s := strings.TrimSpace(string(hdr[:11]))
		var err error
		chan_, err = draw.ParsePix(s)
		if err != nil {
			return nil, fmt.Errorf("creadimage: bad channel string %s", s)
		}
	} else {
		ldepth := (int(hdr[10])) - '0'
		if ldepth < 0 || ldepth > 3 {
			return nil, fmt.Errorf("creadimage: bad ldepth %d", ldepth)
		}
		chan_ = ldepthToPix[ldepth]
	}
	var r draw.Rectangle
	r.Min.X = atoi(hdr[1*12 : 2*12])
	r.Min.Y = atoi(hdr[2*12 : 3*12])
	r.Max.X = atoi(hdr[3*12 : 4*12])
	r.Max.Y = atoi(hdr[4*12 : 5*12])
	if r.Min.X > r.Max.X || r.Min.Y > r.Max.Y {
		return nil, fmt.Errorf("creadimage: bad rectangle")
	}

	i, err := AllocImage(r, chan_)
	if err != nil {
		return nil, err
	}
	ncblock := compblocksize(r, i.Depth)
	buf := make([]byte, ncblock)
	miny := r.Min.Y
	for miny != r.Max.Y {
		if _, err := io.ReadFull(fd, hdr[:2*12]); err != nil {
			Free(i)
			return nil, err
		}
		maxy := atoi(hdr[0*12 : 1*12])
		nb := atoi(hdr[1*12 : 2*12])
		if maxy <= miny || r.Max.Y < maxy {
			Free(i)
			return nil, fmt.Errorf("readimage: bad maxy %d", maxy)
		}
		if nb <= 0 || ncblock < nb {
			Free(i)
			return nil, fmt.Errorf("readimage: bad count %d", nb)
		}
		if _, err := io.ReadFull(fd, buf[:nb]); err != nil {
			Free(i)
			return nil, err
		}
		if new == 0 {
			twiddlecompressed(buf[:nb])
		}
		cloadmemimage(i, draw.Rect(r.Min.X, miny, r.Max.X, maxy), buf[:nb])
		miny = maxy
	}
	return i, nil
}

func compblocksize(r draw.Rectangle, depth int) int {
	bpl := draw.BytesPerLine(r, depth)
	bpl = 2 * bpl /* add plenty extra for blocking, etc. */
	if bpl < _NCBLOCK {
		return _NCBLOCK
	}
	return bpl
}

func twiddlecompressed(buf []byte) {
	i := 0
	for i < len(buf) {
		c := buf[i]
		i++
		if c >= 0x80 {
			k := int(c) - 0x80 + 1
			for j := 0; j < k && i < len(buf); j++ {
				buf[i] ^= 0xFF
				i++
			}
		} else {
			i++
		}
	}
}
