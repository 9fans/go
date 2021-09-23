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
	"unicode/utf8"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

func savemouse(w *wind.Window) {
	prevmouse = Mouse.Point
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

func movetodel(w *wind.Window) {
	n := wind.Delrunepos(w)
	if n < 0 {
		return
	}
	adraw.Display.MoveCursor(w.Tag.Fr.PointOf(n).Add(draw.Pt(4, w.Tag.Fr.Font.Height-4)))
}

func Clearmouse() {
	mousew = nil
}

var Mouse *draw.Mouse

var Mousectl *draw.Mousectl

func Winmousebut(w *wind.Window) {
	adraw.Display.MoveCursor(w.Tag.ScrollR.Min.Add(draw.Pt(w.Tag.ScrollR.Dx(), adraw.Font.Height).Div(2)))
}

var mousew *wind.Window

var prevmouse draw.Point

func XCut(et, t, _ *wind.Text, dosnarf, docut bool, _ []rune) {

	/*
	 * if not executing a mouse chord (et != t) and snarfing (dosnarf)
	 * and executed Cut or Snarf in window tag (et->w != nil),
	 * then use the window body selection or the tag selection
	 * or do nothing at all.
	 */
	if et != t && dosnarf && et.W != nil {
		if et.W.Body.Q1 > et.W.Body.Q0 {
			t = &et.W.Body
			if docut {
				t.File.Mark() // seq has been incremented by execute
			}
		} else if et.W.Tag.Q1 > et.W.Tag.Q0 {
			t = &et.W.Tag
		} else {
			t = nil
		}
	}
	if t == nil { // no selection
		return
	}

	locked := false
	if t.W != nil && et.W != t.W {
		locked = true
		c := 'M'
		if et.W != nil {
			c = et.W.Owner
		}
		wind.Winlock(t.W, c)
	}
	if t.Q0 == t.Q1 {
		if locked {
			wind.Winunlock(t.W)
		}
		return
	}
	if dosnarf {
		q0 := t.Q0
		q1 := t.Q1
		snarfbuf.Delete(0, snarfbuf.Len())
		r := bufs.AllocRunes()
		for q0 < q1 {
			n := q1 - q0
			if n > bufs.RuneLen {
				n = bufs.RuneLen
			}
			t.File.Read(q0, r[:n])
			snarfbuf.Insert(snarfbuf.Len(), r[:n])
			q0 += n
		}
		bufs.FreeRunes(r)
		acmeputsnarf()
	}
	if docut {
		wind.Textdelete(t, t.Q0, t.Q1, true)
		wind.Textsetselect(t, t.Q0, t.Q0)
		if t.W != nil {
			wind.Textscrdraw(t)
			wind.Winsettag(t.W)
		}
	} else if dosnarf { // Snarf command
		wind.Argtext = t
	}
	if locked {
		wind.Winunlock(t.W)
	}
}

func XPaste(et, t, _ *wind.Text, selectall, tobody bool, _ []rune) {

	// if(tobody), use body of executing window  (Paste or Send command)
	if tobody && et != nil && et.W != nil {
		t = &et.W.Body
		t.File.Mark() // seq has been incremented by execute
	}
	if t == nil {
		return
	}

	acmegetsnarf()
	if t == nil || snarfbuf.Len() == 0 {
		return
	}
	if t.W != nil && et.W != t.W {
		c := 'M'
		if et.W != nil {
			c = et.W.Owner
		}
		wind.Winlock(t.W, c)
	}
	XCut(t, t, nil, false, true, nil)
	q := 0
	q0 := t.Q0
	q1 := t.Q0 + snarfbuf.Len()
	r := bufs.AllocRunes()
	for q0 < q1 {
		n := q1 - q0
		if n > bufs.RuneLen {
			n = bufs.RuneLen
		}
		snarfbuf.Read(q, r[:n])
		wind.Textinsert(t, q0, r[:n], true)
		q += n
		q0 += n
	}
	bufs.FreeRunes(r)
	if selectall {
		wind.Textsetselect(t, t.Q0, q1)
	} else {
		wind.Textsetselect(t, q1, q1)
	}
	if t.W != nil {
		wind.Textscrdraw(t)
		wind.Winsettag(t.W)
	}
	if t.W != nil && et.W != t.W {
		wind.Winunlock(t.W)
	}
}

func XUndo(et, _, _ *wind.Text, isundo, _ bool, _ []rune) {
	if et == nil || et.W == nil {
		return
	}
	seq := seqof(et.W, isundo)
	if seq == 0 {
		// nothing to undo
		return
	}
	/*
	 * Undo the executing window first. Its display will update. other windows
	 * in the same file will not call show() and jump to a different location in the file.
	 * Simultaneous changes to other files will be chaotic, however.
	 */
	wind.Winundo(et.W, isundo)
	for i := 0; i < len(wind.TheRow.Col); i++ {
		c := wind.TheRow.Col[i]
		for j := 0; j < len(c.W); j++ {
			w := c.W[j]
			if w == et.W {
				continue
			}
			if seqof(w, isundo) == seq {
				wind.Winundo(w, isundo)
			}
		}
	}
}

const (
	Kscrolloneup   = draw.KeyFn | 0x20
	Kscrollonedown = draw.KeyFn | 0x21
)

/*
 * /dev/snarf updates when the file is closed, so we must open our own
 * fd here rather than use snarffd
 */

/* rio truncates larges snarf buffers, so this avoids using the
 * service if the string is huge */

const MAXSNARF = 100 * 1024

const (
	NSnarf = 1000
)

var snarfrune [NSnarf + 1]rune

var snarfbuf disk.Buffer

func acmeputsnarf() {
	if snarfbuf.Len() == 0 {
		return
	}
	if snarfbuf.Len() > MAXSNARF {
		return
	}

	var buf []byte
	var n int
	for i := 0; i < snarfbuf.Len(); i += n {
		n = snarfbuf.Len() - i
		if n >= NSnarf {
			n = NSnarf
		}
		snarfbuf.Read(i, snarfrune[:n])
		var rbuf [utf8.UTFMax]byte
		for _, r := range snarfrune[:n] {
			w := utf8.EncodeRune(rbuf[:], r)
			buf = append(buf, rbuf[:w]...)
		}
	}
	if len(buf) > 0 {
		adraw.Display.WriteSnarf(buf)
	}
}

func acmegetsnarf() {
	_, m, err := adraw.Display.ReadSnarf(nil)
	if err != nil {
		return
	}
	buf := make([]byte, m+100)
	n, _, err := adraw.Display.ReadSnarf(buf)
	if n == 0 || err != nil {
		return
	}
	buf = buf[:n]

	r := make([]rune, utf8.RuneCount(buf))
	_, nr, _ := runes.Convert(buf, r, true)
	snarfbuf.Reset()
	snarfbuf.Insert(0, r[:nr])
}

func seqof(w *wind.Window, isundo bool) int {
	// if it's undo, see who changed with us
	if isundo {
		return w.Body.File.Seq()
	}
	// if it's redo, see who we'll be sync'ed up with
	return w.Body.File.RedoSeq()
}

/*
 * Heuristic city.
 */
func Makenewwindow(t *wind.Text) *wind.Window {
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
		return ColaddAndMouse(c, nil, nil, -1)
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
	w = ColaddAndMouse(c, nil, nil, y)
	if w.Body.Fr.MaxLines < 2 {
		wind.Colgrow(w.Col, w, 1)
	}
	return w
}
