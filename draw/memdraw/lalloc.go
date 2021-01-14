// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import (
	"9fans.net/go/draw"
)

var memlalloc_paint *Image

func LAlloc(s *Screen, screenr draw.Rectangle, refreshfn Refreshfn, refreshptr interface{}, val draw.Color) (*Image, error) {
	if memlalloc_paint == nil {
		var err error
		memlalloc_paint, err = AllocImage(draw.Rect(0, 0, 1, 1), draw.RGBA32)
		if err != nil {
			return nil, err
		}
		memlalloc_paint.Flags |= Frepl
		memlalloc_paint.Clipr = draw.Rect(-0x3FFFFFF, -0x3FFFFFF, 0x3FFFFFF, 0x3FFFFFF)
	}

	n, err := allocmemimaged(screenr, s.Image.Pix, s.Image.Data, nil)
	if err != nil {
		return nil, err
	}

	l := new(Layer)
	l.Screen = s
	if refreshfn != nil {
		l.save = nil
	} else {
		var err error
		l.save, err = AllocImage(screenr, s.Image.Pix)
		if err != nil {
			return nil, err
		}
		/* allocmemimage doesn't initialize memory; this paints save area */
		if val != draw.NoFill {
			FillColor(l.save, val)
		}
	}
	l.Refreshfn = refreshfn
	l.Refreshptr = nil /* don't set it until we're done */
	l.Screenr = screenr
	l.Delta = draw.Pt(0, 0)

	n.Data.ref++
	n.zero = s.Image.zero
	n.Width = s.Image.Width
	n.Layer = l

	/* start with new window behind all existing ones */
	l.front = s.Rearmost
	l.rear = nil
	if s.Rearmost != nil {
		s.Rearmost.Layer.rear = n
	}
	s.Rearmost = n
	if s.Frontmost == nil {
		s.Frontmost = n
	}
	l.clear = false

	/* now pull new window to front */
	_memltofrontfill(n, val != draw.NoFill)
	l.Refreshptr = refreshptr

	/*
	 * paint with requested color; previously exposed areas are already right
	 * if this window has backing store, but just painting the whole thing is simplest.
	 */
	if val != draw.NoFill {
		memsetchan(memlalloc_paint, n.Pix)
		FillColor(memlalloc_paint, val)
		Draw(n, n.R, memlalloc_paint, n.R.Min, nil, n.R.Min, draw.S)
	}
	return n, nil
}
