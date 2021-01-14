// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import (
	"9fans.net/go/draw"
)

type _Draw struct {
	deltas   draw.Point
	deltam   draw.Point
	dstlayer *Layer
	src      *Image
	mask     *Image
	op       draw.Op
}

func ldrawop(dst *Image, screenr draw.Rectangle, clipr draw.Rectangle, etc interface{}, insave int) {
	d := etc.(*_Draw)
	if insave != 0 && d.dstlayer.save == nil {
		return
	}

	p0 := screenr.Min.Add(d.deltas)
	p1 := screenr.Min.Add(d.deltam)
	var r draw.Rectangle

	if insave != 0 {
		r = screenr.Sub(d.dstlayer.Delta)
		clipr = clipr.Sub(d.dstlayer.Delta)
	} else {
		r = screenr
	}

	/* now in logical coordinates */

	/* clipr may have narrowed what we should draw on, so clip if necessary */
	if !draw.RectInRect(r, clipr) {
		oclipr := dst.Clipr
		dst.Clipr = clipr
		var mr draw.Rectangle
		var srcr draw.Rectangle
		ok := drawclip(dst, &r, d.src, &p0, d.mask, &p1, &srcr, &mr)
		dst.Clipr = oclipr
		if ok == 0 {
			return
		}
	}
	Draw(dst, r, d.src, p0, d.mask, p1, d.op)
}

func Draw(dst *Image, r draw.Rectangle, src *Image, p0 draw.Point, mask *Image, p1 draw.Point, op draw.Op) {
	if drawdebug != 0 {
		iprint("memdraw %p %v %p %v %p %v\n", dst, r, src, p0, mask, p1)
	}

	if mask == nil {
		mask = Opaque
	}

	if mask.Layer != nil {
		if drawdebug != 0 {
			iprint("mask->layer != nil\n")
		}
		return /* too hard, at least for now */
	}

Top:
	if dst.Layer == nil && src.Layer == nil {
		dst.Draw(r, src, p0, mask, p1, op)
		return
	}
	var srcr draw.Rectangle
	var mr draw.Rectangle

	if drawclip(dst, &r, src, &p0, mask, &p1, &srcr, &mr) == 0 {
		if drawdebug != 0 {
			iprint("drawclip dstcr %v srccr %v maskcr %v\n", dst.Clipr, src.Clipr, mask.Clipr)
		}
		return
	}

	/*
	 * Convert to screen coordinates.
	 */
	dl := dst.Layer
	if dl != nil {
		r.Min.X += dl.Delta.X
		r.Min.Y += dl.Delta.Y
		r.Max.X += dl.Delta.X
		r.Max.Y += dl.Delta.Y
	}
Clearlayer:
	if dl != nil && dl.clear {
		if src == dst {
			p0.X += dl.Delta.X
			p0.Y += dl.Delta.Y
			src = dl.Screen.Image
		}
		dst = dl.Screen.Image
		goto Top
	}

	sl := src.Layer
	if sl != nil {
		p0.X += sl.Delta.X
		p0.Y += sl.Delta.Y
		srcr.Min.X += sl.Delta.X
		srcr.Min.Y += sl.Delta.Y
		srcr.Max.X += sl.Delta.X
		srcr.Max.Y += sl.Delta.Y
	}

	/*
	 * Now everything is in screen coordinates.
	 * mask is an image.  dst and src are images or obscured layers.
	 */

	/*
	 * if dst and src are the same layer, just draw in save area and expose.
	 */
	if dl != nil && dst == src {
		if dl.save == nil {
			return /* refresh function makes this case unworkable */
		}
		if draw.RectXRect(r, srcr) {
			tr := r
			if srcr.Min.X < tr.Min.X {
				p1.X += tr.Min.X - srcr.Min.X
				tr.Min.X = srcr.Min.X
			}
			if srcr.Min.Y < tr.Min.Y {
				p1.Y += tr.Min.X - srcr.Min.X
				tr.Min.Y = srcr.Min.Y
			}
			if srcr.Max.X > tr.Max.X {
				tr.Max.X = srcr.Max.X
			}
			if srcr.Max.Y > tr.Max.Y {
				tr.Max.Y = srcr.Max.Y
			}
			memlhide(dst, tr)
		} else {
			memlhide(dst, r)
			memlhide(dst, srcr)
		}
		Draw(dl.save, r.Sub(dl.Delta), dl.save, srcr.Min.Sub(src.Layer.Delta), mask, p1, op)
		memlexpose(dst, r)
		return
	}

	if sl != nil {
		if sl.clear {
			src = sl.Screen.Image
			if dl != nil {
				r.Min.X -= dl.Delta.X
				r.Min.Y -= dl.Delta.Y
				r.Max.X -= dl.Delta.X
				r.Max.Y -= dl.Delta.Y
			}
			goto Top
		}
		/* relatively rare case; use save area */
		if sl.save == nil {
			return /* refresh function makes this case unworkable */
		}
		memlhide(src, srcr)
		/* convert back to logical coordinates */
		p0.X -= sl.Delta.X
		p0.Y -= sl.Delta.Y
		srcr.Min.X -= sl.Delta.X
		srcr.Min.Y -= sl.Delta.Y
		srcr.Max.X -= sl.Delta.X
		srcr.Max.Y -= sl.Delta.Y
		src = src.Layer.save
	}

	/*
	 * src is now an image.  dst may be an image or a clear layer
	 */
	if dst.Layer == nil {
		goto Top
	}
	if dst.Layer.clear {
		goto Clearlayer
	}
	var d _Draw

	/*
	 * dst is an obscured layer
	 */
	d.deltas = p0.Sub(r.Min)
	d.deltam = p1.Sub(r.Min)
	d.dstlayer = dl
	d.src = src
	d.op = op
	d.mask = mask
	_memlayerop(ldrawop, dst, r, r, &d)
}
