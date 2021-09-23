// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <bio.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package ui

import (
	"unicode/utf8"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

func Rowdragcol(row *wind.Row, c *wind.Column, _0 int) {
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

	wind.Rowdragcol1(row, c, op, Mouse.Point)
	Clearmouse()
	Colmousebut(c)
}

func Rowtype(row *wind.Row, r rune, p draw.Point) *wind.Text {
	if r == 0 {
		r = utf8.RuneError
	}

	Clearmouse()
	row.Lk.Lock()
	var t *wind.Text
	if Bartflag {
		t = wind.Barttext
	} else {
		t = wind.Rowwhich(row, p)
	}
	if t != nil && (t.What != wind.Tag || !p.In(t.ScrollR)) {
		w := t.W
		if w == nil {
			Texttype(t, r)
		} else {
			wind.Winlock(w, 'K')
			Wintype(w, t, r)
			// Expand tag if necessary
			if t.What == wind.Tag {
				t.W.Tagsafe = false
				if r == '\n' {
					t.W.Tagexpand = true
				}
				WinresizeAndMouse(w, w.R, true, true)
			}
			wind.Winunlock(w)
		}
	}
	row.Lk.Unlock()
	return t
}

var Bartflag bool
