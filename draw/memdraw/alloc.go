// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

// #define poolalloc(a, b) malloc(b)
// #define poolfree(a, b) free(b)

package memdraw

import (
	"fmt"

	"9fans.net/go/draw"
)

func allocmemimaged(r draw.Rectangle, chan_ draw.Pix, md *_Memdata, X interface{}) (*Image, error) {
	if r.Dx() <= 0 || r.Dy() <= 0 {
		return nil, fmt.Errorf("bad rectangle %v", r)
	}
	d := chan_.Depth()
	if d == 0 {
		return nil, fmt.Errorf("bad channel descriptor %.8x", chan_)
	}

	l := draw.WordsPerLine(r, d)
	i := new(Image)

	i.X = X
	i.Data = md
	i.zero = 4 * l * r.Min.Y

	if r.Min.X >= 0 {
		i.zero += (r.Min.X * d) / 8
	} else {
		i.zero -= (-r.Min.X*d + 7) / 8
	}
	i.zero = -i.zero
	i.Width = uint32(l)
	i.R = r
	i.Clipr = r
	i.Flags = 0
	i.Layer = nil
	i.cmap = memdefcmap
	if err := memsetchan(i, chan_); err != nil {
		return nil, err
	}
	return i, nil
}

func AllocImage(r draw.Rectangle, chan_ draw.Pix) (*Image, error) {
	d := chan_.Depth()
	if d == 0 {
		return nil, fmt.Errorf("bad channel descriptor %.8x", chan_)
	}

	l := draw.WordsPerLine(r, d)
	nw := l * r.Dy()
	md := new(_Memdata)
	md.ref = 1
	md.Bdata = make([]byte, (1+nw)*4)

	i, err := allocmemimaged(r, chan_, md, nil)
	if err != nil {
		return nil, err
	}
	md.imref = i
	return i, nil
}

func byteaddr(i *Image, p draw.Point) []uint8 {
	return i.BytesAt(p)
}

func (i *Image) BytesAt(p draw.Point) []uint8 {
	/* careful to sign-extend negative p.y for 64-bits */
	a := i.zero + 4*p.Y*int(i.Width)

	if i.Depth < 8 {
		np := 8 / i.Depth
		if p.X < 0 {
			a += (p.X - np + 1) / np
		} else {
			a += p.X / np
		}
	} else {
		a += p.X * (i.Depth / 8)
	}
	return i.Data.Bdata[a:]
}

func memsetchan(i *Image, chan_ draw.Pix) error {
	d := chan_.Depth()
	if d == 0 {
		return fmt.Errorf("bad channel descriptor")
	}

	i.Depth = d
	i.Pix = chan_
	i.Flags &^= Fgrey | Falpha | Fcmap | Fbytes
	bytes := 1
	cc := chan_
	j := uint(0)
	k := 0
	for ; cc != 0; func() { j += _NBITS(cc); cc >>= 8; k++ }() {
		t := _TYPE(cc)
		if t < 0 || t >= draw.NChan {
			return fmt.Errorf("bad channel string")
		}
		if t == draw.CGrey {
			i.Flags |= Fgrey
		}
		if t == draw.CAlpha {
			i.Flags |= Falpha
		}
		if t == draw.CMap && i.cmap == nil {
			i.cmap = memdefcmap
			i.Flags |= Fcmap
		}

		i.shift[t] = j
		i.mask[t] = (1 << _NBITS(cc)) - 1
		i.nbits[t] = _NBITS(cc)
		if _NBITS(cc) != 8 {
			bytes = 0
		}
	}
	i.nchan = k
	if bytes != 0 {
		i.Flags |= Fbytes
	}
	return nil
}

func Free(i *Image) {
}
