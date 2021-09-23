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

package main

import (
	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

func coladdAndMouse(c *wind.Column, w *wind.Window, clone *wind.Window, y int) *wind.Window {
	w = wind.Coladd(c, w, clone, y)
	savemouse(w)
	// near the button, but in the body
	adraw.Display.MoveCursor(w.Tag.ScrollR.Max.Add(draw.Pt(3, 3)))
	wind.Barttext = &w.Body
	return w
}

func colcloseAndMouse(c *wind.Column, w *wind.Window, dofree bool) {
	didmouse := restoremouse(w) != 0
	wr := w.R
	w = wind.Colclose(c, w, dofree)
	if !didmouse && w != nil && w.R.Min.Y == wr.Min.Y {
		w.Showdel = true
		wind.Winresize(w, w.R, false, true)
		movetodel(w)
	}
}

func colmousebut(c *wind.Column) {
	adraw.Display.MoveCursor(c.Tag.ScrollR.Min.Add(c.Tag.ScrollR.Max).Div(2))
}

func coldragwin(c *wind.Column, w *wind.Window, but int) {
	clearmouse()
	adraw.Display.SwitchCursor2(&adraw.BoxCursor, &adraw.BoxCursor2)
	b := mouse.Buttons
	op := mouse.Point
	for mouse.Buttons == b {
		mousectl.Read()
	}
	adraw.Display.SwitchCursor(nil)
	if mouse.Buttons != 0 {
		for mouse.Buttons != 0 {
			mousectl.Read()
		}
		return
	}
	wind.Coldragwin1(c, w, but, op, mouse.Point)
	winmousebut(w)
}
