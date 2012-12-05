package draw

import (
	"image"
	"image/color"
)

/*
 * Support for the Image type so it can satisfy the standard Color and Image interfaces.
 */

// At returns the standard Color value for the pixel at (x, y).
// If the location is outside the clipping rectangle, it returns color.Transparent.
// This operation does a round trip to the image server and can be expensive.
func (i *Image) At(x, y int) color.Color {
	if !(image.Point{x, y}.In(i.Clipr)) {
		return color.Transparent
	}
	if i.Repl && !(image.Point{x, y}.In(i.R)) {
		// Translate (x, y) to be within i.R.
		x = (x-i.R.Min.X)%(i.R.Max.X-i.R.Min.X) + i.R.Min.X
		y = (y-i.R.Min.Y)%(i.R.Max.Y-i.R.Min.Y) + i.R.Min.Y
	}
	var buf [4]byte
	_, err := i.Unload(image.Rect(x, y, x+1, y+1), buf[:])
	if err != nil {
		println("image.At: error in Unload: ", err.Error())
		return color.Transparent // As good a value as any.
	}
	// For multi-byte pixels, the ordering is little-endian.
	switch i.Pix {
	case GREY1:
		// CGrey, 1
		if buf[0] == 0 {
			return color.Black
		}
		return color.White
	case GREY2:
		// CGrey, 2
		off := uint(3 - x&3)
		// Place pixel at bottom of word.
		c := (uint16(buf[0]) >> (off << 1)) & 0x3
		// Replicate throughout.
		c |= c << 2
		c |= c << 4
		return color.Gray16{c}
	case GREY4:
		// CGrey 4
		// Place pixel at bottom of word.
		c := uint16(buf[0])
		if x&1 == 0 {
			c >>= 4
		}
		c &= 0xF
		// Replicate throughout.
		c |= c << 4
		return color.Gray16{c}
	case GREY8:
		// CGrey, 8
		c := uint16(buf[0])
		c |= c << 8
		return color.Gray16{c}
	case CMAP8:
		// CMap, 8
		red, grn, blu := cmap2rgb(int(buf[0]))
		return color.RGBA{uint8(red), uint8(grn), uint8(blu), 0xFF}
	case RGB15:
		// CIgnore, 1, CRed, 5, CGreen, 5, CBlue, 5
		red := (buf[1] << 1) & 0xF8
		grn := buf[1] << 6
		grn |= (buf[0] & 0xE0) >> 2
		grn &= 0xF8
		blu := buf[0] << 3
		return color.RGBA{red | red>>5, grn | grn>>5, blu | blu>>5, 0xFF}
	case RGB16:
		// CRed, 5, CGreen, 6, CBlue, 5
		red := buf[1] & 0xF8
		grn := buf[1] << 5
		grn |= (buf[0] & 0xE0) >> 3
		grn &= 0xF8
		blu := buf[0] << 3
		return color.RGBA{red | red>>5, grn | grn>>5, blu | blu>>5, 0xFF}
	case RGB24:
		// CRed, 8, CGreen, 8, CBlue, 8
		return color.RGBA{buf[2], buf[1], buf[0], 0xFF}
	case BGR24:
		// CBlue, 8, CGreen, 8, CRed, 8
		return color.RGBA{buf[0], buf[1], buf[2], 0xFF}
	case RGBA32:
		// CRed, 8, CGreen, 8, CBlue, 8, CAlpha, 8
		return color.RGBA{buf[3], buf[2], buf[1], buf[0]}
	case ARGB32:
		// CAlpha, 8, CRed, 8, CGreen, 8, CBlue, 8 // stupid VGAs
		return color.RGBA{buf[2], buf[1], buf[0], buf[3]}
	case ABGR32:
		// CAlpha, 8, CBlue, 8, CGreen, 8, CRed, 8
		return color.RGBA{buf[0], buf[1], buf[2], buf[3]}
	case XRGB32:
		// CIgnore, 8, CRed, 8, CGreen, 8, CBlue, 8
		return color.RGBA{buf[2], buf[1], buf[0], 0xFF}
	case XBGR32:
		// CIgnore, 8, CBlue, 8, CGreen, 8, CRed, 8
		return color.RGBA{buf[0], buf[1], buf[2], 0xFF}
	default:
		panic("unknown color")
	}
	panic("unimplemented image type for At")
}

func (i *Image) Bounds() image.Rectangle {
	return i.Clipr
}

func (i *Image) ColorModel() color.Model {
	// TODO: use ModelFunc for this
	panic("unimplemented")
}
