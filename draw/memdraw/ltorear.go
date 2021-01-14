// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

package memdraw

import "9fans.net/go/draw"

func _memltorear(i *Image, rear *Image) {
	l := i.Layer
	s := l.Screen
	for l.rear != rear {
		r := l.rear
		x := l.Screenr
		overlap := draw.RectClip(&x, r.Layer.Screenr)
		if overlap {
			memlhide(i, x)
			l.clear = false
		}
		/* swap l and r in screen's list */
		rr := r.Layer.rear
		f := l.front
		if rr == nil {
			s.Rearmost = i
		} else {
			rr.Layer.front = i
		}
		if f == nil {
			s.Frontmost = r
		} else {
			f.Layer.rear = r
		}
		l.rear = rr
		l.front = r
		r.Layer.rear = i
		r.Layer.front = f
		if overlap {
			memlexpose(r, x)
		}
	}
}

func memltorear(i *Image) {
	_memltorear(i, nil)
	_memlsetclear(i.Layer.Screen)
}

func LToRearN(ip []*Image, n int) {
	if n == 0 {
		return
	}
	var rear *Image
	for {
		n--
		if n < 0 {
			break
		}
		i := ip[0]
		ip = ip[1:]
		_memltorear(i, rear)
		rear = i
	}
	s := rear.Layer.Screen
	_memlsetclear(s)
}
