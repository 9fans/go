package draw

import (
	"fmt"
)

type Color uint32

const (
	DOpaque        Color = 0xFFFFFFFF
	DTransparent   Color = 0x00000000 /* only useful for allocimage memfillcolor */
	DBlack         Color = 0x000000FF
	DWhite         Color = 0xFFFFFFFF
	DRed           Color = 0xFF0000FF
	DGreen         Color = 0x00FF00FF
	DBlue          Color = 0x0000FFFF
	DCyan          Color = 0x00FFFFFF
	DMagenta       Color = 0xFF00FFFF
	DYellow        Color = 0xFFFF00FF
	DPaleyellow    Color = 0xFFFFAAFF
	DDarkyellow    Color = 0xEEEE9EFF
	DDarkgreen     Color = 0x448844FF
	DPalegreen     Color = 0xAAFFAAFF
	DMedgreen      Color = 0x88CC88FF
	DDarkblue      Color = 0x000055FF
	DPalebluegreen Color = 0xAAFFFFFF
	DPaleblue      Color = 0x0000BBFF
	DBluegreen     Color = 0x008888FF
	DGreygreen     Color = 0x55AAAAFF
	DPalegreygreen Color = 0x9EEEEEFF
	DYellowgreen   Color = 0x99994CFF
	DMedblue       Color = 0x000099FF
	DGreyblue      Color = 0x005DBBFF
	DPalegreyblue  Color = 0x4993DDFF
	DPurpleblue    Color = 0x8888CCFF

	DNotacolor Color = 0xFFFFFF00
	DNofill    Color = DNotacolor
)

type Pix uint32

const (
	CRed = iota
	CGreen
	CBlue
	CGrey
	CAlpha
	CMap
	CIgnore
	NChan
)

var (
	GREY1  Pix = MakePix(CGrey, 1)
	GREY2  Pix = MakePix(CGrey, 2)
	GREY4  Pix = MakePix(CGrey, 4)
	GREY8  Pix = MakePix(CGrey, 8)
	CMAP8  Pix = MakePix(CMap, 8)
	RGB15  Pix = MakePix(CIgnore, 1, CRed, 5, CGreen, 5, CBlue, 5)
	RGB16      = MakePix(CRed, 5, CGreen, 6, CBlue, 5)
	RGB24      = MakePix(CRed, 8, CGreen, 8, CBlue, 8)
	BGR24      = MakePix(CBlue, 8, CGreen, 8, CRed, 8)
	RGBA32     = MakePix(CRed, 8, CGreen, 8, CBlue, 8, CAlpha, 8)
	ARGB32     = MakePix(CAlpha, 8, CRed, 8, CGreen, 8, CBlue, 8) // stupid VGAs
	ABGR32     = MakePix(CAlpha, 8, CBlue, 8, CGreen, 8, CRed, 8)
	XRGB32     = MakePix(CIgnore, 8, CRed, 8, CGreen, 8, CBlue, 8)
	XBGR32     = MakePix(CIgnore, 8, CBlue, 8, CGreen, 8, CRed, 8)
)

func MakePix(list ...int) Pix {
	var p Pix
	for _, x := range list {
		p <<= 4
		p |= Pix(x)
	}
	return p
}

func ParsePix(s string) (Pix, error) {
	var p Pix
	s0 := s
	if len(s) > 8 {
		goto Malformed
	}
	for ; len(s) > 0; s = s[2:] {
		if len(s) == 1 {
			goto Malformed
		}
		p <<= 4
		switch s[0] {
		default:
			goto Malformed
		case 'r':
			p |= CRed
		case 'g':
			p |= CGreen
		case 'b':
			p |= CBlue
		case 'a':
			p |= CAlpha
		case 'k':
			p |= CGrey
		case 'm':
			p |= CMap
		case 'x':
			p |= CIgnore
		}
		p <<= 4
		if s[1] < '1' || s[1] > '8' {
			goto Malformed
		}
		p |= Pix(s[1] - '0')
	}
	return p, nil

Malformed:
	return 0, fmt.Errorf("malformed pix descriptor %q", s0)
}

func (p Pix) String() string {
	var buf [8]byte
	i := len(buf)
	if p == 0 {
		return "0"
	}
	for p > 0 {
		i -= 2
		buf[i] = "rgbkamxzzzzzzzzz"[(p>>4)&15]
		buf[i+1] = "0123456789abcdef"[p&15]
		p >>= 8
	}
	return string(buf[i:])
}

func (p Pix) Depth() int {
	n := 0
	for p > 0 {
		n += int(p & 15)
		p >>= 8
	}
	return n
}
