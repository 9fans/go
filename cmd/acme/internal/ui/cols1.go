// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package ui

import (
	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

func ColaddAndMouse(c *wind.Column, w *wind.Window, clone *wind.Window, y int) *wind.Window {
	w = wind.Coladd(c, w, clone, y)
	savemouse(w)
	// near the button, but in the body
	adraw.Display.MoveCursor(w.Tag.ScrollR.Max.Add(draw.Pt(3, 3)))
	wind.Barttext = &w.Body
	return w
}

func ColcloseAndMouse(c *wind.Column, w *wind.Window, dofree bool) {
	didmouse := restoremouse(w) != 0
	wr := w.R
	w = wind.Colclose(c, w, dofree)
	if !didmouse && w != nil && w.R.Min.Y == wr.Min.Y {
		w.Showdel = true
		wind.Winresize(w, w.R, false, true)
		movetodel(w)
	}
}

func Colmousebut(c *wind.Column) {
	adraw.Display.MoveCursor(c.Tag.ScrollR.Min.Add(c.Tag.ScrollR.Max).Div(2))
}

func Coldragwin(c *wind.Column, w *wind.Window, but int) {
	Clearmouse()
	adraw.Display.SwitchCursor2(&adraw.BoxCursor, &adraw.BoxCursor2)
	b := Mouse.Buttons
	op := Mouse.Point
	for Mouse.Buttons == b {
		Mousectl.Read()
	}
	adraw.Display.SwitchCursor(nil)
	if Mouse.Buttons != 0 {
		for Mouse.Buttons != 0 {
			Mousectl.Read()
		}
		return
	}
	wind.Coldragwin1(c, w, but, op, Mouse.Point)
	Winmousebut(w)
}
