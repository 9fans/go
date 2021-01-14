// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import "9fans.net/go/draw"

func LDelete(i *Image) {
	l := i.Layer
	/* free backing store and disconnect refresh, to make pushback fast */
	Free(l.save)
	l.save = nil
	l.Refreshptr = nil
	memltorear(i)

	/* window is now the rearmost;  clean up screen structures and deallocate */
	s := i.Layer.Screen
	if s.Fill != nil {
		i.Clipr = i.R
		Draw(i, i.R, s.Fill, i.R.Min, nil, i.R.Min, draw.S)
	}
	if l.front != nil {
		l.front.Layer.rear = nil
		s.Rearmost = l.front
	} else {
		s.Frontmost = nil
		s.Rearmost = nil
	}
	Free(i)
}

/*
 * Just free the data structures, don't do graphics
 */
func LFree(i *Image) {
	l := i.Layer
	Free(l.save)
	Free(i)
}

func _memlsetclear(s *Screen) {
	for i := s.Rearmost; i != nil; i = i.Layer.front {
		l := i.Layer
		l.clear = draw.RectInRect(l.Screenr, l.Screen.Image.Clipr)
		if l.clear {
			for j := l.front; j != nil; j = j.Layer.front {
				if draw.RectXRect(l.Screenr, j.Layer.Screenr) {
					l.clear = false
					break
				}
			}
		}
	}
}
