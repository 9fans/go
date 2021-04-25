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

	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
)

var prevmouse draw.Point
var mousew *Window

var Lpluserrors = []rune("+Errors")

func errorwin1(dir []rune, incl [][]rune) *Window {
	var r []rune
	if len(dir) > 0 {
		r = append(r, dir...)
		r = append(r, '/')
	}
	r = append(r, Lpluserrors...)
	w := lookfile(r)
	if w == nil {
		if len(row.col) == 0 {
			if rowadd(&row, nil, -1) == nil {
				util.Fatal("can't create column to make error window")
			}
		}
		w = coladd(row.col[len(row.col)-1], nil, nil, -1)
		w.filemenu = false
		winsetname(w, r)
		xfidlog(w, "new")
	}
	for i := len(incl) - 1; i >= 0; i-- {
		winaddincl(w, runes.Clone(incl[i]))
	}
	w.autoindent = globalautoindent
	return w
}

/* make new window, if necessary; return with it locked */
func errorwin(md *Mntdir, owner rune) *Window {
	var w *Window
	for {
		if md == nil {
			w = errorwin1(nil, nil)
		} else {
			w = errorwin1(md.dir, md.incl)
		}
		winlock(w, owner)
		if w.col != nil {
			break
		}
		/* window was deleted too fast */
		winunlock(w)
	}
	return w
}

/*
 * Incoming window should be locked.
 * It will be unlocked and returned window
 * will be locked in its place.
 */
func errorwinforwin(w *Window) *Window {
	t := &w.body
	dir := dirname(t, nil)
	if len(dir) == 1 && dir[0] == '.' { /* sigh */
		dir = nil
	}
	incl := make([][]rune, len(w.incl))
	for i := range w.incl {
		incl[i] = runes.Clone(w.incl[i])
	}
	owner := w.owner
	winunlock(w)
	for {
		w = errorwin1(dir, incl)
		winlock(w, owner)
		if w.col != nil {
			break
		}
		/* window deleted too fast */
		winunlock(w)
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

/* called while row is locked */
func flushwarnings() {
	var next *Warning
	for warn := warnings; warn != nil; warn = next {
		w := errorwin(warn.md, 'E')
		t := &w.body
		owner := w.owner
		if owner == 0 {
			w.owner = 'E'
		}
		wincommit(w, t)
		/*
		 * Most commands don't generate much output. For instance,
		 * Edit ,>cat goes through /dev/cons and is already in blocks
		 * because of the i/o system, but a few can.  Edit ,p will
		 * put the entire result into a single hunk.  So it's worth doing
		 * this in blocks (and putting the text in a buffer in the first
		 * place), to avoid a big memory footprint.
		 */
		r := fbufalloc()
		q0 := t.Len()
		var nr int
		for n := 0; n < warn.buf.Len(); n += nr {
			nr = warn.buf.Len() - n
			if nr > RBUFSIZE {
				nr = RBUFSIZE
			}
			warn.buf.Read(n, r[:nr])
			textbsinsert(t, t.Len(), r[:nr], true, &nr)
		}
		textshow(t, q0, t.Len(), true)
		winsettag(t.w)
		textscrdraw(t)
		w.owner = owner
		w.dirty = false
		winunlock(w)
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
	t := a.(*Text)
	if n >= t.Len() {
		return 0
	}
	return t.RuneAt(n)
}

func savemouse(w *Window) {
	prevmouse = mouse.Point
	mousew = w
}

func restoremouse(w *Window) int {
	did := 0
	if mousew != nil && mousew == w {
		display.MoveCursor(prevmouse)
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
func makenewwindow(t *Text) *Window {
	var c *Column
	if activecol != nil {
		c = activecol
	} else if seltext != nil && seltext.col != nil {
		c = seltext.col
	} else if t != nil && t.col != nil {
		c = t.col
	} else {
		if len(row.col) == 0 && rowadd(&row, nil, -1) == nil {
			util.Fatal("can't make column")
		}
		c = row.col[len(row.col)-1]
	}
	activecol = c
	if t == nil || t.w == nil || len(c.w) == 0 {
		return coladd(c, nil, nil, -1)
	}

	/* find biggest window and biggest blank spot */
	emptyw := c.w[0]
	bigw := emptyw
	var w *Window
	for i := 1; i < len(c.w); i++ {
		w = c.w[i]
		/* use >= to choose one near bottom of screen */
		if w.body.fr.MaxLines >= bigw.body.fr.MaxLines {
			bigw = w
		}
		if w.body.fr.MaxLines-w.body.fr.NumLines >= emptyw.body.fr.MaxLines-emptyw.body.fr.NumLines {
			emptyw = w
		}
	}
	emptyb := &emptyw.body
	el := emptyb.fr.MaxLines - emptyb.fr.NumLines
	var y int
	/* if empty space is big, use it */
	if el > 15 || (el > 3 && el > (bigw.body.fr.MaxLines-1)/2) {
		y = emptyb.fr.R.Min.Y + emptyb.fr.NumLines*font.Height
	} else {
		/* if this window is in column and isn't much smaller, split it */
		if t.col == c && t.w.r.Dy() > 2*bigw.r.Dy()/3 {
			bigw = t.w
		}
		y = (bigw.r.Min.Y + bigw.r.Max.Y) / 2
	}
	w = coladd(c, nil, nil, y)
	if w.body.fr.MaxLines < 2 {
		colgrow(w.col, w, 1)
	}
	return w
}
