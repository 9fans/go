package wind

import (
	"fmt"
	"os"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

type Text struct {
	File     *File
	Fr       frame.Frame
	Reffont  *adraw.RefFont
	Org      int
	Q0       int
	Q1       int
	What     int
	Tabstop  int
	W        *Window
	ScrollR  draw.Rectangle
	lastsr   draw.Rectangle
	All      draw.Rectangle
	Row      *Row
	Col      *Column
	IQ1      int
	Eq0      int
	Cq0      int
	Cache    []rune
	Nofill   bool
	Needundo bool
}

func (t *Text) RuneAt(pos int) rune { return textreadc(t, pos) }

func (t *Text) Len() int { return t.File.Len() }

type File struct {
	*file.File
	Curtext *Text
	Text    []*Text
	Info    os.FileInfo
	SHA1    [20]byte
	Unread  bool
	dumpid  int
}

func (f *File) SetName(r []rune) {
	f.File.SetName(r)
	f.Unread = true
}

type fileView File

func (f *fileView) Insert(pos int, data []rune) {
	for _, t := range f.Text {
		Textinsert(t, pos, data, false)
	}
}

func (f *fileView) Delete(pos, end int) {
	for _, t := range f.Text {
		Textdelete(t, pos, end, false)
	}
}

var Argtext *Text

var Typetext *Text // global because Text.close needs to clear it

var Seltext *Text

var Mousetext *Text // global because Text.close needs to clear it

var Barttext *Text // shared between mousetask and keyboardthread

const (
	TABDIR = 3
) // width of tabs in directory windows

var MaxTab int // size of a tab, in units of the '0' character

func textinit(t *Text, f *File, r draw.Rectangle, rf *adraw.RefFont, cols []*draw.Image) {
	t.File = f
	t.All = r
	t.ScrollR = r
	t.ScrollR.Max.X = r.Min.X + adraw.Scrollwid()
	t.lastsr = draw.ZR
	r.Min.X += adraw.Scrollwid() + adraw.Scrollgap()
	t.Eq0 = ^0
	t.Cache = t.Cache[:0]
	t.Reffont = rf
	t.Tabstop = MaxTab
	copy(t.Fr.Cols[:], cols)
	textredraw(t, r, rf.F, adraw.Display.ScreenImage, -1)
}

func textclose(t *Text) {
	t.Fr.Clear(true)
	filedeltext(t.File, t)
	t.File = nil
	adraw.CloseFont(t.Reffont)
	if Argtext == t {
		Argtext = nil
	}
	if Typetext == t {
		Typetext = nil
	}
	if Seltext == t {
		Seltext = nil
	}
	if Mousetext == t {
		Mousetext = nil
	}
	if Barttext == t {
		Barttext = nil
	}
}

func Textreset(t *Text) {
	t.File.SetSeq(0)
	t.Eq0 = ^0
	// do t->delete(0, t->nc, TRUE) without building backup stuff
	Textsetselect(t, t.Org, t.Org)
	t.Fr.Delete(0, t.Fr.NumChars)
	t.Org = 0
	t.Q0 = 0
	t.Q1 = 0
	t.File.ResetLogs()
	t.File.Truncate()
}

func textreadc(t *Text, q int) rune {
	var r [1]rune
	if t.Cq0 <= q && q < t.Cq0+len(t.Cache) {
		r[0] = t.Cache[q-t.Cq0]
	} else {
		t.File.Read(q, r[:])
	}
	return r[0]
}

func textredraw(t *Text, r draw.Rectangle, f *draw.Font, b *draw.Image, odx int) {
	t.Fr.Init(r, f, b, t.Fr.Cols[:])
	rr := t.Fr.R
	rr.Min.X -= adraw.Scrollwid() + adraw.Scrollgap() // back fill to scroll bar
	if t.Fr.NoRedraw == 0 {
		t.Fr.B.Draw(rr, t.Fr.Cols[frame.BACK], nil, draw.ZP)
	}
	// use no wider than 3-space tabs in a directory
	maxt := MaxTab
	if t.What == Body {
		if t.W.IsDir {
			maxt = util.Min(TABDIR, MaxTab)
		} else {
			maxt = t.Tabstop
		}
	}
	t.Fr.MaxTab = maxt * f.StringWidth("0")
	if t.What == Body && t.W.IsDir && odx != t.All.Dx() {
		if t.Fr.MaxLines > 0 {
			Textreset(t)
			Textcolumnate(t, t.W.Dlp)
			Textshow(t, 0, 0, true)
		}
	} else {
		Textfill(t)
		Textsetselect(t, t.Q0, t.Q1)
	}
}

func Textfill(t *Text) {
	if t.Fr.LastLineFull || t.Nofill {
		return
	}
	if len(t.Cache) > 0 {
		Typecommit(t)
	}
	rp := bufs.AllocRunes()
	for {
		n := t.Len() - (t.Org + t.Fr.NumChars)
		if n == 0 {
			break
		}
		if n > 2000 { // educated guess at reasonable amount
			n = 2000
		}
		t.File.Read(t.Org+t.Fr.NumChars, rp[:n])
		/*
		 * it's expensive to frinsert more than we need, so
		 * count newlines.
		 */
		nl := t.Fr.MaxLines - t.Fr.NumLines
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
		t.Fr.Insert(rp[:i], t.Fr.NumChars)
		if t.Fr.LastLineFull {
			break
		}
	}
	bufs.FreeRunes(rp)
}

func Textresize(t *Text, r draw.Rectangle, keepextra bool) int {
	if r.Dy() <= 0 {
		r.Max.Y = r.Min.Y
	} else if !keepextra {
		r.Max.Y -= r.Dy() % t.Fr.Font.Height
	}
	odx := t.All.Dx()
	t.All = r
	t.ScrollR = r
	t.ScrollR.Max.X = r.Min.X + adraw.Scrollwid()
	t.lastsr = draw.ZR
	r.Min.X += adraw.Scrollwid() + adraw.Scrollgap()
	t.Fr.Clear(false)
	textredraw(t, r, t.Fr.Font, t.Fr.B, odx)
	if keepextra && t.Fr.R.Max.Y < t.All.Max.Y && t.Fr.NoRedraw == 0 {
		// draw background in bottom fringe of window
		r.Min.X -= adraw.Scrollgap()
		r.Min.Y = t.Fr.R.Max.Y
		r.Max.Y = t.All.Max.Y
		adraw.Display.ScreenImage.Draw(r, t.Fr.Cols[frame.BACK], nil, draw.ZP)
	}
	return t.All.Max.Y
}

func Textcolumnate(t *Text, dlp []*Dirlist) {
	if len(t.File.Text) > 1 {
		return
	}
	mint := t.Fr.Font.StringWidth("0")
	// go for narrower tabs if set more than 3 wide
	t.Fr.MaxTab = util.Min(MaxTab, TABDIR) * mint
	maxt := t.Fr.MaxTab
	colw := 0
	var i int
	var w int
	var dl *Dirlist
	for i = 0; i < len(dlp); i++ {
		dl = dlp[i]
		w = dl.Wid
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
		ncol = util.Max(1, t.Fr.R.Dx()/colw)
	}
	nrow := (len(dlp) + ncol - 1) / ncol

	q1 := 0
	for i = 0; i < nrow; i++ {
		for j := i; j < len(dlp); j += nrow {
			dl = dlp[j]
			t.File.Insert(q1, dl.R)
			q1 += len(dl.R)
			if j+nrow >= len(dlp) {
				break
			}
			w = dl.Wid
			if maxt-w%maxt < mint {
				t.File.Insert(q1, []rune("\t"))
				q1++
				w += mint
			}
			for {
				t.File.Insert(q1, []rune("\t"))
				q1++
				w += maxt - (w % maxt)
				if w >= colw {
					break
				}
			}
		}
		t.File.Insert(q1, []rune("\n"))
		q1++
	}
}

func Textinsert(t *Text, q0 int, r []rune, tofile bool) {
	if tofile && len(t.Cache) > 0 {
		util.Fatal("text.insert")
	}
	if len(r) == 0 {
		return
	}
	if tofile {
		t.File.Insert(q0, r)
		if t.What == Body {
			t.W.Dirty = true
			t.W.Utflastqid = -1
		}
		if len(t.File.Text) > 1 {
			for i := 0; i < len(t.File.Text); i++ {
				u := t.File.Text[i]
				if u != t {
					u.W.Dirty = true // always a body
					Textinsert(u, q0, r, false)
					Textsetselect(u, u.Q0, u.Q1)
					Textscrdraw(u)
				}
			}
		}

	}
	if q0 < t.IQ1 {
		t.IQ1 += len(r)
	}
	if q0 < t.Q1 {
		t.Q1 += len(r)
	}
	if q0 < t.Q0 {
		t.Q0 += len(r)
	}
	if q0 < t.Org {
		t.Org += len(r)
	} else if q0 <= t.Org+t.Fr.NumChars {
		t.Fr.Insert(r, q0-t.Org)
	}
	if t.W != nil {
		c := 'i'
		if t.What == Body {
			c = 'I'
		}
		if len(r) <= EVENTSIZE {
			Winevent(t.W, "%c%d %d 0 %d %s\n", c, q0, q0+len(r), len(r), string(r))
		} else {
			Winevent(t.W, "%c%d %d 0 0 \n", c, q0, q0+len(r))
		}
	}
}

func Textdelete(t *Text, q0 int, q1 int, tofile bool) {
	if tofile && len(t.Cache) > 0 {
		util.Fatal("text.delete")
	}
	n := q1 - q0
	if n == 0 {
		return
	}
	if tofile {
		t.File.Delete(q0, q1)
		if t.What == Body {
			t.W.Dirty = true
			t.W.Utflastqid = -1
		}
		if len(t.File.Text) > 1 {
			for i := 0; i < len(t.File.Text); i++ {
				u := t.File.Text[i]
				if u != t {
					u.W.Dirty = true // always a body
					Textdelete(u, q0, q1, false)
					Textsetselect(u, u.Q0, u.Q1)
					Textscrdraw(u)
				}
			}
		}
	}
	if q0 < t.IQ1 {
		t.IQ1 -= util.Min(n, t.IQ1-q0)
	}
	if q0 < t.Q0 {
		t.Q0 -= util.Min(n, t.Q0-q0)
	}
	if q0 < t.Q1 {
		t.Q1 -= util.Min(n, t.Q1-q0)
	}
	if q1 <= t.Org {
		t.Org -= n
	} else if q0 < t.Org+t.Fr.NumChars {
		p1 := q1 - t.Org
		if p1 > t.Fr.NumChars {
			p1 = t.Fr.NumChars
		}
		var p0 int
		if q0 < t.Org {
			t.Org = q0
			p0 = 0
		} else {
			p0 = q0 - t.Org
		}
		t.Fr.Delete(p0, p1)
		Textfill(t)
	}
	if t.W != nil {
		c := 'd'
		if t.What == Body {
			c = 'D'
		}
		Winevent(t.W, "%c%d %d 0 0 \n", c, q0, q1)
	}
}

func Textcommit(t *Text, tofile bool) {
	if len(t.Cache) == 0 {
		return
	}
	if tofile {
		t.File.Insert(t.Cq0, t.Cache)
	}
	if t.What == Body {
		t.W.Dirty = true
		t.W.Utflastqid = -1
	}
	t.Cache = t.Cache[:0]
}

func Typecommit(t *Text) {
	if t.W != nil {
		Wincommit(t.W, t)
	} else {
		Textcommit(t, true)
	}
}

func Textbsinsert(t *Text, q0 int, r []rune, tofile bool, nrp *int) int {
	if t.What == Tag { // can't happen but safety first: mustn't backspace over file name
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
				Textdelete(t, q0, q0+initial, tofile)
			}
			Textinsert(t, q0, tp[:ti], tofile)
			*nrp = ti
			return q0
		}
	}

Err:
	Textinsert(t, q0, r, tofile)
	*nrp = len(r)
	return q0
}

func Textbswidth(t *Text, c rune) int {
	// there is known to be at least one character to erase
	if c == 0x08 { // ^H: erase character
		return 1
	}
	q := t.Q0
	skipping := true
	for q > 0 {
		r := t.RuneAt(q - 1)
		if r == '\n' { // eat at most one more character
			if q == t.Q0 { // eat the newline
				q--
			}
			break
		}
		if c == 0x17 {
			eq := runes.IsAlphaNum(r)
			if eq && skipping { // found one; stop skipping
				skipping = false
			} else if !eq && !skipping {
				break
			}
		}
		q--
	}
	return t.Q0 - q
}

func Textfilewidth(t *Text, q0 int, oneelement bool) int {
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

func Textshow(t *Text, q0 int, q1 int, doselect bool) {
	if t.What != Body {
		if doselect {
			Textsetselect(t, q0, q1)
		}
		return
	}
	if t.W != nil && t.Fr.MaxLines == 0 {
		Colgrow(t.Col, t.W, 1)
	}
	if doselect {
		Textsetselect(t, q0, q1)
	}
	qe := t.Org + t.Fr.NumChars
	tsd := false // do we call textscrdraw?
	nc := t.Len() + len(t.Cache)
	if t.Org <= q0 {
		if nc == 0 || q0 < qe {
			tsd = true
		} else if q0 == qe && qe == nc {
			if t.RuneAt(nc-1) == '\n' {
				if t.Fr.NumLines < t.Fr.MaxLines {
					tsd = true
				}
			} else {
				tsd = true
			}
		}
	}
	if tsd {
		Textscrdraw(t)
	} else {
		var nl int
		if t.W.External {
			nl = 3 * t.Fr.MaxLines / 4
		} else {
			nl = t.Fr.MaxLines / 4
		}
		q := Textbacknl(t, q0, nl)
		// avoid going backwards if trying to go forwards - long lines!
		if !(q0 > t.Org) || !(q < t.Org) {
			Textsetorigin(t, q, true)
		}
		for q0 > t.Org+t.Fr.NumChars {
			Textsetorigin(t, t.Org+1, false)
		}
	}
}

func Textbacknl(t *Text, p int, n int) int {
	// look for start of this line if n==0
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
		p-- // it's at a newline now; back over it
		if p == 0 {
			break
		}
		// at 128 chars, call it a line anyway
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

func Textsetorigin(t *Text, org int, exact bool) {
	if org > 0 && !exact && t.RuneAt(org-1) != '\n' {
		// org is an estimate of the char posn; find a newline
		// don't try harder than 256 chars
		for i := 0; i < 256 && org < t.Len(); i++ {
			if t.RuneAt(org) == '\n' {
				org++
				break
			}
			org++
		}
	}
	a := org - t.Org
	fixup := 0
	if a >= 0 && a < t.Fr.NumChars {
		t.Fr.Delete(0, a)
		fixup = 1 // frdelete can leave end of last line in wrong selection mode; it doesn't know what follows
	} else if a < 0 && -a < t.Fr.NumChars {
		n := t.Org - org
		r := make([]rune, n)
		t.File.Read(org, r)
		t.Fr.Insert(r, 0)
	} else {
		t.Fr.Delete(0, t.Fr.NumChars)
	}
	t.Org = org
	Textfill(t)
	Textscrdraw(t)
	Textsetselect(t, t.Q0, t.Q1)
	if fixup != 0 && t.Fr.P1 > t.Fr.P0 {
		t.Fr.Drawsel(t.Fr.PointOf(t.Fr.P1-1), t.Fr.P1-1, t.Fr.P1, true)
	}
}

func Region(a int, b int) int {
	if a < b {
		return -1
	}
	if a == b {
		return 0
	}
	return 1
}

func Selrestore(f *frame.Frame, pt0 draw.Point, p0 int, p1 int) {
	if p1 <= f.P0 || p0 >= f.P1 {
		// no overlap
		f.Drawsel0(pt0, p0, p1, f.Cols[frame.BACK], f.Cols[frame.TEXT])
		return
	}
	if p0 >= f.P0 && p1 <= f.P1 {
		// entirely inside
		f.Drawsel0(pt0, p0, p1, f.Cols[frame.HIGH], f.Cols[frame.HTEXT])
		return
	}

	// they now are known to overlap

	// before selection
	if p0 < f.P0 {
		f.Drawsel0(pt0, p0, f.P0, f.Cols[frame.BACK], f.Cols[frame.TEXT])
		p0 = f.P0
		pt0 = f.PointOf(p0)
	}
	// after selection
	if p1 > f.P1 {
		f.Drawsel0(f.PointOf(f.P1), f.P1, p1, f.Cols[frame.BACK], f.Cols[frame.TEXT])
		p1 = f.P1
	}
	// inside selection
	f.Drawsel0(pt0, p0, p1, f.Cols[frame.HIGH], f.Cols[frame.HTEXT])
}

func Textsetselect(t *Text, q0 int, q1 int) {
	// t->fr.p0 and t->fr.p1 are always right; t->q0 and t->q1 may be off
	t.Q0 = q0
	t.Q1 = q1
	// compute desired p0,p1 from q0,q1
	p0 := q0 - t.Org
	p1 := q1 - t.Org
	ticked := true
	if p0 < 0 {
		ticked = false
		p0 = 0
	}
	if p1 < 0 {
		p1 = 0
	}
	if p0 > t.Fr.NumChars {
		p0 = t.Fr.NumChars
	}
	if p1 > t.Fr.NumChars {
		ticked = false
		p1 = t.Fr.NumChars
	}
	if p0 == t.Fr.P0 && p1 == t.Fr.P1 {
		if p0 == p1 && ticked != t.Fr.Ticked {
			t.Fr.Tick(t.Fr.PointOf(p0), ticked)
		}
		return
	}
	if p0 > p1 {
		panic(fmt.Sprintf("acme: textsetselect p0=%d p1=%d q0=%d q1=%d t->org=%d nchars=%d", p0, p1, q0, q1, int(t.Org), int(t.Fr.NumChars)))
	}
	// screen disagrees with desired selection
	if t.Fr.P1 <= p0 || p1 <= t.Fr.P0 || p0 == p1 || t.Fr.P1 == t.Fr.P0 {
		// no overlap or too easy to bother trying
		t.Fr.Drawsel(t.Fr.PointOf(t.Fr.P0), t.Fr.P0, t.Fr.P1, false)
		if p0 != p1 || ticked {
			t.Fr.Drawsel(t.Fr.PointOf(p0), p0, p1, true)
		}
		goto Return
	}
	// overlap; avoid unnecessary painting
	if p0 < t.Fr.P0 {
		// extend selection backwards
		t.Fr.Drawsel(t.Fr.PointOf(p0), p0, t.Fr.P0, true)
	} else if p0 > t.Fr.P0 {
		// trim first part of selection
		t.Fr.Drawsel(t.Fr.PointOf(t.Fr.P0), t.Fr.P0, p0, false)
	}
	if p1 > t.Fr.P1 {
		// extend selection forwards
		t.Fr.Drawsel(t.Fr.PointOf(t.Fr.P1), t.Fr.P1, p1, true)
	} else if p1 < t.Fr.P1 {
		// trim last part of selection
		t.Fr.Drawsel(t.Fr.PointOf(p1), p1, t.Fr.P1, false)
	}

Return:
	t.Fr.P0 = p0
	t.Fr.P1 = p1
}

var (
	left  = [][]rune{[]rune("{[(<«"), []rune("\n"), []rune("'\"`")}
	right = [][]rune{[]rune("}])>»"), []rune("\n"), []rune("'\"`")}
)

func Textdoubleclick(t *Text, q0 *int, q1 *int) {
	if textclickhtmlmatch(t, q0, q1) != 0 {
		return
	}

	for i := 0; i < len(left); i++ {
		q := *q0
		l := left[i]
		r := right[i]
		var c rune
		// try matching character to left, looking right
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
		// try matching character to right, looking left
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

	// try filling out word to right
	for *q1 < t.Len() && runes.IsAlphaNum(t.RuneAt(*q1)) {
		(*q1)++
	}
	// try filling out word to left
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

func fileaddtext(f *File, t *Text) *File {
	if f == nil {
		f = &File{File: new(file.File)}
		f.File.SetView((*fileView)(f))
		f.Unread = true
	}
	f.Text = append(f.Text, t)
	f.Curtext = t
	return f
}

func filedeltext(f *File, t *Text) {
	var i int
	for i = 0; i < len(f.Text); i++ {
		if f.Text[i] == t {
			goto Found
		}
	}
	util.Fatal("can't find text in filedeltext")

Found:
	copy(f.Text[i:], f.Text[i+1:])
	f.Text = f.Text[:len(f.Text)-1]
	if len(f.Text) == 0 {
		f.Close()
		return
	}
	if f.Curtext == t {
		f.Curtext = f.Text[0]
	}
}

func Dirname(t *Text, r []rune) []rune {
	if t == nil || t.W == nil {
		goto Rescue
	}
	{
		nt := t.W.Tag.Len()
		if nt == 0 {
			goto Rescue
		}
		if len(r) >= 1 && r[0] == '/' {
			goto Rescue
		}
		b, i := parsetag(t.W, len(r))
		slash := -1
		for i--; i >= 0; i-- {
			if b[i] == '/' {
				slash = i
				break
			}
		}
		if slash < 0 {
			goto Rescue
		}
		b = append(b[:slash+1], r...)
		return runes.CleanPath(b)
	}

Rescue:
	tmp := r
	if len(r) >= 1 {
		return runes.CleanPath(tmp)
	}
	return tmp
}
