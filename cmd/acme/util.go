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
	"fmt"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

var prevmouse draw.Point
var mousew *wind.Window

func errorwin1(dir []rune, incl [][]rune) *wind.Window {
	var r []rune
	if len(dir) > 0 {
		r = append(r, dir...)
		r = append(r, '/')
	}
	r = append(r, []rune("+Errors")...)
	w := lookfile(r)
	if w == nil {
		if len(wind.TheRow.Col) == 0 {
			if wind.RowAdd(&wind.TheRow, nil, -1) == nil {
				util.Fatal("can't create column to make error window")
			}
		}
		w = coladdAndMouse(wind.TheRow.Col[len(wind.TheRow.Col)-1], nil, nil, -1)
		w.Filemenu = false
		wind.Winsetname(w, r)
		xfidlog(w, "new")
	}
	for i := len(incl) - 1; i >= 0; i-- {
		wind.Winaddincl(w, runes.Clone(incl[i]))
	}
	w.Autoindent = wind.GlobalAutoindent
	return w
}

// make new window, if necessary; return with it locked
func errorwin(md *Mntdir, owner rune) *wind.Window {
	var w *wind.Window
	for {
		if md == nil {
			w = errorwin1(nil, nil)
		} else {
			w = errorwin1(md.dir, md.incl)
		}
		wind.Winlock(w, owner)
		if w.Col != nil {
			break
		}
		// window was deleted too fast
		wind.Winunlock(w)
	}
	return w
}

/*
 * Incoming window should be locked.
 * It will be unlocked and returned window
 * will be locked in its place.
 */
func errorwinforwin(w *wind.Window) *wind.Window {
	t := &w.Body
	dir := wind.Dirname(t, nil)
	if len(dir) == 1 && dir[0] == '.' { // sigh
		dir = nil
	}
	incl := make([][]rune, len(w.Incl))
	for i := range w.Incl {
		incl[i] = runes.Clone(w.Incl[i])
	}
	owner := w.Owner
	wind.Winunlock(w)
	for {
		w = errorwin1(dir, incl)
		wind.Winlock(w, owner)
		if w.Col != nil {
			break
		}
		// window deleted too fast
		wind.Winunlock(w)
	}
	return w
}

type Warning struct {
	md   *Mntdir
	buf  disk.Buffer
	next *Warning
}

var warnings *Warning

func addwarningtext(md *Mntdir, r []rune) {
	for warn := warnings; warn != nil; warn = warn.next {
		if warn.md == md {
			warn.buf.Insert(warn.buf.Len(), r)
			return
		}
	}
	warn := new(Warning)
	warn.next = warnings
	warn.md = md
	if md != nil {
		fsysincid(md)
	}
	warnings = warn
	warn.buf.Insert(0, r)
	select {
	case cwarn <- 0:
	default:
	}
}

// called while row is locked
func flushwarnings() {
	var next *Warning
	for warn := warnings; warn != nil; warn = next {
		w := errorwin(warn.md, 'E')
		t := &w.Body
		owner := w.Owner
		if owner == 0 {
			w.Owner = 'E'
		}
		wind.Wincommit(w, t)
		/*
		 * Most commands don't generate much output. For instance,
		 * Edit ,>cat goes through /dev/cons and is already in blocks
		 * because of the i/o system, but a few can.  Edit ,p will
		 * put the entire result into a single hunk.  So it's worth doing
		 * this in blocks (and putting the text in a buffer in the first
		 * place), to avoid a big memory footprint.
		 */
		r := bufs.AllocRunes()
		q0 := t.Len()
		var nr int
		for n := 0; n < warn.buf.Len(); n += nr {
			nr = warn.buf.Len() - n
			if nr > bufs.RuneLen {
				nr = bufs.RuneLen
			}
			warn.buf.Read(n, r[:nr])
			wind.Textbsinsert(t, t.Len(), r[:nr], true, &nr)
		}
		wind.Textshow(t, q0, t.Len(), true)
		wind.Winsettag(t.W)
		wind.Textscrdraw(t)
		w.Owner = owner
		w.Dirty = false
		wind.Winunlock(w)
		warn.buf.Close()
		next = warn.next
		if warn.md != nil {
			fsysdelid(warn.md)
		}
	}
	warnings = nil
}

func warning(md *Mntdir, format string, args ...interface{}) {
	addwarningtext(md, []rune(fmt.Sprintf(format, args...)))
}

func rgetc(v interface{}, n int) rune {
	r := v.([]rune)
	if n >= len(r) {
		return 0
	}
	return r[n]
}

func tgetc(a interface{}, n int) rune {
	t := a.(*wind.Text)
	if n >= t.Len() {
		return 0
	}
	return t.RuneAt(n)
}

func savemouse(w *wind.Window) {
	prevmouse = mouse.Point
	mousew = w
}

func restoremouse(w *wind.Window) int {
	did := 0
	if mousew != nil && mousew == w {
		adraw.Display.MoveCursor(prevmouse)
		did = 1
	}
	mousew = nil
	return did
}

func clearmouse() {
	mousew = nil
}

/*
 * Heuristic city.
 */
func makenewwindow(t *wind.Text) *wind.Window {
	var c *wind.Column
	if wind.Activecol != nil {
		c = wind.Activecol
	} else if wind.Seltext != nil && wind.Seltext.Col != nil {
		c = wind.Seltext.Col
	} else if t != nil && t.Col != nil {
		c = t.Col
	} else {
		if len(wind.TheRow.Col) == 0 && wind.RowAdd(&wind.TheRow, nil, -1) == nil {
			util.Fatal("can't make column")
		}
		c = wind.TheRow.Col[len(wind.TheRow.Col)-1]
	}
	wind.Activecol = c
	if t == nil || t.W == nil || len(c.W) == 0 {
		return coladdAndMouse(c, nil, nil, -1)
	}

	// find biggest window and biggest blank spot
	emptyw := c.W[0]
	bigw := emptyw
	var w *wind.Window
	for i := 1; i < len(c.W); i++ {
		w = c.W[i]
		// use >= to choose one near bottom of screen
		if w.Body.Fr.MaxLines >= bigw.Body.Fr.MaxLines {
			bigw = w
		}
		if w.Body.Fr.MaxLines-w.Body.Fr.NumLines >= emptyw.Body.Fr.MaxLines-emptyw.Body.Fr.NumLines {
			emptyw = w
		}
	}
	emptyb := &emptyw.Body
	el := emptyb.Fr.MaxLines - emptyb.Fr.NumLines
	var y int
	// if empty space is big, use it
	if el > 15 || (el > 3 && el > (bigw.Body.Fr.MaxLines-1)/2) {
		y = emptyb.Fr.R.Min.Y + emptyb.Fr.NumLines*adraw.Font.Height
	} else {
		// if this window is in column and isn't much smaller, split it
		if t.Col == c && t.W.R.Dy() > 2*bigw.R.Dy()/3 {
			bigw = t.W
		}
		y = (bigw.R.Min.Y + bigw.R.Max.Y) / 2
	}
	w = coladdAndMouse(c, nil, nil, y)
	if w.Body.Fr.MaxLines < 2 {
		wind.Colgrow(w.Col, w, 1)
	}
	return w
}
