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
	"sort"

	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var Lheader = []rune("New Cut Paste Snarf Sort Zerox Delcol ")

func colinit(c *Column, r draw.Rectangle) {
	display.ScreenImage.Draw(r, display.White, nil, draw.ZP)
	c.r = r
	c.w = nil
	t := &c.tag
	t.w = nil
	t.col = c
	r1 := r
	r1.Max.Y = r1.Min.Y + font.Height
	textinit(t, fileaddtext(nil, t), r1, &reffont, tagcols[:])
	t.what = Columntag
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += Border()
	display.ScreenImage.Draw(r1, display.Black, nil, draw.ZP)
	textinsert(t, 0, Lheader, true)
	textsetselect(t, t.file.b.nc, t.file.b.nc)
	display.ScreenImage.Draw(t.scrollr, colbutton, nil, colbutton.R.Min)
	c.safe = true
}

func coladd(c *Column, w *Window, clone *Window, y int) *Window {
	var v *Window
	r := c.r
	r.Min.Y = c.tag.fr.R.Max.Y + Border()
	if y < r.Min.Y && len(c.w) > 0 { /* steal half of last window by default */
		v = c.w[len(c.w)-1]
		y = v.body.fr.R.Min.Y + v.body.fr.R.Dy()/2
	}
	var i int
	/* look for window we'll land on */
	for i = 0; i < len(c.w); i++ {
		v = c.w[i]
		if y < v.r.Max.Y {
			break
		}
	}
	buggered := 0
	if len(c.w) > 0 {
		if i < len(c.w) {
			i++ /* new window will go after v */
		}
		/*
		 * if landing window (v) is too small, grow it first.
		 */
		minht := v.tag.fr.Font.Height + Border() + 1
		j := 0
		for !c.safe || v.body.fr.MaxLines <= 3 || v.body.all.Dy() <= minht {
			j++
			if j > 10 {
				buggered = 1 /* too many windows in column */
				break
			}
			colgrow(c, v, 1)
		}
		var ymax int

		/*
		 * figure out where to split v to make room for w
		 */

		/* new window stops where next window begins */
		if i < len(c.w) {
			ymax = c.w[i].r.Min.Y - Border()
		} else {
			ymax = c.r.Max.Y
		}

		/* new window must start after v's tag ends */
		y = max(y, v.tagtop.Max.Y+Border())

		/* new window must start early enough to end before ymax */
		y = min(y, ymax-minht)

		/* if y is too small, too many windows in column */
		if y < v.tagtop.Max.Y+Border() {
			buggered = 1
		}

		/*
		 * resize & redraw v
		 */
		r = v.r
		r.Max.Y = ymax
		display.ScreenImage.Draw(r, textcols[frame.BACK], nil, draw.ZP)
		r1 := r
		y = min(y, ymax-(v.tag.fr.Font.Height*v.taglines+v.body.fr.Font.Height+Border()+1))
		r1.Max.Y = min(y, v.body.fr.R.Min.Y+v.body.fr.NumLines*v.body.fr.Font.Height)
		r1.Min.Y = winresize(v, r1, false, false)
		r1.Max.Y = r1.Min.Y + Border()
		display.ScreenImage.Draw(r1, display.Black, nil, draw.ZP)

		/*
		 * leave r with w's coordinates
		 */
		r.Min.Y = r1.Max.Y
	}
	if w == nil {
		w = new(Window)
		w.col = c
		display.ScreenImage.Draw(r, textcols[frame.BACK], nil, draw.ZP)
		wininit(w, clone, r)
	} else {
		w.col = c
		winresize(w, r, false, true)
	}
	w.tag.col = c
	w.tag.row = c.row
	w.body.col = c
	w.body.row = c.row
	c.w = append(c.w, nil)
	copy(c.w[i+1:], c.w[i:])
	c.w[i] = w
	c.safe = true

	/* if there were too many windows, redraw the whole column */
	if buggered != 0 {
		colresize(c, c.r)
	}

	savemouse(w)
	/* near the button, but in the body */
	display.MoveCursor(w.tag.scrollr.Max.Add(draw.Pt(3, 3)))
	barttext = &w.body
	return w
}

func colclose(c *Column, w *Window, dofree bool) {
	/* w is locked */
	if !c.safe {
		colgrow(c, w, 1)
	}
	var i int
	for i = 0; i < len(c.w); i++ {
		if c.w[i] == w {
			goto Found
		}
	}
	error_("can't find window")
Found:
	r := w.r
	w.tag.col = nil
	w.body.col = nil
	w.col = nil
	didmouse := restoremouse(w)
	if dofree {
		windelete(w)
		winclose(w)
	}
	copy(c.w[i:], c.w[i+1:])
	c.w = c.w[:len(c.w)-1]
	if len(c.w) == 0 {
		display.ScreenImage.Draw(r, display.White, nil, draw.ZP)
		return
	}
	up := 0
	if i == len(c.w) { /* extend last window down */
		w = c.w[i-1]
		r.Min.Y = w.r.Min.Y
		r.Max.Y = c.r.Max.Y
	} else { /* extend next window up */
		up = 1
		w = c.w[i]
		r.Max.Y = w.r.Max.Y
	}
	display.ScreenImage.Draw(r, textcols[frame.BACK], nil, draw.ZP)
	if c.safe {
		if didmouse == 0 && up != 0 {
			w.showdel = true
		}
		winresize(w, r, false, true)
		if didmouse == 0 && up != 0 {
			movetodel(w)
		}
	}
}

func colcloseall(c *Column) {
	if c == activecol {
		activecol = nil
	}
	textclose(&c.tag)
	for i := 0; i < len(c.w); i++ {
		w := c.w[i]
		winclose(w)
	}
	clearmouse()
}

func colmousebut(c *Column) {
	display.MoveCursor(c.tag.scrollr.Min.Add(c.tag.scrollr.Max).Div(2))
}

func colresize(c *Column, r draw.Rectangle) {
	clearmouse()
	r1 := r
	r1.Max.Y = r1.Min.Y + c.tag.fr.Font.Height
	textresize(&c.tag, r1, true)
	display.ScreenImage.Draw(c.tag.scrollr, colbutton, nil, colbutton.R.Min)
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += Border()
	display.ScreenImage.Draw(r1, display.Black, nil, draw.ZP)
	r1.Max.Y = r.Max.Y
	new_ := r.Dy() - len(c.w)*(Border()+font.Height)
	old := c.r.Dy() - len(c.w)*(Border()+font.Height)
	for i := 0; i < len(c.w); i++ {
		w := c.w[i]
		w.maxlines = 0
		if i == len(c.w)-1 {
			r1.Max.Y = r.Max.Y
		} else {
			r1.Max.Y = r1.Min.Y
			if new_ > 0 && old > 0 && w.r.Dy() > Border()+font.Height {
				r1.Max.Y += (w.r.Dy()-Border()-font.Height)*new_/old + Border() + font.Height
			}
		}
		r1.Max.Y = max(r1.Max.Y, r1.Min.Y+Border()+font.Height)
		r2 := r1
		r2.Max.Y = r2.Min.Y + Border()
		display.ScreenImage.Draw(r2, display.Black, nil, draw.ZP)
		r1.Min.Y = r2.Max.Y
		r1.Min.Y = winresize(w, r1, false, i == len(c.w)-1)
	}
	c.r = r
}

func colsort(c *Column) {
	if len(c.w) == 0 {
		return
	}
	clearmouse()
	rp := make([]draw.Rectangle, len(c.w))
	wp := make([]*Window, len(c.w))
	copy(wp, c.w)
	sort.Slice(wp, func(i, j int) bool {
		return runes.Compare(wp[i].body.file.name, wp[j].body.file.name) < 0
	})

	for i := 0; i < len(c.w); i++ {
		rp[i] = wp[i].r
	}
	r := c.r
	r.Min.Y = c.tag.fr.R.Max.Y
	display.ScreenImage.Draw(r, textcols[frame.BACK], nil, draw.ZP)
	y := r.Min.Y
	for i := 0; i < len(c.w); i++ {
		w := wp[i]
		r.Min.Y = y
		if i == len(c.w)-1 {
			r.Max.Y = c.r.Max.Y
		} else {
			r.Max.Y = r.Min.Y + w.r.Dy() + Border()
		}
		r1 := r
		r1.Max.Y = r1.Min.Y + Border()
		display.ScreenImage.Draw(r1, display.Black, nil, draw.ZP)
		r.Min.Y = r1.Max.Y
		y = winresize(w, r, false, i == len(c.w)-1)
	}
	c.w = wp
}

func colgrow(c *Column, w *Window, but int) {
	var i int
	for i = 0; i < len(c.w); i++ {
		if c.w[i] == w {
			goto Found
		}
	}
	error_("can't find window")

Found:
	cr := c.r
	var r draw.Rectangle
	if but < 0 { /* make sure window fills its own space properly */
		r = w.r
		if i == len(c.w)-1 || !c.safe {
			r.Max.Y = cr.Max.Y
		} else {
			r.Max.Y = c.w[i+1].r.Min.Y - Border()
		}
		winresize(w, r, false, true)
		return
	}
	cr.Min.Y = c.w[0].r.Min.Y
	var v *Window
	if but == 3 { /* full size */
		if i != 0 {
			v = c.w[0]
			c.w[0] = w
			c.w[i] = v
		}
		display.ScreenImage.Draw(cr, textcols[frame.BACK], nil, draw.ZP)
		winresize(w, cr, false, true)
		for i = 1; i < len(c.w); i++ {
			c.w[i].body.fr.MaxLines = 0
		}
		c.safe = false
		return
	}
	/* store old #lines for each window */
	onl := w.body.fr.MaxLines
	nl := make([]int, len(c.w))
	ny := make([]int, len(c.w))
	tot := 0
	var j int
	var l int
	for j = 0; j < len(c.w); j++ {
		l = c.w[j].taglines - 1 + c.w[j].body.fr.MaxLines
		nl[j] = l
		tot += l
	}
	/* approximate new #lines for this window */
	if but == 2 { /* as big as can be */
		for i := range nl {
			nl[i] = 0
		}
	} else {
		nnl := min(onl+max(min(5, w.taglines-1+w.maxlines), onl/2), tot)
		if nnl < w.taglines-1+w.maxlines {
			nnl = (w.taglines - 1 + w.maxlines + nnl) / 2
		}
		if nnl == 0 {
			nnl = 2
		}
		dnl := nnl - onl
		/* compute new #lines for each window */
		for k := 1; k < len(c.w); k++ {
			/* prune from later window */
			j = i + k
			if j < len(c.w) && nl[j] != 0 {
				l = min(dnl, max(1, nl[j]/2))
				nl[j] -= l
				nl[i] += l
				dnl -= l
			}
			/* prune from earlier window */
			j = i - k
			if j >= 0 && nl[j] != 0 {
				l = min(dnl, max(1, nl[j]/2))
				nl[j] -= l
				nl[i] += l
				dnl -= l
			}
		}
	}
	/* pack everyone above */
	y1 := cr.Min.Y
	for j = 0; j < i; j++ {
		v = c.w[j]
		r = v.r
		r.Min.Y = y1
		r.Max.Y = y1 + v.tagtop.Dy()
		if nl[j] != 0 {
			r.Max.Y += 1 + nl[j]*v.body.fr.Font.Height
		}
		r.Min.Y = winresize(v, r, c.safe, false)
		r.Max.Y += Border()
		display.ScreenImage.Draw(r, display.Black, nil, draw.ZP)
		y1 = r.Max.Y
	}
	/* scan to see new size of everyone below */
	y2 := c.r.Max.Y
	for j = len(c.w) - 1; j > i; j-- {
		v = c.w[j]
		r = v.r
		r.Min.Y = y2 - v.tagtop.Dy()
		if nl[j] != 0 {
			r.Min.Y -= 1 + nl[j]*v.body.fr.Font.Height
		}
		r.Min.Y -= Border()
		ny[j] = r.Min.Y
		y2 = r.Min.Y
	}
	/* compute new size of window */
	r = w.r
	r.Min.Y = y1
	r.Max.Y = y2
	h := w.body.fr.Font.Height
	if r.Dy() < w.tagtop.Dy()+1+h+Border() {
		r.Max.Y = r.Min.Y + w.tagtop.Dy() + 1 + h + Border()
	}
	/* draw window */
	r.Max.Y = winresize(w, r, c.safe, true)
	if i < len(c.w)-1 {
		r.Min.Y = r.Max.Y
		r.Max.Y += Border()
		display.ScreenImage.Draw(r, display.Black, nil, draw.ZP)
		for j = i + 1; j < len(c.w); j++ {
			ny[j] -= (y2 - r.Max.Y)
		}
	}
	/* pack everyone below */
	y1 = r.Max.Y
	for j = i + 1; j < len(c.w); j++ {
		v = c.w[j]
		r = v.r
		r.Min.Y = y1
		r.Max.Y = y1 + v.tagtop.Dy()
		if nl[j] != 0 {
			r.Max.Y += 1 + nl[j]*v.body.fr.Font.Height
		}
		y1 = winresize(v, r, c.safe, j == len(c.w)-1)
		if j < len(c.w)-1 { /* no border on last window */
			r.Min.Y = y1
			r.Max.Y += Border()
			display.ScreenImage.Draw(r, display.Black, nil, draw.ZP)
			y1 = r.Max.Y
		}
	}
	c.safe = true
	winmousebut(w)
}

func coldragwin(c *Column, w *Window, but int) {
	clearmouse()
	display.SwitchCursor2(&boxcursor, &boxcursor2)
	b := mouse.Buttons
	op := mouse.Point
	for mouse.Buttons == b {
		mousectl.Read()
	}
	display.SwitchCursor(nil)
	if mouse.Buttons != 0 {
		for mouse.Buttons != 0 {
			mousectl.Read()
		}
		return
	}
	var i int

	for i = 0; i < len(c.w); i++ {
		if c.w[i] == w {
			goto Found
		}
	}
	error_("can't find window")

Found:
	if w.tagexpand { /* force recomputation of window tag size */
		w.taglines = 1
	}
	p := mouse.Point
	if abs(p.X-op.X) < 5 && abs(p.Y-op.Y) < 5 {
		colgrow(c, w, but)
		winmousebut(w)
		return
	}
	/* is it a flick to the right? */
	if abs(p.Y-op.Y) < 10 && p.X > op.X+30 && rowwhichcol(c.row, p) == c {
		p.X = op.X + w.r.Dx() /* yes: toss to next column */
	}
	nc := rowwhichcol(c.row, p)
	if nc != nil && nc != c {
		colclose(c, w, false)
		coladd(nc, w, nil, p.Y)
		winmousebut(w)
		return
	}
	if i == 0 && len(c.w) == 1 {
		return /* can't do it */
	}
	if (i > 0 && p.Y < c.w[i-1].r.Min.Y) || (i < len(c.w)-1 && p.Y > w.r.Max.Y) || (i == 0 && p.Y > w.r.Max.Y) {
		/* shuffle */
		colclose(c, w, false)
		coladd(c, w, nil, p.Y)
		winmousebut(w)
		return
	}
	if i == 0 {
		return
	}
	v := c.w[i-1]
	if p.Y < v.tagtop.Max.Y {
		p.Y = v.tagtop.Max.Y
	}
	if p.Y > w.r.Max.Y-w.tagtop.Dy()-Border() {
		p.Y = w.r.Max.Y - w.tagtop.Dy() - Border()
	}
	r := v.r
	r.Max.Y = p.Y
	if r.Max.Y > v.body.fr.R.Min.Y {
		r.Max.Y -= (r.Max.Y - v.body.fr.R.Min.Y) % v.body.fr.Font.Height
		if v.body.fr.R.Min.Y == v.body.fr.R.Max.Y {
			r.Max.Y++
		}
	}
	r.Min.Y = winresize(v, r, c.safe, false)
	r.Max.Y = r.Min.Y + Border()
	display.ScreenImage.Draw(r, display.Black, nil, draw.ZP)
	r.Min.Y = r.Max.Y
	if i == len(c.w)-1 {
		r.Max.Y = c.r.Max.Y
	} else {
		r.Max.Y = c.w[i+1].r.Min.Y - Border()
	}
	winresize(w, r, c.safe, true)
	c.safe = true
	winmousebut(w)
}

func colwhich(c *Column, p draw.Point) *Text {
	if !p.In(c.r) {
		return nil
	}
	if p.In(c.tag.all) {
		return &c.tag
	}
	for i := 0; i < len(c.w); i++ {
		w := c.w[i]
		if p.In(w.r) {
			if p.In(w.tagtop) || p.In(w.tag.all) {
				return &w.tag
			}
			/* exclude partial line at bottom */
			if p.X >= w.body.scrollr.Max.X && p.Y >= w.body.fr.R.Max.Y {
				return nil
			}
			return &w.body
		}
	}
	return nil
}

func colclean(c *Column) bool {
	clean := true
	for i := 0; i < len(c.w); i++ {
		clean = winclean(c.w[i], true) && clean
	}
	return clean
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
