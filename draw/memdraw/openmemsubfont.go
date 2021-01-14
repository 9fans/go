// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"fmt"
	"io"
	"os"

	"9fans.net/go/draw"
)

func openmemsubfont(name string) (*subfont, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	i, err := readmemimage(f)
	if err != nil {
		return nil, err
	}
	var hdr [3*12 + 4]byte
	if _, err := io.ReadFull(f, hdr[:3*12]); err != nil {
		Free(i)
		return nil, fmt.Errorf("openmemsubfont: header read error: %v", err)
	}
	n := atoi(hdr[:1*12])
	p := make([]byte, 6*(n+1))
	if _, err := io.ReadFull(f, p[:6*(n+1)]); err != nil {
		Free(i)
		return nil, fmt.Errorf("openmemsubfont: fontchar read error: %v", err)
	}

	fc := make([]draw.Fontchar, n+1)
	unpackinfo(fc, p, n)
	sf := allocmemsubfont(name, n, atoi(hdr[1*12:2*12]), atoi(hdr[2*12:3*12]), fc, i)
	return sf, nil
}

func unpackinfo(fc []draw.Fontchar, p []byte, n int) {
	for j := 0; j <= n; j++ {
		fc[j].X = int(p[0]) | int(p[1])<<8
		fc[j].Top = uint8(p[2])
		fc[j].Bottom = uint8(p[3])
		fc[j].Left = int8(p[4])
		fc[j].Width = uint8(p[5])
		p = p[6:]
	}
}
