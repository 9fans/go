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
// #include <complete.h>
// #include "dat.h"
// #include "fns.h"

package ui

import (
	"time"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var Textcomplete func(*wind.Text) []rune

func Textconstrain(t *wind.Text, q0 int, q1 int, p0 *int, p1 *int) {
	*p0 = util.Min(q0, t.Len())
	*p1 = util.Min(q1, t.Len())
}

func Texttype(t *wind.Text, r rune) {
	if t.What != wind.Body && t.What != wind.Tag && r == '\n' {
		return
	}
	if t.What == wind.Tag {
		t.W.Tagsafe = false
	}

	var q0 int
	var nnb int
	var n int
	switch r {
	case draw.KeyLeft:
		wind.Typecommit(t)
		if t.Q0 > 0 {
			wind.Textshow(t, t.Q0-1, t.Q0-1, true)
		}
		return
	case draw.KeyRight:
		wind.Typecommit(t)
		if t.Q1 < t.Len() {
			wind.Textshow(t, t.Q1+1, t.Q1+1, true)
		}
		return
	case draw.KeyDown, draw.KeyPageDown, Kscrollonedown:
		if t.What == wind.Tag {
			// expand tag to show all text
			if !t.W.Tagexpand {
				t.W.Tagexpand = true
				WinresizeAndMouse(t.W, t.W.R, false, true)
			}
			return
		}
		switch r {
		case draw.KeyDown:
			n = t.Fr.MaxLines / 3
		case draw.KeyPageDown:
			n = 2 * t.Fr.MaxLines / 3
		case Kscrollonedown:
			n = draw.MouseScrollSize(t.Fr.MaxLines)
			if n <= 0 {
				n = 1
			}
		}
		q0 = t.Org + t.Fr.CharOf(draw.Pt(t.Fr.R.Min.X, t.Fr.R.Min.Y+n*t.Fr.Font.Height))
		wind.Textsetorigin(t, q0, true)
		return
	case draw.KeyUp, draw.KeyPageUp, Kscrolloneup:
		if t.What == wind.Tag {
			// shrink tag to single line
			if t.W.Tagexpand {
				t.W.Tagexpand = false
				t.W.Taglines = 1
				WinresizeAndMouse(t.W, t.W.R, false, true)
			}
			return
		}
		switch r {
		case draw.KeyUp:
			n = t.Fr.MaxLines / 3
		case draw.KeyPageUp:
			n = 2 * t.Fr.MaxLines / 3
		case Kscrolloneup:
			n = draw.MouseScrollSize(t.Fr.MaxLines)
		}
		q0 = wind.Textbacknl(t, t.Org, n)
		wind.Textsetorigin(t, q0, true)
		return
	case draw.KeyHome:
		wind.Typecommit(t)
		if t.Org > t.IQ1 {
			q0 = wind.Textbacknl(t, t.IQ1, 1)
			wind.Textsetorigin(t, q0, true)
		} else {
			wind.Textshow(t, 0, 0, false)
		}
		return
	case draw.KeyEnd:
		wind.Typecommit(t)
		if t.IQ1 > t.Org+t.Fr.NumChars {
			if t.IQ1 > t.Len() {
				// should not happen, but does. and it will crash textbacknl.
				t.IQ1 = t.Len()
			}
			q0 = wind.Textbacknl(t, t.IQ1, 1)
			wind.Textsetorigin(t, q0, true)
		} else {
			wind.Textshow(t, t.Len(), t.Len(), false)
		}
		return
	case 0x01: // ^A: beginning of line
		wind.Typecommit(t)
		// go to where ^U would erase, if not already at BOL
		nnb = 0
		if t.Q0 > 0 && t.RuneAt(t.Q0-1) != '\n' {
			nnb = wind.Textbswidth(t, 0x15)
		}
		wind.Textshow(t, t.Q0-nnb, t.Q0-nnb, true)
		return
	case 0x05: // ^E: end of line
		wind.Typecommit(t)
		q0 = t.Q0
		for q0 < t.Len() && t.RuneAt(q0) != '\n' {
			q0++
		}
		wind.Textshow(t, q0, q0, true)
		return
	case draw.KeyCmd + 'c': // %C: copy
		wind.Typecommit(t)
		XCut(t, t, nil, true, false, nil)
		return
	case draw.KeyCmd + 'z': // %Z: undo
		wind.Typecommit(t)
		XUndo(t, nil, nil, true, false, nil)
		return
	case draw.KeyCmd + 'Z': // %-shift-Z: redo
		wind.Typecommit(t)
		XUndo(t, nil, nil, false, false, nil)
		return
	}
	if t.What == wind.Body {
		file.Seq++
		t.File.Mark()
	}
	// cut/paste must be done after the seq++/filemark
	switch r {
	case draw.KeyCmd + 'x': // %X: cut
		wind.Typecommit(t)
		if t.What == wind.Body {
			file.Seq++
			t.File.Mark()
		}
		XCut(t, t, nil, true, true, nil)
		wind.Textshow(t, t.Q0, t.Q0, true)
		t.IQ1 = t.Q0
		return
	case draw.KeyCmd + 'v': // %V: paste
		wind.Typecommit(t)
		if t.What == wind.Body {
			file.Seq++
			t.File.Mark()
		}
		XPaste(t, t, nil, true, false, nil)
		wind.Textshow(t, t.Q0, t.Q1, true)
		t.IQ1 = t.Q1
		return
	}
	if t.Q1 > t.Q0 {
		if len(t.Cache) != 0 {
			util.Fatal("text.type")
		}
		XCut(t, t, nil, true, true, nil)
		t.Eq0 = ^0
	}
	wind.Textshow(t, t.Q0, t.Q0, true)
	var q1 int
	var nb int
	var i int
	var u *wind.Text
	rp := []rune{r}
	switch r {
	case 0x06, // ^F: complete
		draw.KeyInsert:
		wind.Typecommit(t)
		if Textcomplete == nil {
			rp = nil
		} else {
			rp = Textcomplete(t)
		}
		if rp == nil {
			return
		}

		// break to normal insertion case
	case 0x1B:
		if t.Eq0 != ^0 {
			if t.Eq0 <= t.Q0 {
				wind.Textsetselect(t, t.Eq0, t.Q0)
			} else {
				wind.Textsetselect(t, t.Q0, t.Eq0)
			}
		}
		if len(t.Cache) > 0 {
			wind.Typecommit(t)
		}
		t.IQ1 = t.Q0
		return
	case 0x08, // ^H: erase character
		0x15, // ^U: erase line
		0x17: // ^W: erase word
		if t.Q0 == 0 { // nothing to erase
			return
		}
		nnb = wind.Textbswidth(t, r)
		q1 = t.Q0
		q0 = q1 - nnb
		// if selection is at beginning of window, avoid deleting invisible text
		if q0 < t.Org {
			q0 = t.Org
			nnb = q1 - q0
		}
		if nnb <= 0 {
			return
		}
		for i = 0; i < len(t.File.Text); i++ {
			u = t.File.Text[i]
			u.Nofill = true
			nb = nnb
			n = len(u.Cache)
			if n > 0 {
				if q1 != u.Cq0+n {
					util.Fatal("text.type backspace")
				}
				if n > nb {
					n = nb
				}
				u.Cache = u.Cache[:len(u.Cache)-n]
				wind.Textdelete(u, q1-n, q1, false)
				nb -= n
			}
			if u.Eq0 == q1 || u.Eq0 == ^0 {
				u.Eq0 = q0
			}
			if nb != 0 && u == t {
				wind.Textdelete(u, q0, q0+nb, true)
			}
			if u != t {
				wind.Textsetselect(u, u.Q0, u.Q1)
			} else {
				wind.Textsetselect(t, q0, q0)
			}
			u.Nofill = false
		}
		for i = 0; i < len(t.File.Text); i++ {
			wind.Textfill(t.File.Text[i])
		}
		t.IQ1 = t.Q0
		return
	case '\n':
		if t.W.Autoindent {
			// find beginning of previous line using backspace code
			nnb = wind.Textbswidth(t, 0x15) // ^U case
			rp = make([]rune, 1, nnb+1)
			rp[0] = '\n'
			for i = 0; i < nnb; i++ {
				r = t.RuneAt(t.Q0 - nnb + i)
				if r != ' ' && r != '\t' {
					break
				}
				rp = append(rp, r)
			}
		}
		// break to normal code
	}
	// otherwise ordinary character; just insert, typically in caches of all texts
	for i = 0; i < len(t.File.Text); i++ {
		u = t.File.Text[i]
		if u.Eq0 == ^0 {
			u.Eq0 = t.Q0
		}
		if len(u.Cache) == 0 {
			u.Cq0 = t.Q0
		} else if t.Q0 != u.Cq0+len(u.Cache) {
			util.Fatal("text.type cq1")
		}
		/*
		 * Change the tag before we add to ncache,
		 * so that if the window body is resized the
		 * commit will not find anything in ncache.
		 */
		if u.What == wind.Body && len(u.Cache) == 0 {
			u.Needundo = true
			wind.Winsettag(t.W)
			u.Needundo = false
		}
		wind.Textinsert(u, t.Q0, rp, false)
		if u != t {
			wind.Textsetselect(u, u.Q0, u.Q1)
		}
		u.Cache = append(u.Cache, rp...)
	}
	wind.Textsetselect(t, t.Q0+len(rp), t.Q0+len(rp))
	if r == '\n' && t.W != nil {
		wind.Wincommit(t.W, t)
	}
	t.IQ1 = t.Q0
}

var clicktext *wind.Text

var clickmsec uint32

var selecttext *wind.Text

var selectq int

/*
 * called from frame library
 */
func framescroll(f *frame.Frame, dl int) {
	if f != &selecttext.Fr {
		util.Fatal("frameselect not right frame")
	}
	textframescroll(selecttext, dl)
}

func textframescroll(t *wind.Text, dl int) {
	if dl == 0 {
		scrsleep(100 * time.Millisecond)
		return
	}
	var q0 int
	if dl < 0 {
		q0 = wind.Textbacknl(t, t.Org, -dl)
		if selectq > t.Org+t.Fr.P0 {
			wind.Textsetselect(t, t.Org+t.Fr.P0, selectq)
		} else {
			wind.Textsetselect(t, selectq, t.Org+t.Fr.P0)
		}
	} else {
		if t.Org+t.Fr.NumChars == t.Len() {
			return
		}
		q0 = t.Org + t.Fr.CharOf(draw.Pt(t.Fr.R.Min.X, t.Fr.R.Min.Y+dl*t.Fr.Font.Height))
		if selectq > t.Org+t.Fr.P1 {
			wind.Textsetselect(t, t.Org+t.Fr.P1, selectq)
		} else {
			wind.Textsetselect(t, selectq, t.Org+t.Fr.P1)
		}
	}
	wind.Textsetorigin(t, q0, true)
}

func Textselect(t *wind.Text) {
	const (
		None = iota
		Cut
		Paste
	)

	selecttext = t
	/*
	 * To have double-clicking and chording, we double-click
	 * immediately if it might make sense.
	 */
	b := Mouse.Buttons
	q0 := t.Q0
	q1 := t.Q1
	selectq = t.Org + t.Fr.CharOf(Mouse.Point)
	if clicktext == t && Mouse.Msec-clickmsec < 500 {
		if q0 == q1 && selectq == q0 {
			wind.Textdoubleclick(t, &q0, &q1)
			wind.Textsetselect(t, q0, q1)
			adraw.Display.Flush()
			x := Mouse.Point.X
			y := Mouse.Point.Y
			// stay here until something interesting happens
			for {
				Mousectl.Read()
				if !(Mouse.Buttons == b && util.Abs(Mouse.Point.X-x) < 3) || !(util.Abs(Mouse.Point.Y-y) < 3) {
					break
				}
			}
			Mouse.Point.X = x // in case we're calling frselect
			Mouse.Point.Y = y
			q0 = t.Q0 // may have changed
			q1 = t.Q1
			selectq = q0
		}
	}
	if Mouse.Buttons == b {
		t.Fr.Scroll = framescroll
		t.Fr.Select(Mousectl)
		// horrible botch: while asleep, may have lost selection altogether
		if selectq > t.Len() {
			selectq = t.Org + t.Fr.P0
		}
		t.Fr.Scroll = nil
		if selectq < t.Org {
			q0 = selectq
		} else {
			q0 = t.Org + t.Fr.P0
		}
		if selectq > t.Org+t.Fr.NumChars {
			q1 = selectq
		} else {
			q1 = t.Org + t.Fr.P1
		}
	}
	if q0 == q1 {
		if q0 == t.Q0 && clicktext == t && Mouse.Msec-clickmsec < 500 {
			wind.Textdoubleclick(t, &q0, &q1)
			clicktext = nil
		} else {
			clicktext = t
			clickmsec = Mouse.Msec
		}
	} else {
		clicktext = nil
	}
	wind.Textsetselect(t, q0, q1)
	adraw.Display.Flush()
	state := None // what we've done; undo when possible
	for Mouse.Buttons != 0 {
		Mouse.Msec = 0
		b = Mouse.Buttons
		if b&1 != 0 && b&6 != 0 {
			if state == None && t.What == wind.Body {
				file.Seq++
				t.W.Body.File.Mark()
			}
			if b&2 != 0 {
				if state == Paste && t.What == wind.Body {
					wind.Winundo(t.W, true)
					wind.Textsetselect(t, q0, t.Q1)
					state = None
				} else if state != Cut {
					XCut(t, t, nil, true, true, nil)
					state = Cut
				}
			} else {
				if state == Cut && t.What == wind.Body {
					wind.Winundo(t.W, true)
					wind.Textsetselect(t, q0, t.Q1)
					state = None
				} else if state != Paste {
					XPaste(t, t, nil, true, false, nil)
					state = Paste
				}
			}
			wind.Textscrdraw(t)
			Clearmouse()
		}
		adraw.Display.Flush()

		// Mousectl.Read does both the Flush
		// and the receive. We did the flush.
		// Do just the receive, dropping biglock
		// to let other goroutines proceed.
		// Note that *Mouse is Mousectl.Mouse.
		BigUnlock()
		for Mouse.Buttons == b {
			*Mouse = <-Mousectl.C
		}
		BigLock()
		clicktext = nil
	}
}

var BigLock = func(){}
var BigUnlock = func(){}

/*
 * Release the button in less than DELAY ms and it's considered a null selection
 * if the mouse hardly moved, regardless of whether it crossed a char boundary.
 */

const (
	DELAY   = 2
	MINMOVE = 4
)

func xselect(f *frame.Frame, mc *draw.Mousectl, col *draw.Image, p1p *int) int {
	mp := mc.Point
	b := mc.Buttons
	msec := mc.Msec

	// remove tick
	if f.P0 == f.P1 {
		f.Tick(f.PointOf(f.P0), false)
	}
	p1 := f.CharOf(mp)
	p0 := p1
	pt0 := f.PointOf(p0)
	pt1 := f.PointOf(p1)
	reg := 0
	f.Tick(pt0, true)
	for {
		q := f.CharOf(mc.Point)
		if p1 != q {
			if p0 == p1 {
				f.Tick(pt0, false)
			}
			if reg != wind.Region(q, p0) { // crossed starting point; reset
				if reg > 0 {
					wind.Selrestore(f, pt0, p0, p1)
				} else if reg < 0 {
					wind.Selrestore(f, pt1, p1, p0)
				}
				p1 = p0
				pt1 = pt0
				reg = wind.Region(q, p0)
				if reg == 0 {
					f.Drawsel0(pt0, p0, p1, col, adraw.Display.White)
				}
			}
			qt := f.PointOf(q)
			if reg > 0 {
				if q > p1 {
					f.Drawsel0(pt1, p1, q, col, adraw.Display.White)
				} else if q < p1 {
					wind.Selrestore(f, qt, q, p1)
				}
			} else if reg < 0 {
				if q > p1 {
					wind.Selrestore(f, pt1, p1, q)
				} else {
					f.Drawsel0(qt, q, p1, col, adraw.Display.White)
				}
			}
			p1 = q
			pt1 = qt
		}
		if p0 == p1 {
			f.Tick(pt0, true)
		}
		f.Display.Flush()
		mc.Read()
		if mc.Buttons != b {
			break
		}
	}
	if mc.Msec-msec < DELAY && p0 != p1 && util.Abs(mp.X-mc.X) < MINMOVE && util.Abs(mp.Y-mc.Y) < MINMOVE {
		if reg > 0 {
			wind.Selrestore(f, pt0, p0, p1)
		} else if reg < 0 {
			wind.Selrestore(f, pt1, p1, p0)
		}
		p1 = p0
	}
	if p1 < p0 {
		tmp := p0
		p0 = p1
		p1 = tmp
	}
	pt0 = f.PointOf(p0)
	if p0 == p1 {
		f.Tick(pt0, false)
	}
	wind.Selrestore(f, pt0, p0, p1)
	// restore tick
	if f.P0 == f.P1 {
		f.Tick(f.PointOf(f.P0), true)
	}
	f.Display.Flush()
	*p1p = p1
	return p0
}

func textselect23(t *wind.Text, q0 *int, q1 *int, high *draw.Image, mask int) int {
	var p1 int
	p0 := xselect(&t.Fr, Mousectl, high, &p1)
	buts := Mousectl.Buttons
	if buts&mask == 0 {
		*q0 = p0 + t.Org
		*q1 = p1 + t.Org
	}

	for Mousectl.Buttons != 0 {
		Mousectl.Read()
	}
	return buts
}

func Textselect2(t *wind.Text, q0 *int, q1 *int, tp **wind.Text) int {
	*tp = nil
	buts := textselect23(t, q0, q1, adraw.Button2Color, 4)
	if buts&4 != 0 {
		return 0
	}
	if buts&1 != 0 { // pick up argument
		*tp = wind.Argtext
		return 1
	}
	return 1
}

func Textselect3(t *wind.Text, q0 *int, q1 *int) bool {
	return textselect23(t, q0, q1, adraw.Button3Color, 1|2) == 0
}
