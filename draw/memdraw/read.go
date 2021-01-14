package memdraw

import (
	"fmt"
	"io"
	"os"

	"9fans.net/go/draw"
)

func readmemimage(fd *os.File) (*Image, error) {
	var hdr [5*12 + 1]byte
	if _, err := io.ReadFull(fd, hdr[:11]); err != nil {
		return nil, fmt.Errorf("readimage: %v", err)
	}
	if string(hdr[:11]) == "compressed\n" {
		return creadmemimage(fd)
	}
	if _, err := io.ReadFull(fd, hdr[11:5*12]); err != nil {
		return nil, fmt.Errorf("readimage: %v", err)
	}

	/*
	 * distinguish new channel descriptor from old ldepth.
	 * channel descriptors have letters as well as numbers,
	 * while ldepths are a single digit formatted as %-11d.
	 */
	new := false
	var m int
	for m = 0; m < 10; m++ {
		if hdr[m] != ' ' {
			new = true
			break
		}
	}
	if hdr[11] != ' ' {
		return nil, fmt.Errorf("readimage: bad format")
	}
	var chan_ draw.Pix
	if new {
		s := string(hdr[:11])
		var err error
		chan_, err = draw.ParsePix(s)
		if err != nil {
			return nil, fmt.Errorf("readimage: %v", err)
		}
	} else {
		ldepth := (int(hdr[10])) - '0'
		if ldepth < 0 || ldepth > 3 {
			return nil, fmt.Errorf("readimage: bad ldepth %d", ldepth)
		}
		chan_ = ldepthToPix[ldepth]
	}
	var r draw.Rectangle

	r.Min.X = atoi(hdr[1*12 : 2*12])
	r.Min.Y = atoi(hdr[2*12 : 3*12])
	r.Max.X = atoi(hdr[3*12 : 4*12])
	r.Max.Y = atoi(hdr[4*12 : 5*12])
	if r.Min.X > r.Max.X || r.Min.Y > r.Max.Y {
		return nil, fmt.Errorf("readimage: bad rectangle")
	}

	miny := r.Min.Y
	maxy := r.Max.Y

	l := draw.BytesPerLine(r, chan_.Depth())
	i, err := AllocImage(r, chan_)
	if err != nil {
		return nil, err
	}
	chunk := 32 * 1024
	if chunk < l {
		chunk = l
	}
	tmp := make([]byte, chunk)
	for maxy > miny {
		dy := maxy - miny
		if dy*l > chunk {
			dy = chunk / l
		}
		if dy <= 0 {
			Free(i)
			return nil, fmt.Errorf("readmemimage: image too wide for buffer")
		}
		n := dy * l
		if _, err = io.ReadFull(fd, tmp[:n]); err != nil {
			Free(i)
			return nil, fmt.Errorf("readmemimage: %v", err)
		}
		if !new {
			for j := 0; j < chunk; j++ {
				tmp[j] ^= 0xFF
			}
		}

		if _, err := loadmemimage(i, draw.Rect(r.Min.X, miny, r.Max.X, miny+dy), tmp[:n]); err != nil {
			Free(i)
			return nil, err
		}
		miny += dy
	}
	return i, nil
}
