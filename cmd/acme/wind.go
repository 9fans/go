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
	"os"
	"strings"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var winid int

func wininit(w *Window, clone *Window, r draw.Rectangle) {
	w.tag.w = w
	w.taglines = 1
	w.tagexpand = true
	w.body.w = w
	winid++
	w.id = winid
	util.Incref(&w.ref)
	if globalincref != 0 {
		util.Incref(&w.ref)
	}
	w.ctlfid = ^0
	w.utflastqid = -1
	r1 := r

	w.tagtop = r
	w.tagtop.Max.Y = r.Min.Y + font.Height
	r1.Max.Y = r1.Min.Y + w.taglines*font.Height

	util.Incref(&reffont.ref)
	f := fileaddtext(nil, &w.tag)
	textinit(&w.tag, f, r1, &reffont, tagcols[:])
	w.tag.what = Tag
	/* tag is a copy of the contents, not a tracked image */
	if clone != nil {
		textdelete(&w.tag, 0, w.tag.file.b.Len(), true)
		nc := clone.tag.file.b.Len()
		rp := make([]rune, nc)
		clone.tag.file.b.Read(0, rp)
		textinsert(&w.tag, 0, rp, true)
		filereset(w.tag.file)
		textsetselect(&w.tag, nc, nc)
	}
	r1 = r
	r1.Min.Y += w.taglines*font.Height + 1
	if r1.Max.Y < r1.Min.Y {
		r1.Max.Y = r1.Min.Y
	}
	f = nil
	var rf *Reffont
	if clone != nil {
		f = clone.body.file
		w.body.org = clone.body.org
		w.isscratch = clone.isscratch
		rf = rfget(false, false, false, clone.body.reffont.f.Name)
	} else {
		rf = rfget(false, false, false, "")
	}
	f = fileaddtext(f, &w.body)
	w.body.what = Body
	textinit(&w.body, f, r1, rf, textcols[:])
	r1.Min.Y -= 1
	r1.Max.Y = r1.Min.Y + 1
	display.ScreenImage.Draw(r1, tagcols[frame.BORD], nil, draw.ZP)
	textscrdraw(&w.body)
	w.r = r
	var br draw.Rectangle
	br.Min = w.tag.scrollr.Min
	br.Max.X = br.Min.X + button.R.Dx()
	br.Max.Y = br.Min.Y + button.R.Dy()
	display.ScreenImage.Draw(br, button, nil, button.R.Min)
	w.filemenu = true
	w.maxlines = w.body.fr.MaxLines
	w.autoindent = globalautoindent
	if clone != nil {
		w.dirty = clone.dirty
		w.autoindent = clone.autoindent
		textsetselect(&w.body, clone.body.q0, clone.body.q1)
		winsettag(w)
	}
}

/*
 * Draw the appropriate button.
 */
func windrawbutton(w *Window) {
	b := button
	if !w.isdir && !w.isscratch && (w.body.file.mod || len(w.body.cache) != 0) {
		b = modbutton
	}
	var br draw.Rectangle
	br.Min = w.tag.scrollr.Min
	br.Max.X = br.Min.X + b.R.Dx()
	br.Max.Y = br.Min.Y + b.R.Dy()
	display.ScreenImage.Draw(br, b, nil, b.R.Min)
}

func delrunepos(w *Window) int {
	_, i := parsetag(w, 0)
	i += 2
	if i >= w.tag.file.b.Len() {
		return -1
	}
	return i
}

func movetodel(w *Window) {
	n := delrunepos(w)
	if n < 0 {
		return
	}
	display.MoveCursor(w.tag.fr.PointOf(n).Add(draw.Pt(4, w.tag.fr.Font.Height-4)))
}

/*
 * Compute number of tag lines required
 * to display entire tag text.
 */
func wintaglines(w *Window, r draw.Rectangle) int {
	if !w.tagexpand && !w.showdel {
		return 1
	}
	w.showdel = false
	w.tag.fr.NoRedraw = 1
	textresize(&w.tag, r, true)
	w.tag.fr.NoRedraw = 0
	w.tagsafe = false
	var n int

	if !w.tagexpand {
		/* use just as many lines as needed to show the Del */
		n = delrunepos(w)
		if n < 0 {
			return 1
		}
		p := w.tag.fr.PointOf(n).Sub(w.tag.fr.R.Min)
		return 1 + p.Y/w.tag.fr.Font.Height
	}

	/* can't use more than we have */
	if w.tag.fr.NumLines >= w.tag.fr.MaxLines {
		return w.tag.fr.MaxLines
	}

	/* if tag ends with \n, include empty line at end for typing */
	n = w.tag.fr.NumLines
	if w.tag.file.b.Len() > 0 {
		var rune_ [1]rune
		w.tag.file.b.Read(w.tag.file.b.Len()-1, rune_[:])
		if rune_[0] == '\n' {
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

func winresize(w *Window, r draw.Rectangle, safe, keepextra bool) int {
	mouseintag := mouse.Point.In(w.tag.all)
	mouseinbody := mouse.Point.In(w.body.all)

	/* tagtop is first line of tag */
	w.tagtop = r
	w.tagtop.Max.Y = r.Min.Y + font.Height

	r1 := r
	r1.Max.Y = util.Min(r.Max.Y, r1.Min.Y+w.taglines*font.Height)

	/* If needed, recompute number of lines in tag. */
	if !safe || !w.tagsafe || !(w.tag.all == r1) {
		w.taglines = wintaglines(w, r)
		r1.Max.Y = util.Min(r.Max.Y, r1.Min.Y+w.taglines*font.Height)
	}

	/* If needed, resize & redraw tag. */
	y := r1.Max.Y
	if !safe || !w.tagsafe || !(w.tag.all == r1) {
		textresize(&w.tag, r1, true)
		y = w.tag.fr.R.Max.Y
		windrawbutton(w)
		w.tagsafe = true
		var p draw.Point

		/* If mouse is in tag, pull up as tag closes. */
		if mouseintag && !mouse.Point.In(w.tag.all) {
			p = mouse.Point
			p.Y = w.tag.all.Max.Y - 3
			display.MoveCursor(p)
		}

		/* If mouse is in body, push down as tag expands. */
		if mouseinbody && mouse.Point.In(w.tag.all) {
			p = mouse.Point
			p.Y = w.tag.all.Max.Y + 3
			display.MoveCursor(p)
		}
	}

	/* If needed, resize & redraw body. */
	r1 = r
	r1.Min.Y = y
	if !safe || !(w.body.all == r1) {
		oy := y
		if y+1+w.body.fr.Font.Height <= r.Max.Y { /* room for one line */
			r1.Min.Y = y
			r1.Max.Y = y + 1
			display.ScreenImage.Draw(r1, tagcols[frame.BORD], nil, draw.ZP)
			y++
			r1.Min.Y = util.Min(y, r.Max.Y)
			r1.Max.Y = r.Max.Y
		} else {
			r1.Min.Y = y
			r1.Max.Y = y
		}
		y = textresize(&w.body, r1, keepextra)
		w.r = r
		w.r.Max.Y = y
		textscrdraw(&w.body)
		w.body.all.Min.Y = oy
	}
	w.maxlines = util.Min(w.body.fr.NumLines, util.Max(w.maxlines, w.body.fr.MaxLines))
	return w.r.Max.Y
}

func winlock1(w *Window, owner rune) {
	util.Incref(&w.ref)
	w.lk.Lock()
	w.owner = owner
}

func winlock(w *Window, owner rune) {
	f := w.body.file
	for i := 0; i < len(f.text); i++ {
		winlock1(f.text[i].w, owner)
	}
}

func winunlock(w *Window) {
	/*
	 * subtle: loop runs backwards to avoid tripping over
	 * winclose indirectly editing f->text and freeing f
	 * on the last iteration of the loop.
	 */
	f := w.body.file
	for i := len(f.text) - 1; i >= 0; i-- {
		w = f.text[i].w
		w.owner = 0
		w.lk.Unlock()
		winclose(w)
	}
}

func winmousebut(w *Window) {
	display.MoveCursor(w.tag.scrollr.Min.Add(draw.Pt(w.tag.scrollr.Dx(), font.Height).Div(2)))
}

func windirfree(w *Window) {
	w.dlp = nil
}

func winclose(w *Window) {
	if util.Decref(&w.ref) == 0 {
		xfidlog(w, "del")
		windirfree(w)
		textclose(&w.tag)
		textclose(&w.body)
		if activewin == w {
			activewin = nil
		}
	}
}

func windelete(w *Window) {
	x := w.eventx
	if x != nil {
		w.events = nil
		w.eventx = nil
		x.c <- nil /* wake him up */
	}
}

func winundo(w *Window, isundo bool) {
	w.utflastqid = -1
	body := &w.body
	fileundo(body.file, isundo, &body.q0, &body.q1)
	textshow(body, body.q0, body.q1, true)
	f := body.file
	for i := 0; i < len(f.text); i++ {
		v := f.text[i].w
		v.dirty = (f.seq != v.putseq)
		if v != w {
			v.body.q0 = v.body.fr.P0 + v.body.org
			v.body.q1 = v.body.fr.P1 + v.body.org
		}
	}
	winsettag(w)
}

var Lslashguide = []rune("/guide")

func winsetname(w *Window, name []rune) {
	t := &w.body
	if runes.Equal(t.file.name, name) {
		return
	}
	w.isscratch = false
	if len(name) >= 6 && runes.Equal(Lslashguide, name[len(name)-6:]) {
		w.isscratch = true
	} else if len(name) >= 7 && runes.Equal(Lpluserrors, name[len(name)-7:]) {
		w.isscratch = true
	}
	filesetname(t.file, name)
	for i := 0; i < len(t.file.text); i++ {
		v := t.file.text[i].w
		winsettag(v)
		v.isscratch = w.isscratch
	}
}

func wintype(w *Window, t *Text, r rune) {
	texttype(t, r)
	if t.what == Body {
		for i := 0; i < len(t.file.text); i++ {
			textscrdraw(t.file.text[i])
		}
	}
	winsettag(w)
}

func wincleartag(w *Window) {
	/* w must be committed */
	n := w.tag.file.b.Len()
	r, i := parsetag(w, 0)
	for ; i < n; i++ {
		if r[i] == '|' {
			break
		}
	}
	if i == n {
		return
	}
	i++
	textdelete(&w.tag, i, n, true)
	w.tag.file.mod = false
	if w.tag.q0 > i {
		w.tag.q0 = i
	}
	if w.tag.q1 > i {
		w.tag.q1 = i
	}
	textsetselect(&w.tag, w.tag.q0, w.tag.q1)
}

var Ldelsnarf = []rune(" Del Snarf")
var Lspacepipe = []rune(" |")
var Ltabpipe = []rune("\t|")

func parsetag(w *Window, extra int) ([]rune, int) {
	r := make([]rune, w.tag.file.b.Len(), w.tag.file.b.Len()+extra+1)
	w.tag.file.b.Read(0, r)

	/*
	 * " |" or "\t|" ends left half of tag
	 * If we find " Del Snarf" in the left half of the tag
	 * (before the pipe), that ends the file name.
	 */
	pipe := runes.Index(r, Lspacepipe)
	p := runes.Index(r, Ltabpipe)
	if p >= 0 && (pipe < 0 || p < pipe) {
		pipe = p
	}
	p = runes.Index(r, Ldelsnarf)
	var i int
	if p >= 0 && (pipe < 0 || p < pipe) {
		i = p
	} else {
		for i = 0; i < w.tag.file.b.Len(); i++ {
			if r[i] == ' ' || r[i] == '\t' {
				break
			}
		}
	}
	return r, i
}

var Lundo = []rune(" Undo")
var Lredo = []rune(" Redo")
var Lget = []rune(" Get")
var Lput = []rune(" Put")
var Llook = []rune(" Look ")
var Lpipe = []rune(" |")

func winsettag1(w *Window) {

	/* there are races that get us here with stuff in the tag cache, so we take extra care to sync it */
	if len(w.tag.cache) != 0 || w.tag.file.mod {
		wincommit(w, &w.tag) /* check file name; also guarantees we can modify tag contents */
	}
	old, ii := parsetag(w, 0)
	if !runes.Equal(old[:ii], w.body.file.name) {
		textdelete(&w.tag, 0, ii, true)
		textinsert(&w.tag, 0, w.body.file.name, true)
		old = make([]rune, w.tag.file.b.Len())
		w.tag.file.b.Read(0, old)
	}

	/* compute the text for the whole tag, replacing current only if it differs */
	new_ := make([]rune, 0, len(w.body.file.name)+100)
	new_ = append(new_, w.body.file.name...)
	new_ = append(new_, Ldelsnarf...)
	if w.filemenu {
		if w.body.needundo || w.body.file.delta.Len() > 0 || len(w.body.cache) != 0 {
			new_ = append(new_, Lundo...)
		}
		if w.body.file.epsilon.Len() > 0 {
			new_ = append(new_, Lredo...)
		}
		dirty := len(w.body.file.name) != 0 && (len(w.body.cache) != 0 || w.body.file.seq != w.putseq)
		if !w.isdir && dirty {
			new_ = append(new_, Lput...)
		}
	}
	if w.isdir {
		new_ = append(new_, Lget...)
	}
	new_ = append(new_, Lpipe...)
	r := runes.IndexRune(old, '|')
	var k int
	if r >= 0 {
		k = r + 1
	} else {
		k = len(old)
		if w.body.file.seq == 0 {
			new_ = append(new_, Llook...)
		}
	}

	/* replace tag if the new one is different */
	resize := 0
	var n int
	if !runes.Equal(new_, old[:k]) {
		resize = 1
		n = k
		if n > len(new_) {
			n = len(new_)
		}
		var j int
		for j = 0; j < n; j++ {
			if old[j] != new_[j] {
				break
			}
		}
		q0 := w.tag.q0
		q1 := w.tag.q1
		textdelete(&w.tag, j, k, true)
		textinsert(&w.tag, j, new_[j:], true)
		/* try to preserve user selection */
		r = runes.IndexRune(old, '|')
		if r >= 0 {
			bar := r
			if q0 > bar {
				bar = runes.IndexRune(new_, '|') - bar
				w.tag.q0 = q0 + bar
				w.tag.q1 = q1 + bar
			}
		}
	}
	w.tag.file.mod = false
	n = w.tag.file.b.Len() + len(w.tag.cache)
	if w.tag.q0 > n {
		w.tag.q0 = n
	}
	if w.tag.q1 > n {
		w.tag.q1 = n
	}
	textsetselect(&w.tag, w.tag.q0, w.tag.q1)
	windrawbutton(w)
	if resize != 0 {
		w.tagsafe = false
		winresize(w, w.r, true, true)
	}
}

func winsettag(w *Window) {
	f := w.body.file
	for i := 0; i < len(f.text); i++ {
		v := f.text[i].w
		if v.col.safe || v.body.fr.MaxLines > 0 {
			winsettag1(v)
		}
	}
}

func wincommit(w *Window, t *Text) {
	textcommit(t, true)
	f := t.file
	var i int
	if len(f.text) > 1 {
		for i = 0; i < len(f.text); i++ {
			textcommit(f.text[i], false) /* no-op for t */
		}
	}
	if t.what == Body {
		return
	}
	r, i := parsetag(w, 0)
	if !runes.Equal(r[:i], w.body.file.name) {
		seq++
		filemark(w.body.file)
		w.body.file.mod = true
		w.dirty = true
		winsetname(w, r[:i])
		winsettag(w)
	}
}

func winaddincl(w *Window, r []rune) {
	a := string(r)
	info, err := os.Stat(a)
	if err != nil {
		if !strings.HasPrefix(a, "/") {
			rs := dirname(&w.body, r)
			r = rs
			a = string(r)
			info, err = os.Stat(a)
		}
		if err != nil {
			alog.Printf("%s: %v", a, err)
			return
		}
	}
	if !info.IsDir() {
		alog.Printf("%s: not a directory\n", a)
		return
	}
	w.incl = append(w.incl, nil)
	copy(w.incl[1:], w.incl)
	w.incl[0] = runes.Clone(r)
}

func winclean(w *Window, conservative bool) bool {
	if w.isscratch || w.isdir { /* don't whine if it's a guide file, error window, etc. */
		return true
	}
	if !conservative && w.nopen[QWevent] > 0 {
		return true
	}
	if w.dirty {
		if len(w.body.file.name) != 0 {
			alog.Printf("%s modified\n", string(w.body.file.name))
		} else {
			if w.body.file.b.Len() < 100 { /* don't whine if it's too small */
				return true
			}
			alog.Printf("unnamed file modified\n")
		}
		w.dirty = false
		return false
	}
	return true
}

func winctlprint(w *Window, fonts bool) string {
	isdir := 0
	if w.isdir {
		isdir = 1
	}
	dirty := 0
	if w.dirty {
		dirty = 1
	}
	base := fmt.Sprintf("%11d %11d %11d %11d %11d ", w.id, w.tag.file.b.Len(), w.body.file.b.Len(), isdir, dirty)
	if fonts {
		base += fmt.Sprintf("%11d %q %11d ", w.body.fr.R.Dx(), w.body.reffont.f.Name, w.body.fr.MaxTab)
	}
	return base
}

func winevent(w *Window, format string, args ...interface{}) {
	if w.nopen[QWevent] == 0 {
		return
	}
	if w.owner == 0 {
		util.Fatal("no window owner")
	}
	b := fmt.Sprintf(format, args...)
	w.events = append(w.events, byte(w.owner))
	w.events = append(w.events, b...)
	x := w.eventx
	if x != nil {
		w.eventx = nil
		x.c <- nil
	}
}
