// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import (
	"9fans.net/go/draw"
)

/*
 * Place i so i->r.min = log, i->layer->screenr.min == scr.
 */
func LOrigin(i *Image, log draw.Point, scr draw.Point) (int, error) {
	l := i.Layer
	s := l.Screen
	oldr := l.Screenr
	newr := draw.Rect(scr.X, scr.Y, scr.X+oldr.Dx(), scr.Y+oldr.Dy())
	eqscr := scr.Eq(oldr.Min)
	eqlog := log.Eq(i.R.Min)
	if eqscr && eqlog {
		return 0, nil
	}
	var nsave *Image
	if !eqlog && l.save != nil {
		var err error
		nsave, err = AllocImage(draw.Rect(log.X, log.Y, log.X+oldr.Dx(), log.Y+oldr.Dy()), i.Pix)
		if err != nil {
			return 0, err
		}
	}

	/*
	 * Bring it to front and move logical coordinate system.
	 */
	memltofront(i)
	wasclear := l.clear
	if nsave != nil {
		if !wasclear {
			nsave.Draw(nsave.R, l.save, l.save.R.Min, nil, draw.Pt(0, 0), draw.S)
		}
		Free(l.save)
		l.save = nsave
	}
	delta := log.Sub(i.R.Min)
	i.R = i.R.Add(delta)
	i.Clipr = i.Clipr.Add(delta)
	l.Delta = l.Screenr.Min.Sub(i.R.Min)
	if eqscr {
		return 0, nil
	}

	/*
	 * To clean up old position, make a shadow window there, don't paint it,
	 * push it behind this one, and (later) delete it.  Because the refresh function
	 * for this fake window is a no-op, this will cause no graphics action except
	 * to restore the background and expose the windows previously hidden.
	 */
	shad, err := LAlloc(s, oldr, LNoRefresh, nil, draw.NoFill)
	if err != nil {
		return 0, err
	}
	s.Frontmost = i
	if s.Rearmost == i {
		s.Rearmost = shad
	} else {
		l.rear.Layer.front = shad
	}
	shad.Layer.front = i
	shad.Layer.rear = l.rear
	l.rear = shad
	l.front = nil
	shad.Layer.clear = false

	/*
	 * Shadow is now holding down the fort at the old position.
	 * Move the window and hide things obscured by new position.
	 */
	for t := l.rear.Layer.rear; t != nil; t = t.Layer.rear {
		x := newr
		overlap := draw.RectClip(&x, t.Layer.Screenr)
		if overlap {
			memlhide(t, x)
			t.Layer.clear = false
		}
	}
	l.Screenr = newr
	l.Delta = scr.Sub(i.R.Min)
	l.clear = draw.RectInRect(newr, l.Screen.Image.Clipr)

	/*
	 * Everything's covered.  Copy to new position and delete shadow window.
	 */
	if wasclear {
		Draw(s.Image, newr, s.Image, oldr.Min, nil, draw.Pt(0, 0), draw.S)
	} else {
		memlexpose(i, newr)
	}
	LDelete(shad)

	return 1, nil
}

func LNoRefresh(l *Image, r draw.Rectangle, v interface{}) {
}
