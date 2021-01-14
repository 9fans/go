package memdraw

import (
	"9fans.net/go/draw"
)

func _layerop(fn func(*Image, draw.Rectangle, draw.Rectangle, interface{}, int), i *Image, r draw.Rectangle, clipr draw.Rectangle, etc interface{}, front *Image) {
	RECUR := func(a, b, c, d draw.Point) {
		_layerop(fn, i, draw.Rect(a.X, b.Y, c.X, d.Y), clipr, etc, front.Layer.rear)
	}

Top:
	if front == i {
		/* no one is in front of this part of window; use the screen */
		fn(i.Layer.Screen.Image, r, clipr, etc, 0)
		return
	}
	fr := front.Layer.Screenr
	if !draw.RectXRect(r, fr) {
		/* r doesn't touch this window; continue on next rearmost */
		/* assert(front && front->layer && front->layer->screen && front->layer->rear); */
		front = front.Layer.rear
		goto Top
	}
	if fr.Max.Y < r.Max.Y {
		RECUR(r.Min, fr.Max, r.Max, r.Max)
		r.Max.Y = fr.Max.Y
	}
	if r.Min.Y < fr.Min.Y {
		RECUR(r.Min, r.Min, r.Max, fr.Min)
		r.Min.Y = fr.Min.Y
	}
	if fr.Max.X < r.Max.X {
		RECUR(fr.Max, r.Min, r.Max, r.Max)
		r.Max.X = fr.Max.X
	}
	if r.Min.X < fr.Min.X {
		RECUR(r.Min, r.Min, fr.Min, r.Max)
		r.Min.X = fr.Min.X
	}
	/* r is covered by front, so put in save area */
	fn(i.Layer.save, r, clipr, etc, 1)
}

/*
 * Assumes incoming rectangle has already been clipped to i's logical r and clipr
 */
func _memlayerop(fn func(*Image, draw.Rectangle, draw.Rectangle, interface{}, int), i *Image, screenr draw.Rectangle, clipr draw.Rectangle, etc interface{}) {
	l := i.Layer
	if !draw.RectClip(&screenr, l.Screenr) {
		return
	}
	if l.clear {
		fn(l.Screen.Image, screenr, clipr, etc, 0)
		return
	}
	r := screenr
	scr := l.Screen.Image.Clipr

	/*
	 * Do the piece on the screen
	 */
	if draw.RectClip(&screenr, scr) {
		_layerop(fn, i, screenr, clipr, etc, l.Screen.Frontmost)
	}
	if draw.RectInRect(r, scr) {
		return
	}

	/*
	 * Do the piece off the screen
	 */
	if !draw.RectXRect(r, scr) {
		/* completely offscreen; easy */
		fn(l.save, r, clipr, etc, 1)
		return
	}
	if r.Min.Y < scr.Min.Y {
		/* above screen */
		fn(l.save, draw.Rect(r.Min.X, r.Min.Y, r.Max.X, scr.Min.Y), clipr, etc, 1)
		r.Min.Y = scr.Min.Y
	}
	if r.Max.Y > scr.Max.Y {
		/* below screen */
		fn(l.save, draw.Rect(r.Min.X, scr.Max.Y, r.Max.X, r.Max.Y), clipr, etc, 1)
		r.Max.Y = scr.Max.Y
	}
	if r.Min.X < scr.Min.X {
		/* left of screen */
		fn(l.save, draw.Rect(r.Min.X, r.Min.Y, scr.Min.X, r.Max.Y), clipr, etc, 1)
		r.Min.X = scr.Min.X
	}
	if r.Max.X > scr.Max.X {
		/* right of screen */
		fn(l.save, draw.Rect(scr.Max.X, r.Min.Y, r.Max.X, r.Max.Y), clipr, etc, 1)
	}
}
