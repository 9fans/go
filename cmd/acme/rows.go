// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <bio.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package main

import (
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"unicode/utf8"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
)

var Lcolhdr = []rune("Newcol Kill Putall Dump Exit ")

func rowinit(row *Row, r draw.Rectangle) {
	display.ScreenImage.Draw(r, display.White, nil, draw.ZP)
	row.r = r
	row.col = nil
	r1 := r
	r1.Max.Y = r1.Min.Y + font.Height
	t := &row.tag
	textinit(t, fileaddtext(nil, t), r1, rfget(false, false, false, ""), tagcols[:])
	t.what = Rowtag
	t.row = row
	t.w = nil
	t.col = nil
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += Border()
	display.ScreenImage.Draw(r1, display.Black, nil, draw.ZP)
	textinsert(t, 0, Lcolhdr, true)
	textsetselect(t, t.Len(), t.Len())
}

func rowadd(row *Row, c *Column, x int) *Column {
	var d *Column
	r := row.r
	r.Min.Y = row.tag.fr.R.Max.Y + Border()
	if x < r.Min.X && len(row.col) > 0 { /*steal 40% of last column by default */
		d = row.col[len(row.col)-1]
		x = d.r.Min.X + 3*d.r.Dx()/5
	}
	var i int
	/* look for column we'll land on */
	for i = 0; i < len(row.col); i++ {
		d = row.col[i]
		if x < d.r.Max.X {
			break
		}
	}
	if len(row.col) > 0 {
		if i < len(row.col) {
			i++ /* new column will go after d */
		}
		r = d.r
		if r.Dx() < 100 {
			return nil
		}
		display.ScreenImage.Draw(r, display.White, nil, draw.ZP)
		r1 := r
		r1.Max.X = util.Min(x-Border(), r.Max.X-50)
		if r1.Dx() < 50 {
			r1.Max.X = r1.Min.X + 50
		}
		colresize(d, r1)
		r1.Min.X = r1.Max.X
		r1.Max.X = r1.Min.X + Border()
		display.ScreenImage.Draw(r1, display.Black, nil, draw.ZP)
		r.Min.X = r1.Max.X
	}
	if c == nil {
		c = new(Column)
		colinit(c, r)
		util.Incref(&reffont.ref)
	} else {
		colresize(c, r)
	}
	c.row = row
	c.tag.row = row
	row.col = append(row.col, nil)
	copy(row.col[i+1:], row.col[i:])
	row.col[i] = c
	clearmouse()
	return c
}

func rowresize(row *Row, r draw.Rectangle) {
	or := row.r
	deltax := r.Min.X - or.Min.X
	row.r = r
	r1 := r
	r1.Max.Y = r1.Min.Y + font.Height
	textresize(&row.tag, r1, true)
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += Border()
	display.ScreenImage.Draw(r1, display.Black, nil, draw.ZP)
	r.Min.Y = r1.Max.Y
	r1 = r
	r1.Max.X = r1.Min.X
	for i := 0; i < len(row.col); i++ {
		c := row.col[i]
		r1.Min.X = r1.Max.X
		/* the test should not be necessary, but guarantee we don't lose a pixel */
		if i == len(row.col)-1 {
			r1.Max.X = r.Max.X
		} else {
			r1.Max.X = (c.r.Max.X-or.Min.X)*r.Dx()/or.Dx() + deltax
		}
		if i > 0 {
			r2 := r1
			r2.Max.X = r2.Min.X + Border()
			display.ScreenImage.Draw(r2, display.Black, nil, draw.ZP)
			r1.Min.X = r2.Max.X
		}
		colresize(c, r1)
	}
}

func rowdragcol(row *Row, c *Column, _0 int) {
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

	for i = 0; i < len(row.col); i++ {
		if row.col[i] == c {
			goto Found
		}
	}
	util.Fatal("can't find column")

Found:
	p := mouse.Point
	if abs(p.X-op.X) < 5 && abs(p.Y-op.Y) < 5 {
		return
	}
	if (i > 0 && p.X < row.col[i-1].r.Min.X) || (i < len(row.col)-1 && p.X > c.r.Max.X) {
		/* shuffle */
		x := c.r.Min.X
		rowclose(row, c, false)
		if rowadd(row, c, p.X) == nil { /* whoops! */
			if rowadd(row, c, x) == nil { /* WHOOPS! */
				if rowadd(row, c, -1) == nil { /* shit! */
					rowclose(row, c, true)
					return
				}
			}
		}
		colmousebut(c)
		return
	}
	if i == 0 {
		return
	}
	d := row.col[i-1]
	if p.X < d.r.Min.X+80+Scrollwid() {
		p.X = d.r.Min.X + 80 + Scrollwid()
	}
	if p.X > c.r.Max.X-80-Scrollwid() {
		p.X = c.r.Max.X - 80 - Scrollwid()
	}
	r := d.r
	r.Max.X = c.r.Max.X
	display.ScreenImage.Draw(r, display.White, nil, draw.ZP)
	r.Max.X = p.X
	colresize(d, r)
	r = c.r
	r.Min.X = p.X
	r.Max.X = r.Min.X
	r.Max.X += Border()
	display.ScreenImage.Draw(r, display.Black, nil, draw.ZP)
	r.Min.X = r.Max.X
	r.Max.X = c.r.Max.X
	colresize(c, r)
	colmousebut(c)
}

func rowclose(row *Row, c *Column, dofree bool) {
	var i int
	for i = 0; i < len(row.col); i++ {
		if row.col[i] == c {
			goto Found
		}
	}
	util.Fatal("can't find column")
Found:
	r := c.r
	if dofree {
		colcloseall(c)
	}
	copy(row.col[i:], row.col[i+1:])
	row.col = row.col[:len(row.col)-1]
	if len(row.col) == 0 {
		display.ScreenImage.Draw(r, display.White, nil, draw.ZP)
		return
	}
	if i == len(row.col) { /* extend last column right */
		c = row.col[i-1]
		r.Min.X = c.r.Min.X
		r.Max.X = row.r.Max.X
	} else { /* extend next window left */
		c = row.col[i]
		r.Max.X = c.r.Max.X
	}
	display.ScreenImage.Draw(r, display.White, nil, draw.ZP)
	colresize(c, r)
}

func rowwhichcol(row *Row, p draw.Point) *Column {
	for i := 0; i < len(row.col); i++ {
		c := row.col[i]
		if p.In(c.r) {
			return c
		}
	}
	return nil
}

func rowwhich(row *Row, p draw.Point) *Text {
	if p.In(row.tag.all) {
		return &row.tag
	}
	c := rowwhichcol(row, p)
	if c != nil {
		return colwhich(c, p)
	}
	return nil
}

func rowtype(row *Row, r rune, p draw.Point) *Text {
	if r == 0 {
		r = utf8.RuneError
	}

	clearmouse()
	row.lk.Lock()
	var t *Text
	if bartflag {
		t = barttext
	} else {
		t = rowwhich(row, p)
	}
	if t != nil && (t.what != Tag || !p.In(t.scrollr)) {
		w := t.w
		if w == nil {
			texttype(t, r)
		} else {
			winlock(w, 'K')
			wintype(w, t, r)
			/* Expand tag if necessary */
			if t.what == Tag {
				t.w.tagsafe = false
				if r == '\n' {
					t.w.tagexpand = true
				}
				winresize(w, w.r, true, true)
			}
			winunlock(w)
		}
	}
	row.lk.Unlock()
	return t
}

func rowclean(row *Row) bool {
	clean := true
	for i := 0; i < len(row.col); i++ {
		clean = colclean(row.col[i]) && clean
	}
	return clean
}

func rowdump(row *Row, file *string) {
	if len(row.col) == 0 {
		return
	}
	// defer fbuffree(buf)
	if file == nil {
		if home == "" {
			alog.Printf("can't find file for dump: $home not defined\n")
			return
		}
		s := fmt.Sprintf("%s/acme.dump", home)
		file = &s
	}
	f, err := os.Create(*file)
	if err != nil {
		alog.Printf("can't open %s: %v\n", *file, err)
		return
	}
	b := bufio.NewWriter(f)
	r := bufs.AllocRunes()
	fmt.Fprintf(b, "%s\n", wdir)
	fmt.Fprintf(b, "%s\n", fontnames[0])
	fmt.Fprintf(b, "%s\n", fontnames[1])
	var i int
	var c *Column
	for i = 0; i < len(row.col); i++ {
		c = row.col[i]
		fmt.Fprintf(b, "%11.7f", 100.0*float64(c.r.Min.X-row.r.Min.X)/float64(row.r.Dx()))
		if i == len(row.col)-1 {
			b.WriteByte('\n')
		} else {
			b.WriteByte(' ')
		}
	}
	for _, c := range row.col {
		for _, w := range c.w {
			w.body.file.dumpid = 0
		}
	}
	m := util.Min(bufs.RuneLen, row.tag.Len())
	row.tag.file.b.Read(0, r[:m])
	n := 0
	for n < m && r[n] != '\n' {
		n++
	}
	fmt.Fprintf(b, "w %s\n", string(r[:n]))
	for i = 0; i < len(row.col); i++ {
		c = row.col[i]
		m = util.Min(bufs.RuneLen, c.tag.Len())
		c.tag.file.b.Read(0, r[:m])
		n = 0
		for n < m && r[n] != '\n' {
			n++
		}
		fmt.Fprintf(b, "c%11d %s\n", i, string(r[:n]))
	}
	for i, c := range row.col {
	Windows:
		for j, w := range c.w {
			wincommit(w, &w.tag)
			t := &w.body
			/* windows owned by others get special treatment */
			if w.nopen[QWevent] > 0 {
				if w.dumpstr == "" {
					continue
				}
			}
			/* zeroxes of external windows are tossed */
			if len(t.file.text) > 1 {
				for n = 0; n < len(t.file.text); n++ {
					w1 := t.file.text[n].w
					if w == w1 {
						continue
					}
					if w1.nopen[QWevent] != 0 {
						continue Windows
					}
				}
			}
			fontname := ""
			if t.reffont.f != font {
				fontname = t.reffont.f.Name
			}
			var a string
			if len(t.file.name) != 0 {
				a = string(t.file.name)
			}
			var dumped bool
			if t.file.dumpid != 0 {
				dumped = false
				fmt.Fprintf(b, "x%11d %11d %11d %11d %11.7f %s\n", i, t.file.dumpid, w.body.q0, w.body.q1, 100.0*float64(w.r.Min.Y-c.r.Min.Y)/float64(c.r.Dy()), fontname)
			} else if w.dumpstr != "" {
				dumped = false
				fmt.Fprintf(b, "e%11d %11d %11d %11d %11.7f %s\n", i, t.file.dumpid, 0, 0, 100.0*float64(w.r.Min.Y-c.r.Min.Y)/float64(c.r.Dy()), fontname)
			} else if (!w.dirty && !exists(a)) || w.isdir {
				dumped = false
				t.file.dumpid = w.id
				fmt.Fprintf(b, "f%11d %11d %11d %11d %11.7f %s\n", i, w.id, w.body.q0, w.body.q1, 100.0*float64(w.r.Min.Y-c.r.Min.Y)/float64(c.r.Dy()), fontname)
			} else {
				dumped = true
				t.file.dumpid = w.id
				fmt.Fprintf(b, "F%11d %11d %11d %11d %11.7f %11d %s\n", i, j, w.body.q0, w.body.q1, 100.0*float64(w.r.Min.Y-c.r.Min.Y)/float64(c.r.Dy()), w.body.Len(), fontname)
			}
			b.WriteString(winctlprint(w, false))
			m = util.Min(bufs.RuneLen, w.tag.Len())
			w.tag.file.b.Read(0, r[:m])
			n = 0
			for n < m {
				start := n
				for n < m && r[n] != '\n' {
					n++
				}
				fmt.Fprintf(b, "%s", string(r[start:n]))
				if n < m {
					b.WriteByte(0xff) // \n in tag becomes 0xff byte (invalid UTF)
					n++
				}
			}
			fmt.Fprintf(b, "\n")
			if dumped {
				q0 := 0
				q1 := t.Len()
				for q0 < q1 {
					n = q1 - q0
					if n > bufs.Len/utf8.UTFMax {
						n = bufs.Len / utf8.UTFMax
					}
					t.file.b.Read(q0, r[:n])
					fmt.Fprintf(b, "%s", string(r[:n]))
					q0 += n
				}
			}
			if w.dumpstr != "" {
				if w.dumpdir != "" {
					fmt.Fprintf(b, "%s\n%s\n", w.dumpdir, w.dumpstr)
				} else {
					fmt.Fprintf(b, "\n%s\n", w.dumpstr)
				}
			}
		}
	}
	b.Flush() // TODO(rsc): err check
	f.Close() // TODO(rsc): err check
	bufs.FreeRunes(r)
}

func exists(file string) bool {
	_, err := os.Stat(file)
	return err == nil
}

func rdline(b *bufio.Reader, linep *int) (string, error) {
	l, err := b.ReadString('\n')
	if err == nil {
		(*linep)++
	}
	return l, err
}

/*
 * Get font names from load file so we don't load fonts we won't use
 */
func rowloadfonts(file string) {
	f, err := os.Open(file)
	if err != nil {
		return
	}
	defer f.Close()
	b := bufio.NewReader(f)
	/* current directory */
	_, err = b.ReadString('\n')
	if err != nil {
		return
	}
	/* global fonts */
	for i := 0; i < 2; i++ {
		l, err := b.ReadString('\n')
		if err != nil {
			return
		}
		l = l[:len(l)-1]
		if l != "" && l != fontnames[i] {
			fontnames[i] = l
		}
	}
}

func rowload(row *Row, file *string, initing bool) bool {
	if file == nil {
		if home == "" {
			alog.Printf("can't find file for load: $home not defined\n")
			return false
		}
		s := fmt.Sprintf("%s/acme.dump", home)
		file = &s
	}
	f, err := os.Open(*file)
	if err != nil {
		alog.Printf("can't open load file %s: %v\n", *file, err)
		return false
	}
	defer f.Close()
	b := bufio.NewReader(f)

	/* current directory */
	line := 0
	bad := func() bool {
		alog.Printf("bad load file %s:%d\n", *file, line)
		return false
	}
	l, err := rdline(b, &line)
	if err != nil {
		return bad()
	}
	l = l[:len(l)-1]
	if err := os.Chdir(l); err != nil {
		alog.Printf("can't chdir %s\n", l)
		return bad()
	}

	/* global fonts */
	var i int
	for i = 0; i < 2; i++ {
		l, err := rdline(b, &line)
		if err != nil {
			return bad()
		}
		l = l[:len(l)-1]
		if l != "" && l != fontnames[i] {
			rfget(i != 0, true, i == 0 && initing, l)
		}
	}
	if initing && len(row.col) == 0 {
		rowinit(row, display.ScreenImage.Clipr)
	}
	l, err = rdline(b, &line)
	if err != nil {
		return bad()
	}
	j := len(l) / 12
	if j <= 0 || j > 10 {
		return bad()
	}
	var percent float64
	for i = 0; i < j; i++ {
		percent = atof(l[i*12 : (i+1)*12])
		if percent < 0 || percent >= 100 {
			return bad()
		}
		x := row.r.Min.X + int(percent*float64(row.r.Dx())/100+0.5)
		if i < len(row.col) {
			if i == 0 {
				continue
			}
			c1 := row.col[i-1]
			c2 := row.col[i]
			r1 := c1.r
			r2 := c2.r
			if x < Border() {
				x = Border()
			}
			r1.Max.X = x - Border()
			r2.Min.X = x
			if r1.Dx() < 50 || r2.Dx() < 50 {
				continue
			}
			display.ScreenImage.Draw(draw.Rpt(r1.Min, r2.Max), display.White, nil, draw.ZP)
			colresize(c1, r1)
			colresize(c2, r2)
			r2.Min.X = x - Border()
			r2.Max.X = x
			display.ScreenImage.Draw(r2, display.Black, nil, draw.ZP)
		}
		if i >= len(row.col) {
			rowadd(row, nil, x)
		}
	}
	var n int
	var ns int
	var r []rune
	hdrdone := false
	for {
		l, err = rdline(b, &line)
		if err != nil {
			break
		}
		if !hdrdone {
			switch l[0] {
			case 'c':
				l = l[:len(l)-1]
				i = atoi(l[1:12])
				r = []rune(l[1*12:])
				ns = -1
				for n = 0; n < len(r); n++ {
					if r[n] == '/' {
						ns = n
					}
					if r[n] == ' ' {
						break
					}
				}
				textdelete(&row.col[i].tag, 0, row.col[i].tag.Len(), true)
				textinsert(&row.col[i].tag, 0, r[n+1:], true)
				continue
			case 'w':
				l = l[:len(l)-1]
				r = []rune(l[2:])
				ns = -1
				for n = 0; n < len(r); n++ {
					if r[n] == '/' {
						ns = n
					}
					if r[n] == ' ' {
						break
					}
				}
				textdelete(&row.tag, 0, row.tag.Len(), true)
				textinsert(&row.tag, 0, r, true)
				continue
			}
			hdrdone = true
		}
		dumpid := 0
		var fontname string
		var ndumped int
		switch l[0] {
		case 'e':
			if len(l) < 1+5*12+1 {
				return bad()
			}
			l, err = rdline(b, &line) /* ctl line; ignored */
			if err != nil {
				return bad()
			}
			l, err = rdline(b, &line) /* directory */
			if err != nil {
				return bad()
			}
			l = l[:len(l)-1]
			if len(l) == 0 {
				if home == "" {
					r = []rune("./")
				} else {
					r = []rune(home + "/")
				}
			} else {
				r = []rune(l)
			}
			l, err = rdline(b, &line) /* command */
			if err != nil {
				return bad()
			}
			run(nil, l, r, true, nil, nil, false)
			continue
		case 'f':
			if len(l) < 1+5*12+1 {
				return bad()
			}
			l = l[:len(l)-1]
			fontname = l[1+5*12:]
			ndumped = -1
		case 'F':
			if len(l) < 1+6*12+1 {
				return bad()
			}
			l = l[:len(l)-1]
			fontname = l[1+6*12:]
			ndumped = atoi(l[1+5*12+1:])
		case 'x':
			if len(l) < 1+5*12+1 {
				return bad()
			}
			l = l[:len(l)-1]
			fontname = l[1+5*12:]
			ndumped = -1
			dumpid = atoi(l[1+1*12:])
		default:
			return bad()
		}
		var fontr []rune
		if fontname != "" {
			fontr = []rune(fontname)
		}
		i = atoi(l[1+0*12:])
		j = atoi(l[1+1*12:])
		q0 := atoi(l[1+2*12:])
		q1 := atoi(l[1+3*12:])
		percent = atof(l[1+4*12:])
		if i < 0 || i > 10 {
			return bad()
		}
		if i > len(row.col) {
			i = len(row.col)
		}
		c := row.col[i]
		y := c.r.Min.Y + int((percent*float64(c.r.Dy()))/100+0.5)
		if y < c.r.Min.Y || y >= c.r.Max.Y {
			y = -1
		}
		var w *Window
		if dumpid == 0 {
			w = coladd(c, nil, nil, y)
		} else {
			w = coladd(c, nil, lookid(dumpid, true), y)
		}
		if w == nil {
			continue
		}
		w.dumpid = j
		l, err = rdline(b, &line)
		if err != nil {
			return bad()
		}
		l = l[:len(l)-1]
		/* convert 0xff in multiline tag back to \n */
		lb := []byte(l)
		for i = 0; i < len(lb); i++ {
			if lb[i] == 0xff {
				lb[i] = '\n'
			}
		}
		l = string(lb)
		r = []rune(l[5*12:])
		ns = -1
		for n = 0; n < len(r); n++ {
			if r[n] == '/' {
				ns = n
			}
			if r[n] == ' ' {
				break
			}
		}
		if dumpid == 0 {
			winsetname(w, r[:n])
		}
		for ; n < len(r); n++ {
			if r[n] == '|' {
				break
			}
		}
		wincleartag(w)
		textinsert(&w.tag, w.tag.Len(), r[n+1:], true)
		if ndumped >= 0 {
			/* simplest thing is to put it in a file and load that */
			f, err := ioutil.TempFile("", fmt.Sprintf("acme.%d.*", os.Getpid()))
			if err != nil {
				alog.Printf("can't create temp file: %v\n", err)
				return bad()
			}
			defer f.Close()
			bout := bufio.NewWriter(f)
			for n = 0; n < ndumped; n++ {
				ch, _, err := b.ReadRune()
				if err != nil {
					return bad()
				}
				bout.WriteRune(ch)
			}
			if err := bout.Flush(); err != nil {
				return bad()
			}
			tmp := f.Name()
			if err := f.Close(); err != nil {
				return bad()
			}
			textload(&w.body, 0, tmp, true)
			os.Remove(tmp)
			w.body.file.mod = true
			for n = 0; n < len(w.body.file.text); n++ {
				w.body.file.text[n].w.dirty = true
			}
			winsettag(w)
		} else if dumpid == 0 && r[ns+1] != '+' && r[ns+1] != '-' {
			get(&w.body, nil, nil, false, XXX, nil)
		}
		if fontr != nil {
			fmt.Fprintf(os.Stderr, "FONTR %q\n", fontr)
			fontx(&w.body, nil, nil, false, false, fontr)
		}
		if q0 > w.body.Len() || q1 > w.body.Len() || q0 > q1 {
			q1 = 0
			q0 = q1
		}
		textshow(&w.body, q0, q1, true)
		w.maxlines = util.Min(w.body.fr.NumLines, util.Max(w.maxlines, w.body.fr.MaxLines))
		xfidlog(w, "new")
	}
	return true
}

func allwindows(f func(*Window, interface{}), arg interface{}) {
	for _, c := range row.col {
		for _, w := range c.w {
			f(w, arg)
		}
	}
}

func atoi(s string) int {
	for s != "" && s[0] == ' ' {
		s = s[1:]
	}
	v := 0
	for i := 0; i < len(s) && '0' <= s[i] && s[i] <= '9'; i++ {
		v = v*10 + int(s[i]-'0')
	}
	return v
}

func atof(s string) float64 {
	for s != "" && s[0] == ' ' {
		s = s[1:]
	}
	i := 0
	for i < len(s) && ('0' <= s[i] && s[i] <= '9' || s[i] == '.') {
		i++
	}
	f, err := strconv.ParseFloat(s[:i], 64)
	if err != nil {
		f = 0
	}
	return f
}
