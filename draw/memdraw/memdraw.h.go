package memdraw

import "9fans.net/go/draw"

/*
 * Memdata is allocated from main pool, but .data from the image pool.
 * Memdata is allocated separately to permit patching its pointer after
 * compaction when windows share the image data.
 * The first word of data is a back pointer to the Memdata, to find
 * The word to patch.
 */

type _Memdata struct {
	Bdata []uint8 /* pointer to first byte of actual data; word-aligned */
	ref   int     /* number of Memimages using this data */
	imref *Image
}

const (
	Frepl   = 1 << 0 /* is replicated */
	Fsimple = 1 << 1 /* is 1x1 */
	Fgrey   = 1 << 2 /* is grey */
	Falpha  = 1 << 3 /* has explicit alpha */
	Fcmap   = 1 << 4 /* has cmap channel */
	Fbytes  = 1 << 5 /* has only 8-bit channels */
)

type Image struct {
	R     draw.Rectangle /* rectangle in data area, local coords */
	Clipr draw.Rectangle /* clipping region */
	Depth int            /* number of bits of storage per pixel */
	nchan int            /* number of channels */
	Pix   draw.Pix       /* channel descriptions */
	cmap  *_CMap

	Data      *_Memdata /* pointer to data; shared by windows in this image */
	zero      int       /* data->bdata+zero==&byte containing (0,0) */
	Width     uint32    /* width in words of a single scan line */
	Layer     *Layer    /* nil if not a layer*/
	Flags     uint32
	X         interface{}
	ScreenRef int /* reference count if this is a screen */

	shift [draw.NChan]uint
	mask  [draw.NChan]uint32
	nbits [draw.NChan]uint
}

type _CMap struct {
	Cmap2rgb [3 * 256]uint8
	Rgb2cmap [16 * 16 * 16]uint8
}

/*
 * Subfonts
 *
 * given char c, Subfont *f, Fontchar *i, and Point p, one says
 *	i = f->info+c;
 *	draw(b, Rect(p.x+i->left, p.y+i->top,
 *		p.x+i->left+((i+1)->x-i->x), p.y+i->bottom),
 *		color, f->bits, Pt(i->x, i->top));
 *	p.x += i->width;
 * to draw characters in the specified color (itself a Memimage) in Memimage b.
 */

type subfont struct {
	name   string
	n      int             /* number of chars in font */
	height uint8           /* height of bitmap */
	ascent int8            /* top of bitmap to baseline */
	info   []draw.Fontchar /* n+1 character descriptors */
	bits   *Image          /* of font */
}

/*
 * Encapsulated parameters and information for sub-draw routines.
 */

const (
	_Simplesrc  = 1 << 0
	_Simplemask = 1 << 1
	_Replsrc    = 1 << 2
	_Replmask   = 1 << 3
	_Fullsrc    = 1 << 4
	_Fullmask   = 1 << 5
)

type memDrawParam struct {
	dst   *Image
	r     draw.Rectangle
	src   *Image
	sr    draw.Rectangle
	mask  *Image
	mr    draw.Rectangle
	op    draw.Op
	state uint32
	mval  uint32     /* if Simplemask, the mask pixel in mask format */
	mrgba draw.Color /* mval in rgba */
	sval  uint32     /* if Simplesrc, the source pixel in src format */
	srgba draw.Color /* sval in rgba */
	sdval uint32     /* sval in dst format */
}
