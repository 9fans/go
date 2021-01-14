// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>

/*
 * Hide puts that portion of screenr now on the screen into the window's save area.
 * Expose puts that portion of screenr now in the save area onto the screen.
 *
 * Hide and Expose both require that the layer structures in the screen
 * match the geometry they are being asked to update, that is, they update the
 * save area (hide) or screen (expose) based on what those structures tell them.
 * This means they must be called at the correct time during window shuffles.
 */

package memdraw

import (
	"9fans.net/go/draw"
)

func lhideop(src *Image, screenr draw.Rectangle, clipr draw.Rectangle, etc interface{}, insave int) {
	l := etc.(*Layer)
	if src != l.save { /* do nothing if src is already in save area */
		r := screenr.Sub(l.Delta)
		Draw(l.save, r, src, screenr.Min, nil, screenr.Min, draw.S)
	}
}

func memlhide(i *Image, screenr draw.Rectangle) {
	if i.Layer.save == nil {
		return
	}
	if !draw.RectClip(&screenr, i.Layer.Screen.Image.R) {
		return
	}
	_memlayerop(lhideop, i, screenr, screenr, i.Layer)
}

func lexposeop(dst *Image, screenr draw.Rectangle, clipr draw.Rectangle, etc interface{}, insave int) {
	if insave != 0 { /* if dst is save area, don't bother */
		return
	}
	l := etc.(*Layer)
	r := screenr.Sub(l.Delta)
	if l.save != nil {
		Draw(dst, screenr, l.save, r.Min, nil, r.Min, draw.S)
	} else {
		l.Refreshfn(dst, r, l.Refreshptr)
	}
}

func memlexpose(i *Image, screenr draw.Rectangle) {
	if !draw.RectClip(&screenr, i.Layer.Screen.Image.R) {
		return
	}
	_memlayerop(lexposeop, i, screenr, screenr, i.Layer)
}
