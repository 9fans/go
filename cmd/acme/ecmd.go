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
// #include "edit.h"
// #include "fns.h"

package main

import (
	"fmt"
	"os"
	"strings"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/regx"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
)

var Glooping int
var nest int
var Enoname = "no file name given"

var addr Address
var menu *File

// extern var curtext *Text
var collection []rune

func clearcollection() {
	collection = nil
}

func resetxec() {
	nest = 0
	Glooping = nest
	clearcollection()
}

func mkaddr(a *Address, f *File) {
	a.r.Pos = f.curtext.q0
	a.r.End = f.curtext.q1
	a.f = f
}

func cmdexec(t *Text, cp *Cmd) bool {
	var w *Window
	if t == nil {
		w = nil
	} else {
		w = t.w
	}
	if w == nil && (cp.addr == nil || cp.addr.typ != '"') && !strings.ContainsRune("bBnqUXY!", cp.cmdc) && (!(cp.cmdc == 'D') || cp.u.text == nil) {
		editerror("no current window")
	}
	i := cmdlookup(cp.cmdc) // will be -1 for '{'
	var f *File
	if t != nil && t.w != nil {
		t = &t.w.body
		f = t.file
		f.curtext = t
	}
	var dot Address
	if i >= 0 && cmdtab[i].defaddr != aNo {
		ap := cp.addr
		if ap == nil && cp.cmdc != '\n' {
			ap = newaddr()
			cp.addr = ap
			ap.typ = '.'
			if cmdtab[i].defaddr == aAll {
				ap.typ = '*'
			}
		} else if ap != nil && ap.typ == '"' && ap.next == nil && cp.cmdc != '\n' {
			ap.next = newaddr()
			ap.next.typ = '.'
			if cmdtab[i].defaddr == aAll {
				ap.next.typ = '*'
			}
		}
		if cp.addr != nil { // may be false for '\n' (only)
			none := Address{}
			if f != nil {
				mkaddr(&dot, f)
				addr = cmdaddress(ap, dot, 0)
			} else { // a "
				addr = cmdaddress(ap, none, 0)
			}
			f = addr.f
			t = f.curtext
		}
	}
	switch cp.cmdc {
	case '{':
		mkaddr(&dot, f)
		if cp.addr != nil {
			dot = cmdaddress(cp.addr, dot, 0)
		}
		for cp = cp.u.cmd; cp != nil; cp = cp.next {
			if dot.r.End > t.Len() {
				editerror("dot extends past end of buffer during { command")
			}
			t.q0 = dot.r.Pos
			t.q1 = dot.r.End
			cmdexec(t, cp)
		}
	default:
		if i < 0 {
			editerror("unknown command %c in cmdexec", cp.cmdc)
		}
		return cmdtab[i].fn(t, cp)
	}
	return true
}

func edittext(w *Window, q int, r []rune) error {
	f := w.body.file
	switch editing {
	case Inactive:
		return fmt.Errorf("permission denied")
	case Inserting:
		eloginsert(f, q, r)
		return nil
	case Collecting:
		collection = append(collection, r...)
		return nil
	default:
		return fmt.Errorf("unknown state in edittext")
	}
}

// string is known to be NUL-terminated
func filelist(t *Text, r []rune) []rune {
	if len(r) == 0 {
		return nil
	}
	r = runes.SkipBlank(r)
	if len(r) == 0 || r[0] != '<' {
		return runes.Clone(r)
	}
	// use < command to collect text
	clearcollection()
	runpipe(t, '<', r[1:], Collecting)
	return collection
}

func a_cmd(t *Text, cp *Cmd) bool {
	return fappend(t.file, cp, addr.r.End)
}

func b_cmd(t *Text, cp *Cmd) bool {
	f := tofile(cp.u.text)
	if nest == 0 {
		pfilename(f)
	}
	curtext = f.curtext
	return true
}

func B_cmd(t *Text, cp *Cmd) bool {
	list := filelist(t, cp.u.text.r)
	if list == nil {
		editerror(Enoname)
	}
	r := runes.SkipBlank(list)
	if len(r) == 0 {
		new_(t, t, nil, false, false, r)
	} else {
		for len(r) > 0 {
			s := runes.SkipNonBlank(r)
			new_(t, t, nil, false, false, r[:len(r)-len(s)])
			r = runes.SkipBlank(s)
		}
	}
	clearcollection()
	return true
}

func c_cmd(t *Text, cp *Cmd) bool {
	elogreplace(t.file, addr.r.Pos, addr.r.End, cp.u.text.r)
	t.q0 = addr.r.Pos
	t.q1 = addr.r.End
	return true
}

func d_cmd(t *Text, cp *Cmd) bool {
	if addr.r.End > addr.r.Pos {
		elogdelete(t.file, addr.r.Pos, addr.r.End)
	}
	t.q0 = addr.r.Pos
	t.q1 = addr.r.Pos
	return true
}

func D1(t *Text) {
	if len(t.w.body.file.text) > 1 || winclean(t.w, false) {
		colclose(t.col, t.w, true)
	}
}

func D_cmd(t *Text, cp *Cmd) bool {
	list := filelist(t, cp.u.text.r)
	if list == nil {
		D1(t)
		return true
	}
	dir := dirname(t, nil)
	r := runes.SkipBlank(list)
	for {
		s := runes.SkipNonBlank(r)
		r = r[:len(r)-len(s)]
		var rs []rune
		// first time through, could be empty string, meaning delete file empty name
		if len(r) == 0 || r[0] == '/' || len(dir) == 0 {
			rs = runes.Clone(r)
		} else {
			n := make([]rune, len(dir)+1+len(r))
			copy(n, dir)
			n[len(dir)] = '/'
			copy(n[len(dir)+1:], r)
			rs = runes.CleanPath(n)
		}
		w := lookfile(rs)
		if w == nil {
			editerror(fmt.Sprintf("no such file %s", string(rs)))
		}
		D1(&w.body)
		r = runes.SkipBlank(s)
		if len(r) == 0 {
			break
		}
	}
	clearcollection()
	return true
}

func readloader(f *File) func(pos int, data []rune) int {
	return func(pos int, data []rune) int {
		if len(data) > 0 {
			eloginsert(f, pos, data)
		}
		return 0
	}
}

func e_cmd(t *Text, cp *Cmd) bool {
	f := t.file
	q0 := addr.r.Pos
	q1 := addr.r.End
	if cp.cmdc == 'e' {
		if !winclean(t.w, true) {
			editerror("") // winclean generated message already
		}
		q0 = 0
		q1 = f.Len()
	}
	allreplaced := (q0 == 0 && q1 == f.Len())
	name := cmdname(f, cp.u.text, cp.cmdc == 'e')
	if name == nil {
		editerror(Enoname)
	}
	samename := runes.Equal(name, t.file.Name())
	s := string(name)
	fd, err := os.Open(s)
	if err != nil {
		editerror(fmt.Sprintf("can't open %s: %v", s, err))
	}
	defer fd.Close()
	if info, err := fd.Stat(); err == nil && info.IsDir() {
		editerror(fmt.Sprintf("%s is a directory", s))
	}
	elogdelete(f, q0, q1)
	nulls := false
	loadfile(fd, q1, &nulls, readloader(f), nil)
	if nulls {
		alog.Printf("%s: NUL bytes elided\n", s)
	} else if allreplaced && samename {
		elogfind(f).editclean = true
	}
	return true
}

func f_cmd(t *Text, cp *Cmd) bool {
	var str *String
	if cp.u.text == nil {
		str = new(String) // empty
	} else {
		str = cp.u.text
	}
	cmdname(t.file, str, true)
	pfilename(t.file)
	return true
}

func g_cmd(t *Text, cp *Cmd) bool {
	if t.file != addr.f {
		alog.Printf("internal error: g_cmd f!=addr.f\n")
		return false
	}
	if !regx.Compile(cp.re.r) {
		editerror("bad regexp in g command")
	}
	if regx.Match(t, nil, addr.r.Pos, addr.r.End, &regx.Sel) != (cp.cmdc == 'v') {
		t.q0 = addr.r.Pos
		t.q1 = addr.r.End
		return cmdexec(t, cp.u.cmd)
	}
	return true
}

func i_cmd(t *Text, cp *Cmd) bool {
	return fappend(t.file, cp, addr.r.Pos)
}

func fbufalloc() []rune {
	return make([]rune, bufs.Len/runes.RuneSize)
}

func fbuffree(b []rune) {}

func fcopy(f *File, addr2 Address) {
	buf := bufs.AllocRunes()
	var ni int
	for p := addr.r.Pos; p < addr.r.End; p += ni {
		ni = addr.r.End - p
		if ni > bufs.RuneLen {
			ni = bufs.RuneLen
		}
		f.Read(p, buf[:ni])
		eloginsert(addr2.f, addr2.r.End, buf[:ni])
	}
	bufs.FreeRunes(buf)
}

func move(f *File, addr2 Address) {
	if addr.f != addr2.f || addr.r.End <= addr2.r.Pos {
		elogdelete(f, addr.r.Pos, addr.r.End)
		fcopy(f, addr2)
	} else if addr.r.Pos >= addr2.r.End {
		fcopy(f, addr2)
		elogdelete(f, addr.r.Pos, addr.r.End)
	} else if addr.r.Pos == addr2.r.Pos && addr.r.End == addr2.r.End { // move to self; no-op
	} else {
		editerror("move overlaps itself")
	}
}

func m_cmd(t *Text, cp *Cmd) bool {
	var dot Address
	mkaddr(&dot, t.file)
	addr2 := cmdaddress(cp.u.mtaddr, dot, 0)
	if cp.cmdc == 'm' {
		move(t.file, addr2)
	} else {
		fcopy(t.file, addr2)
	}
	return true
}

func p_cmd(t *Text, cp *Cmd) bool {
	return pdisplay(t.file)
}

func s_cmd(t *Text, cp *Cmd) bool {
	n := cp.num
	op := -1
	if !regx.Compile(cp.re.r) {
		editerror("bad regexp in s command")
	}
	var rp []regx.Ranges
	delta := 0
	didsub := false
	for p1 := addr.r.Pos; p1 <= addr.r.End && regx.Match(t, nil, p1, addr.r.End, &regx.Sel); {
		if regx.Sel.R[0].Pos == regx.Sel.R[0].End { // empty match?
			if regx.Sel.R[0].Pos == op {
				p1++
				continue
			}
			p1 = regx.Sel.R[0].End + 1
		} else {
			p1 = regx.Sel.R[0].End
		}
		op = regx.Sel.R[0].End
		n--
		if n > 0 {
			continue
		}
		rp = append(rp, regx.Sel)
	}
	rbuf := bufs.AllocRunes()
	buf := allocstring(0)
	var err string
	for m := 0; m < len(rp); m++ {
		buf.r = buf.r[:0]
		regx.Sel = rp[m]
		for i := 0; i < len(cp.u.text.r); i++ {
			c := cp.u.text.r[i]
			if c == '\\' && i < len(cp.u.text.r)-1 {
				i++
				c = cp.u.text.r[i]
				if '1' <= c && c <= '9' {
					j := c - '0'
					if regx.Sel.R[j].End-regx.Sel.R[j].Pos > bufs.RuneLen {
						err = "replacement string too long"
						goto Err
					}
					t.file.Read(regx.Sel.R[j].Pos, rbuf[:regx.Sel.R[j].End-regx.Sel.R[j].Pos])
					for k := 0; k < regx.Sel.R[j].End-regx.Sel.R[j].Pos; k++ {
						Straddc(buf, rbuf[k])
					}
				} else {
					Straddc(buf, c)
				}
			} else if c != '&' {
				Straddc(buf, c)
			} else {
				if regx.Sel.R[0].End-regx.Sel.R[0].Pos > bufs.RuneLen {
					err = "right hand side too long in substitution"
					goto Err
				}
				t.file.Read(regx.Sel.R[0].Pos, rbuf[:regx.Sel.R[0].End-regx.Sel.R[0].Pos])
				for k := 0; k < regx.Sel.R[0].End-regx.Sel.R[0].Pos; k++ {
					Straddc(buf, rbuf[k])
				}
			}
		}
		elogreplace(t.file, regx.Sel.R[0].Pos, regx.Sel.R[0].End, buf.r)
		delta -= regx.Sel.R[0].End - regx.Sel.R[0].Pos
		delta += len(buf.r)
		didsub = true
		if !cp.flag {
			break
		}
	}
	freestring(buf)
	bufs.FreeRunes(rbuf)
	if !didsub && nest == 0 {
		editerror("no substitution")
	}
	t.q0 = addr.r.Pos
	t.q1 = addr.r.End
	return true

Err:
	freestring(buf)
	bufs.FreeRunes(rbuf)
	editerror(err)
	return false
}

func u_cmd(t *Text, cp *Cmd) bool {
	n := cp.num
	flag := true
	if n < 0 {
		n = -n
		flag = false
	}
	oseq := -1
	for {
		tmp3 := n
		n--
		if !(tmp3 > 0) || !(t.file.Seq() != oseq) {
			break
		}
		oseq = t.file.Seq()
		undo(t, nil, nil, flag, XXX, nil)
	}
	return true
}

func w_cmd(t *Text, cp *Cmd) bool {
	f := t.file
	if f.Seq() == file.Seq {
		editerror("can't write file with pending modifications")
	}
	r := cmdname(f, cp.u.text, false)
	if r == nil {
		editerror("no name specified for 'w' command")
	}
	putfile(f, addr.r.Pos, addr.r.End, r)
	// r is freed by putfile
	return true
}

func x_cmd(t *Text, cp *Cmd) bool {
	if cp.re != nil {
		looper(t.file, cp, cp.cmdc == 'x')
	} else {
		linelooper(t.file, cp)
	}
	return true
}

func X_cmd(t *Text, cp *Cmd) bool {

	filelooper(t, cp, cp.cmdc == 'X')
	return true
}

func runpipe(t *Text, cmd rune, cr []rune, state int) {
	r := runes.SkipBlank(cr)
	if len(r) == 0 {
		editerror("no command specified for %c", cmd)
	}
	var w *Window
	if state == Inserting {
		w = t.w
		t.q0 = addr.r.Pos
		t.q1 = addr.r.End
		if cmd == '<' || cmd == '|' {
			elogdelete(t.file, t.q0, t.q1)
		}
	}
	s := make([]rune, len(r)+1)
	s[0] = cmd
	copy(s[1:], r)
	var dir []rune
	dir = nil
	if t != nil {
		dir = dirname(t, nil)
	}
	if len(dir) == 1 && dir[0] == '.' { // sigh
		dir = nil
	}
	editing = state
	if t != nil && t.w != nil {
		util.Incref(&t.w.ref) // run will decref
	}
	run(w, string(s), dir, true, nil, nil, true)
	if t != nil && t.w != nil {
		winunlock(t.w)
	}
	row.lk.Unlock()
	<-cedit
	var q *util.QLock
	/*
	 * The editoutlk exists only so that we can tell when
	 * the editout file has been closed.  It can get closed *after*
	 * the process exits because, since the process cannot be
	 * connected directly to editout (no 9P kernel support),
	 * the process is actually connected to a pipe to another
	 * process (arranged via 9pserve) that reads from the pipe
	 * and then writes the data in the pipe to editout using
	 * 9P transactions.  This process might still have a couple
	 * writes left to copy after the original process has exited.
	 */
	if w != nil {
		q = &w.editoutlk
	} else {
		q = &editoutlk
	}
	q.Lock() // wait for file to close
	q.Unlock()
	row.lk.Lock()
	editing = Inactive
	if t != nil && t.w != nil {
		winlock(t.w, 'M')
	}
}

func pipe_cmd(t *Text, cp *Cmd) bool {
	runpipe(t, cp.cmdc, cp.u.text.r, Inserting)
	return true
}

func nlcount(t *Text, q0 int, q1 int, pnr *int) int {
	buf := bufs.AllocRunes()
	nbuf := 0
	nl := 0
	i := nl
	start := q0
	for q0 < q1 {
		if i == nbuf {
			nbuf = q1 - q0
			if nbuf > bufs.RuneLen {
				nbuf = bufs.RuneLen
			}
			t.file.Read(q0, buf[:nbuf])
			i = 0
		}
		if buf[i] == '\n' {
			start = q0 + 1
			nl++
		}
		i++
		q0++
	}
	bufs.FreeRunes(buf)
	if pnr != nil {
		*pnr = q0 - start
	}
	return nl
}

const (
	PosnLine      = 0
	PosnChars     = 1
	PosnLineChars = 2
)

func printposn(t *Text, mode int) {
	if t != nil && t.file != nil && t.file.Name() != nil {
		alog.Printf("%s:", string(t.file.Name()))
	}
	var l1 int
	var l2 int
	var r1 int
	var r2 int

	switch mode {
	case PosnChars:
		alog.Printf("#%d", addr.r.Pos)
		if addr.r.End != addr.r.Pos {
			alog.Printf(",#%d", addr.r.End)
		}
		alog.Printf("\n")
		return

	default:
	case PosnLine:
		l1 = 1 + nlcount(t, 0, addr.r.Pos, nil)
		l2 = l1 + nlcount(t, addr.r.Pos, addr.r.End, nil)
		// check if addr ends with '\n'
		if addr.r.End > 0 && addr.r.End > addr.r.Pos && t.RuneAt(addr.r.End-1) == '\n' {
			l2--
		}
		alog.Printf("%d", l1)
		if l2 != l1 {
			alog.Printf(",%d", l2)
		}
		alog.Printf("\n")
		return

	case PosnLineChars:
		l1 = 1 + nlcount(t, 0, addr.r.Pos, &r1)
		l2 = l1 + nlcount(t, addr.r.Pos, addr.r.End, &r2)
		if l2 == l1 {
			r2 += r1
		}
		alog.Printf("%d+#%d", l1, r1)
		if l2 != l1 {
			alog.Printf(",%d+#%d", l2, r2)
		}
		alog.Printf("\n")
		return
	}
}

func eq_cmd(t *Text, cp *Cmd) bool {
	var mode int
	switch len(cp.u.text.r) {
	case 0:
		mode = PosnLine
	case 1:
		if cp.u.text.r[0] == '#' {
			mode = PosnChars
			break
		}
		if cp.u.text.r[0] == '+' {
			mode = PosnLineChars
			break
		}
		fallthrough
	default:
		editerror("newline expected")
	}
	printposn(t, mode)
	return true
}

func nl_cmd(t *Text, cp *Cmd) bool {
	f := t.file
	if cp.addr == nil {
		var a Address
		// First put it on newline boundaries
		mkaddr(&a, f)
		addr = lineaddr(0, a, -1)
		a = lineaddr(0, a, 1)
		addr.r.End = a.r.End
		if addr.r.Pos == t.q0 && addr.r.End == t.q1 {
			mkaddr(&a, f)
			addr = lineaddr(1, a, 1)
		}
	}
	textshow(t, addr.r.Pos, addr.r.End, true)
	return true
}

func fappend(f *File, cp *Cmd, p int) bool {
	if len(cp.u.text.r) > 0 {
		eloginsert(f, p, cp.u.text.r)
	}
	f.curtext.q0 = p
	f.curtext.q1 = p
	return true
}

func pdisplay(f *File) bool {
	p1 := addr.r.Pos
	p2 := addr.r.End
	if p2 > f.Len() {
		p2 = f.Len()
	}
	buf := bufs.AllocRunes()
	for p1 < p2 {
		np := p2 - p1
		if np > bufs.RuneLen-1 {
			np = bufs.RuneLen - 1
		}
		f.Read(p1, buf[:np])
		alog.Printf("%s", string(buf[:np]))
		p1 += np
	}
	bufs.FreeRunes(buf)
	f.curtext.q0 = addr.r.Pos
	f.curtext.q1 = addr.r.End
	return true
}

func pfilename(f *File) {
	w := f.curtext.w
	// same check for dirty as in settag, but we know ncache==0
	dirty := !w.isdir && !w.isscratch && f.Mod()
	ch := func(s string, b bool) byte {
		if b {
			return s[1]
		}
		return s[0]
	}
	alog.Printf("%c%c%c %s\n", ch(" '", dirty), '+', ch(" .", curtext != nil && curtext.file == f), string(f.Name()))
}

func loopcmd(f *File, cp *Cmd, rp []runes.Range) {
	for i := 0; i < len(rp); i++ {
		f.curtext.q0 = rp[i].Pos
		f.curtext.q1 = rp[i].End
		cmdexec(f.curtext, cp)
	}
}

func looper(f *File, cp *Cmd, xy bool) {
	r := addr.r
	op := r.Pos
	if xy {
		op = -1
	}
	nest++
	if !regx.Compile(cp.re.r) {
		editerror("bad regexp in %c command", cp.cmdc)
	}
	var rp []runes.Range
	for p := r.Pos; p <= r.End; {
		var tr runes.Range
		if !regx.Match(f.curtext, nil, p, r.End, &regx.Sel) { // no match, but y should still run
			if xy || op > r.End {
				break
			}
			tr.Pos = op
			tr.End = r.End
			p = r.End + 1 // exit next loop
		} else {
			if regx.Sel.R[0].Pos == regx.Sel.R[0].End { // empty match?
				if regx.Sel.R[0].Pos == op {
					p++
					continue
				}
				p = regx.Sel.R[0].End + 1
			} else {
				p = regx.Sel.R[0].End
			}
			if xy {
				tr = regx.Sel.R[0]
			} else {
				tr.Pos = op
				tr.End = regx.Sel.R[0].Pos
			}
		}
		op = regx.Sel.R[0].End
		rp = append(rp, tr)
	}
	loopcmd(f, cp.u.cmd, rp)
	nest--
}

func linelooper(f *File, cp *Cmd) {
	nest++
	var rp []runes.Range
	r := addr.r
	var a3 Address
	a3.f = f
	a3.r.End = r.Pos
	a3.r.Pos = a3.r.End
	a := lineaddr(0, a3, 1)
	linesel := a.r
	for p := r.Pos; p < r.End; p = a3.r.End {
		a3.r.Pos = a3.r.End
		if p != r.Pos || linesel.End == p {
			a = lineaddr(1, a3, 1)
			linesel = a.r
		}
		if linesel.Pos >= r.End {
			break
		}
		if linesel.End >= r.End {
			linesel.End = r.End
		}
		if linesel.End > linesel.Pos {
			if linesel.Pos >= a3.r.End && linesel.End > a3.r.End {
				a3.r = linesel
				rp = append(rp, linesel)
				continue
			}
		}
		break
	}
	loopcmd(f, cp.u.cmd, rp)
	nest--
}

type Looper struct {
	cp *Cmd
	XY bool
	w  []*Window
}

var loopstruct Looper // only one; X and Y can't nest

func alllooper(w *Window, v interface{}) {
	lp := v.(*Looper)
	cp := lp.cp
	//	if(w->isscratch || w->isdir)
	//		return;
	t := &w.body
	// only use this window if it's the current window for the file
	if t.file.curtext != t {
		return
	}
	//	if(w->nopen[QWevent] > 0)
	//		return;
	// no auto-execute on files without names
	if cp.re == nil && len(t.file.Name()) == 0 {
		return
	}
	if cp.re == nil || filematch(t.file, cp.re) == lp.XY {
		lp.w = append(lp.w, w)
	}
}

func alllocker(w *Window, v interface{}) {
	if v.(bool) {
		util.Incref(&w.ref)
	} else {
		winclose(w)
	}
}

func filelooper(t *Text, cp *Cmd, XY bool) {
	tmp6 := Glooping
	Glooping++
	if tmp6 != 0 {
		cmd := 'Y'
		if XY {
			cmd = 'X'
		}
		editerror("can't nest %c command", cmd)
	}
	nest++

	loopstruct.cp = cp
	loopstruct.XY = XY
	loopstruct.w = nil
	allwindows(alllooper, &loopstruct)
	/*
	 * add a ref to all windows to keep safe windows accessed by X
	 * that would not otherwise have a ref to hold them up during
	 * the shenanigans.  note this with globalincref so that any
	 * newly created windows start with an extra reference.
	 */
	allwindows(alllocker, true)
	globalincref = 1

	/*
	 * Unlock the window running the X command.
	 * We'll need to lock and unlock each target window in turn.
	 */
	if t != nil && t.w != nil {
		winunlock(t.w)
	}

	for i := 0; i < len(loopstruct.w); i++ {
		targ := &loopstruct.w[i].body
		if targ != nil && targ.w != nil {
			winlock(targ.w, cp.cmdc)
		}
		cmdexec(targ, cp.u.cmd)
		if targ != nil && targ.w != nil {
			winunlock(targ.w)
		}
	}

	if t != nil && t.w != nil {
		winlock(t.w, cp.cmdc)
	}

	allwindows(alllocker, false)
	globalincref = 0
	loopstruct.w = nil

	Glooping--
	nest--
}

func nextmatch(f *File, r *String, p int, sign int) {
	if !regx.Compile(r.r) {
		editerror("bad regexp in command address")
	}
	if sign >= 0 {
		if !regx.Match(f.curtext, nil, p, 0x7FFFFFFF, &regx.Sel) {
			editerror("no match for regexp")
		}
		if regx.Sel.R[0].Pos == regx.Sel.R[0].End && regx.Sel.R[0].Pos == p {
			p++
			if p > f.Len() {
				p = 0
			}
			if !regx.Match(f.curtext, nil, p, 0x7FFFFFFF, &regx.Sel) {
				editerror("address")
			}
		}
	} else {
		if !regx.MatchBackward(f.curtext, p, &regx.Sel) {
			editerror("no match for regexp")
		}
		if regx.Sel.R[0].Pos == regx.Sel.R[0].End && regx.Sel.R[0].End == p {
			p--
			if p < 0 {
				p = f.Len()
			}
			if !regx.MatchBackward(f.curtext, p, &regx.Sel) {
				editerror("address")
			}
		}
	}
}

func cmdaddress(ap *Addr, a Address, sign int) Address {
	f := a.f
	for {
		var a2 Address
		var a1 Address
		switch ap.typ {
		case 'l':
			a = lineaddr(ap.num, a, sign)

		case '#':
			a = charaddr(ap.num, a, sign)

		case '.':
			mkaddr(&a, f)

		case '$':
			a.r.End = f.Len()
			a.r.Pos = a.r.End

		case '\'':
			editerror("can't handle '")
			//			a.r = f->mark;

		case '?':
			sign = -sign
			if sign == 0 {
				sign = -1
			}
			fallthrough
		// fall through
		case '/':
			start := a.r.End
			if sign < 0 {
				start = a.r.Pos
			}
			nextmatch(f, ap.u.re, start, sign)
			a.r = regx.Sel.R[0]

		case '"':
			f = matchfile(ap.u.re)
			mkaddr(&a, f)

		case '*':
			a.r.Pos = 0
			a.r.End = f.Len()
			return a

		case ',',
			';':
			if ap.u.left != nil {
				a1 = cmdaddress(ap.u.left, a, 0)
			} else {
				a1.f = a.f
				a1.r.End = 0
				a1.r.Pos = a1.r.End
			}
			if ap.typ == ';' {
				f = a1.f
				a = a1
				f.curtext.q0 = a1.r.Pos
				f.curtext.q1 = a1.r.End
			}
			if ap.next != nil {
				a2 = cmdaddress(ap.next, a, 0)
			} else {
				a2.f = a.f
				a2.r.End = f.Len()
				a2.r.Pos = a2.r.End
			}
			if a1.f != a2.f {
				editerror("addresses in different files")
			}
			a.f = a1.f
			a.r.Pos = a1.r.Pos
			a.r.End = a2.r.End
			if a.r.End < a.r.Pos {
				editerror("addresses out of order")
			}
			return a

		case '+',
			'-':
			sign = 1
			if ap.typ == '-' {
				sign = -1
			}
			if ap.next == nil || ap.next.typ == '+' || ap.next.typ == '-' {
				a = lineaddr(1, a, sign)
			}
		default:
			util.Fatal("cmdaddress")
			return a
		}
		ap = ap.next
		if ap == nil { // assign =
			break
		}
	}
	return a
}

type Tofile struct {
	f *File
	r *String
}

func alltofile(w *Window, v interface{}) {
	tp := v.(*Tofile)
	if tp.f != nil {
		return
	}
	if w.isscratch || w.isdir {
		return
	}
	t := &w.body
	// only use this window if it's the current window for the file
	if t.file.curtext != t {
		return
	}
	//	if(w->nopen[QWevent] > 0)
	//		return;
	if runes.Equal(tp.r.r, t.file.Name()) {
		tp.f = t.file
	}
}

func tofile(r *String) *File {
	var rr String
	rr.r = runes.SkipBlank(r.r)
	var t Tofile
	t.f = nil
	t.r = &rr
	allwindows(alltofile, &t)
	if t.f == nil {
		editerror("no such file\"%s\"", string(rr.r))
	}
	return t.f
}

func allmatchfile(w *Window, v interface{}) {
	tp := v.(*Tofile)
	if w.isscratch || w.isdir {
		return
	}
	t := &w.body
	// only use this window if it's the current window for the file
	if t.file.curtext != t {
		return
	}
	//	if(w->nopen[QWevent] > 0)
	//		return;
	if filematch(w.body.file, tp.r) {
		if tp.f != nil {
			editerror("too many files match \"%s\"", string(tp.r.r))
		}
		tp.f = w.body.file
	}
}

func matchfile(r *String) *File {
	var tf Tofile
	tf.f = nil
	tf.r = r
	allwindows(allmatchfile, &tf)

	if tf.f == nil {
		editerror("no file matches \"%s\"", string(r.r))
	}
	return tf.f
}

func filematch(f *File, r *String) bool {
	// compile expr first so if we get an error, we haven't allocated anything
	if !regx.Compile(r.r) {
		editerror("bad regexp in file match")
	}
	w := f.curtext.w
	// same check for dirty as in settag, but we know ncache==0
	dirty := !w.isdir && !w.isscratch && f.Mod()
	ch := func(s string, b bool) byte {
		if b {
			return s[1]
		}
		return s[0]
	}
	rbuf := []rune(fmt.Sprintf("%c%c%c %s\n", ch(" '", dirty), '+', ch(" .", curtext != nil && curtext.file == f), string(f.Name())))
	var s regx.Ranges
	return regx.Match(nil, rbuf, 0, len(rbuf), &s)
}

func charaddr(l int, addr Address, sign int) Address {
	if sign == 0 {
		addr.r.End = l
		addr.r.Pos = addr.r.End
	} else if sign < 0 {
		addr.r.Pos -= l
		addr.r.End = addr.r.Pos
	} else if sign > 0 {
		addr.r.End += l
		addr.r.Pos = addr.r.End
	}
	if addr.r.Pos < 0 || addr.r.End > addr.f.Len() {
		editerror("address out of range")
	}
	return addr
}

func lineaddr(l int, addr Address, sign int) Address {
	f := addr.f
	var a Address
	a.f = f
	if sign >= 0 {
		var p int
		if l == 0 {
			if sign == 0 || addr.r.End == 0 {
				a.r.End = 0
				a.r.Pos = a.r.End
				return a
			}
			a.r.Pos = addr.r.End
			p = addr.r.End - 1
		} else {
			var n int
			if sign == 0 || addr.r.End == 0 {
				p = 0
				n = 1
			} else {
				p = addr.r.End - 1
				if f.curtext.RuneAt(p) == '\n' {
					n = 1
				}
				p++
			}
			for n < l {
				if p >= f.Len() {
					editerror("address out of range")
				}
				tmp9 := p
				p++
				if f.curtext.RuneAt(tmp9) == '\n' {
					n++
				}
			}
			a.r.Pos = p
		}
		for p < f.Len() {
		}
		a.r.End = p
	} else {
		p := addr.r.Pos
		if l == 0 {
			a.r.End = addr.r.Pos
		} else {
			for n := 0; n < l; { // always runs once
				if p == 0 {
					n++
					if n != l {
						editerror("address out of range")
					}
				} else {
					c := f.curtext.RuneAt(p - 1)
					if c != '\n' || func() bool { n++; return n != l }() {
						p--
					}
				}
			}
			a.r.End = p
			if p > 0 {
				p--
			}
		}
		for p > 0 && f.curtext.RuneAt(p-1) != '\n' { // lines start after a newline
			p--
		}
		a.r.Pos = p
	}
	return a
}

type Filecheck struct {
	f *File
	r []rune
}

func allfilecheck(w *Window, v interface{}) {
	fp := v.(*Filecheck)
	f := w.body.file
	if w.body.file == fp.f {
		return
	}
	if runes.Equal(fp.r, f.Name()) {
		alog.Printf("warning: duplicate file name \"%s\"\n", string(fp.r))
	}
}

func cmdname(f *File, str *String, set bool) []rune {
	s := str.r
	if len(s) == 0 {
		// no name; use existing
		if len(f.Name()) == 0 {
			return nil
		}
		return runes.Clone(f.Name())
	}
	s = runes.SkipBlank(s)
	var r []rune
	if len(s) > 0 {
		if s[0] == '/' {
			r = runes.Clone(s)
		} else {
			newname := dirname(f.curtext, runes.Clone(s))
			r = newname
		}
		var fc Filecheck
		fc.f = f
		fc.r = r
		allwindows(allfilecheck, &fc)
		if len(f.Name()) == 0 {
			set = true
		}
	}

	if set && !runes.Equal(r, f.Name()) {
		f.Mark()
		f.SetMod(true)
		f.curtext.w.dirty = true
		winsetname(f.curtext.w, r)
	}
	return r
}
