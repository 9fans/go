package main

import (
	"image"
	"unicode/utf8"

	"9fans.net/go/draw"
)

var name [][]uint8 /* first byte is ' ' or '\'': modified state */
var text []*Text   /* pointer to Text associated with file */
var tag []int      /* text[i].tag, even if text[i] not defined */
var mw int

const (
	Cut = iota
	Paste
	Snarf
	Plumb
	Look
	Exch
	Search
	Send
	NMENU2C
	NMENU2 = Search
)

const (
	New = iota
	Zerox
	Resize
	Close
	Write
	NMENU3
)

var menu2str = []string{
	"cut",
	"paste",
	"snarf",
	"plumb",
	"look",
	"<rio>",
	"", /* storage for last pattern */
}

var menu3str = []string{
	"new",
	"zerox",
	"resize",
	"close",
	"write",
}

var menu2 = draw.Menu{Gen: genmenu2}
var menu2c = draw.Menu{Gen: genmenu2c}
var menu3 = draw.Menu{Gen: genmenu3}

func menu2hit() {
	t := which.text
	w := t.find(which)
	if hversion == 0 || plumbfd == nil {
		menu2str[Plumb] = "(plumb)"
	}
	menu := &menu2
	if t == &cmd {
		menu = &menu2c
	}
	m := draw.MenuHit(2, mousectl, menu, nil)
	if hostlock != 0 || t.lock != 0 {
		return
	}

	switch m {
	case Cut:
		cut(t, w, true, true)

	case Paste:
		paste(t, w)

	case Snarf:
		snarf(t, w)

	case Plumb:
		if hversion > 0 {
			outTsll(Tplumb, t.tag, which.p0, which.p1)
		}

	case Exch:
		snarf(t, w)
		outT0(Tstartsnarf)
		setlock()

	case Look:
		outTsll(Tlook, t.tag, which.p0, which.p1)
		setlock()

	case Search:
		outcmd()
		if t == &cmd {
			outTsll(Tsend, 0, which.p0, which.p1) /*ignored*/
		} else {
			outT0(Tsearch)
		}
		setlock()
	}
}

func menu3hit() {
	mw = -1
	m := draw.MenuHit(3, mousectl, &menu3, nil)
	var r image.Rectangle
	var l *Flayer
	var i int
	var t *Text
	switch m {
	case -1:
		break

	case New:
		if hostlock == 0 {
			sweeptext(true, 0)
		}

	case Zerox,
		Resize:
		if hostlock == 0 {
			display.SwitchCursor(&bullseye)
			buttons(Down)
			if mousep.Buttons&4 != 0 && func() bool { l = flwhich(mousep.Point); return l != nil }() && getr(&r) {
				duplicate(l, r, l.f.Font, m == Resize)
			} else {
				display.SwitchCursor(cursor)
			}
			buttons(Up)
		}

	case Close:
		if hostlock == 0 {
			display.SwitchCursor(&bullseye)
			buttons(Down)
			if mousep.Buttons&4 != 0 {
				l = flwhich(mousep.Point)
				if l != nil && hostlock == 0 {
					t = l.text
					if t.nwin > 1 {
						closeup(l)
					} else if t != &cmd {
						outTs(Tclose, t.tag)
						setlock()
					}
				}
			}
			display.SwitchCursor(cursor)
			buttons(Up)
		}

	case Write:
		if hostlock == 0 {
			display.SwitchCursor(&bullseye)
			buttons(Down)
			if mousep.Buttons&4 != 0 && func() bool { l = flwhich(mousep.Point); return l != nil }() {
				outTs(Twrite, l.text.tag)
				setlock()
			} else {
				display.SwitchCursor(cursor)
			}
			buttons(Up)
		}

	default:
		t = text[m-NMENU3]
		if t != nil {
			i = t.front
			if t.nwin == 0 || t.l[i].textfn == nil {
				return /* not ready yet; try again later */
			}
			if t.nwin > 1 && which == &t.l[i] {
				for {
					i++
					if i == NL {
						i = 0
					}
					if !(i != t.front) || !(t.l[i].textfn == nil) {
						break
					}
				}
			}
			current(&t.l[i])
		} else if hostlock == 0 {
			sweeptext(false, tag[m-NMENU3])
		}
	}
}

func sweeptext(isNew bool, tag int) *Text {
	var r image.Rectangle
	if !getr(&r) {
		return nil
	}

	t := new(Text)
	current(nil)
	flnew(&t.l[0], gettext, t)
	flinit(&t.l[0], r, font, maincols[:]) /*bnl*/
	textID++
	t.id = textID
	textByID[t.id] = t
	t.nwin = 1
	rinit(&t.rasp)
	if isNew {
		startnewfile(Tstartnewfile, t)
	} else {
		rinit(&t.rasp)
		t.tag = tag
		startfile(t)
	}
	return t
}

func whichmenu(tg int) int {
	for i := range tag {
		if tag[i] == tg {
			return i
		}
	}
	return -1
}

func menuins(n int, s []byte, t *Text, m byte, tg int) {
	name = append(name, nil)
	text = append(text, nil)
	tag = append(tag, 0)
	copy(name[n+1:], name[n:])
	copy(text[n+1:], text[n:])
	copy(tag[n+1:], tag[n:])
	text[n] = t
	tag[n] = tg
	name[n] = make([]byte, 1+len(s))
	name[n][0] = m
	copy(name[n][1:], s)
	menu3.LastHit = n + NMENU3
}

func menudel(n int) {
	if n >= len(text) || text[n] != nil {
		panic("menudel")
	}
	copy(name[n:], name[n+1:])
	copy(text[n:], text[n+1:])
	copy(tag[n:], tag[n+1:])
	name = name[:len(name)-1]
	text = text[:len(text)-1]
	tag = tag[:len(tag)-1]
}

func setpat(s []byte) {
	if len(s) > 15 {
		s = s[:15]
	}
	menu2str[Search] = "/" + string(s)
}

func paren(buf []byte, s string) []byte {
	buf = append(buf, '(')
	buf = append(buf, s...)
	buf = append(buf, ')')
	return buf
}

func genmenu2(n int, buf []byte) ([]byte, bool) {
	t := which.text
	if n >= NMENU2+1 || menu2str[Search] == "" && n >= NMENU2 {
		return nil, false
	}
	p := menu2str[n]
	if hostlock == 0 && t.lock == 0 || n == Search || n == Look {
		return append(buf, p...), true
	}
	return paren(buf, p), true
}

func genmenu2c(n int, buf []byte) ([]byte, bool) {
	t := which.text
	if n >= NMENU2C {
		return nil, false
	}
	var p string
	if n == Send {
		p = "send"
	} else {
		p = menu2str[n]
	}
	if hostlock == 0 && t.lock == 0 {
		return append(buf, p...), true
	}
	return paren(buf, p), true
}

func genmenu3(n int, buf []byte) ([]byte, bool) {
	if n >= NMENU3+len(name) {
		return nil, false
	}
	if n < NMENU3 {
		p := menu3str[n]
		if hostlock != 0 {
			return paren(buf, p), true
		}
		return append(buf, p...), true
	}
	n -= NMENU3
	if n == 0 { /* unless we've been fooled, this is cmd */
		return append(buf, name[n][1:]...), true
	}
	if mw == -1 {
		mw = 7 /* strlen("~~sam~~"); */
		for i := 1; i < len(name); i++ {
			w := utf8.RuneCount(name[i][1:]) + 4 /* include "'+. " */
			if w > mw {
				mw = w
			}
		}
	}
	const NBUF = 64
	if mw > NBUF {
		mw = NBUF
	}
	t := text[n]
	buf = append(buf, name[n][0], '-', ' ', ' ')
	if t != nil {
		if t.nwin == 1 {
			buf[1] = '+'
		} else if t.nwin > 1 {
			buf[1] = '*'
		}
		if work != nil && t == work.text {
			buf[2] = '.'
			if modified {
				buf[0] = '\''
			}
		}
	}
	l := utf8.RuneCount(name[n][1:])
	var k int
	if l > NBUF-4-2 {
		i := 4
		k = 1
		for i < NBUF/2 {
			_, w := utf8.DecodeRune(name[n][k:])
			k += w
			i++
		}
		buf = append(buf, name[n][1:k]...)
		buf = append(buf, "..."...)
		for (l - i) >= NBUF/2-4 {
			_, w := utf8.DecodeRune(name[n][k:])
			k += w
			i++
		}
		buf = append(buf, name[n][k:]...)
	} else {
		buf = append(buf, name[n][1:]...)
	}
	i := utf8.RuneCount(buf)
	for i < mw {
		buf = append(buf, ' ')
		i++
	}
	return buf, true
}
