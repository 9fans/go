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

package main

import (
	"crypto/sha1"
	"fmt"
	"hash"
	"os"
	"sort"
	"time"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var tagcols [frame.NCOL]*draw.Image
var textcols [frame.NCOL]*draw.Image
var Ldot = []rune(".")

const (
	TABDIR = 3
) /* width of tabs in directory windows */

func textinit(t *Text, f *File, r draw.Rectangle, rf *Reffont, cols []*draw.Image) {
	t.file = f
	t.all = r
	t.scrollr = r
	t.scrollr.Max.X = r.Min.X + Scrollwid()
	t.lastsr = nullrect
	r.Min.X += Scrollwid() + Scrollgap()
	t.eq0 = ^0
	t.cache = t.cache[:0]
	t.reffont = rf
	t.tabstop = maxtab
	copy(t.fr.Cols[:], cols)
	textredraw(t, r, rf.f, display.ScreenImage, -1)
}

func textredraw(t *Text, r draw.Rectangle, f *draw.Font, b *draw.Image, odx int) {
	t.fr.Init(r, f, b, t.fr.Cols[:])
	rr := t.fr.R
	rr.Min.X -= Scrollwid() + Scrollgap() /* back fill to scroll bar */
	if t.fr.NoRedraw == 0 {
		t.fr.B.Draw(rr, t.fr.Cols[frame.BACK], nil, draw.ZP)
	}
	/* use no wider than 3-space tabs in a directory */
	maxt := maxtab
	if t.what == Body {
		if t.w.isdir {
			maxt = util.Min(TABDIR, maxtab)
		} else {
			maxt = t.tabstop
		}
	}
	t.fr.MaxTab = maxt * f.StringWidth("0")
	if t.what == Body && t.w.isdir && odx != t.all.Dx() {
		if t.fr.MaxLines > 0 {
			textreset(t)
			textcolumnate(t, t.w.dlp)
			textshow(t, 0, 0, true)
		}
	} else {
		textfill(t)
		textsetselect(t, t.q0, t.q1)
	}
}

func textresize(t *Text, r draw.Rectangle, keepextra bool) int {
	if r.Dy() <= 0 {
		r.Max.Y = r.Min.Y
	} else if !keepextra {
		r.Max.Y -= r.Dy() % t.fr.Font.Height
	}
	odx := t.all.Dx()
	t.all = r
	t.scrollr = r
	t.scrollr.Max.X = r.Min.X + Scrollwid()
	t.lastsr = nullrect
	r.Min.X += Scrollwid() + Scrollgap()
	t.fr.Clear(false)
	textredraw(t, r, t.fr.Font, t.fr.B, odx)
	if keepextra && t.fr.R.Max.Y < t.all.Max.Y && t.fr.NoRedraw == 0 {
		/* draw background in bottom fringe of window */
		r.Min.X -= Scrollgap()
		r.Min.Y = t.fr.R.Max.Y
		r.Max.Y = t.all.Max.Y
		display.ScreenImage.Draw(r, t.fr.Cols[frame.BACK], nil, draw.ZP)
	}
	return t.all.Max.Y
}

func textclose(t *Text) {
	t.fr.Clear(true)
	filedeltext(t.file, t)
	t.file = nil
	rfclose(t.reffont)
	if argtext == t {
		argtext = nil
	}
	if typetext == t {
		typetext = nil
	}
	if seltext == t {
		seltext = nil
	}
	if mousetext == t {
		mousetext = nil
	}
	if barttext == t {
		barttext = nil
	}
}

var Ltab = []rune("\t")

func textcolumnate(t *Text, dlp []*Dirlist) {
	if len(t.file.text) > 1 {
		return
	}
	mint := t.fr.Font.StringWidth("0")
	/* go for narrower tabs if set more than 3 wide */
	t.fr.MaxTab = util.Min(maxtab, TABDIR) * mint
	maxt := t.fr.MaxTab
	colw := 0
	var i int
	var w int
	var dl *Dirlist
	for i = 0; i < len(dlp); i++ {
		dl = dlp[i]
		w = dl.wid
		if maxt-w%maxt < mint || w%maxt == 0 {
			w += mint
		}
		if w%maxt != 0 {
			w += maxt - (w % maxt)
		}
		if w > colw {
			colw = w
		}
	}
	var ncol int
	if colw == 0 {
		ncol = 1
	} else {
		ncol = util.Max(1, t.fr.R.Dx()/colw)
	}
	nrow := (len(dlp) + ncol - 1) / ncol

	q1 := 0
	for i = 0; i < nrow; i++ {
		for j := i; j < len(dlp); j += nrow {
			dl = dlp[j]
			fileinsert(t.file, q1, dl.r)
			q1 += len(dl.r)
			if j+nrow >= len(dlp) {
				break
			}
			w = dl.wid
			if maxt-w%maxt < mint {
				fileinsert(t.file, q1, Ltab)
				q1++
				w += mint
			}
			for {
				fileinsert(t.file, q1, Ltab)
				q1++
				w += maxt - (w % maxt)
				if w >= colw {
					break
				}
			}
		}
		fileinsert(t.file, q1, Lnl)
		q1++
	}
}

func textload(t *Text, q0 int, file string, setqid bool) int {
	if len(t.cache) > 0 || t.Len() != 0 || t.w == nil || t != &t.w.body {
		util.Fatal("text.load")
	}
	if t.w.isdir && len(t.file.name) == 0 {
		alog.Printf("empty directory name")
		return -1
	}
	if ismtpt(file) {
		alog.Printf("will not open self mount point %s\n", file)
		return -1
	}
	f, err := os.Open(file)
	if err != nil {
		alog.Printf("can't open %s: %v\n", file, err)
		return -1
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		alog.Printf("can't fstat %s: %v\n", file, err)
		return -1
	}
	nulls := false
	var h hash.Hash
	var rp []rune
	var i int
	var n int
	var q1 int
	if info.IsDir() {
		/* this is checked in get() but it's possible the file changed underfoot */
		if len(t.file.text) > 1 {
			alog.Printf("%s is a directory; can't read with multiple windows on it\n", file)
			return -1
		}
		t.w.isdir = true
		t.w.filemenu = false
		if len(t.file.name) > 0 && t.file.name[len(t.file.name)-1] != '/' {
			rp := make([]rune, len(t.file.name)+1)
			copy(rp, t.file.name)
			rp[len(t.file.name)] = '/'
			winsetname(t.w, rp)
		}
		var dlp []*Dirlist
		for {
			// TODO(rsc): sort order here should be before /, not after
			// Can let ReadDir(-1) do it.
			dirs, err := f.ReadDir(100)
			for _, dir := range dirs {
				dl := new(Dirlist)
				name := dir.Name()
				if dir.IsDir() {
					name += "/"
				}
				dl.r = []rune(name)
				dl.wid = t.fr.Font.StringWidth(name)
				dlp = append(dlp, dl)
			}
			if err != nil {
				break
			}
		}
		sort.Slice(dlp, func(i, j int) bool {
			return runes.Compare(dlp[i].r, dlp[j].r) < 0
		})
		t.w.dlp = dlp
		textcolumnate(t, dlp)
		q1 = t.Len()
	} else {
		t.w.isdir = false
		t.w.filemenu = true
		if q0 == 0 {
			h = sha1.New()
		}
		q1 = q0 + fileload(t.file, q0, f, &nulls, h)
	}
	if setqid {
		if h != nil {
			h.Sum(t.file.sha1[:0])
			h = nil
		} else {
			t.file.sha1 = [20]byte{}
		}
		t.file.info = info
	}
	f.Close()
	rp = fbufalloc()
	for q := q0; q < q1; q += n {
		n = q1 - q
		if n > RBUFSIZE {
			n = RBUFSIZE
		}
		t.file.b.Read(q, rp[:n])
		if q < t.org {
			t.org += n
		} else if q <= t.org+t.fr.NumChars {
			t.fr.Insert(rp[:n], q-t.org)
		}
		if t.fr.LastLineFull {
			break
		}
	}
	fbuffree(rp)
	for i = 0; i < len(t.file.text); i++ {
		u := t.file.text[i]
		if u != t {
			if u.org > u.Len() { /* will be 0 because of reset(), but safety first */
				u.org = 0
			}
			textresize(u, u.all, true)
			textbacknl(u, u.org, 0) /* go to beginning of line */
		}
		textsetselect(u, q0, q0)
	}
	if nulls {
		alog.Printf("%s: NUL bytes elided\n", file)
	}
	return q1 - q0
}

func textbsinsert(t *Text, q0 int, r []rune, tofile bool, nrp *int) int {
	if t.what == Tag { /* can't happen but safety first: mustn't backspace over file name */
		goto Err
	}

	for i := 0; i < len(r); i++ {
		if r[i] == '\b' {
			initial := 0
			tp := make([]rune, len(r))
			copy(tp, r[:i])
			ti := i
			for ; i < len(r); i++ {
				tp[ti] = r[i]
				if tp[ti] == '\b' {
					if ti == 0 {
						initial++
					} else {
						ti--
					}
				} else {
					ti++
				}
			}
			if initial != 0 {
				if initial > q0 {
					initial = q0
				}
				q0 -= initial
				textdelete(t, q0, q0+initial, tofile)
			}
			textinsert(t, q0, tp[:ti], tofile)
			*nrp = ti
			return q0
		}
	}

Err:
	textinsert(t, q0, r, tofile)
	*nrp = len(r)
	return q0
}

func textinsert(t *Text, q0 int, r []rune, tofile bool) {
	if tofile && len(t.cache) > 0 {
		util.Fatal("text.insert")
	}
	if len(r) == 0 {
		return
	}
	if tofile {
		fileinsert(t.file, q0, r)
		if t.what == Body {
			t.w.dirty = true
			t.w.utflastqid = -1
		}
		if len(t.file.text) > 1 {
			for i := 0; i < len(t.file.text); i++ {
				u := t.file.text[i]
				if u != t {
					u.w.dirty = true /* always a body */
					textinsert(u, q0, r, false)
					textsetselect(u, u.q0, u.q1)
					textscrdraw(u)
				}
			}
		}

	}
	if q0 < t.iq1 {
		t.iq1 += len(r)
	}
	if q0 < t.q1 {
		t.q1 += len(r)
	}
	if q0 < t.q0 {
		t.q0 += len(r)
	}
	if q0 < t.org {
		t.org += len(r)
	} else if q0 <= t.org+t.fr.NumChars {
		t.fr.Insert(r, q0-t.org)
	}
	if t.w != nil {
		c := 'i'
		if t.what == Body {
			c = 'I'
		}
		if len(r) <= EVENTSIZE {
			winevent(t.w, "%c%d %d 0 %d %s\n", c, q0, q0+len(r), len(r), string(r))
		} else {
			winevent(t.w, "%c%d %d 0 0 \n", c, q0, q0+len(r))
		}
	}
}

func typecommit(t *Text) {
	if t.w != nil {
		wincommit(t.w, t)
	} else {
		textcommit(t, true)
	}
}

func textfill(t *Text) {
	if t.fr.LastLineFull || t.nofill {
		return
	}
	if len(t.cache) > 0 {
		typecommit(t)
	}
	rp := fbufalloc()
	for {
		n := t.Len() - (t.org + t.fr.NumChars)
		if n == 0 {
			break
		}
		if n > 2000 { /* educated guess at reasonable amount */
			n = 2000
		}
		t.file.b.Read(t.org+t.fr.NumChars, rp[:n])
		/*
		 * it's expensive to frinsert more than we need, so
		 * count newlines.
		 */
		nl := t.fr.MaxLines - t.fr.NumLines
		m := 0
		var i int
		for i = 0; i < n; {
			tmp25 := i
			i++
			if rp[tmp25] == '\n' {
				m++
				if m >= nl {
					break
				}
			}
		}
		t.fr.Insert(rp[:i], t.fr.NumChars)
		if t.fr.LastLineFull {
			break
		}
	}
	fbuffree(rp)
}

func textdelete(t *Text, q0 int, q1 int, tofile bool) {
	if tofile && len(t.cache) > 0 {
		util.Fatal("text.delete")
	}
	n := q1 - q0
	if n == 0 {
		return
	}
	if tofile {
		filedelete(t.file, q0, q1)
		if t.what == Body {
			t.w.dirty = true
			t.w.utflastqid = -1
		}
		if len(t.file.text) > 1 {
			for i := 0; i < len(t.file.text); i++ {
				u := t.file.text[i]
				if u != t {
					u.w.dirty = true /* always a body */
					textdelete(u, q0, q1, false)
					textsetselect(u, u.q0, u.q1)
					textscrdraw(u)
				}
			}
		}
	}
	if q0 < t.iq1 {
		t.iq1 -= util.Min(n, t.iq1-q0)
	}
	if q0 < t.q0 {
		t.q0 -= util.Min(n, t.q0-q0)
	}
	if q0 < t.q1 {
		t.q1 -= util.Min(n, t.q1-q0)
	}
	if q1 <= t.org {
		t.org -= n
	} else if q0 < t.org+t.fr.NumChars {
		p1 := q1 - t.org
		if p1 > t.fr.NumChars {
			p1 = t.fr.NumChars
		}
		var p0 int
		if q0 < t.org {
			t.org = q0
			p0 = 0
		} else {
			p0 = q0 - t.org
		}
		t.fr.Delete(p0, p1)
		textfill(t)
	}
	if t.w != nil {
		c := 'd'
		if t.what == Body {
			c = 'D'
		}
		winevent(t.w, "%c%d %d 0 0 \n", c, q0, q1)
	}
}

func textconstrain(t *Text, q0 int, q1 int, p0 *int, p1 *int) {
	*p0 = util.Min(q0, t.Len())
	*p1 = util.Min(q1, t.Len())
}

func textreadc(t *Text, q int) rune {
	var r [1]rune
	if t.cq0 <= q && q < t.cq0+len(t.cache) {
		r[0] = t.cache[q-t.cq0]
	} else {
		t.file.b.Read(q, r[:])
	}
	return r[0]
}

func textbswidth(t *Text, c rune) int {
	/* there is known to be at least one character to erase */
	if c == 0x08 { /* ^H: erase character */
		return 1
	}
	q := t.q0
	skipping := true
	for q > 0 {
		r := t.RuneAt(q - 1)
		if r == '\n' { /* eat at most one more character */
			if q == t.q0 { /* eat the newline */
				q--
			}
			break
		}
		if c == 0x17 {
			eq := runes.IsAlphaNum(r)
			if eq && skipping { /* found one; stop skipping */
				skipping = false
			} else if !eq && !skipping {
				break
			}
		}
		q--
	}
	return t.q0 - q
}

func textfilewidth(t *Text, q0 int, oneelement bool) int {
	q := q0
	for q > 0 {
		r := t.RuneAt(q - 1)
		if r <= ' ' {
			break
		}
		if oneelement && r == '/' {
			break
		}
		q--
	}
	return q0 - q
}

func textcomplete(t *Text) []rune {
	/* control-f: filename completion; works back to white space or / */
	if t.q0 < t.Len() && t.RuneAt(t.q0) > ' ' { /* must be at end of word */
		return nil
	}
	nstr := textfilewidth(t, t.q0, true)
	str := make([]rune, nstr)
	npath := textfilewidth(t, t.q0-nstr, false)
	path_ := make([]rune, npath)

	q := t.q0 - nstr
	var i int
	for i = 0; i < nstr; i++ {
		str[i] = t.RuneAt(q)
		q++
	}
	q = t.q0 - nstr - npath
	for i = 0; i < npath; i++ {
		path_[i] = t.RuneAt(q)
		q++
	}
	var dir []rune
	/* is path rooted? if not, we need to make it relative to window path */
	if npath > 0 && path_[0] == '/' {
		dir = path_
	} else {
		dir = dirname(t, nil)
		tmp := make([]rune, 200)
		if len(dir)+1+npath > len(tmp) {
			return nil
		}
		if len(dir) == 0 {
			dir = runes.Clone(Ldot)
		}
		copy(tmp, dir)
		tmp[len(dir)] = '/'
		copy(tmp[len(dir)+1:], path_)
		dir = tmp
		dir = runes.CleanPath(dir)
	}

	c, err := complete(string(dir), string(str))
	if err != nil {
		alog.Printf("error attempting completion: %v\n", err)
		return nil
	}
	defer freecompletion(c)

	if !c.advance {
		sep := ""
		if len(dir) > 0 && dir[len(dir)-1] != '/' {
			sep = "/"
		}
		more := ""
		if c.nmatch == 0 {
			more = ": no matches in:"
		}
		alog.Printf("%s%s%s*%s\n", string(dir), sep, string(str), more)
		for i = 0; i < len(c.filename); i++ {
			alog.Printf(" %s\n", c.filename[i])
		}
	}

	var rp []rune
	if c.advance {
		rp = []rune(c.string)
	}

	return rp
}

func texttype(t *Text, r rune) {
	if t.what != Body && t.what != Tag && r == '\n' {
		return
	}
	if t.what == Tag {
		t.w.tagsafe = false
	}

	var q0 int
	var nnb int
	var n int
	switch r {
	case draw.KeyLeft:
		typecommit(t)
		if t.q0 > 0 {
			textshow(t, t.q0-1, t.q0-1, true)
		}
		return
	case draw.KeyRight:
		typecommit(t)
		if t.q1 < t.Len() {
			textshow(t, t.q1+1, t.q1+1, true)
		}
		return
	case draw.KeyDown, draw.KeyPageDown, Kscrollonedown:
		if t.what == Tag {
			/* expand tag to show all text */
			if !t.w.tagexpand {
				t.w.tagexpand = true
				winresize(t.w, t.w.r, false, true)
			}
			return
		}
		switch r {
		case draw.KeyDown:
			n = t.fr.MaxLines / 3
		case draw.KeyPageDown:
			n = 2 * t.fr.MaxLines / 3
		case Kscrollonedown:
			n = draw.MouseScrollSize(t.fr.MaxLines)
			if n <= 0 {
				n = 1
			}
		}
		q0 = t.org + t.fr.CharOf(draw.Pt(t.fr.R.Min.X, t.fr.R.Min.Y+n*t.fr.Font.Height))
		textsetorigin(t, q0, true)
		return
	case draw.KeyUp, draw.KeyPageUp, Kscrolloneup:
		if t.what == Tag {
			/* shrink tag to single line */
			if t.w.tagexpand {
				t.w.tagexpand = false
				t.w.taglines = 1
				winresize(t.w, t.w.r, false, true)
			}
			return
		}
		switch r {
		case draw.KeyUp:
			n = t.fr.MaxLines / 3
		case draw.KeyPageUp:
			n = 2 * t.fr.MaxLines / 3
		case Kscrolloneup:
			n = draw.MouseScrollSize(t.fr.MaxLines)
		}
		q0 = textbacknl(t, t.org, n)
		textsetorigin(t, q0, true)
		return
	case draw.KeyHome:
		typecommit(t)
		if t.org > t.iq1 {
			q0 = textbacknl(t, t.iq1, 1)
			textsetorigin(t, q0, true)
		} else {
			textshow(t, 0, 0, false)
		}
		return
	case draw.KeyEnd:
		typecommit(t)
		if t.iq1 > t.org+t.fr.NumChars {
			if t.iq1 > t.Len() {
				// should not happen, but does. and it will crash textbacknl.
				t.iq1 = t.Len()
			}
			q0 = textbacknl(t, t.iq1, 1)
			textsetorigin(t, q0, true)
		} else {
			textshow(t, t.Len(), t.Len(), false)
		}
		return
	case 0x01: /* ^A: beginning of line */
		typecommit(t)
		/* go to where ^U would erase, if not already at BOL */
		nnb = 0
		if t.q0 > 0 && t.RuneAt(t.q0-1) != '\n' {
			nnb = textbswidth(t, 0x15)
		}
		textshow(t, t.q0-nnb, t.q0-nnb, true)
		return
	case 0x05: /* ^E: end of line */
		typecommit(t)
		q0 = t.q0
		for q0 < t.Len() && t.RuneAt(q0) != '\n' {
			q0++
		}
		textshow(t, q0, q0, true)
		return
	case draw.KeyCmd + 'c': /* %C: copy */
		typecommit(t)
		cut(t, t, nil, true, false, nil)
		return
	case draw.KeyCmd + 'z': /* %Z: undo */
		typecommit(t)
		undo(t, nil, nil, true, false, nil)
		return
	case draw.KeyCmd + 'Z': /* %-shift-Z: redo */
		typecommit(t)
		undo(t, nil, nil, false, false, nil)
		return
	}
	if t.what == Body {
		seq++
		filemark(t.file)
	}
	/* cut/paste must be done after the seq++/filemark */
	switch r {
	case draw.KeyCmd + 'x': /* %X: cut */
		typecommit(t)
		if t.what == Body {
			seq++
			filemark(t.file)
		}
		cut(t, t, nil, true, true, nil)
		textshow(t, t.q0, t.q0, true)
		t.iq1 = t.q0
		return
	case draw.KeyCmd + 'v': /* %V: paste */
		typecommit(t)
		if t.what == Body {
			seq++
			filemark(t.file)
		}
		paste(t, t, nil, true, false, nil)
		textshow(t, t.q0, t.q1, true)
		t.iq1 = t.q1
		return
	}
	if t.q1 > t.q0 {
		if len(t.cache) != 0 {
			util.Fatal("text.type")
		}
		cut(t, t, nil, true, true, nil)
		t.eq0 = ^0
	}
	textshow(t, t.q0, t.q0, true)
	var q1 int
	var nb int
	var i int
	var u *Text
	rp := []rune{r}
	switch r {
	case 0x06, /* ^F: complete */
		draw.KeyInsert:
		typecommit(t)
		rp = textcomplete(t)
		if rp == nil {
			return
		}
		/* break to normal insertion case */
	case 0x1B:
		if t.eq0 != ^0 {
			if t.eq0 <= t.q0 {
				textsetselect(t, t.eq0, t.q0)
			} else {
				textsetselect(t, t.q0, t.eq0)
			}
		}
		if len(t.cache) > 0 {
			typecommit(t)
		}
		t.iq1 = t.q0
		return
	case 0x08, /* ^H: erase character */
		0x15, /* ^U: erase line */
		0x17: /* ^W: erase word */
		if t.q0 == 0 { /* nothing to erase */
			return
		}
		nnb = textbswidth(t, r)
		q1 = t.q0
		q0 = q1 - nnb
		/* if selection is at beginning of window, avoid deleting invisible text */
		if q0 < t.org {
			q0 = t.org
			nnb = q1 - q0
		}
		if nnb <= 0 {
			return
		}
		for i = 0; i < len(t.file.text); i++ {
			u = t.file.text[i]
			u.nofill = true
			nb = nnb
			n = len(u.cache)
			if n > 0 {
				if q1 != u.cq0+n {
					util.Fatal("text.type backspace")
				}
				if n > nb {
					n = nb
				}
				u.cache = u.cache[:len(u.cache)-n]
				textdelete(u, q1-n, q1, false)
				nb -= n
			}
			if u.eq0 == q1 || u.eq0 == ^0 {
				u.eq0 = q0
			}
			if nb != 0 && u == t {
				textdelete(u, q0, q0+nb, true)
			}
			if u != t {
				textsetselect(u, u.q0, u.q1)
			} else {
				textsetselect(t, q0, q0)
			}
			u.nofill = false
		}
		for i = 0; i < len(t.file.text); i++ {
			textfill(t.file.text[i])
		}
		t.iq1 = t.q0
		return
	case '\n':
		if t.w.autoindent {
			/* find beginning of previous line using backspace code */
			nnb = textbswidth(t, 0x15) /* ^U case */
			rp = make([]rune, 1, nnb+1)
			rp[0] = '\n'
			for i = 0; i < nnb; i++ {
				r = t.RuneAt(t.q0 - nnb + i)
				if r != ' ' && r != '\t' {
					break
				}
				rp = append(rp, r)
			}
		}
		/* break to normal code */
	}
	/* otherwise ordinary character; just insert, typically in caches of all texts */
	for i = 0; i < len(t.file.text); i++ {
		u = t.file.text[i]
		if u.eq0 == ^0 {
			u.eq0 = t.q0
		}
		if len(u.cache) == 0 {
			u.cq0 = t.q0
		} else if t.q0 != u.cq0+len(u.cache) {
			util.Fatal("text.type cq1")
		}
		/*
		 * Change the tag before we add to ncache,
		 * so that if the window body is resized the
		 * commit will not find anything in ncache.
		 */
		if u.what == Body && len(u.cache) == 0 {
			u.needundo = true
			winsettag(t.w)
			u.needundo = false
		}
		textinsert(u, t.q0, rp, false)
		if u != t {
			textsetselect(u, u.q0, u.q1)
		}
		u.cache = append(u.cache, rp...)
	}
	textsetselect(t, t.q0+len(rp), t.q0+len(rp))
	if r == '\n' && t.w != nil {
		wincommit(t.w, t)
	}
	t.iq1 = t.q0
}

func textcommit(t *Text, tofile bool) {
	if len(t.cache) == 0 {
		return
	}
	if tofile {
		fileinsert(t.file, t.cq0, t.cache)
	}
	if t.what == Body {
		t.w.dirty = true
		t.w.utflastqid = -1
	}
	t.cache = t.cache[:0]
}

var clicktext *Text
var clickmsec uint32
var selecttext *Text
var selectq int

/*
 * called from frame library
 */
func framescroll(f *frame.Frame, dl int) {
	if f != &selecttext.fr {
		util.Fatal("frameselect not right frame")
	}
	textframescroll(selecttext, dl)
}

func textframescroll(t *Text, dl int) {
	if dl == 0 {
		scrsleep(100 * time.Millisecond)
		return
	}
	var q0 int
	if dl < 0 {
		q0 = textbacknl(t, t.org, -dl)
		if selectq > t.org+t.fr.P0 {
			textsetselect(t, t.org+t.fr.P0, selectq)
		} else {
			textsetselect(t, selectq, t.org+t.fr.P0)
		}
	} else {
		if t.org+t.fr.NumChars == t.Len() {
			return
		}
		q0 = t.org + t.fr.CharOf(draw.Pt(t.fr.R.Min.X, t.fr.R.Min.Y+dl*t.fr.Font.Height))
		if selectq > t.org+t.fr.P1 {
			textsetselect(t, t.org+t.fr.P1, selectq)
		} else {
			textsetselect(t, selectq, t.org+t.fr.P1)
		}
	}
	textsetorigin(t, q0, true)
}

func textselect(t *Text) {
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
	b := mouse.Buttons
	q0 := t.q0
	q1 := t.q1
	selectq = t.org + t.fr.CharOf(mouse.Point)
	if clicktext == t && mouse.Msec-clickmsec < 500 {
		if q0 == q1 && selectq == q0 {
			textdoubleclick(t, &q0, &q1)
			textsetselect(t, q0, q1)
			display.Flush()
			x := mouse.Point.X
			y := mouse.Point.Y
			/* stay here until something interesting happens */
			for {
				mousectl.Read()
				if !(mouse.Buttons == b && abs(mouse.Point.X-x) < 3) || !(abs(mouse.Point.Y-y) < 3) {
					break
				}
			}
			mouse.Point.X = x /* in case we're calling frselect */
			mouse.Point.Y = y
			q0 = t.q0 /* may have changed */
			q1 = t.q1
			selectq = q0
		}
	}
	if mouse.Buttons == b {
		t.fr.Scroll = framescroll
		t.fr.Select(mousectl)
		/* horrible botch: while asleep, may have lost selection altogether */
		if selectq > t.Len() {
			selectq = t.org + t.fr.P0
		}
		t.fr.Scroll = nil
		if selectq < t.org {
			q0 = selectq
		} else {
			q0 = t.org + t.fr.P0
		}
		if selectq > t.org+t.fr.NumChars {
			q1 = selectq
		} else {
			q1 = t.org + t.fr.P1
		}
	}
	if q0 == q1 {
		if q0 == t.q0 && clicktext == t && mouse.Msec-clickmsec < 500 {
			textdoubleclick(t, &q0, &q1)
			clicktext = nil
		} else {
			clicktext = t
			clickmsec = mouse.Msec
		}
	} else {
		clicktext = nil
	}
	textsetselect(t, q0, q1)
	display.Flush()
	state := None /* what we've done; undo when possible */
	for mouse.Buttons != 0 {
		mouse.Msec = 0
		b = mouse.Buttons
		if b&1 != 0 && b&6 != 0 {
			if state == None && t.what == Body {
				seq++
				filemark(t.w.body.file)
			}
			if b&2 != 0 {
				if state == Paste && t.what == Body {
					winundo(t.w, true)
					textsetselect(t, q0, t.q1)
					state = None
				} else if state != Cut {
					cut(t, t, nil, true, true, nil)
					state = Cut
				}
			} else {
				if state == Cut && t.what == Body {
					winundo(t.w, true)
					textsetselect(t, q0, t.q1)
					state = None
				} else if state != Paste {
					paste(t, t, nil, true, false, nil)
					state = Paste
				}
			}
			textscrdraw(t)
			clearmouse()
		}
		display.Flush()
		for mouse.Buttons == b {
			mousectl.Read()
		}
		clicktext = nil
	}
}

func textshow(t *Text, q0 int, q1 int, doselect bool) {
	if t.what != Body {
		if doselect {
			textsetselect(t, q0, q1)
		}
		return
	}
	if t.w != nil && t.fr.MaxLines == 0 {
		colgrow(t.col, t.w, 1)
	}
	if doselect {
		textsetselect(t, q0, q1)
	}
	qe := t.org + t.fr.NumChars
	tsd := false /* do we call textscrdraw? */
	nc := t.Len() + len(t.cache)
	if t.org <= q0 {
		if nc == 0 || q0 < qe {
			tsd = true
		} else if q0 == qe && qe == nc {
			if t.RuneAt(nc-1) == '\n' {
				if t.fr.NumLines < t.fr.MaxLines {
					tsd = true
				}
			} else {
				tsd = true
			}
		}
	}
	if tsd {
		textscrdraw(t)
	} else {
		var nl int
		if t.w.nopen[QWevent] > 0 {
			nl = 3 * t.fr.MaxLines / 4
		} else {
			nl = t.fr.MaxLines / 4
		}
		q := textbacknl(t, q0, nl)
		/* avoid going backwards if trying to go forwards - long lines! */
		if !(q0 > t.org) || !(q < t.org) {
			textsetorigin(t, q, true)
		}
		for q0 > t.org+t.fr.NumChars {
			textsetorigin(t, t.org+1, false)
		}
	}
}

func region(a int, b int) int {
	if a < b {
		return -1
	}
	if a == b {
		return 0
	}
	return 1
}

func selrestore(f *frame.Frame, pt0 draw.Point, p0 int, p1 int) {
	if p1 <= f.P0 || p0 >= f.P1 {
		/* no overlap */
		f.Drawsel0(pt0, p0, p1, f.Cols[frame.BACK], f.Cols[frame.TEXT])
		return
	}
	if p0 >= f.P0 && p1 <= f.P1 {
		/* entirely inside */
		f.Drawsel0(pt0, p0, p1, f.Cols[frame.HIGH], f.Cols[frame.HTEXT])
		return
	}

	/* they now are known to overlap */

	/* before selection */
	if p0 < f.P0 {
		f.Drawsel0(pt0, p0, f.P0, f.Cols[frame.BACK], f.Cols[frame.TEXT])
		p0 = f.P0
		pt0 = f.PointOf(p0)
	}
	/* after selection */
	if p1 > f.P1 {
		f.Drawsel0(f.PointOf(f.P1), f.P1, p1, f.Cols[frame.BACK], f.Cols[frame.TEXT])
		p1 = f.P1
	}
	/* inside selection */
	f.Drawsel0(pt0, p0, p1, f.Cols[frame.HIGH], f.Cols[frame.HTEXT])
}

func textsetselect(t *Text, q0 int, q1 int) {
	/* t->fr.p0 and t->fr.p1 are always right; t->q0 and t->q1 may be off */
	t.q0 = q0
	t.q1 = q1
	/* compute desired p0,p1 from q0,q1 */
	p0 := q0 - t.org
	p1 := q1 - t.org
	ticked := true
	if p0 < 0 {
		ticked = false
		p0 = 0
	}
	if p1 < 0 {
		p1 = 0
	}
	if p0 > t.fr.NumChars {
		p0 = t.fr.NumChars
	}
	if p1 > t.fr.NumChars {
		ticked = false
		p1 = t.fr.NumChars
	}
	if p0 == t.fr.P0 && p1 == t.fr.P1 {
		if p0 == p1 && ticked != t.fr.Ticked {
			t.fr.Tick(t.fr.PointOf(p0), ticked)
		}
		return
	}
	if p0 > p1 {
		panic(fmt.Sprintf("acme: textsetselect p0=%d p1=%d q0=%d q1=%d t->org=%d nchars=%d", p0, p1, q0, q1, int(t.org), int(t.fr.NumChars)))
	}
	/* screen disagrees with desired selection */
	if t.fr.P1 <= p0 || p1 <= t.fr.P0 || p0 == p1 || t.fr.P1 == t.fr.P0 {
		/* no overlap or too easy to bother trying */
		t.fr.Drawsel(t.fr.PointOf(t.fr.P0), t.fr.P0, t.fr.P1, false)
		if p0 != p1 || ticked {
			t.fr.Drawsel(t.fr.PointOf(p0), p0, p1, true)
		}
		goto Return
	}
	/* overlap; avoid unnecessary painting */
	if p0 < t.fr.P0 {
		/* extend selection backwards */
		t.fr.Drawsel(t.fr.PointOf(p0), p0, t.fr.P0, true)
	} else if p0 > t.fr.P0 {
		/* trim first part of selection */
		t.fr.Drawsel(t.fr.PointOf(t.fr.P0), t.fr.P0, p0, false)
	}
	if p1 > t.fr.P1 {
		/* extend selection forwards */
		t.fr.Drawsel(t.fr.PointOf(t.fr.P1), t.fr.P1, p1, true)
	} else if p1 < t.fr.P1 {
		/* trim last part of selection */
		t.fr.Drawsel(t.fr.PointOf(p1), p1, t.fr.P1, false)
	}

Return:
	t.fr.P0 = p0
	t.fr.P1 = p1
}

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

	/* remove tick */
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
			if reg != region(q, p0) { /* crossed starting point; reset */
				if reg > 0 {
					selrestore(f, pt0, p0, p1)
				} else if reg < 0 {
					selrestore(f, pt1, p1, p0)
				}
				p1 = p0
				pt1 = pt0
				reg = region(q, p0)
				if reg == 0 {
					f.Drawsel0(pt0, p0, p1, col, display.White)
				}
			}
			qt := f.PointOf(q)
			if reg > 0 {
				if q > p1 {
					f.Drawsel0(pt1, p1, q, col, display.White)
				} else if q < p1 {
					selrestore(f, qt, q, p1)
				}
			} else if reg < 0 {
				if q > p1 {
					selrestore(f, pt1, p1, q)
				} else {
					f.Drawsel0(qt, q, p1, col, display.White)
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
	if mc.Msec-msec < DELAY && p0 != p1 && abs(mp.X-mc.X) < MINMOVE && abs(mp.Y-mc.Y) < MINMOVE {
		if reg > 0 {
			selrestore(f, pt0, p0, p1)
		} else if reg < 0 {
			selrestore(f, pt1, p1, p0)
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
	selrestore(f, pt0, p0, p1)
	/* restore tick */
	if f.P0 == f.P1 {
		f.Tick(f.PointOf(f.P0), true)
	}
	f.Display.Flush()
	*p1p = p1
	return p0
}

func textselect23(t *Text, q0 *int, q1 *int, high *draw.Image, mask int) int {
	var p1 int
	p0 := xselect(&t.fr, mousectl, high, &p1)
	buts := mousectl.Buttons
	if buts&mask == 0 {
		*q0 = p0 + t.org
		*q1 = p1 + t.org
	}

	for mousectl.Buttons != 0 {
		mousectl.Read()
	}
	return buts
}

func textselect2(t *Text, q0 *int, q1 *int, tp **Text) int {
	*tp = nil
	buts := textselect23(t, q0, q1, but2col, 4)
	if buts&4 != 0 {
		return 0
	}
	if buts&1 != 0 { /* pick up argument */
		*tp = argtext
		return 1
	}
	return 1
}

func textselect3(t *Text, q0 *int, q1 *int) bool {
	return textselect23(t, q0, q1, but3col, 1|2) == 0
}

var (
	left  = [][]rune{[]rune("{[(<«"), []rune("\n"), []rune("'\"`")}
	right = [][]rune{[]rune("}])>»"), []rune("\n"), []rune("'\"`")}
)

func textdoubleclick(t *Text, q0 *int, q1 *int) {
	if textclickhtmlmatch(t, q0, q1) != 0 {
		return
	}

	for i := 0; i < len(left); i++ {
		q := *q0
		l := left[i]
		r := right[i]
		var c rune
		/* try matching character to left, looking right */
		if q == 0 {
			c = '\n'
		} else {
			c = t.RuneAt(q - 1)
		}
		pi := runes.IndexRune(l, c)
		if pi >= 0 {
			if textclickmatch(t, c, r[pi], 1, &q) {
				if c != '\n' {
					q--
				}
				*q1 = q
			}
			return
		}
		/* try matching character to right, looking left */
		if q == t.Len() {
			c = '\n'
		} else {
			c = t.RuneAt(q)
		}
		pi = runes.IndexRune(r, c)
		if pi >= 0 {
			if textclickmatch(t, c, l[pi], -1, &q) {
				*q1 = *q0
				if *q0 < t.Len() && c == '\n' {
					(*q1)++
				}
				*q0 = q
				if c != '\n' || q != 0 || t.RuneAt(0) == '\n' {
					(*q0)++
				}
			}
			return
		}
	}

	/* try filling out word to right */
	for *q1 < t.Len() && runes.IsAlphaNum(t.RuneAt(*q1)) {
		(*q1)++
	}
	/* try filling out word to left */
	for *q0 > 0 && runes.IsAlphaNum(t.RuneAt(*q0-1)) {
		(*q0)--
	}
}

func textclickmatch(t *Text, cl rune, cr rune, dir int, q *int) bool {
	nest := 1
	for {
		var c rune
		if dir > 0 {
			if *q == t.Len() {
				break
			}
			c = t.RuneAt(*q)
			(*q)++
		} else {
			if *q == 0 {
				break
			}
			(*q)--
			c = t.RuneAt(*q)
		}
		if c == cr {
			nest--
			if nest == 0 {
				return true
			}
		} else if c == cl {
			nest++
		}
	}
	return cl == '\n' && nest == 1
}

// Is the text starting at location q an html tag?
// Return 1 for <a>, -1 for </a>, 0 for no tag or <a />.
// Set *q1, if non-nil, to the location after the tag.
func ishtmlstart(t *Text, q int, q1 *int) int {
	if q+2 > t.Len() {
		return 0
	}
	tmp28 := q
	q++
	if t.RuneAt(tmp28) != '<' {
		return 0
	}
	c := t.RuneAt(q)
	q++
	c1 := c
	c2 := c
	for c != '>' {
		if q >= t.Len() {
			return 0
		}
		c2 = c
		c = t.RuneAt(q)
		q++
	}
	if q1 != nil {
		*q1 = q
	}
	if c1 == '/' { // closing tag
		return -1
	}
	if c2 == '/' || c2 == '!' { // open + close tag or comment
		return 0
	}
	return 1
}

// Is the text ending at location q an html tag?
// Return 1 for <a>, -1 for </a>, 0 for no tag or <a />.
// Set *q0, if non-nil, to the start of the tag.
func ishtmlend(t *Text, q int, q0 *int) int {
	if q < 2 {
		return 0
	}
	q--
	if t.RuneAt(q) != '>' {
		return 0
	}
	q--
	c := t.RuneAt(q)
	c1 := c
	c2 := c
	for c != '<' {
		if q == 0 {
			return 0
		}
		c1 = c
		q--
		c = t.RuneAt(q)
	}
	if q0 != nil {
		*q0 = q
	}
	if c1 == '/' { // closing tag
		return -1
	}
	if c2 == '/' || c2 == '!' { // open + close tag or comment
		return 0
	}
	return 1
}

func textclickhtmlmatch(t *Text, q0 *int, q1 *int) int {
	q := *q0
	var depth int
	var n int
	var nq int
	// after opening tag?  scan forward for closing tag
	if ishtmlend(t, q, nil) == 1 {
		depth = 1
		for q < t.Len() {
			n = ishtmlstart(t, q, &nq)
			if n != 0 {
				depth += n
				if depth == 0 {
					*q1 = q
					return 1
				}
				q = nq
				continue
			}
			q++
		}
	}

	// before closing tag?  scan backward for opening tag
	if ishtmlstart(t, q, nil) == -1 {
		depth = -1
		for q > 0 {
			n = ishtmlend(t, q, &nq)
			if n != 0 {
				depth += n
				if depth == 0 {
					*q0 = q
					return 1
				}
				q = nq
				continue
			}
			q--
		}
	}

	return 0
}

func textbacknl(t *Text, p int, n int) int {
	/* look for start of this line if n==0 */
	if n == 0 && p > 0 && t.RuneAt(p-1) != '\n' {
		n = 1
	}
	i := n
	for {
		tmp29 := i
		i--
		if !(tmp29 > 0) || !(p > 0) {
			break
		}
		p-- /* it's at a newline now; back over it */
		if p == 0 {
			break
		}
		/* at 128 chars, call it a line anyway */
		for j := 128; ; p-- {
			j--
			if !(j > 0) || !(p > 0) {
				break
			}
			if t.RuneAt(p-1) == '\n' {
				break
			}
		}
	}
	return p
}

func textsetorigin(t *Text, org int, exact bool) {
	if org > 0 && !exact && t.RuneAt(org-1) != '\n' {
		/* org is an estimate of the char posn; find a newline */
		/* don't try harder than 256 chars */
		for i := 0; i < 256 && org < t.Len(); i++ {
			if t.RuneAt(org) == '\n' {
				org++
				break
			}
			org++
		}
	}
	a := org - t.org
	fixup := 0
	if a >= 0 && a < t.fr.NumChars {
		t.fr.Delete(0, a)
		fixup = 1 /* frdelete can leave end of last line in wrong selection mode; it doesn't know what follows */
	} else if a < 0 && -a < t.fr.NumChars {
		n := t.org - org
		r := make([]rune, n)
		t.file.b.Read(org, r)
		t.fr.Insert(r, 0)
	} else {
		t.fr.Delete(0, t.fr.NumChars)
	}
	t.org = org
	textfill(t)
	textscrdraw(t)
	textsetselect(t, t.q0, t.q1)
	if fixup != 0 && t.fr.P1 > t.fr.P0 {
		t.fr.Drawsel(t.fr.PointOf(t.fr.P1-1), t.fr.P1-1, t.fr.P1, true)
	}
}

func textreset(t *Text) {
	t.file.seq = 0
	t.eq0 = ^0
	/* do t->delete(0, t->nc, TRUE) without building backup stuff */
	textsetselect(t, t.org, t.org)
	t.fr.Delete(0, t.fr.NumChars)
	t.org = 0
	t.q0 = 0
	t.q1 = 0
	filereset(t.file)
	t.file.b.Reset()
}

func (t *Text) RuneAt(pos int) rune { return textreadc(t, pos) }
func (t *Text) Len() int            { return t.file.b.Len() }
