// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"fmt"

	"9fans.net/go/draw"
)

// Compressed image file parameters.
const (
	_NMATCH  = 3              /* shortest match possible */
	_NRUN    = (_NMATCH + 31) /* longest match possible */
	_NMEM    = 1024           /* window size */
	_NDUMP   = 128            /* maximum length of dump */
	_NCBLOCK = 6000           /* size of compressed blocks */
)

func cloadmemimage(i *Image, r draw.Rectangle, data []uint8) (int, error) {
	if !draw.RectInRect(r, i.R) {
		return 0, fmt.Errorf("invalid rectangle")
	}
	bpl := draw.BytesPerLine(r, i.Depth)
	u := data
	var mem [_NMEM]byte
	memp := mem[:]
	y := r.Min.Y
	linep := byteaddr(i, draw.Pt(r.Min.X, y))
	linep = linep[:bpl]
	for {
		if len(linep) == 0 {
			y++
			if y == r.Max.Y {
				break
			}
			linep = byteaddr(i, draw.Pt(r.Min.X, y))
			linep = linep[:bpl]
		}
		if len(u) == 0 { /* buffer too small */
			return len(data) - len(u), fmt.Errorf("buffer too small")
		}
		c := u[0]
		u = u[1:]
		if c >= 128 {
			for cnt := c - 128 + 1; cnt != 0; cnt-- {
				if len(u) == 0 { /* buffer too small */
					return len(data) - len(u), fmt.Errorf("buffer too small")
				}
				if len(linep) == 0 { /* phase error */
					return len(data) - len(u), fmt.Errorf("phase error")
				}
				linep[0] = u[0]
				linep = linep[1:]
				memp[0] = u[0]
				memp = memp[1:]
				u = u[1:]
				if len(memp) == 0 {
					memp = mem[:]
				}
			}
		} else {
			if len(u) == 0 { /* short buffer */
				return len(data) - len(u), fmt.Errorf("buffer too small")
			}
			offs := int(u[0]) + (int(c&3) << 8) + 1
			u = u[1:]
			var omemp []byte
			if δ := len(mem) - len(memp); δ < offs {
				omemp = mem[δ+(_NMEM-offs):]
			} else {
				omemp = mem[δ-offs:]
			}
			for cnt := (c >> 2) + _NMATCH; cnt != 0; cnt-- {
				if len(linep) == 0 { /* phase error */
					return len(data) - len(u), fmt.Errorf("phase error")
				}
				linep[0] = omemp[0]
				linep = linep[1:]
				memp[0] = omemp[0]
				memp = memp[1:]
				omemp = omemp[1:]
				if len(omemp) == 0 {
					omemp = mem[:]
				}
				if len(memp) == 0 {
					memp = mem[:]
				}
			}
		}
	}
	return len(data) - len(u), nil
}
