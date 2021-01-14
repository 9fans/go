// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import "9fans.net/go/draw"

/*
 * Pull i towards top of screen, just behind front
 */
func _memltofront(i *Image, front *Image, fill bool) {
	l := i.Layer
	s := l.Screen
	for l.front != front {
		f := l.front
		x := l.Screenr
		overlap := draw.RectClip(&x, f.Layer.Screenr)
		if overlap {
			memlhide(f, x)
			f.Layer.clear = false
		}
		/* swap l and f in screen's list */
		ff := f.Layer.front
		rr := l.rear
		if ff == nil {
			s.Frontmost = i
		} else {
			ff.Layer.rear = i
		}
		if rr == nil {
			s.Rearmost = f
		} else {
			rr.Layer.front = f
		}
		l.front = ff
		l.rear = f
		f.Layer.front = i
		f.Layer.rear = rr
		if overlap && fill {
			memlexpose(i, x)
		}
	}
}

func _memltofrontfill(i *Image, fill bool) {
	_memltofront(i, nil, fill)
	_memlsetclear(i.Layer.Screen)
}

func memltofront(i *Image) {
	_memltofront(i, nil, true)
	_memlsetclear(i.Layer.Screen)
}

func LToFrontN(ip []*Image, n int) {
	if n == 0 {
		return
	}
	var front *Image
	for {
		if n--; n < 0 {
			break
		}
		i := ip[0]
		ip = ip[1:]
		_memltofront(i, front, true)
		front = i
	}
	s := front.Layer.Screen
	_memlsetclear(s)
}
