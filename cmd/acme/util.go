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
	"log"
	"strings"
	"unicode/utf8"

	"9fans.net/go/draw"
)

var prevmouse draw.Point
var mousew *Window

func range_(q0 int, q1 int) Range {
	var r Range
	r.q0 = q0
	r.q1 = q1
	return r
}

func runestr(r []rune) Runestr {
	return Runestr{r}
}

// cvttorunes converts bytes in b to runes in r,
// returning the number of bytes processed from b,
// the number of runes written to r,
// and whether any null bytes were elided.
// If eof is true, then any partial runes at the end of b
// should be processed, and nb == len(b) at return.
// Otherwise, partial runes are left behind and
// nb may be up to utf8.UTFMax-1 bytes short of len(b).
func cvttorunes(b []byte, r []rune, eof bool) (nb, nr int, nulls bool) {
	b0 := b
	for len(b) > 0 && (eof || len(b) >= utf8.UTFMax || utf8.FullRune(b)) {
		rr, w := utf8.DecodeRune(b)
		if rr == 0 {
			nulls = true
		} else {
			r[nr] = rr
			nr++
		}
		b = b[w:]
	}
	nb = len(b0) - len(b)
	return nb, nr, nulls
}

func error_(s string) {
	log.Fatalf("acme: %s\n", s)
}

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
				error_("can't create column to make error window")
			}
		}
		w = coladd(row.col[len(row.col)-1], nil, nil, -1)
		w.filemenu = false
		winsetname(w, r)
		xfidlog(w, "new")
	}
	for i := len(incl) - 1; i >= 0; i-- {
		winaddincl(w, runestrdup(incl[i]))
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
	if len(dir.r) == 1 && dir.r[0] == '.' { /* sigh */
		dir.r = nil
	}
	incl := make([][]rune, len(w.incl))
	for i := range w.incl {
		incl[i] = runestrdup(w.incl[i])
	}
	owner := w.owner
	winunlock(w)
	for {
		w = errorwin1(dir.r, incl)
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
	buf  Buffer
	next *Warning
}

var warnings *Warning

func addwarningtext(md *Mntdir, r []rune) {
	for warn := warnings; warn != nil; warn = warn.next {
		if warn.md == md {
			bufinsert(&warn.buf, warn.buf.nc, r)
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
	bufinsert(&warn.buf, 0, r)
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
		q0 := t.file.b.nc
		var nr int
		for n := 0; n < warn.buf.nc; n += nr {
			nr = warn.buf.nc - n
			if nr > RBUFSIZE {
				nr = RBUFSIZE
			}
			bufread(&warn.buf, n, r[:nr])
			textbsinsert(t, t.file.b.nc, r[:nr], true, &nr)
		}
		textshow(t, q0, t.file.b.nc, true)
		winsettag(t.w)
		textscrdraw(t)
		w.owner = owner
		w.dirty = false
		winunlock(w)
		bufclose(&warn.buf)
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

func runeeq(s1, s2 []rune) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func runetobyte(r []rune) string {
	return string(r)
}

func bytetorune(s string) []rune {
	r := make([]rune, utf8.RuneCountInString(s))
	_, nr, _ := cvttorunes([]byte(s), r, true) // TODO avoid alloc
	return r[:nr]
}

func isalnum(c rune) bool {
	/*
	 * Hard to get absolutely right.  Use what we know about ASCII
	 * and assume anything above the Latin control characters is
	 * potentially an alphanumeric.
	 */
	if c <= ' ' {
		return false
	}
	if 0x7F <= c && c <= 0xA0 {
		return false
	}
	if strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^`{|}~", c) {
		return false
	}
	return true
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
	if n >= t.file.b.nc {
		return 0
	}
	return textreadc(t, n)
}

func skipbl(r []rune) []rune {
	for len(r) > 0 && (r[0] == ' ' || r[0] == '\t' || r[0] == '\n') {
		r = r[1:]
	}
	return r
}

func findbl(r []rune) []rune {
	for len(r) > 0 && r[0] != ' ' && r[0] != '\t' && r[0] != '\n' {
		r = r[1:]
	}
	return r
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

func estrdup(s string) string {
	return s
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
			error_("can't make column")
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
