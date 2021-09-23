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
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

func movetodel(w *wind.Window) {
	n := wind.Delrunepos(w)
	if n < 0 {
		return
	}
	adraw.Display.MoveCursor(w.Tag.Fr.PointOf(n).Add(draw.Pt(4, w.Tag.Fr.Font.Height-4)))
}

func winresizeAndMouse(w *wind.Window, r draw.Rectangle, safe, keepextra bool) int {
	mouseintag := mouse.Point.In(w.Tag.All)
	mouseinbody := mouse.Point.In(w.Body.All)

	y := wind.Winresize(w, r, safe, keepextra)

	// If mouse is in tag, pull up as tag closes.
	if mouseintag && !mouse.Point.In(w.Tag.All) {
		p := mouse.Point
		p.Y = w.Tag.All.Max.Y - 3
		adraw.Display.MoveCursor(p)
	}

	// If mouse is in body, push down as tag expands.
	if mouseinbody && mouse.Point.In(w.Tag.All) {
		p := mouse.Point
		p.Y = w.Tag.All.Max.Y + 3
		adraw.Display.MoveCursor(p)
	}

	return y
}

func winmousebut(w *wind.Window) {
	adraw.Display.MoveCursor(w.Tag.ScrollR.Min.Add(draw.Pt(w.Tag.ScrollR.Dx(), adraw.Font.Height).Div(2)))
}

func wintype(w *wind.Window, t *wind.Text, r rune) {
	texttype(t, r)
	if t.What == wind.Body {
		for i := 0; i < len(t.File.Text); i++ {
			wind.Textscrdraw(t.File.Text[i])
		}
	}
	wind.Winsettag(w)
}

var fff *file.File
