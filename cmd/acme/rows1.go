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

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

func rowdragcol(row *wind.Row, c *wind.Column, _0 int) {
	clearmouse()
	adraw.Display.SwitchCursor2(&adraw.BoxCursor, &adraw.BoxCursor2)
	b := mouse.Buttons
	op := mouse.Point
	for mouse.Buttons == b {
		mousectl.Read()
	}
	adraw.Display.SwitchCursor(nil)
	if mouse.Buttons != 0 {
		for mouse.Buttons != 0 {
			mousectl.Read()
		}
		return
	}

	wind.Rowdragcol1(row, c, op, mouse.Point)
	clearmouse()
	colmousebut(c)
}

func rowtype(row *wind.Row, r rune, p draw.Point) *wind.Text {
	if r == 0 {
		r = utf8.RuneError
	}

	clearmouse()
	row.Lk.Lock()
	var t *wind.Text
	if bartflag {
		t = wind.Barttext
	} else {
		t = wind.Rowwhich(row, p)
	}
	if t != nil && (t.What != wind.Tag || !p.In(t.ScrollR)) {
		w := t.W
		if w == nil {
			texttype(t, r)
		} else {
			wind.Winlock(w, 'K')
			wintype(w, t, r)
			// Expand tag if necessary
			if t.What == wind.Tag {
				t.W.Tagsafe = false
				if r == '\n' {
					t.W.Tagexpand = true
				}
				winresizeAndMouse(w, w.R, true, true)
			}
			wind.Winunlock(w)
		}
	}
	row.Lk.Unlock()
	return t
}

func rowdump(row *wind.Row, file *string) {
	if len(row.Col) == 0 {
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
	fmt.Fprintf(b, "%s\n", adraw.FontNames[0])
	fmt.Fprintf(b, "%s\n", adraw.FontNames[1])
	var i int
	var c *wind.Column
	for i = 0; i < len(row.Col); i++ {
		c = row.Col[i]
		fmt.Fprintf(b, "%11.7f", 100.0*float64(c.R.Min.X-row.R.Min.X)/float64(row.R.Dx()))
		if i == len(row.Col)-1 {
			b.WriteByte('\n')
		} else {
			b.WriteByte(' ')
		}
	}
	dumpid := make(map[*wind.File]int)
	m := util.Min(bufs.RuneLen, row.Tag.Len())
	row.Tag.File.Read(0, r[:m])
	n := 0
	for n < m && r[n] != '\n' {
		n++
	}
	fmt.Fprintf(b, "w %s\n", string(r[:n]))
	for i = 0; i < len(row.Col); i++ {
		c = row.Col[i]
		m = util.Min(bufs.RuneLen, c.Tag.Len())
		c.Tag.File.Read(0, r[:m])
		n = 0
		for n < m && r[n] != '\n' {
			n++
		}
		fmt.Fprintf(b, "c%11d %s\n", i, string(r[:n]))
	}
	for i, c := range row.Col {
	Windows:
		for j, w := range c.W {
			wind.Wincommit(w, &w.Tag)
			t := &w.Body
			// windows owned by others get special treatment
			if w.External {
				if w.Dumpstr == "" {
					continue
				}
			}
			// zeroxes of external windows are tossed
			if len(t.File.Text) > 1 {
				for n = 0; n < len(t.File.Text); n++ {
					w1 := t.File.Text[n].W
					if w == w1 {
						continue
					}
					if w1.External {
						continue Windows
					}
				}
			}
			fontname := ""
			if t.Reffont.F != adraw.Font {
				fontname = t.Reffont.F.Name
			}
			var a string
			if len(t.File.Name()) != 0 {
				a = string(t.File.Name())
			}
			var dumped bool
			if dumpid[t.File] != 0 {
				dumped = false
				fmt.Fprintf(b, "x%11d %11d %11d %11d %11.7f %s\n", i, dumpid[t.File], w.Body.Q0, w.Body.Q1, 100.0*float64(w.R.Min.Y-c.R.Min.Y)/float64(c.R.Dy()), fontname)
			} else if w.Dumpstr != "" {
				dumped = false
				fmt.Fprintf(b, "e%11d %11d %11d %11d %11.7f %s\n", i, 0, 0, 0, 100.0*float64(w.R.Min.Y-c.R.Min.Y)/float64(c.R.Dy()), fontname)
			} else if (!w.Dirty && !exists(a)) || w.IsDir {
				dumped = false
				dumpid[t.File] = w.ID
				fmt.Fprintf(b, "f%11d %11d %11d %11d %11.7f %s\n", i, w.ID, w.Body.Q0, w.Body.Q1, 100.0*float64(w.R.Min.Y-c.R.Min.Y)/float64(c.R.Dy()), fontname)
			} else {
				dumped = true
				dumpid[t.File] = w.ID
				fmt.Fprintf(b, "F%11d %11d %11d %11d %11.7f %11d %s\n", i, j, w.Body.Q0, w.Body.Q1, 100.0*float64(w.R.Min.Y-c.R.Min.Y)/float64(c.R.Dy()), w.Body.Len(), fontname)
			}
			b.WriteString(wind.Winctlprint(w, false))
			m = util.Min(bufs.RuneLen, w.Tag.Len())
			w.Tag.File.Read(0, r[:m])
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
					t.File.Read(q0, r[:n])
					fmt.Fprintf(b, "%s", string(r[:n]))
					q0 += n
				}
			}
			if w.Dumpstr != "" {
				if w.Dumpdir != "" {
					fmt.Fprintf(b, "%s\n%s\n", w.Dumpdir, w.Dumpstr)
				} else {
					fmt.Fprintf(b, "\n%s\n", w.Dumpstr)
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
	// current directory
	_, err = b.ReadString('\n')
	if err != nil {
		return
	}
	// global fonts
	for i := 0; i < 2; i++ {
		l, err := b.ReadString('\n')
		if err != nil {
			return
		}
		l = l[:len(l)-1]
		if l != "" && l != adraw.FontNames[i] {
			adraw.FontNames[i] = l
		}
	}
}

func rowload(row *wind.Row, file *string, initing bool) bool {
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

	// current directory
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

	// global fonts
	var i int
	for i = 0; i < 2; i++ {
		l, err := rdline(b, &line)
		if err != nil {
			return bad()
		}
		l = l[:len(l)-1]
		if l != "" && l != adraw.FontNames[i] {
			adraw.FindFont(i != 0, true, i == 0 && initing, l)
		}
	}
	if initing && len(row.Col) == 0 {
		wind.RowInit(row, adraw.Display.ScreenImage.Clipr)
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
		x := row.R.Min.X + int(percent*float64(row.R.Dx())/100+0.5)
		if i < len(row.Col) {
			if i == 0 {
				continue
			}
			c1 := row.Col[i-1]
			c2 := row.Col[i]
			r1 := c1.R
			r2 := c2.R
			if x < adraw.Border() {
				x = adraw.Border()
			}
			r1.Max.X = x - adraw.Border()
			r2.Min.X = x
			if r1.Dx() < 50 || r2.Dx() < 50 {
				continue
			}
			adraw.Display.ScreenImage.Draw(draw.Rpt(r1.Min, r2.Max), adraw.Display.White, nil, draw.ZP)
			wind.Colresize(c1, r1)
			wind.Colresize(c2, r2)
			r2.Min.X = x - adraw.Border()
			r2.Max.X = x
			adraw.Display.ScreenImage.Draw(r2, adraw.Display.Black, nil, draw.ZP)
		}
		if i >= len(row.Col) {
			wind.RowAdd(row, nil, x)
		}
	}
	var n int
	var ns int
	var r []rune
	hdrdone := false
	byDumpID := make(map[int]*wind.Window)
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
				wind.Textdelete(&row.Col[i].Tag, 0, row.Col[i].Tag.Len(), true)
				wind.Textinsert(&row.Col[i].Tag, 0, r[n+1:], true)
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
				wind.Textdelete(&row.Tag, 0, row.Tag.Len(), true)
				wind.Textinsert(&row.Tag, 0, r, true)
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
			l, err = rdline(b, &line) // ctl line; ignored
			if err != nil {
				return bad()
			}
			l, err = rdline(b, &line) // directory
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
			l, err = rdline(b, &line) // command
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
		if i > len(row.Col) {
			i = len(row.Col)
		}
		c := row.Col[i]
		y := c.R.Min.Y + int((percent*float64(c.R.Dy()))/100+0.5)
		if y < c.R.Min.Y || y >= c.R.Max.Y {
			y = -1
		}
		var w *wind.Window
		if dumpid == 0 {
			w = wind.Coladd(c, nil, nil, y)
		} else {
			w = wind.Coladd(c, nil, byDumpID[dumpid], y)
		}
		if w == nil {
			continue
		}
		byDumpID[j] = w
		l, err = rdline(b, &line)
		if err != nil {
			return bad()
		}
		l = l[:len(l)-1]
		// convert 0xff in multiline tag back to \n
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
			wind.Winsetname(w, r[:n])
		}
		for ; n < len(r); n++ {
			if r[n] == '|' {
				break
			}
		}
		wind.Wincleartatg(w)
		wind.Textinsert(&w.Tag, w.Tag.Len(), r[n+1:], true)
		if ndumped >= 0 {
			// simplest thing is to put it in a file and load that
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
			textload(&w.Body, 0, tmp, true)
			os.Remove(tmp)
			w.Body.File.SetMod(true)
			for n = 0; n < len(w.Body.File.Text); n++ {
				w.Body.File.Text[n].W.Dirty = true
			}
			wind.Winsettag(w)
		} else if dumpid == 0 && r[ns+1] != '+' && r[ns+1] != '-' {
			get(&w.Body, nil, nil, false, XXX, nil)
		}
		if fontr != nil {
			fmt.Fprintf(os.Stderr, "FONTR %q\n", fontr)
			fontx(&w.Body, nil, nil, false, false, fontr)
		}
		if q0 > w.Body.Len() || q1 > w.Body.Len() || q0 > q1 {
			q1 = 0
			q0 = q1
		}
		wind.Textshow(&w.Body, q0, q1, true)
		w.Maxlines = util.Min(w.Body.Fr.NumLines, util.Max(w.Maxlines, w.Body.Fr.MaxLines))
		xfidlog(w, "new")
	}
	return true
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
