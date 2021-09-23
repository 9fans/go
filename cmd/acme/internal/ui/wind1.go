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
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

func WinresizeAndMouse(w *wind.Window, r draw.Rectangle, safe, keepextra bool) int {
	mouseintag := Mouse.Point.In(w.Tag.All)
	mouseinbody := Mouse.Point.In(w.Body.All)

	y := wind.Winresize(w, r, safe, keepextra)

	// If mouse is in tag, pull up as tag closes.
	if mouseintag && !Mouse.Point.In(w.Tag.All) {
		p := Mouse.Point
		p.Y = w.Tag.All.Max.Y - 3
		adraw.Display.MoveCursor(p)
	}

	// If mouse is in body, push down as tag expands.
	if mouseinbody && Mouse.Point.In(w.Tag.All) {
		p := Mouse.Point
		p.Y = w.Tag.All.Max.Y + 3
		adraw.Display.MoveCursor(p)
	}

	return y
}

func Wintype(w *wind.Window, t *wind.Text, r rune) {
	Texttype(t, r)
	if t.What == wind.Body {
		for i := 0; i < len(t.File.Text); i++ {
			wind.Textscrdraw(t.File.Text[i])
		}
	}
	wind.Winsettag(w)
}

var fff *file.File
