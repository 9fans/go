// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import (
	"9fans.net/go/draw"
)

type lline struct {
	p0       draw.Point
	p1       draw.Point
	delta    draw.Point
	end0     draw.End
	end1     draw.End
	radius   int
	sp       draw.Point
	dstlayer *Layer
	src      *Image
	op       draw.Op
}

func _memline(dst *Image, p0 draw.Point, p1 draw.Point, end0, end1 draw.End, radius int, src *Image, sp draw.Point, clipr draw.Rectangle, op draw.Op) {
	if radius < 0 {
		return
	}
	if src.Layer != nil { /* can't draw line with layered source */
		return
	}
	srcclipped := 0

Top:
	dl := dst.Layer
	if dl == nil {
		_memimageline(dst, p0, p1, end0, end1, radius, src, sp, clipr, op)
		return
	}
	if srcclipped == 0 {
		d := sp.Sub(p0)
		if !draw.RectClip(&clipr, src.Clipr.Sub(d)) {
			return
		}
		if src.Flags&Frepl == 0 && !draw.RectClip(&clipr, src.R.Sub(d)) {
			return
		}
		srcclipped = 1
	}

	/* dst is known to be a layer */
	p0.X += dl.Delta.X
	p0.Y += dl.Delta.Y
	p1.X += dl.Delta.X
	p1.Y += dl.Delta.Y
	clipr.Min.X += dl.Delta.X
	clipr.Min.Y += dl.Delta.Y
	clipr.Max.X += dl.Delta.X
	clipr.Max.Y += dl.Delta.Y
	if dl.clear {
		dst = dst.Layer.Screen.Image
		goto Top
	}

	/* XXX */
	/* this is not the correct set of tests */
	/*	if(log2[dst->depth] != log2[src->depth] || log2[dst->depth]!=3) */
	/*		return; */

	/* can't use sutherland-cohen clipping because lines are wide */
	r := LineBBox(p0, p1, end0, end1, radius)
	/*
	 * r is now a bounding box for the line;
	 * use it as a clipping rectangle for subdivision
	 */
	if !draw.RectClip(&r, clipr) {
		return
	}
	var ll lline
	ll.p0 = p0
	ll.p1 = p1
	ll.end0 = end0
	ll.end1 = end1
	ll.sp = sp
	ll.dstlayer = dst.Layer
	ll.src = src
	ll.radius = radius
	ll.delta = dl.Delta
	ll.op = op
	_memlayerop(llineop, dst, r, r, &ll)
}

func llineop(dst *Image, screenr draw.Rectangle, clipr draw.Rectangle, etc interface{}, insave int) {
	ll := etc.(*lline)
	if insave != 0 && ll.dstlayer.save == nil {
		return
	}
	if !draw.RectClip(&clipr, screenr) {
		return
	}
	var p0 draw.Point
	var p1 draw.Point
	if insave != 0 {
		p0 = ll.p0.Sub(ll.delta)
		p1 = ll.p1.Sub(ll.delta)
		clipr = clipr.Sub(ll.delta)
	} else {
		p0 = ll.p0
		p1 = ll.p1
	}
	_memline(dst, p0, p1, ll.end0, ll.end1, ll.radius, ll.src, ll.sp, clipr, ll.op)
}

func Line(dst *Image, p0 draw.Point, p1 draw.Point, end0, end1 draw.End, radius int, src *Image, sp draw.Point, op draw.Op) {
	_memline(dst, p0, p1, end0, end1, radius, src, sp, dst.Clipr, op)
}
