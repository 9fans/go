// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"fmt"
	"os"
	"reflect"
	"runtime"
	"strings"
	"unsafe"

	"9fans.net/go/draw"
)

const _DBG = false

var drawdebug int
var tablesbuilt int

/* perfect approximation to NTSC = .299r+.587g+.114b when 0 ≤ r,g,b < 256 */
func _RGB2K(r, g, b uint8) uint8 {
	//	fmt.Printf("RGB2K %#x %#x %#x -> %#x\n%s", r, g, b,
	//		uint8((156763*int(r) + 307758*int(g) + 59769*int(b)) >> 19),
	//		string(debug.Stack()))
	return uint8((156763*int(r) + 307758*int(g) + 59769*int(b)) >> 19)
}

/*
 * For 16-bit values, x / 255 == (t = x+1, (t+(t>>8)) >> 8).
 * We add another 127 to round to the nearest value rather
 * than truncate.
 *
 * CALCxy does x bytewise calculations on y input images (x=1,4; y=1,2).
 * CALC2x does two parallel 16-bit calculations on y input images (y=1,2).
 */
func _CALC11(a, v uint8) uint8 {
	t := uint32(a)*uint32(v) + 128
	return uint8((t - 1) / 255)
}

func _CALC12(a1, v1, a2, v2 uint8) uint8 {
	t := uint32(a1)*uint32(v1) + uint32(a2)*uint32(v2) + 128
	return uint8((t - 1) / 255)
}

const _MASK = 0x00FF00FF

func _CALC21(a uint8, vvuu uint32) uint32 {
	// vvuu is masked by MASK
	panic("CALC21")
	t := uint32(a)*vvuu + 0x0080_0080
	return ((t + ((t >> 8) & _MASK)) >> 8) & _MASK
}

func _CALC41(a uint8, rgba uint32) uint32 {
	return _CALC21(a, rgba&_MASK) | _CALC21(a, (rgba>>8)&_MASK)<<8
}

func _CALC22(a1 uint8, vvuu1 uint32, a2 uint8, vvuu2 uint32) uint32 {
	// vvuu is masked by MASK
	panic("CALC22")
	t := uint32(a1)*vvuu1 + uint32(a2)*vvuu2 + 0x0080_0080
	return ((t + ((t >> 8) & _MASK)) >> 8) & _MASK
}

func _CALC42(a1 uint8, rgba1 uint32, a2 uint8, rgba2 uint32) uint32 {
	return uint32(_CALC12(a1, uint8(rgba1>>24), a2, uint8(rgba2>>24)))<<24 |
		uint32(_CALC12(a1, uint8(rgba1>>16), a2, uint8(rgba2>>16)))<<16 |
		uint32(_CALC12(a1, uint8(rgba1>>8), a2, uint8(rgba2>>8)))<<8 |
		uint32(_CALC12(a1, uint8(rgba1>>0), a2, uint8(rgba2>>0)))<<0

	return _CALC22(a1, rgba1&_MASK, a2, rgba2&_MASK) | _CALC22(a1, (rgba1>>8)&_MASK, a2, (rgba2>>8)&_MASK)<<8
}

type _Subdraw func(*memDrawParam) int

var memones *Image
var memzeros *Image
var memwhite *Image
var Black *Image
var memtransparent *Image
var Opaque *Image

var memimageinit_didinit int = 0

func Init() {

	if memimageinit_didinit != 0 {
		return
	}

	memimageinit_didinit = 1

	mktables()
	_memmkcmap()

	var err error
	memones, err = AllocImage(draw.Rect(0, 0, 1, 1), draw.GREY1)
	if err != nil {
		panic("cannot initialize memimage library")
	}
	memones.Flags |= Frepl
	memones.Clipr = draw.Rect(-0x3FFFFFF, -0x3FFFFFF, 0x3FFFFFF, 0x3FFFFFF)
	byteaddr(memones, draw.ZP)[0] = ^uint8(0)

	memzeros, err = AllocImage(draw.Rect(0, 0, 1, 1), draw.GREY1)
	if err != nil {
		panic("cannot initialize memimage library")
	}
	memzeros.Flags |= Frepl
	memzeros.Clipr = draw.Rect(-0x3FFFFFF, -0x3FFFFFF, 0x3FFFFFF, 0x3FFFFFF)
	byteaddr(memzeros, draw.ZP)[0] = 0

	memwhite = memones
	Black = memzeros
	Opaque = memones
	memtransparent = memzeros
}

// #define DBG drawdebug
var par memDrawParam

func _memimagedrawsetup(dst *Image, r draw.Rectangle, src *Image, p0 draw.Point, mask *Image, p1 draw.Point, op draw.Op) *memDrawParam {
	if mask == nil {
		mask = Opaque
	}

	if _DBG {
		fmt.Fprintf(os.Stderr, "memimagedraw %p/%X %v @ %p %p/%X %v %p/%X %v... ", dst, dst.Pix, r, dst.Data.Bdata, src, src.Pix, p0, mask, mask.Pix, p1)
	}

	if drawclip(dst, &r, src, &p0, mask, &p1, &par.sr, &par.mr) == 0 {
		/*		if(drawdebug) */
		/*			fmt.Fprintf(os.Stderr, "empty clipped rectangle\n"); */
		return nil
	}

	if _DBG {
		fmt.Fprintf(os.Stderr, "->clip %v %v %v\n", r, p0, p1)
	}

	if op < draw.Clear || op > draw.SoverD {
		/*		if(drawdebug) */
		/*			fmt.Fprintf(os.Stderr, "op out of range: %d\n", op); */
		return nil
	}

	par.op = op
	par.dst = dst
	par.r = r
	par.src = src
	/* par.sr set by drawclip */
	par.mask = mask
	/* par.mr set by drawclip */

	par.state = 0
	if src.Flags&Frepl != 0 {
		par.state |= _Replsrc
		if src.R.Dx() == 1 && src.R.Dy() == 1 {
			par.sval = pixelbits(src, src.R.Min)
			par.state |= _Simplesrc
			par.srgba = _imgtorgba(src, par.sval)
			par.sdval = _rgbatoimg(dst, par.srgba)
			if par.srgba&0xFF == 0 && op&draw.DoutS != 0 {
				/*				if (drawdebug) fmt.Fprintf(os.Stderr, "fill with transparent source\n"); */
				return nil /* no-op successfully handled */
			}
			if par.srgba&0xFF == 0xFF {
				par.state |= _Fullsrc
			}
		}
	}

	if mask.Flags&Frepl != 0 {
		par.state |= _Replmask
		if mask.R.Dx() == 1 && mask.R.Dy() == 1 {
			par.mval = pixelbits(mask, mask.R.Min)
			if par.mval == 0 && op&draw.DoutS != 0 {
				/*				if(drawdebug) fmt.Fprintf(os.Stderr, "fill with zero mask\n"); */
				return nil /* no-op successfully handled */
			}
			par.state |= _Simplemask
			if ^par.mval == 0 {
				par.state |= _Fullmask
			}
			par.mrgba = _imgtorgba(mask, par.mval)
		}
	}

	/*	if(drawdebug) */
	/*		fmt.Fprintf(os.Stderr, "dr %v sr %v mr %v...", r, par.sr, par.mr); */
	if _DBG {
		fmt.Fprintf(os.Stderr, "draw dr %v sr %v mr %v %x\n", r, par.sr, par.mr, par.state)
	}

	return &par
}

func _memimagedraw(par *memDrawParam) {
	/*
	 * Now that we've clipped the parameters down to be consistent, we
	 * simply try sub-drawing routines in order until we find one that was able
	 * to handle us.  If the sub-drawing routine returns zero, it means it was
	 * unable to satisfy the request, so we do not return.
	 */

	/*
	 * Hardware support.  Each video driver provides this function,
	 * which checks to see if there is anything it can help with.
	 * There could be an if around this checking to see if dst is in video memory.
	 */
	if _DBG {
		fmt.Fprintf(os.Stderr, "test hwdraw\n")
	}
	if hwdraw(par) != 0 {
		/*if(drawdebug) fmt.Fprintf(os.Stderr, "hw handled\n"); */
		if _DBG {
			fmt.Fprintf(os.Stderr, "hwdraw handled\n")
		}
		return
	}
	/*
	 * Optimizations using memmove and memset.
	 */
	if _DBG {
		fmt.Fprintf(os.Stderr, "test memoptdraw\n")
	}
	if memoptdraw(par) != 0 {
		/*if(drawdebug) fmt.Fprintf(os.Stderr, "memopt handled\n"); */
		if _DBG {
			fmt.Fprintf(os.Stderr, "memopt handled\n")
		}
		return
	}

	/*
	 * Character drawing.
	 * Solid source color being painted through a boolean mask onto a high res image.
	 */
	if _DBG {
		fmt.Fprintf(os.Stderr, "test chardraw\n")
	}
	if chardraw(par) != 0 {
		/*if(drawdebug) fmt.Fprintf(os.Stderr, "chardraw handled\n"); */
		if _DBG {
			fmt.Fprintf(os.Stderr, "chardraw handled\n")
		}
		return
	}

	/*
	 * General calculation-laden case that does alpha for each pixel.
	 */
	if _DBG {
		fmt.Fprintf(os.Stderr, "do alphadraw\n")
	}
	alphadraw(par)
	/*if(drawdebug) fmt.Fprintf(os.Stderr, "alphadraw handled\n"); */
	if _DBG {
		fmt.Fprintf(os.Stderr, "alphadraw handled\n")
	}
}

// #undef DBG

func assert(b bool) {
	if !b {
		panic("assert failed")
	}
}

/*
 * Clip the destination rectangle further based on the properties of the
 * source and mask rectangles.  Once the destination rectangle is properly
 * clipped, adjust the source and mask rectangles to be the same size.
 * Then if source or mask is replicated, move its clipped rectangle
 * so that its minimum point falls within the repl rectangle.
 *
 * Return zero if the final rectangle is null.
 */
func drawclip(dst *Image, r *draw.Rectangle, src *Image, p0 *draw.Point, mask *Image, p1 *draw.Point, sr *draw.Rectangle, mr *draw.Rectangle) int {
	if r.Min.X >= r.Max.X || r.Min.Y >= r.Max.Y {
		return 0
	}
	splitcoords := (p0.X != p1.X) || (p0.Y != p1.Y)
	/* clip to destination */
	rmin := r.Min
	if !draw.RectClip(r, dst.R) || !draw.RectClip(r, dst.Clipr) {
		return 0
	}
	/* move mask point */
	p1.X += r.Min.X - rmin.X
	p1.Y += r.Min.Y - rmin.Y
	/* move source point */
	p0.X += r.Min.X - rmin.X
	p0.Y += r.Min.Y - rmin.Y
	/* map destination rectangle into source */
	sr.Min = *p0
	sr.Max.X = p0.X + r.Dx()
	sr.Max.Y = p0.Y + r.Dy()
	/* sr is r in source coordinates; clip to source */
	if src.Flags&Frepl == 0 && !draw.RectClip(sr, src.R) {
		return 0
	}
	if !draw.RectClip(sr, src.Clipr) {
		return 0
	}
	/* compute and clip rectangle in mask */
	if splitcoords {
		/* move mask point with source */
		p1.X += sr.Min.X - p0.X
		p1.Y += sr.Min.Y - p0.Y
		mr.Min = *p1
		mr.Max.X = p1.X + sr.Dx()
		mr.Max.Y = p1.Y + sr.Dy()
		omr := *mr
		/* mr is now rectangle in mask; clip it */
		if mask.Flags&Frepl == 0 && !draw.RectClip(mr, mask.R) {
			return 0
		}
		if !draw.RectClip(mr, mask.Clipr) {
			return 0
		}
		/* reflect any clips back to source */
		sr.Min.X += mr.Min.X - omr.Min.X
		sr.Min.Y += mr.Min.Y - omr.Min.Y
		sr.Max.X += mr.Max.X - omr.Max.X
		sr.Max.Y += mr.Max.Y - omr.Max.Y
		*p1 = mr.Min
	} else {
		if mask.Flags&Frepl == 0 && !draw.RectClip(sr, mask.R) {
			return 0
		}
		if !draw.RectClip(sr, mask.Clipr) {
			return 0
		}
		*p1 = sr.Min
	}
	var delta draw.Point

	/* move source clipping back to destination */
	delta.X = r.Min.X - p0.X
	delta.Y = r.Min.Y - p0.Y
	r.Min.X = sr.Min.X + delta.X
	r.Min.Y = sr.Min.Y + delta.Y
	r.Max.X = sr.Max.X + delta.X
	r.Max.Y = sr.Max.Y + delta.Y

	/* move source rectangle so sr->min is in src->r */
	if src.Flags&Frepl != 0 {
		delta.X = draw.ReplXY(src.R.Min.X, src.R.Max.X, sr.Min.X) - sr.Min.X
		delta.Y = draw.ReplXY(src.R.Min.Y, src.R.Max.Y, sr.Min.Y) - sr.Min.Y
		sr.Min.X += delta.X
		sr.Min.Y += delta.Y
		sr.Max.X += delta.X
		sr.Max.Y += delta.Y
	}
	*p0 = sr.Min

	/* move mask point so it is in mask->r */
	*p1 = draw.Repl(mask.R, *p1)
	mr.Min = *p1
	mr.Max.X = p1.X + sr.Dx()
	mr.Max.Y = p1.Y + sr.Dy()

	assert(sr.Dx() == mr.Dx() && mr.Dx() == r.Dx())
	assert(sr.Dy() == mr.Dy() && mr.Dy() == r.Dy())
	assert(p0.In(src.R))
	assert(p1.In(mask.R))
	assert(r.Min.In(dst.R))

	return 1
}

/*
 * Conversion tables.
 */
var replbit [1 + 8][256]uint8 /* replbit[x][y] is the replication of the x-bit quantity y to 8-bit depth */
var conv18 [256][8]uint8      /* conv18[x][y] is the yth pixel in the depth-1 pixel x */
var conv28 [256][4]uint8      /* ... */
var conv48 [256][2]uint8

/*
 * bitmap of how to replicate n bits to fill 8, for 1 ≤ n ≤ 8.
 * the X's are where to put the bottom (ones) bit of the n-bit pattern.
 * only the top 8 bits of the result are actually used.
 * (the lower 8 bits are needed to get bits in the right place
 * when n is not a divisor of 8.)
 *
 * Should check to see if its easier to just refer to replmul than
 * use the precomputed values in replbit.  On PCs it may well
 * be; on machines with slow multiply instructions it probably isn't.
 */
var replmul = [1 + 8]uint32{
	0,
	0b1111111111111111,
	0b0101010101010101,
	0b0010010010010010,
	0b0001000100010001,
	0b0000100001000010,
	0b0000010000010000,
	0b0000001000000100,
	0b0000000100000001,
}

func mktables() {
	if tablesbuilt != 0 {
		return
	}

	tablesbuilt = 1

	/* bit replication up to 8 bits */
	for i := uint32(0); i < 256; i++ {
		for j := uint32(0); j <= 8; j++ { /* j <= 8 [sic] */
			small := i & ((1 << j) - 1)
			replbit[j][i] = uint8((small * replmul[j]) >> 8)
		}
	}

	/* bit unpacking up to 8 bits, only powers of 2 */
	for i := uint32(0); i < 256; i++ {
		j := uint32(0)
		sh := uint(7)
		mask := uint32(1)
		for ; j < 8; func() { j++; sh-- }() {
			conv18[i][j] = replbit[1][(i>>sh)&mask]
		}

		j = 0
		sh = 6
		mask = 3
		for ; j < 4; func() { j++; sh -= 2 }() {
			conv28[i][j] = replbit[2][(i>>sh)&mask]
		}

		j = 0
		sh = 4
		mask = 15
		for ; j < 2; func() { j++; sh -= 4 }() {
			conv48[i][j] = replbit[4][(i>>sh)&mask]
		}
	}
}

var ones = [8]uint8{0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff, 0xff}

/*
 * General alpha drawing case.  Can handle anything.
 */

type _Buffer struct {
	red    []uint8
	grn    []uint8
	blu    []uint8
	alpha  []uint8
	grey   []uint8
	rgba   []uint8
	delta  int
	m      []uint8
	mskip  int
	bm     []uint8
	bmskip int
	em     []uint8
	emskip int
}

type _Readfn func(*drawParam, []uint8, int) _Buffer
type _Writefn func(*drawParam, []uint8, _Buffer)
type _Calcfn func(_Buffer, _Buffer, _Buffer, int, bool, draw.Op) _Buffer

const (
	_MAXBCACHE = 16
)

/* giant rathole to customize functions with */
type drawParam struct {
	replcall      _Readfn
	greymaskcall  _Readfn
	convreadcall  _Readfn
	convwritecall _Writefn
	img           *Image
	r             draw.Rectangle
	dx            int
	needbuf       bool
	convgrey      bool
	alphaonly     bool
	bytey0s       []uint8
	bytermin      []uint8
	bytey0e       []uint8
	bwidth        int
	replcache     int
	bcache        [_MAXBCACHE]_Buffer
	bfilled       uint32
	bufbase       []uint8
	bufoff        int
	bufdelta      int
	dir           int
	convbufoff    int
	convbuf       []uint8
	convdpar      *drawParam
	convdx        int
}

var drawbuf []uint8
var ndrawbuf int
var spar drawParam
var mpar drawParam /* easier on the stacks */
var dpar drawParam

var alphacalc = [draw.Ncomp]_Calcfn{
	alphacalc0,    /* Clear */
	alphacalc14,   /* DoutS */
	alphacalc2810, /* SoutD */
	alphacalc3679, /* DxorS */
	alphacalc14,   /* DinS */
	alphacalc5,    /* D */
	alphacalc3679, /* DatopS */
	alphacalc3679, /* DoverS */
	alphacalc2810, /* SinD */
	alphacalc3679, /* SatopD */
	alphacalc2810, /* S */
	alphacalc11,   /* SoverD */
}

var boolcalc = [draw.Ncomp]_Calcfn{
	alphacalc0,     /* Clear */
	boolcalc14,     /* DoutS */
	boolcalc236789, /* SoutD */
	boolcalc236789, /* DxorS */
	boolcalc14,     /* DinS */
	alphacalc5,     /* D */
	boolcalc236789, /* DatopS */
	boolcalc236789, /* DoverS */
	boolcalc236789, /* SinD */
	boolcalc236789, /* SatopD */
	boolcalc1011,   /* S */
	boolcalc1011,   /* SoverD */
}

func allocdrawbuf() {
	for cap(drawbuf) < ndrawbuf {
		drawbuf = append(drawbuf[:cap(drawbuf)], 0)
	}
	drawbuf = drawbuf[:ndrawbuf]
}

func getparam(p *drawParam, img *Image, r draw.Rectangle, convgrey, needbuf bool) {
	*p = drawParam{}
	p.img = img
	p.r = r
	p.dx = r.Dx()
	p.needbuf = needbuf
	p.convgrey = convgrey

	assert(img.R.Min.X <= r.Min.X && r.Min.X < img.R.Max.X)

	p.bytey0s = byteaddr(img, draw.Pt(img.R.Min.X, img.R.Min.Y))
	p.bytermin = byteaddr(img, draw.Pt(r.Min.X, img.R.Min.Y))
	p.bytey0e = byteaddr(img, draw.Pt(img.R.Max.X, img.R.Min.Y))
	p.bwidth = int(4 * img.Width)

	assert(len(p.bytey0s) >= len(p.bytermin) && len(p.bytermin) >= len(p.bytey0e))

	if p.r.Min.X == p.img.R.Min.X {
		assert(len(p.bytermin) == len(p.bytey0s))
	}

	nbuf := 1
	if img.Flags&Frepl != 0 && img.R.Dy() <= _MAXBCACHE && img.R.Dy() < r.Dy() {
		p.replcache = 1
		nbuf = img.R.Dy()
	}
	p.bufdelta = 4 * p.dx
	p.bufoff = ndrawbuf
	ndrawbuf += p.bufdelta * nbuf
}

func clipy(img *Image, y *int) {
	dy := img.R.Dy()
	if *y == dy {
		*y = 0
	} else if *y == -1 {
		*y = dy - 1
	}
	assert(0 <= *y && *y < dy)
}

func dumpbuf(s string, b _Buffer, n int) {
	fmt.Fprintf(os.Stderr, "%s", s)
	for i := 0; i < n; i++ {
		fmt.Fprintf(os.Stderr, " ")
		p := b.grey
		if len(p) != 0 {
			fmt.Fprintf(os.Stderr, " k%.2X", p[0])
			b.grey = b.grey[b.delta:]
		} else {
			p = b.red
			if len(p) != 0 {
				fmt.Fprintf(os.Stderr, " r%.2X", p[0])
				b.red = b.red[b.delta:]
			}
			p = b.grn
			if len(p) != 0 {
				fmt.Fprintf(os.Stderr, " g%.2X", p[0])
				b.grn = b.grn[b.delta:]
			}
			p = b.blu
			if len(p) != 0 {
				fmt.Fprintf(os.Stderr, " b%.2X", p[0])
				b.blu = b.blu[b.delta:]
			}
		}
		p = b.alpha
		if &p[0] != &ones[0] {
			fmt.Fprintf(os.Stderr, " α%.2X", p[0])
			b.alpha = b.alpha[b.delta:]
		}
	}
	fmt.Fprintf(os.Stderr, "\n")
}

/*
 * For each scan line, we expand the pixels from source, mask, and destination
 * into byte-aligned red, green, blue, alpha, and grey channels.  If buffering is not
 * needed and the channels were already byte-aligned (grey8, rgb24, rgba32, rgb32),
 * the readers need not copy the data: they can simply return pointers to the data.
 * If the destination image is grey and the source is not, it is converted using the NTSC
 * formula.
 *
 * Once we have all the channels, we call either rgbcalc or greycalc, depending on
 * whether the destination image is color.  This is allowed to overwrite the dst buffer (perhaps
 * the actual data, perhaps a copy) with its result.  It should only overwrite the dst buffer
 * with the same format (i.e. red bytes with red bytes, etc.)  A new buffer is returned from
 * the calculator, and that buffer is passed to a function to write it to the destination.
 * If the buffer is already pointing at the destination, the writing function is a no-op.
 */
// #define DBG drawdebug
func alphadraw(par *memDrawParam) int {
	if drawdebug != 0 {
		fmt.Fprintf(os.Stderr, "alphadraw %v\n", par.r)
	}
	r := par.r
	dx := r.Dx()
	dy := r.Dy()

	if _DBG {
		fmt.Fprintf(os.Stderr, "alphadraw %v\n", r)
	}
	ndrawbuf = 0

	src := par.src
	mask := par.mask
	dst := par.dst
	sr := par.sr
	mr := par.mr
	op := par.op

	isgrey := dst.Flags&Fgrey != 0

	/*
	 * Buffering when src and dst are the same bitmap is sufficient but not
	 * necessary.  There are stronger conditions we could use.  We could
	 * check to see if the rectangles intersect, and if simply moving in the
	 * correct y direction can avoid the need to buffer.
	 */
	needbuf := src.Data == dst.Data

	getparam(&spar, src, sr, isgrey, needbuf)
	getparam(&dpar, dst, r, isgrey, needbuf)
	getparam(&mpar, mask, mr, false, needbuf)

	dir := 1
	if needbuf && len(byteaddr(dst, r.Min)) < len(byteaddr(src, sr.Min)) {
		dir = -1
	}
	dpar.dir = dir
	mpar.dir = dpar.dir
	spar.dir = mpar.dir
	var rdsrc _Readfn
	var rdmask _Readfn
	var rddst _Readfn
	var calc _Calcfn
	var wrdst _Writefn

	/*
	 * If the mask is purely boolean, we can convert from src to dst format
	 * when we read src, and then just copy it to dst where the mask tells us to.
	 * This requires a boolean (1-bit grey) mask and lack of a source alpha channel.
	 *
	 * The computation is accomplished by assigning the function pointers as follows:
	 *	rdsrc - read and convert source into dst format in a buffer
	 * 	rdmask - convert mask to bytes, set pointer to it
	 * 	rddst - fill with pointer to real dst data, but do no reads
	 *	calc - copy src onto dst when mask says to.
	 *	wrdst - do nothing
	 * This is slightly sleazy, since things aren't doing exactly what their names say,
	 * but it avoids a fair amount of code duplication to make this a case here
	 * rather than have a separate booldraw.
	 */
	/*if(drawdebug) fmt.Fprintf(os.Stderr, "flag %lud mchan %x=?%x dd %d\n", src->flags&Falpha, mask->chan, GREY1, dst->depth); */
	if src.Flags&Falpha == 0 && mask.Pix == draw.GREY1 && dst.Depth >= 8 && op == draw.SoverD {
		/*if(drawdebug) fmt.Fprintf(os.Stderr, "boolcopy..."); */
		rdsrc = convfn(dst, &dpar, src, &spar)
		rddst = readptr
		rdmask = readfn(mask)
		calc = boolcopyfn(dst, mask)
		wrdst = nullwrite
	} else {
		/* usual alphadraw parameter fetching */
		rdsrc = readfn(src)
		rddst = readfn(dst)
		wrdst = writefn(dst)
		calc = alphacalc[op]

		/*
		 * If there is no alpha channel, we'll ask for a grey channel
		 * and pretend it is the alpha.
		 */
		if mask.Flags&Falpha != 0 {
			rdmask = readalphafn(mask)
			mpar.alphaonly = true
		} else {
			mpar.greymaskcall = readfn(mask)
			mpar.convgrey = true
			rdmask = greymaskread

			/*
			 * Should really be above, but then boolcopyfns would have
			 * to deal with bit alignment, and I haven't written that.
			 *
			 * This is a common case for things like ellipse drawing.
			 * When there's no alpha involved and the mask is boolean,
			 * we can avoid all the division and multiplication.
			 */
			if mask.Pix == draw.GREY1 && src.Flags&Falpha == 0 {
				calc = boolcalc[op]
			} else if op == draw.SoverD && src.Flags&Falpha == 0 {
				calc = alphacalcS
			}
		}
	}

	/*
	 * If the image has a small enough repl rectangle,
	 * we can just read each line once and cache them.
	 */
	if spar.replcache != 0 {
		spar.replcall = rdsrc
		rdsrc = replread
	}
	if mpar.replcache != 0 {
		mpar.replcall = rdmask
		rdmask = replread
	}

	allocdrawbuf()

	/*
	 * Before we were saving only offsets from drawbuf in the parameter
	 * structures; now that drawbuf has been grown to accomodate us,
	 * we can fill in the pointers.
	 */
	spar.bufbase = drawbuf[spar.bufoff:]
	mpar.bufbase = drawbuf[mpar.bufoff:]
	dpar.bufbase = drawbuf[dpar.bufoff:]
	spar.convbuf = drawbuf[spar.convbufoff:]
	var starty int
	var endy int

	if dir == 1 {
		starty = 0
		endy = dy
	} else {
		starty = dy - 1
		endy = -1
	}

	/*
	 * srcy, masky, and dsty are offsets from the top of their
	 * respective Rectangles.  they need to be contained within
	 * the rectangles, so clipy can keep them there without division.
	 */
	srcy := (starty + sr.Min.Y - src.R.Min.Y) % src.R.Dy()
	masky := (starty + mr.Min.Y - mask.R.Min.Y) % mask.R.Dy()
	dsty := starty + r.Min.Y - dst.R.Min.Y

	assert(0 <= srcy && srcy < src.R.Dy())
	assert(0 <= masky && masky < mask.R.Dy())
	assert(0 <= dsty && dsty < dst.R.Dy())

	if drawdebug != 0 {
		fmt.Fprintf(os.Stderr, "alphadraw: rdsrc=%p rdmask=%p rddst=%p calc=%p wrdst=%p\n", rdsrc, rdmask, rddst, calc, wrdst)
	}
	for y := starty; y != endy; func() { y += dir; srcy += dir; masky += dir; dsty += dir }() {
		clipy(src, &srcy)
		clipy(dst, &dsty)
		clipy(mask, &masky)

		bsrc := rdsrc(&spar, spar.bufbase, srcy)
		if _DBG {
			fmt.Fprintf(os.Stderr, "[")
		}
		bmask := rdmask(&mpar, mpar.bufbase, masky)
		if _DBG {
			fmt.Fprintf(os.Stderr, "]\n")
		}
		bdst := rddst(&dpar, dpar.bufbase, dsty)
		if _DBG {
			fmt.Fprintf(os.Stderr, "src %v %+v mask %v dst %v calc %v write %v\n", nameof(rdsrc), spar, nameof(rdmask), nameof(rddst), nameof(calc), nameof(wrdst))
			dumpbuf("src", bsrc, dx)
			dumpbuf("mask", bmask, dx)
			dumpbuf("dst", bdst, dx)
		}
		bdst = calc(bdst, bsrc, bmask, dx, isgrey, op)
		if _DBG {
			dumpbuf("bdst", bdst, dx)
		}
		wrdst(&dpar, dpar.bytermin[dsty*dpar.bwidth:], bdst)
	}

	return 1
}

type eface struct {
	_type unsafe.Pointer
	data  unsafe.Pointer
}

func funcPC(f interface{}) uintptr {
	return *(*uintptr)(efaceOf(&f).data)
}
func efaceOf(ep *interface{}) *eface {
	return (*eface)(unsafe.Pointer(ep))
}

func nameof(x interface{}) string {
	f := runtime.FuncForPC(funcPC(x))
	i := strings.LastIndex(f.Name(), ".")
	return f.Name()[i+1:]
}

// #undef DBG

func uint8words(b []uint8) []uint32 {
	var w []uint32
	h := (*reflect.SliceHeader)(unsafe.Pointer(&w))
	h.Data = uintptr(unsafe.Pointer(&b[0]))
	h.Len = len(b) / 4
	h.Cap = cap(b) / 4
	if h.Data&3 != 0 {
		panic("unaligned")
	}
	return w
}

func uint8shorts(b []uint8) []uint16 {
	var w []uint16
	h := (*reflect.SliceHeader)(unsafe.Pointer(&w))
	h.Data = uintptr(unsafe.Pointer(&b[0]))
	h.Len = len(b) / 2
	h.Cap = cap(b) / 2
	if h.Data&1 != 0 {
		panic("unaligned")
	}
	return w
}

func alphacalc0(bdst _Buffer, b1 _Buffer, b2 _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	b := bdst.rgba
	for i := range b[:dx*bdst.delta] {
		b[i] = 0
	}
	return bdst
}

/*
 * Do the channels in the buffers match enough
 * that we can do word-at-a-time operations
 * on the pixels?
 */
func chanmatch(bdst *_Buffer, bsrc *_Buffer) int {
	/*
	 * first, r, g, b must be in the same place
	 * in the rgba word.
	 */
	drgb := bdst.rgba
	srgb := bsrc.rgba
	if len(bdst.red)-len(drgb) != len(bsrc.red)-len(srgb) || len(bdst.blu)-len(drgb) != len(bsrc.blu)-len(srgb) || len(bdst.grn)-len(drgb) != len(bsrc.grn)-len(srgb) {
		return 0
	}

	/*
	 * that implies alpha is in the same place,
	 * if it is there at all (it might be == ones[:]).
	 * if the destination is ones[:], we can scribble
	 * over the rgba slot just fine.
	 */
	if &bdst.alpha[0] == &ones[0] {
		return 1
	}

	/*
	 * if the destination is not ones but the src is,
	 * then the simultaneous calculation will use
	 * bogus bytes from the src's rgba.  no good.
	 */
	if &bsrc.alpha[0] == &ones[0] {
		return 0
	}

	/*
	 * otherwise, alphas are in the same place.
	 */
	return 1
}

func alphacalc14(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst
	sadelta := bsrc.delta
	if &bsrc.alpha[0] == &ones[0] {
		sadelta = 0
	}
	q := bsrc.delta == 4 && bdst.delta == 4 && chanmatch(&bdst, &bsrc) != 0
	var drgba, srgba []uint32
	if q {
		drgba = uint8words(bdst.rgba)
		srgba = uint8words(bsrc.rgba)
	}

	for i := 0; i < dx; i++ {
		sa := bsrc.alpha[0]
		ma := bmask.alpha[0]
		fd := _CALC11(sa, ma)
		if op == draw.DoutS {
			fd = 255 - fd
		}

		if grey {
			bdst.grey[0] = _CALC11(fd, bdst.grey[0])
			bsrc.grey = bsrc.grey[bsrc.delta:]
			bdst.grey = bdst.grey[bdst.delta:]
		} else {
			if q {
				drgba[0] = _CALC41(fd, drgba[0])
				srgba = srgba[1:]
				drgba = drgba[1:]
				bsrc.alpha = bsrc.alpha[sadelta:]
				bmask.alpha = bmask.alpha[bmask.delta:]
				continue
			}
			bdst.red[0] = _CALC11(fd, bdst.red[0])
			bdst.grn[0] = _CALC11(fd, bdst.grn[0])
			bdst.blu[0] = _CALC11(fd, bdst.blu[0])
			bsrc.red = bsrc.red[bsrc.delta:]
			bsrc.blu = bsrc.blu[bsrc.delta:]
			bsrc.grn = bsrc.grn[bsrc.delta:]
			bdst.red = bdst.red[bdst.delta:]
			bdst.blu = bdst.blu[bdst.delta:]
			bdst.grn = bdst.grn[bdst.delta:]
		}
		if &bdst.alpha[0] != &ones[0] {
			bdst.alpha[0] = _CALC11(fd, bdst.alpha[0])
			bdst.alpha = bdst.alpha[bdst.delta:]
		}
		bmask.alpha = bmask.alpha[bmask.delta:]
		bsrc.alpha = bsrc.alpha[sadelta:]
	}
	return obdst
}

func alphacalc2810(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst
	sadelta := bsrc.delta
	if &bsrc.alpha[0] == &ones[0] {
		sadelta = 0
	}
	q := bsrc.delta == 4 && bdst.delta == 4 && chanmatch(&bdst, &bsrc) != 0
	var drgba, srgba []uint32
	if q {
		drgba = uint8words(bdst.rgba)
		srgba = uint8words(bsrc.rgba)
	}

	for i := 0; i < dx; i++ {
		ma := bmask.alpha[0]
		da := bdst.alpha[0]
		if op == draw.SoutD {
			da = 255 - da
		}
		fs := ma
		if op != draw.S {
			fs = _CALC11(fs, da)
		}

		if grey {
			bdst.grey[0] = _CALC11(fs, bsrc.grey[0])
			bsrc.grey = bsrc.grey[bsrc.delta:]
			bdst.grey = bdst.grey[bdst.delta:]
		} else {
			if q {
				drgba[0] = _CALC41(fs, srgba[0])
				srgba = srgba[1:]
				drgba = drgba[1:]
				bmask.alpha = bmask.alpha[bmask.delta:]
				bdst.alpha = bdst.alpha[bdst.delta:]
				continue
			}
			bdst.red[0] = _CALC11(fs, bsrc.red[0])
			bdst.grn[0] = _CALC11(fs, bsrc.grn[0])
			bdst.blu[0] = _CALC11(fs, bsrc.blu[0])
			bsrc.red = bsrc.red[bsrc.delta:]
			bsrc.blu = bsrc.blu[bsrc.delta:]
			bsrc.grn = bsrc.grn[bsrc.delta:]
			bdst.red = bdst.red[bdst.delta:]
			bdst.blu = bdst.blu[bdst.delta:]
			bdst.grn = bdst.grn[bdst.delta:]
		}
		if &bdst.alpha[0] != &ones[0] {
			bdst.alpha[0] = _CALC11(fs, bsrc.alpha[0])
			bdst.alpha = bdst.alpha[bdst.delta:]
		}
		bmask.alpha = bmask.alpha[bmask.delta:]
		bsrc.alpha = bsrc.alpha[sadelta:]
	}
	return obdst
}

func alphacalc3679(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst
	sadelta := bsrc.delta
	if &bsrc.alpha[0] == &ones[0] {
		sadelta = 0
	}
	q := bsrc.delta == 4 && bdst.delta == 4 && chanmatch(&bdst, &bsrc) != 0
	var drgba, srgba []uint32
	if q {
		drgba = uint8words(bdst.rgba)
		srgba = uint8words(bsrc.rgba)
	}

	for i := 0; i < dx; i++ {
		sa := bsrc.alpha[0]
		ma := bmask.alpha[0]
		da := bdst.alpha[0]
		var fs uint8
		if op == draw.SatopD {
			fs = _CALC11(ma, da)
		} else {
			fs = _CALC11(ma, 255-da)
		}
		var fd uint8
		if op == draw.DoverS {
			fd = 255
		} else {
			fd = _CALC11(sa, ma)
			if op != draw.DatopS {
				fd = 255 - fd
			}
		}

		if grey {
			bdst.grey[0] = _CALC12(fs, bsrc.grey[0], fd, bdst.grey[0])
			bsrc.grey = bsrc.grey[bsrc.delta:]
			bdst.grey = bdst.grey[bdst.delta:]
		} else {
			if q {
				drgba[0] = _CALC42(fs, srgba[0], fd, drgba[0])
				srgba = srgba[1:]
				drgba = drgba[1:]
				bsrc.alpha = bsrc.alpha[sadelta:]
				bmask.alpha = bmask.alpha[bmask.delta:]
				bdst.alpha = bdst.alpha[bdst.delta:]
				continue
			}
			bdst.red[0] = _CALC12(fs, bsrc.red[0], fd, bdst.red[0])
			bdst.grn[0] = _CALC12(fs, bsrc.grn[0], fd, bdst.grn[0])
			bdst.blu[0] = _CALC12(fs, bsrc.blu[0], fd, bdst.blu[0])
			bsrc.red = bsrc.red[bsrc.delta:]
			bsrc.blu = bsrc.blu[bsrc.delta:]
			bsrc.grn = bsrc.grn[bsrc.delta:]
			bdst.red = bdst.red[bdst.delta:]
			bdst.blu = bdst.blu[bdst.delta:]
			bdst.grn = bdst.grn[bdst.delta:]
		}
		if &bdst.alpha[0] != &ones[0] {
			bdst.alpha[0] = _CALC12(fs, sa, fd, da)
			bdst.alpha = bdst.alpha[bdst.delta:]
		}
		bmask.alpha = bmask.alpha[bmask.delta:]
		bsrc.alpha = bsrc.alpha[sadelta:]
	}
	return obdst
}

func alphacalc5(bdst _Buffer, b1 _Buffer, b2 _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	return bdst
}

func alphacalc11(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst
	sadelta := bsrc.delta
	if &bsrc.alpha[0] == &ones[0] {
		sadelta = 0
	}
	q := bsrc.delta == 4 && bdst.delta == 4 && chanmatch(&bdst, &bsrc) != 0
	var drgba, srgba []uint32
	if q {
		drgba = uint8words(bdst.rgba)
		srgba = uint8words(bsrc.rgba)
	}

	for i := 0; i < dx; i++ {
		di := i * bdst.delta
		si := i * bsrc.delta
		ai := i * bmask.delta
		asi := i * sadelta
		ma := bmask.alpha[ai]
		sa := bsrc.alpha[asi]
		fd := 255 - _CALC11(sa, ma)

		if grey {
			bdst.grey[di] = _CALC12(ma, bsrc.grey[si], fd, bdst.grey[di])
		} else {
			if q {
				drgba[di/4] = _CALC42(ma, srgba[si/4], fd, drgba[di/4])
				continue
			}
			if _DBG {
				fmt.Fprintf(os.Stderr, "%x %x %x * %x / %x %x %x * %x",
					bdst.red[0], bdst.grn[0], bdst.blu[0], fd, bsrc.red[0], bsrc.grn[0], bsrc.blu[0], ma)
			}
			bdst.red[di] = _CALC12(ma, bsrc.red[si], fd, bdst.red[di])
			bdst.grn[di] = _CALC12(ma, bsrc.grn[si], fd, bdst.grn[di])
			bdst.blu[di] = _CALC12(ma, bsrc.blu[si], fd, bdst.blu[di])
			if _DBG {
				fmt.Fprintf(os.Stderr, " -> %x %x %x\n",
					bdst.red[0], bdst.grn[0], bdst.blu[0])
			}
		}
		if &bdst.alpha[0] != &ones[0] {
			bdst.alpha[di] = _CALC12(ma, sa, fd, bdst.alpha[di])
		}
	}
	return obdst
}

/*
not used yet
source and mask alpha 1
static Buffer
alphacalcS0(Buffer bdst, Buffer bsrc, Buffer bmask, int dx, int grey, int op)
{
	Buffer obdst;
	int i;

	USED(op);
	obdst = bdst;
	if(bsrc.delta == bdst.delta){
		memmove(bdst.rgba, bsrc.rgba, dx*bdst.delta);
		return obdst;
	}
	for(i=0; i<dx; i++){
		if(grey){
			bdst.grey[0] = bsrc.grey[0];
			bsrc.grey = bsrc.grey[bsrc.delta;:]
			bdst.grey = bdst.grey[bdst.delta;:]
		}else{
			bdst.red[0] = bsrc.red[0];
			bdst.grn[0] = bsrc.grn[0];
			bdst.blu[0] = bsrc.blu[0];
			bsrc.red = bsrc.red[bsrc.delta;:]
			bsrc.blu = bsrc.blu[bsrc.delta;:]
			bsrc.grn = bsrc.grn[bsrc.delta;:]
			bdst.red = bdst.red[bdst.delta;:]
			bdst.blu = bdst.blu[bdst.delta;:]
			bdst.grn = bdst.grn[bdst.delta;:]
		}
		if(&bdst.alpha[0] != &ones[0]){
			bdst.alpha[0] = 255;
			bdst.alpha = bdst.alpha[bdst.delta;:]
		}
	}
	return obdst;
}
*/

/* source alpha 1 */
func alphacalcS(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst

	for i := 0; i < dx; i++ {
		di := i * bdst.delta
		si := i * bsrc.delta
		ai := i * bmask.delta
		ma := bmask.alpha[ai]
		fd := 255 - ma
		if grey {
			bdst.grey[di] = _CALC12(ma, bsrc.grey[si], fd, bdst.grey[di])
		} else {
			if _DBG {
				fmt.Fprintf(os.Stderr, "calc %x %x %x * %x / %x %x %x * %x -> ", bdst.red[di], bdst.grn[di], bdst.blu[di], fd, bsrc.red[si], bsrc.grn[si], bsrc.blu[si], ma)
			}
			bdst.red[di] = _CALC12(ma, bsrc.red[si], fd, bdst.red[di])
			bdst.grn[di] = _CALC12(ma, bsrc.grn[si], fd, bdst.grn[di])
			bdst.blu[di] = _CALC12(ma, bsrc.blu[si], fd, bdst.blu[di])
			if _DBG {
				fmt.Fprintf(os.Stderr, "-> %x %x %x\n", bdst.red[di], bdst.grn[di], bdst.blu[di])
			}
		}
		if &bdst.alpha[0] != &ones[0] {
			bdst.alpha[di] = ma + _CALC11(fd, bdst.alpha[di])
		}
	}
	return obdst
}

func boolcalc14(bdst _Buffer, b1 _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst

	for i := 0; i < dx; i++ {
		ma := bmask.alpha[0]
		var zero bool
		if ma != 0 {
			zero = op == draw.DoutS
		} else {
			zero = op == draw.DinS
		}

		if grey {
			if zero {
				bdst.grey[0] = 0
			}
			bdst.grey = bdst.grey[bdst.delta:]
		} else {
			if zero {
				bdst.blu[0] = 0
				bdst.grn[0] = bdst.blu[0]
				bdst.red[0] = bdst.grn[0]
			}
			bdst.red = bdst.red[bdst.delta:]
			bdst.blu = bdst.blu[bdst.delta:]
			bdst.grn = bdst.grn[bdst.delta:]
		}
		bmask.alpha = bmask.alpha[bmask.delta:]
		if &bdst.alpha[0] != &ones[0] {
			if zero {
				bdst.alpha[0] = 0
			}
			bdst.alpha = bdst.alpha[bdst.delta:]
		}
	}
	return obdst
}

func boolcalc236789(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst
	zero := op&1 == 0

	for i := 0; i < dx; i++ {
		ma := bmask.alpha[0]
		da := bdst.alpha[0]
		fs := da
		if op&2 != 0 {
			fs = 255 - da
		}
		fd := uint8(0)
		if op&4 != 0 {
			fd = 255
		}

		if grey {
			if ma != 0 {
				bdst.grey[0] = _CALC12(fs, bsrc.grey[0], fd, bdst.grey[0])
			} else if zero {
				bdst.grey[0] = 0
			}
			bsrc.grey = bsrc.grey[bsrc.delta:]
			bdst.grey = bdst.grey[bdst.delta:]
		} else {
			if ma != 0 {
				bdst.red[0] = _CALC12(fs, bsrc.red[0], fd, bdst.red[0])
				bdst.grn[0] = _CALC12(fs, bsrc.grn[0], fd, bdst.grn[0])
				bdst.blu[0] = _CALC12(fs, bsrc.blu[0], fd, bdst.blu[0])
			} else if zero {
				bdst.blu[0] = 0
				bdst.grn[0] = bdst.blu[0]
				bdst.red[0] = bdst.grn[0]
			}
			bsrc.red = bsrc.red[bsrc.delta:]
			bsrc.blu = bsrc.blu[bsrc.delta:]
			bsrc.grn = bsrc.grn[bsrc.delta:]
			bdst.red = bdst.red[bdst.delta:]
			bdst.blu = bdst.blu[bdst.delta:]
			bdst.grn = bdst.grn[bdst.delta:]
		}
		bmask.alpha = bmask.alpha[bmask.delta:]
		if &bdst.alpha[0] != &ones[0] {
			if ma != 0 {
				bdst.alpha[0] = fs + _CALC11(fd, da)
			} else if zero {
				bdst.alpha[0] = 0
			}
			bdst.alpha = bdst.alpha[bdst.delta:]
		}
	}
	return obdst
}

func boolcalc1011(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, grey bool, op draw.Op) _Buffer {
	obdst := bdst
	zero := op&1 == 0

	for i := 0; i < dx; i++ {
		ma := bmask.alpha[0]

		if grey {
			if ma != 0 {
				bdst.grey[0] = bsrc.grey[0]
			} else if zero {
				bdst.grey[0] = 0
			}
			bsrc.grey = bsrc.grey[bsrc.delta:]
			bdst.grey = bdst.grey[bdst.delta:]
		} else {
			if ma != 0 {
				bdst.red[0] = bsrc.red[0]
				bdst.grn[0] = bsrc.grn[0]
				bdst.blu[0] = bsrc.blu[0]
			} else if zero {
				bdst.blu[0] = 0
				bdst.grn[0] = bdst.blu[0]
				bdst.red[0] = bdst.grn[0]
			}
			bsrc.red = bsrc.red[bsrc.delta:]
			bsrc.blu = bsrc.blu[bsrc.delta:]
			bsrc.grn = bsrc.grn[bsrc.delta:]
			bdst.red = bdst.red[bdst.delta:]
			bdst.blu = bdst.blu[bdst.delta:]
			bdst.grn = bdst.grn[bdst.delta:]
		}
		bmask.alpha = bmask.alpha[bmask.delta:]
		if &bdst.alpha[0] != &ones[0] {
			if ma != 0 {
				bdst.alpha[0] = 255
			} else if zero {
				bdst.alpha[0] = 0
			}
			bdst.alpha = bdst.alpha[bdst.delta:]
		}
	}
	return obdst
}

/*
 * Replicated cached scan line read.  Call the function listed in the drawParam,
 * but cache the result so that for replicated images we only do the work once.
 */
func replread(p *drawParam, s []uint8, y int) _Buffer {
	b := &p.bcache[y]
	if p.bfilled&(1<<y) == 0 {
		p.bfilled |= 1 << y
		*b = p.replcall(p, p.bufbase[y*p.bufdelta:], y)
	}
	return *b
}

/*
 * Alpha reading function that simply relabels the grey pointer.
 */
func greymaskread(p *drawParam, buf []uint8, y int) _Buffer {
	b := p.greymaskcall(p, buf, y)
	b.alpha = b.grey
	return b
}

// #define DBG 0
func readnbit(p *drawParam, buf []uint8, y int) _Buffer {
	var b _Buffer
	b.rgba = buf
	w := buf
	b.grey = w
	b.grn = w
	b.blu = b.grn
	b.red = b.blu
	b.alpha = ones[:]
	b.delta = 1

	dx := p.dx
	img := p.img
	depth := img.Depth
	repl := &replbit[depth]
	npack := 8 / depth
	sh := 8 - depth

	/* copy from p->r.min.x until end of repl rectangle */
	x := p.r.Min.X
	n := dx
	if n > p.img.R.Max.X-x {
		n = p.img.R.Max.X - x
	}

	r := p.bytermin[y*p.bwidth:]
	if _DBG {
		fmt.Fprintf(os.Stderr, "readnbit dx %d %p=%p+%d*%d, *r=%d fetch %d ", dx, r, p.bytermin, y, p.bwidth, r[0], n)
	}
	bits := r[0]
	r = r[1:]
	nbits := 8
	i := x & (npack - 1)
	if i != 0 {
		if _DBG {
			fmt.Fprintf(os.Stderr, "throwaway %d...", i)
		}
		bits <<= depth * i
		nbits -= depth * i
	}
	for i = 0; i < n; i++ {
		if nbits == 0 {
			if _DBG {
				fmt.Fprintf(os.Stderr, "(%.2x)...", r[0])
			}
			bits = r[0]
			r = r[1:]
			nbits = 8
		}
		w[0] = repl[bits>>sh]
		w = w[1:]
		if _DBG {
			fmt.Fprintf(os.Stderr, "bit %x...", repl[bits>>sh])
		}
		bits <<= depth
		nbits -= depth
	}
	dx -= n
	if dx == 0 {
		return b
	}

	assert(x+i == p.img.R.Max.X)

	/* copy from beginning of repl rectangle until where we were before. */
	x = p.img.R.Min.X
	n = dx
	if n > p.r.Min.X-x {
		n = p.r.Min.X - x
	}

	r = p.bytey0s[y*p.bwidth:]
	if _DBG {
		fmt.Fprintf(os.Stderr, "x=%d r=%p...", x, r)
	}
	bits = r[0]
	r = r[1:]
	nbits = 8
	i = x & (npack - 1)
	if i != 0 {
		bits <<= depth * i
		nbits -= depth * i
	}
	if _DBG {
		fmt.Fprintf(os.Stderr, "nbits=%d...", nbits)
	}
	for i = 0; i < n; i++ {
		if nbits == 0 {
			bits = r[0]
			r = r[1:]
			nbits = 8
		}
		w[0] = repl[bits>>sh]
		w = w[1:]
		if _DBG {
			fmt.Fprintf(os.Stderr, "bit %x...", repl[bits>>sh])
		}
		bits <<= depth
		nbits -= depth
		if _DBG {
			fmt.Fprintf(os.Stderr, "bits %x nbits %d...", bits, nbits)
		}
	}
	dx -= n
	if dx == 0 {
		return b
	}

	assert(dx > 0)
	/* now we have exactly one full scan line: just replicate the buffer itself until we are done */
	ow := buf
	for {
		tmp8 := dx
		dx--
		if tmp8 == 0 {
			break
		}
		w[0] = ow[0]
		w = w[1:]
		ow = ow[1:]
	}

	return b
}

// #undef DBG

// #define DBG 0
func writenbit(p *drawParam, w []uint8, src _Buffer) {
	assert(src.grey != nil && src.delta == 1)

	x := p.r.Min.X
	ex := x + p.dx
	depth := p.img.Depth
	npack := 8 / depth

	i := x & (npack - 1)
	bits := uint8(0)
	if i != 0 {
		bits = w[0] >> (8 - depth*i)
	}
	nbits := depth * i
	sh := 8 - depth
	r := src.grey

	for ; x < ex; x++ {
		bits <<= depth
		if _DBG {
			fmt.Fprintf(os.Stderr, " %x", r[0])
		}
		bits |= r[0] >> sh
		r = r[1:]
		nbits += depth
		if nbits == 8 {
			w[0] = bits
			w = w[1:]
			nbits = 0
		}
	}

	if nbits != 0 {
		sh = 8 - nbits
		bits <<= sh
		bits |= w[0] & ((1 << sh) - 1)
		w[0] = bits
	}
	if _DBG {
		fmt.Fprintf(os.Stderr, "\n")
	}
	return
}

// #undef DBG

func readcmap(p *drawParam, buf []uint8, y int) _Buffer {
	var b _Buffer
	begin := p.bytey0s[y*p.bwidth:]
	r := p.bytermin[y*p.bwidth:]
	end := p.bytey0e[y*p.bwidth:]
	r = r[:len(r)-len(end)]
	begin = begin[:len(begin)-len(end)]
	cmap := p.img.cmap.Cmap2rgb[:]
	convgrey := p.convgrey
	copyalpha := p.img.Flags&Falpha != 0

	w := buf
	dx := p.dx
	if copyalpha {
		b.alpha = buf
		buf = buf[1:]
		a := p.img.shift[draw.CAlpha] / 8
		m := p.img.shift[draw.CMap] / 8
		for i := 0; i < dx; i++ {
			w[0] = r[a]
			w = w[1:]
			q := cmap[int(r[m])*3:]
			if _DBG {
				fmt.Fprintf(os.Stderr, "A %x -> %x %x %x\n", r[m], q[0], q[1], q[2])
			}
			r = r[2:]
			if len(r) == 0 {
				r = begin
			}
			if convgrey {
				w[0] = _RGB2K(q[0], q[1], q[2])
				w = w[1:]
			} else { /* blue */
				w[0] = q[2]
				w = w[1:] /* green */
				w[0] = q[1]
				w = w[1:] /* red */
				w[0] = q[0]
				w = w[1:]
			}
		}
	} else {
		b.alpha = ones[:]
		for i := 0; i < dx; i++ {
			q := cmap[int(r[0])*3:]
			if _DBG {
				fmt.Fprintf(os.Stderr, "D %p %x -> %x %x %x\n", cmap, r[0], q[0], q[1], q[2])
			}
			r = r[1:]
			if len(r) == 0 {
				r = begin
			}
			if convgrey {
				w[0] = _RGB2K(q[0], q[1], q[2])
				w = w[1:]
			} else { /* blue */
				w[0] = q[2]
				w = w[1:] /* green */
				w[0] = q[1]
				w = w[1:] /* red */
				w[0] = q[0]
				w = w[1:]
			}
		}
	}

	b.rgba = nil // (*uint32)(buf - copyalpha)

	if convgrey {
		b.grey = buf
		b.grn = buf
		b.blu = b.grn
		b.red = b.blu
		b.delta = 1
		if copyalpha {
			b.delta++
		}
	} else {
		b.blu = buf
		b.grn = buf[1:]
		b.red = buf[2:]
		b.grey = nil
		b.delta = 3
		if copyalpha {
			b.delta++
		}
	}
	return b
}

func writecmap(p *drawParam, w []uint8, src _Buffer) {
	cmap := p.img.cmap.Rgb2cmap[:]

	delta := src.delta
	red := src.red
	grn := src.grn
	blu := src.blu

	dx := p.dx
	for i := 0; i < dx; func() { i++; red = red[delta:]; grn = grn[delta:]; blu = blu[delta:] }() {
		w[0] = cmap[(uint32(red[0])>>4)*256+(uint32(grn[0])>>4)*16+(uint32(blu[0])>>4)]
		if _DBG {
			fmt.Fprintf(os.Stderr, "%x %x %x -> %x\n", red[0], grn[0], blu[0], w[0])
		}
		w = w[1:]
	}
}

// #define DBG drawdebug
func readbyte(p *drawParam, buf []uint8, y int) _Buffer {
	img := p.img
	begin := p.bytey0s[y*p.bwidth:]
	r := p.bytermin[y*p.bwidth:]
	end := p.bytey0e[y*p.bwidth:]
	r = r[:len(r)-len(end)]
	begin = begin[:len(begin)-len(end)]

	if _DBG {
		fmt.Fprintf(os.Stderr, "readbyte dx=%d begin %p r %p end %p len %d buf %p\n",
			p.dx, begin, p.bytermin[y*p.bwidth:], end, len(r), buf)
	}

	w := buf
	dx := p.dx
	nb := img.Depth / 8

	convgrey := p.convgrey /* convert rgb to grey */
	isgrey := img.Flags & Fgrey
	alphaonly := p.alphaonly
	copyalpha := img.Flags&Falpha != 0

	/* if we can, avoid processing everything */
	if img.Flags&Frepl == 0 && !convgrey && img.Flags&Fbytes != 0 {
		var b _Buffer
		if p.needbuf {
			copy(buf[:dx*nb], r[:dx*nb])
			r = buf[:dx*nb]
		}
		b.rgba = r
		if copyalpha {
			b.alpha = r[img.shift[draw.CAlpha]/8:]
		} else {
			b.alpha = ones[:]
		}
		if isgrey != 0 {
			b.grey = r[img.shift[draw.CGrey]/8:]
			b.blu = b.grey
			b.grn = b.blu
			b.red = b.grn
		} else {
			b.red = r[img.shift[draw.CRed]/8:]
			b.grn = r[img.shift[draw.CGreen]/8:]
			b.blu = r[img.shift[draw.CBlue]/8:]
		}
		b.delta = nb
		return b
	}

	rrepl := replbit[img.nbits[draw.CRed]]
	grepl := replbit[img.nbits[draw.CGreen]]
	brepl := replbit[img.nbits[draw.CBlue]]
	arepl := replbit[img.nbits[draw.CAlpha]]
	krepl := replbit[img.nbits[draw.CGrey]]

	for i := 0; i < dx; i++ {
		var u uint32
		if img.Depth == 32 {
			u = uint32(r[0]) | uint32(r[1])<<8 | uint32(r[2])<<16 | uint32(r[3])<<24
		} else if img.Depth == 24 {
			u = uint32(r[0]) | uint32(r[1])<<8 | uint32(r[2])<<16
		} else if img.Depth > 8 {
			u = uint32(r[0]) | uint32(r[1])<<8
		} else {
			u = uint32(r[0])
		}
		if copyalpha {
			w[0] = arepl[(u>>img.shift[draw.CAlpha])&img.mask[draw.CAlpha]]
			w = w[1:]
		}

		if isgrey != 0 {
			w[0] = krepl[(u>>img.shift[draw.CGrey])&img.mask[draw.CGrey]]
			w = w[1:]
		} else if !alphaonly {
			ured := rrepl[(u>>img.shift[draw.CRed])&img.mask[draw.CRed]]
			ugrn := grepl[(u>>img.shift[draw.CGreen])&img.mask[draw.CGreen]]
			ublu := brepl[(u>>img.shift[draw.CBlue])&img.mask[draw.CBlue]]
			if convgrey {
				w[0] = _RGB2K(ured, ugrn, ublu)
				w = w[1:]
			} else {
				w[0] = brepl[(u>>img.shift[draw.CBlue])&img.mask[draw.CBlue]]
				w = w[1:]
				w[0] = grepl[(u>>img.shift[draw.CGreen])&img.mask[draw.CGreen]]
				w = w[1:]
				w[0] = rrepl[(u>>img.shift[draw.CRed])&img.mask[draw.CRed]]
				w = w[1:]
			}
		}
		r = r[nb:]
		if len(r) == 0 {
			r = begin
		}
	}

	var b _Buffer
	if copyalpha {
		b.alpha = buf
	} else {
		b.alpha = ones[:]
	}
	b.rgba = buf
	if alphaonly {
		b.grey = nil
		b.blu = b.grey
		b.grn = b.blu
		b.red = b.grn
		if !copyalpha {
			b.rgba = nil
		}
		b.delta = 1
	} else if isgrey != 0 || convgrey {
		a := 0
		if copyalpha {
			a = 1
		}
		b.grey = buf[a:]
		b.blu = buf[a:]
		b.grn = b.blu
		b.red = b.grn
		b.delta = a + 1
	} else {
		a := 0
		if copyalpha {
			a = 1
		}
		b.blu = buf[a:]
		b.grn = buf[a+1:]
		b.grey = nil
		b.red = buf[a+2:]
		b.delta = a + 3
	}

	if _DBG {
		fmt.Fprintf(os.Stderr, "END readbyte buf %p w %p (%x %x %x %x) grey %p alpha %p\n",
			buf, w, buf[0], buf[1], buf[2], buf[3], b.grey, b.alpha)
		dumpbuf("readbyte", b, dx)
	}

	return b
}

// #undef DBG

// #define DBG drawdebug
func writebyte(p *drawParam, w []uint8, src _Buffer) {
	img := p.img

	red := src.red
	grn := src.grn
	blu := src.blu
	alpha := src.alpha
	delta := src.delta
	grey := src.grey
	dx := p.dx

	nb := img.Depth / 8
	var mask uint32
	if nb == 4 {
		mask = 0
	} else {
		mask = ^((1 << img.Depth) - 1)
	}

	isalpha := img.Flags & Falpha
	isgrey := img.Flags & Fgrey
	adelta := src.delta

	if isalpha != 0 && alpha == nil {
		alpha = ones[:]
		adelta = 0
	}

	for i := 0; i < dx; i++ {
		di := i * delta
		ai := i * adelta
		var u uint32
		if nb == 4 {
			u = uint32(w[0]) | uint32(w[1])<<8 | uint32(w[2])<<16 | uint32(w[3])<<24
		} else if nb == 3 {
			u = uint32(w[0]) | uint32(w[1])<<8 | uint32(w[2])<<16
		} else if nb == 2 {
			u = uint32(w[0]) | uint32(w[1])<<8
		} else {
			u = uint32(w[0])
		}
		if _DBG {
			fmt.Fprintf(os.Stderr, "u %.8x...", u)
		}
		u &= mask
		if _DBG {
			fmt.Fprintf(os.Stderr, "&mask %.8x...", u)
		}
		if isgrey != 0 {
			u |= ((uint32(grey[di]) >> (8 - img.nbits[draw.CGrey])) & img.mask[draw.CGrey]) << img.shift[draw.CGrey]
			if _DBG {
				fmt.Fprintf(os.Stderr, "|grey %.8x...", u)
			}
		} else {
			u |= ((uint32(red[di]) >> (8 - img.nbits[draw.CRed])) & img.mask[draw.CRed]) << img.shift[draw.CRed]
			u |= ((uint32(grn[di]) >> (8 - img.nbits[draw.CGreen])) & img.mask[draw.CGreen]) << img.shift[draw.CGreen]
			u |= ((uint32(blu[di]) >> (8 - img.nbits[draw.CBlue])) & img.mask[draw.CBlue]) << img.shift[draw.CBlue]
			if _DBG {
				fmt.Fprintf(os.Stderr, "|rgb %.8x...", u)
			}
		}

		if isalpha != 0 {
			u |= ((uint32(alpha[ai]) >> (8 - img.nbits[draw.CAlpha])) & img.mask[draw.CAlpha]) << img.shift[draw.CAlpha]
			if _DBG {
				fmt.Fprintf(os.Stderr, "|alpha %.8x...", u)
			}
		}

		if nb == 4 {
			w[0] = uint8(u)
			w[1] = uint8(u >> 8)
			w[2] = uint8(u >> 16)
			w[3] = uint8(u >> 24)
		} else if nb == 3 {
			w[0] = uint8(u)
			w[1] = uint8(u >> 8)
			w[2] = uint8(u >> 16)
		} else if nb == 2 {
			w[0] = uint8(u)
			w[1] = uint8(u >> 8)
		} else {
			w[0] = uint8(u)
		}
		if _DBG {
			fmt.Fprintf(os.Stderr, "write back %.8x...", u)
		}
		w = w[nb:]
	}
}

// #undef DBG

func readfn(img *Image) _Readfn {
	if img.Depth < 8 {
		return readnbit
	}
	if img.nbits[draw.CMap] == 8 {
		return readcmap
	}
	return readbyte
}

func readalphafn(m *Image) _Readfn {
	return readbyte
}

func writefn(img *Image) _Writefn {
	if img.Depth < 8 {
		return writenbit
	}
	if img.Pix == draw.CMAP8 {
		return writecmap
	}
	return writebyte
}

func nullwrite(p *drawParam, s []uint8, b _Buffer) {
}

func readptr(p *drawParam, s []uint8, y int) _Buffer {
	var b _Buffer
	q := p.bytermin[y*p.bwidth:]
	b.red = q /* ptr to data */
	b.alpha = nil
	b.grey = b.alpha
	b.blu = b.grey
	b.grn = b.blu
	b.rgba = q
	b.delta = p.img.Depth / 8
	return b
}

func boolmemmove(bdst _Buffer, bsrc _Buffer, b1 _Buffer, dx int, i bool, o draw.Op) _Buffer {
	copy(bdst.red[:dx*bdst.delta], bsrc.red[:dx*bdst.delta])
	return bdst
}

func boolcopy8(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, i bool, o draw.Op) _Buffer {
	m := bmask.grey
	w := bdst.red
	r := bsrc.red
	for i := 0; i < dx; i++ {
		if m[i] != 0 {
			w[i] = r[i]
		}
	}
	return bdst /* not used */
}

func boolcopy16(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, i bool, o draw.Op) _Buffer {
	m := bmask.grey
	w := uint8shorts(bdst.red)
	r := uint8shorts(bsrc.red)
	for i := 0; i < dx; i++ {
		if m[i] != 0 {
			w[i] = r[i]
		}
	}
	return bdst /* not used */
}

func boolcopy24(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, i bool, o draw.Op) _Buffer {
	m := bmask.grey
	w := bdst.red
	r := bsrc.red
	for i, j := 0, 0; i < dx; i, j = i+1, j+3 {
		if m[i] != 0 {
			w[j] = r[j]
			w[j+1] = r[j+1]
			w[j+2] = r[j+2]
		}
	}
	return bdst /* not used */
}

func boolcopy32(bdst _Buffer, bsrc _Buffer, bmask _Buffer, dx int, i bool, o draw.Op) _Buffer {
	m := bmask.grey
	w := uint8words(bdst.red)
	r := uint8words(bsrc.red)
	for i := 0; i < dx; i++ {
		if m[i] != 0 {
			w[i] = r[i]
		}
	}
	return bdst /* not used */
}

func genconv(p *drawParam, buf []uint8, y int) _Buffer {
	/* read from source into RGB format in convbuf */
	b := p.convreadcall(p, p.convbuf, y)

	/* write RGB format into dst format in buf */
	p.convwritecall(p.convdpar, buf, b)

	if p.convdx != 0 {
		nb := p.convdpar.img.Depth / 8
		r := buf
		w := buf[nb*p.dx : nb*p.convdx]
		copy(w, r)
	}

	b.red = buf
	b.alpha = nil
	b.grey = b.alpha
	b.grn = b.grey
	b.blu = b.grn
	b.rgba = buf
	b.delta = 0

	return b
}

func convfn(dst *Image, dpar *drawParam, src *Image, spar *drawParam) _Readfn {
	if dst.Pix == src.Pix && src.Flags&Frepl == 0 {
		/*if(drawdebug) fmt.Fprintf(os.Stderr, "readptr..."); */
		return readptr
	}

	if dst.Pix == draw.CMAP8 && (src.Pix == draw.GREY1 || src.Pix == draw.GREY2 || src.Pix == draw.GREY4) {
		/* cheat because we know the replicated value is exactly the color map entry. */
		/*if(drawdebug) fmt.Fprintf(os.Stderr, "Readnbit..."); */
		return readnbit
	}

	spar.convreadcall = readfn(src)
	spar.convwritecall = writefn(dst)
	spar.convdpar = dpar

	/* allocate a conversion buffer */
	spar.convbufoff = ndrawbuf
	ndrawbuf += spar.dx * 4

	if spar.dx > spar.img.R.Dx() {
		spar.convdx = spar.dx
		spar.dx = spar.img.R.Dx()
	}

	/*if(drawdebug) fmt.Fprintf(os.Stderr, "genconv..."); */
	return genconv
}

/*
 * Do NOT call this directly.  pixelbits is a wrapper
 * around this that fetches the bits from the X server
 * when necessary.
 */
func _pixelbits(i *Image, pt draw.Point) uint32 {
	val := uint32(0)
	p := byteaddr(i, pt)
	bpp := i.Depth
	var off int
	var npack int
	switch bpp {
	case 1, 2, 4:
		npack = 8 / bpp
		off = pt.X % npack
		val = uint32(p[0]) >> (bpp * (npack - 1 - off))
		val &= (1 << bpp) - 1
	case 8:
		val = uint32(p[0])
	case 16:
		val = uint32(p[0]) | uint32(p[1])<<8
	case 24:
		val = uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16
	case 32:
		val = uint32(p[0]) | uint32(p[1])<<8 | uint32(p[2])<<16 | uint32(p[3])<<24
	}
	for bpp < 32 {
		val |= val << bpp
		bpp *= 2
	}
	return val
}

func boolcopyfn(img *Image, mask *Image) _Calcfn {
	if mask.Flags&Frepl != 0 && mask.R.Dx() == 1 && mask.R.Dy() == 1 && ^pixelbits(mask, mask.R.Min) == 0 {
		return boolmemmove
	}

	switch img.Depth {
	case 8:
		return boolcopy8
	case 16:
		return boolcopy16
	case 24:
		return boolcopy24
	case 32:
		return boolcopy32
	default:
		panic("boolcopyfn")
	}
	return nil
}

/*
 * Optimized draw for filling and scrolling; uses memset and memmove.
 */
func memsets(vp []byte, val uint16, n int) {
	p := uint8shorts(vp)[:n]
	for i := range p {
		p[i] = val
	}
}

func memsetl(vp []byte, val uint32, n int) {
	p := uint8words(vp)[:n]
	for i := range p {
		p[i] = val
	}
}

func memset24(vp []byte, val uint32, n int) {
	p := vp
	a := uint8(val)
	b := uint8(val >> 8)
	c := uint8(val >> 16)
	n *= 3
	for j := 0; j < n; j += 3 {
		p[j] = a
		p[j+1] = b
		p[j+2] = c
	}
}

func _NBITS(c draw.Pix) uint { return uint(c & 15) }
func _TYPE(c draw.Pix) int   { return int((c >> 4) & 15) }

func _imgtorgba(img *Image, val uint32) draw.Color {
	a := uint32(0xFF) /* garbage */
	b := uint32(0xAA)
	g := uint32(b)
	r := uint32(g)
	for chan_ := img.Pix; chan_ != 0; chan_ >>= 8 {
		nb := _NBITS(chan_)
		v := val & ((1 << nb) - 1)
		ov := v
		val >>= nb

		for nb < 8 {
			v |= v << nb
			nb *= 2
		}
		v >>= (nb - 8)

		switch _TYPE(chan_) {
		case draw.CRed:
			r = v
		case draw.CGreen:
			g = v
		case draw.CBlue:
			b = v
		case draw.CAlpha:
			a = v
		case draw.CGrey:
			b = v
			g = b
			r = g
		case draw.CMap:
			p := img.cmap.Cmap2rgb[3*ov:]
			r = uint32(p[0])
			g = uint32(p[1])
			b = uint32(p[2])
			if _DBG {
				fmt.Fprintf(os.Stderr, "%x -> %x %x %x\n", ov, r, g, b)
			}
		}
	}
	return draw.Color(r<<24 | g<<16 | b<<8 | a)
}

func _rgbatoimg(img *Image, rgba draw.Color) uint32 {
	v := uint32(0)
	r := uint32(rgba>>24) & 0xFF
	g := uint32(rgba>>16) & 0xFF
	b := uint32(rgba>>8) & 0xFF
	a := uint32(rgba) & 0xFF
	d := uint(0)
	for chan_ := img.Pix; chan_ != 0; chan_ >>= 8 {
		nb := _NBITS(chan_)
		var m uint32
		switch _TYPE(chan_) {
		case draw.CRed:
			v |= (r >> (8 - nb)) << d
		case draw.CGreen:
			v |= (g >> (8 - nb)) << d
		case draw.CBlue:
			v |= (b >> (8 - nb)) << d
		case draw.CAlpha:
			v |= (a >> (8 - nb)) << d
		case draw.CMap:
			p := img.cmap.Rgb2cmap[:]
			m = uint32(p[(uint32(r)>>4)*256+(uint32(g)>>4)*16+(uint32(b)>>4)])
			if _DBG {
				fmt.Fprintf(os.Stderr, "%x %x %x -> %x\n", r, g, b, m)
			}
			v |= (m >> (8 - nb)) << d
		case draw.CGrey:
			m = uint32(_RGB2K(uint8(r), uint8(g), uint8(b)))
			v |= (m >> (8 - nb)) << d
		}
		d += nb
	}
	/*	fmt.Fprintf(os.Stderr, "rgba2img %.8x = %.*lux\n", rgba, 2*d/8, v); */
	return v
}

// #define DBG 0
func memoptdraw(par *memDrawParam) int {
	dx := par.r.Dx()
	dy := par.r.Dy()
	src := par.src
	dst := par.dst
	op := par.op

	if _DBG {
		fmt.Fprintf(os.Stderr, "state %x mval %x dd %d\n", par.state, par.mval, dst.Depth)
	}
	/*
	 * If we have an opaque mask and source is one opaque pixel we can convert to the
	 * destination format and just replicate with memset.
	 */
	m := uint32(_Simplesrc | _Simplemask | _Fullmask)
	if par.state&m == m && par.srgba&0xFF == 0xFF && (op == draw.S || op == draw.SoverD) {
		if _DBG {
			fmt.Fprintf(os.Stderr, "memopt, dst %p, dst->data->bdata %p\n", dst, dst.Data.Bdata)
		}
		dwid := int(dst.Width) * 4
		dp := byteaddr(dst, par.r.Min)
		v := par.sdval
		if _DBG {
			fmt.Fprintf(os.Stderr, "sdval %d, depth %d\n", v, dst.Depth)
		}
		switch dst.Depth {
		case 1, 2, 4:
			for d := dst.Depth; d < 8; d *= 2 {
				v |= v << d
			}
			ppb := 8 / dst.Depth /* pixels per byte */
			m := ppb - 1
			/* left edge */
			np := par.r.Min.X & m /* no. pixels unused on left side of word */
			dx -= (ppb - np)
			nb := 8 - np*dst.Depth /* no. bits used on right side of word */
			lm := (uint8(1) << nb) - 1
			if _DBG {
				fmt.Fprintf(os.Stderr, "np %d x %d nb %d lm %x ppb %d m %x\n", np, par.r.Min.X, nb, lm, ppb, m)
			}

			/* right edge */
			np = par.r.Max.X & m /* no. pixels used on left side of word */
			dx -= np
			nb = 8 - np*dst.Depth /* no. bits unused on right side of word */
			rm := ^((uint8(1) << nb) - 1)
			if _DBG {
				fmt.Fprintf(os.Stderr, "np %d x %d nb %d rm %x ppb %d m %x\n", np, par.r.Max.X, nb, rm, ppb, m)
			}

			if _DBG {
				fmt.Fprintf(os.Stderr, "dx %d Dx %d\n", dx, par.r.Dx())
			}
			/* lm, rm are masks that are 1 where we should touch the bits */
			if dx < 0 { /* just one byte */
				lm &= rm
				for y := 0; y < dy; y++ {
					dp[0] ^= (uint8(v) ^ dp[0]) & lm
					dp = dp[dwid:]
				}
			} else if dx == 0 { /* no full bytes */
				if lm != 0 {
					dwid--
				}
				for y := 0; y < dy; y++ {
					if lm != 0 {
						if _DBG {
							fmt.Fprintf(os.Stderr, "dp %p v %x lm %x (v ^ *dp) & lm %x\n", dp, v, lm, (uint8(v)^dp[0])&lm)
						}
						dp[0] ^= (uint8(v) ^ dp[0]) & lm
						dp = dp[1:]
					}
					dp[0] ^= (uint8(v) ^ dp[0]) & rm
					dp = dp[dwid:]
				}
			} else { /* full bytes in middle */
				dx /= ppb
				if lm != 0 {
					dwid--
				}
				dwid -= dx

				for y := 0; y < dy; y++ {
					if lm != 0 {
						dp[0] ^= (uint8(v) ^ dp[0]) & lm
						dp = dp[1:]
					}
					row := dp[:dx]
					for i := range row {
						row[i] = uint8(v)
					}
					dp = dp[dx:]
					dp[0] ^= (uint8(v) ^ dp[0]) & rm
					dp = dp[dwid:]
				}
			}
			return 1
		case 8:
			for y := 0; y < dy; y++ {
				row := dp[:dx]
				for i := range row {
					row[i] = uint8(v)
				}
				dp = dp[dwid:]
			}
			return 1
		case 16:
			var p [2]uint8
			p[0] = uint8(v) /* make little endian */
			p[1] = uint8(v >> 8)
			v := *(*uint16)(unsafe.Pointer(&p[0]))
			if _DBG {
				fmt.Fprintf(os.Stderr, "dp=%p; dx=%d; for(y=0; y<%d; y++, dp+=%d)\nmemsets(dp, v, dx);\n", dp, dx, dy, dwid)
			}
			for y := 0; y < dy; y++ {
				memsets(dp, v, dx)
				dp = dp[dwid:]
			}
			return 1
		case 24:
			for y := 0; y < dy; y++ {
				memset24(dp, v, dx)
				dp = dp[dwid:]
			}
			return 1
		case 32:
			var p [4]uint8
			p[0] = uint8(v) /* make little endian */
			p[1] = uint8(v >> 8)
			p[2] = uint8(v >> 16)
			p[3] = uint8(v >> 24)
			v := *(*uint32)(unsafe.Pointer(&p[0]))
			for y := 0; y < dy; y++ {
				memsetl(dp[y*dwid:], v, dx)
			}
			return 1
		default:
			panic("bad dest depth in memoptdraw")
		}
	}

	/*
	 * If no source alpha, an opaque mask, we can just copy the
	 * source onto the destination.  If the channels are the same and
	 * the source is not replicated, memmove suffices.
	 */
	m = _Simplemask | _Fullmask
	if par.state&(m|_Replsrc) == m && src.Depth >= 8 && src.Pix == dst.Pix && src.Flags&Falpha == 0 && (op == draw.S || op == draw.SoverD) {
		var dir int
		if src.Data == dst.Data && len(byteaddr(dst, par.r.Min)) < len(byteaddr(src, par.sr.Min)) {
			dir = -1
		} else {
			dir = 1
		}

		swid := int(src.Width) * 4
		dwid := int(dst.Width) * 4
		sp := byteaddr(src, par.sr.Min)
		dp := byteaddr(dst, par.r.Min)
		nb := (dx * src.Depth) / 8

		if dir == -1 {
			for y := dy - 1; y >= 0; y-- {
				copy(dp[dwid*y:dwid*y+nb], sp[swid*y:swid*y+nb])
			}
		} else {
			for y := 0; y < dy; y++ {
				copy(dp[dwid*y:dwid*y+nb], sp[swid*y:swid*y+nb])
			}
		}
		return 1
	}

	/*
	 * If we have a 1-bit mask, 1-bit source, and 1-bit destination, and
	 * they're all bit aligned, we can just use bit operators.  This happens
	 * when we're manipulating boolean masks, e.g. in the arc code.
	 */
	if par.state&(_Simplemask|_Simplesrc|_Replmask|_Replsrc) == 0 && dst.Pix == draw.GREY1 && src.Pix == draw.GREY1 && par.mask.Pix == draw.GREY1 && par.r.Min.X&7 == par.sr.Min.X&7 && par.r.Min.X&7 == par.mr.Min.X&7 {
		sp := byteaddr(src, par.sr.Min)
		dp := byteaddr(dst, par.r.Min)
		mp := byteaddr(par.mask, par.mr.Min)
		swid := int(src.Width) * 4
		dwid := int(dst.Width) * 4
		mwid := int(par.mask.Width) * 4
		var dir int

		if src.Data == dst.Data && len(byteaddr(dst, par.r.Min)) < len(byteaddr(src, par.sr.Min)) {
			dir = -1
		} else {
			dir = 1
		}

		lm := uint8(0xFF) >> (par.r.Min.X & 7)
		rm := uint8(0xFF) << (8 - (par.r.Max.X & 7))
		dx -= (8 - (par.r.Min.X & 7)) + (par.r.Max.X & 7)

		if dx < 0 { /* one byte wide */
			lm &= rm
			if dir == -1 {
				for y := dy - 1; y >= 0; y-- {
					dp[y*dwid] ^= (dp[y*dwid] ^ sp[y*swid]) & mp[y*mwid] & lm
				}
			} else {
				for y := 0; y < dy; y++ {
					dp[y*dwid] ^= (dp[y*dwid] ^ sp[y*swid]) & mp[y*mwid] & lm
				}
			}
			return 1
		}

		dx /= 8
		if dir == 1 {
			for y := 0; y < dy; y++ {
				j := 0
				if lm != 0 {
					dp[y*dwid] ^= (dp[y*dwid] ^ sp[y*swid]) & mp[y*mwid] & lm
					j = 1
				}
				for x := 0; x < dx; x++ {
					dp[y*dwid+j+x] ^= (dp[y*dwid+j+x] ^ sp[y*swid+j+x]) & mp[y*mwid+j+x]
				}
				if rm != 0 {
					dp[y*dwid+j+dx] ^= (dp[y*dwid+j+dx] ^ sp[y*swid+j+dx]) & mp[y*mwid+j+dx] & rm
				}
			}
			return 1
		} else {
			/* dir == -1 */
			for y := dy - 1; y >= 0; y-- {
				j := 0
				if lm != 0 {
					j = 1
				}
				if rm != 0 {
					dp[y*dwid+j+dx] ^= (dp[y*dwid+j+dx] ^ sp[y*swid+j+dx]) & mp[y*mwid+j+dx] & rm
				}
				for x := dx - 1; x >= 0; x-- {
					dp[y*dwid+j+x] ^= (dp[y*dwid+j+x] ^ sp[y*swid+j+x]) & mp[y*mwid+j+x]
				}
				if lm != 0 {
					dp[y*dwid] ^= (dp[y*dwid] ^ sp[y*swid]) & mp[y*mwid] & lm
				}
			}
		}
		return 1
	}
	return 0
}

// #undef DBG

/*
 * Boolean character drawing.
 * Solid opaque color through a 1-bit greyscale mask.
 */
// #define DBG 0
func chardraw(par *memDrawParam) int {
	// black box to hide pointer conversions from gcc.
	// we'll see how long this works.

	if 0 != 0 {
		if drawdebug != 0 {
			fmt.Fprintf(os.Stderr, "chardraw? mf %x md %d sf %x dxs %d dys %d dd %d ddat %p sdat %p\n", par.mask.Flags, par.mask.Depth, par.src.Flags, par.src.R.Dx(), par.src.R.Dy(), par.dst.Depth, par.dst.Data, par.src.Data)
		}
	}

	mask := par.mask
	src := par.src
	dst := par.dst
	r := par.r
	mr := par.mr
	op := par.op

	if par.state&(_Replsrc|_Simplesrc|_Fullsrc|_Replmask) != _Replsrc|_Simplesrc|_Fullsrc || mask.Depth != 1 || dst.Depth < 8 || dst.Data == src.Data || op != draw.SoverD {
		return 0
	}

	/*if(drawdebug) fmt.Fprintf(os.Stderr, "chardraw..."); */

	depth := mask.Depth
	maskwid := int(mask.Width) * 4
	rp := byteaddr(mask, mr.Min)
	npack := 8 / depth
	bsh := (mr.Min.X % npack) * depth

	wp := byteaddr(dst, r.Min)
	dstwid := int(dst.Width) * 4
	if _DBG {
		fmt.Fprintf(os.Stderr, "bsh %d\n", bsh)
	}
	dy := r.Dy()
	dx := r.Dx()

	ddepth := dst.Depth

	/*
	 * for loop counts from bsh to bsh+dx
	 *
	 * we want the bottom bits to be the amount
	 * to shift the pixels down, so for n≡0 (mod 8) we want
	 * bottom bits 7.  for n≡1, 6, etc.
	 * the bits come from -n-1.
	 */

	bx := -bsh - 1
	ex := -bsh - 1 - dx
	v := par.sdval

	/* make little endian */
	var sp [4]uint8
	sp[0] = uint8(v)
	sp[1] = uint8(v >> 8)
	sp[2] = uint8(v >> 16)
	sp[3] = uint8(v >> 24)

	/*fmt.Fprintf(os.Stderr, "sp %x %x %x %x\n", sp[0], sp[1], sp[2], sp[3]); */
	for y := 0; y < dy; {
		q := rp
		var bits uint32
		if bsh != 0 {
			bits = uint32(q[0])
			q = q[1:]
		}

		switch ddepth {
		/*if(drawdebug) fmt.Fprintf(os.Stderr, "8loop..."); */
		case 8:
			wc := wp
			for x := bx; x > ex; x-- {
				i := x & 7
				if i == 8-1 {
					bits = uint32(q[0])
					q = q[1:]
				}
				if _DBG {
					fmt.Fprintf(os.Stderr, "bits %x sh %d...", bits, i)
				}
				if (bits>>i)&1 != 0 {
					wc[0] = uint8(v)
				}
				wc = wc[1:]
			}
		case 16:
			ws := uint8shorts(wp)
			v := *(*uint16)(unsafe.Pointer(&sp[0]))
			for x := bx; x > ex; x-- {
				i := x & 7
				if i == 8-1 {
					bits = uint32(q[0])
					q = q[1:]
				}
				if _DBG {
					fmt.Fprintf(os.Stderr, "bits %x sh %d...", bits, i)
				}
				if (bits>>i)&1 != 0 {
					ws[0] = v
				}
				ws = ws[1:]
			}
		case 24:
			wc := wp
			for x := bx; x > ex; x-- {
				i := x & 7
				if i == 8-1 {
					bits = uint32(q[0])
					q = q[1:]
				}
				if _DBG {
					fmt.Fprintf(os.Stderr, "bits %x sh %d...", bits, i)
				}
				if (bits>>i)&1 != 0 {
					wc[0] = sp[0]
					wc[1] = sp[1]
					wc[2] = sp[2]
				}
				wc = wc[3:]
			}
		case 32:
			wl := uint8words(wp)
			v := *(*uint32)(unsafe.Pointer(&sp[0]))
			for x := bx; x > ex; x-- {
				i := x & 7
				if i == 8-1 {
					bits = uint32(q[0])
					q = q[1:]
				}
				if _DBG {
					fmt.Fprintf(os.Stderr, "bits %x sh %d...", bits, i)
				}
				if (bits>>i)&1 != 0 {
					wl[0] = v
				}
				wl = wl[1:]
			}
		}
		if y++; y >= dy {
			break
		}
		rp = rp[maskwid:]
		wp = wp[dstwid:]
	}

	if _DBG {
		fmt.Fprintf(os.Stderr, "\n")
	}
	return 1
}

// #undef DBG

/*
 * Fill entire byte with replicated (if necessary) copy of source pixel,
 * assuming destination ldepth is >= source ldepth.
 *
 * This code is just plain wrong for >8bpp.
 *
u32int
membyteval(Memimage *src)
{
	int i, val, bpp;
	uchar uc;

	unloadmemimage(src, src->r, &uc, 1);
	bpp = src->depth;
	uc <<= (src->r.min.x&(7/src->depth))*src->depth;
	uc &= ~(0xFF>>bpp);
	* pixel value is now in high part of byte. repeat throughout byte
	val = uc;
	for(i=bpp; i<8; i<<=1)
		val |= val>>i;
	return val;
}
 *
*/

func _memfillcolor(i *Image, val draw.Color) {
	if val == draw.NoFill {
		return
	}

	bits := _rgbatoimg(i, val)
	switch i.Depth {
	case 24: /* 24-bit images suck */
		for y := i.R.Min.Y; y < i.R.Max.Y; y++ {
			memset24(byteaddr(i, draw.Pt(i.R.Min.X, y)), bits, i.R.Dx())
		}
	default: /* 1, 2, 4, 8, 16, 32 */
		for d := i.Depth; d < 32; d *= 2 {
			bits = bits<<d | bits
		}
		var p [4]uint8
		p[0] = uint8(bits) /* make little endian */
		p[1] = uint8(bits >> 8)
		p[2] = uint8(bits >> 16)
		p[3] = uint8(bits >> 24)
		bits := *(*uint32)(unsafe.Pointer(&p[0]))
		memsetl(byteaddr(i, i.R.Min), bits, int(i.Width)*i.R.Dy())
	}
}
