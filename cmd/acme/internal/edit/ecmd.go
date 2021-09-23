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

package edit

import (
	"fmt"
	"os"
	"strings"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/fileload"
	"9fans.net/go/cmd/acme/internal/regx"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/ui"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
)

var Glooping int
var nest int
var Enoname = "no file name given"

var TheAddr Address
var menu *wind.File

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

func mkaddr(a *Address, f *wind.File) {
	a.r.Pos = f.Curtext.Q0
	a.r.End = f.Curtext.Q1
	a.f = f
}

func cmdexec(t *wind.Text, cp *Cmd) bool {
	var w *wind.Window
	if t == nil {
		w = nil
	} else {
		w = t.W
	}
	if w == nil && (cp.addr == nil || cp.addr.typ != '"') && !strings.ContainsRune("bBnqUXY!", cp.cmdc) && (!(cp.cmdc == 'D') || cp.u.text == nil) {
		editerror("no current window")
	}
	i := cmdlookup(cp.cmdc) // will be -1 for '{'
	var f *wind.File
	if t != nil && t.W != nil {
		t = &t.W.Body
		f = t.File
		f.Curtext = t
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
				TheAddr = cmdaddress(ap, dot, 0)
			} else { // a "
				TheAddr = cmdaddress(ap, none, 0)
			}
			f = TheAddr.f
			t = f.Curtext
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
			t.Q0 = dot.r.Pos
			t.Q1 = dot.r.End
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

func Edittext(w *wind.Window, q int, r []rune) error {
	f := w.Body.File
	switch Editing {
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
func filelist(t *wind.Text, r []rune) []rune {
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

func a_cmd(t *wind.Text, cp *Cmd) bool {
	return fappend(t.File, cp, TheAddr.r.End)
}

func b_cmd(t *wind.Text, cp *Cmd) bool {
	f := tofile(cp.u.text)
	if nest == 0 {
		pfilename(f)
	}
	curtext = f.Curtext
	return true
}

func B_cmd(t *wind.Text, cp *Cmd) bool {
	list := filelist(t, cp.u.text.r)
	if list == nil {
		editerror(Enoname)
	}
	r := runes.SkipBlank(list)
	if len(r) == 0 {
		ui.New(t, t, nil, false, false, r)
	} else {
		for len(r) > 0 {
			s := runes.SkipNonBlank(r)
			ui.New(t, t, nil, false, false, r[:len(r)-len(s)])
			r = runes.SkipBlank(s)
		}
	}
	clearcollection()
	return true
}

func c_cmd(t *wind.Text, cp *Cmd) bool {
	elogreplace(t.File, TheAddr.r.Pos, TheAddr.r.End, cp.u.text.r)
	t.Q0 = TheAddr.r.Pos
	t.Q1 = TheAddr.r.End
	return true
}

func d_cmd(t *wind.Text, cp *Cmd) bool {
	if TheAddr.r.End > TheAddr.r.Pos {
		elogdelete(t.File, TheAddr.r.Pos, TheAddr.r.End)
	}
	t.Q0 = TheAddr.r.Pos
	t.Q1 = TheAddr.r.Pos
	return true
}

func D1(t *wind.Text) {
	if len(t.W.Body.File.Text) > 1 || wind.Winclean(t.W, false) {
		ui.ColcloseAndMouse(t.Col, t.W, true)
	}
}

func D_cmd(t *wind.Text, cp *Cmd) bool {
	list := filelist(t, cp.u.text.r)
	if list == nil {
		D1(t)
		return true
	}
	dir := wind.Dirname(t, nil)
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
		w := ui.LookFile(rs)
		if w == nil {
			editerror(fmt.Sprintf("no such file %s", string(rs)))
		}
		D1(&w.Body)
		r = runes.SkipBlank(s)
		if len(r) == 0 {
			break
		}
	}
	clearcollection()
	return true
}

func readloader(f *wind.File) func(pos int, data []rune) int {
	return func(pos int, data []rune) int {
		if len(data) > 0 {
			eloginsert(f, pos, data)
		}
		return 0
	}
}

func e_cmd(t *wind.Text, cp *Cmd) bool {
	f := t.File
	q0 := TheAddr.r.Pos
	q1 := TheAddr.r.End
	if cp.cmdc == 'e' {
		if !wind.Winclean(t.W, true) {
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
	samename := runes.Equal(name, t.File.Name())
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
	fileload.Loadfile(fd, q1, &nulls, readloader(f), nil)
	if nulls {
		alog.Printf("%s: NUL bytes elided\n", s)
	} else if allreplaced && samename {
		elogfind(f).editclean = true
	}
	return true
}

func f_cmd(t *wind.Text, cp *Cmd) bool {
	var str *String
	if cp.u.text == nil {
		str = new(String) // empty
	} else {
		str = cp.u.text
	}
	cmdname(t.File, str, true)
	pfilename(t.File)
	return true
}

func g_cmd(t *wind.Text, cp *Cmd) bool {
	if t.File != TheAddr.f {
		alog.Printf("internal error: g_cmd f!=addr.f\n")
		return false
	}
	if !regx.Compile(cp.re.r) {
		editerror("bad regexp in g command")
	}
	if regx.Match(t, nil, TheAddr.r.Pos, TheAddr.r.End, &regx.Sel) != (cp.cmdc == 'v') {
		t.Q0 = TheAddr.r.Pos
		t.Q1 = TheAddr.r.End
		return cmdexec(t, cp.u.cmd)
	}
	return true
}

func i_cmd(t *wind.Text, cp *Cmd) bool {
	return fappend(t.File, cp, TheAddr.r.Pos)
}

func fbufalloc() []rune {
	return make([]rune, bufs.Len/runes.RuneSize)
}

func fbuffree(b []rune) {}

func fcopy(f *wind.File, addr2 Address) {
	buf := bufs.AllocRunes()
	var ni int
	for p := TheAddr.r.Pos; p < TheAddr.r.End; p += ni {
		ni = TheAddr.r.End - p
		if ni > bufs.RuneLen {
			ni = bufs.RuneLen
		}
		f.Read(p, buf[:ni])
		eloginsert(addr2.f, addr2.r.End, buf[:ni])
	}
	bufs.FreeRunes(buf)
}

func move(f *wind.File, addr2 Address) {
	if TheAddr.f != addr2.f || TheAddr.r.End <= addr2.r.Pos {
		elogdelete(f, TheAddr.r.Pos, TheAddr.r.End)
		fcopy(f, addr2)
	} else if TheAddr.r.Pos >= addr2.r.End {
		fcopy(f, addr2)
		elogdelete(f, TheAddr.r.Pos, TheAddr.r.End)
	} else if TheAddr.r.Pos == addr2.r.Pos && TheAddr.r.End == addr2.r.End { // move to self; no-op
	} else {
		editerror("move overlaps itself")
	}
}

func m_cmd(t *wind.Text, cp *Cmd) bool {
	var dot Address
	mkaddr(&dot, t.File)
	addr2 := cmdaddress(cp.u.mtaddr, dot, 0)
	if cp.cmdc == 'm' {
		move(t.File, addr2)
	} else {
		fcopy(t.File, addr2)
	}
	return true
}

func p_cmd(t *wind.Text, cp *Cmd) bool {
	return pdisplay(t.File)
}

func s_cmd(t *wind.Text, cp *Cmd) bool {
	n := cp.num
	op := -1
	if !regx.Compile(cp.re.r) {
		editerror("bad regexp in s command")
	}
	var rp []regx.Ranges
	delta := 0
	didsub := false
	for p1 := TheAddr.r.Pos; p1 <= TheAddr.r.End && regx.Match(t, nil, p1, TheAddr.r.End, &regx.Sel); {
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
					t.File.Read(regx.Sel.R[j].Pos, rbuf[:regx.Sel.R[j].End-regx.Sel.R[j].Pos])
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
				t.File.Read(regx.Sel.R[0].Pos, rbuf[:regx.Sel.R[0].End-regx.Sel.R[0].Pos])
				for k := 0; k < regx.Sel.R[0].End-regx.Sel.R[0].Pos; k++ {
					Straddc(buf, rbuf[k])
				}
			}
		}
		elogreplace(t.File, regx.Sel.R[0].Pos, regx.Sel.R[0].End, buf.r)
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
	t.Q0 = TheAddr.r.Pos
	t.Q1 = TheAddr.r.End
	return true

Err:
	freestring(buf)
	bufs.FreeRunes(rbuf)
	editerror(err)
	return false
}

func u_cmd(t *wind.Text, cp *Cmd) bool {
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
		if !(tmp3 > 0) || !(t.File.Seq() != oseq) {
			break
		}
		oseq = t.File.Seq()
		const XXX = false
		ui.XUndo(t, nil, nil, flag, XXX, nil)
	}
	return true
}

func w_cmd(t *wind.Text, cp *Cmd) bool {
	f := t.File
	if f.Seq() == file.Seq {
		editerror("can't write file with pending modifications")
	}
	r := cmdname(f, cp.u.text, false)
	if r == nil {
		editerror("no name specified for 'w' command")
	}
	Putfile(f, TheAddr.r.Pos, TheAddr.r.End, r)
	// r is freed by putfile
	return true
}

var Putfile = func(*wind.File, int, int, []rune) {}

func x_cmd(t *wind.Text, cp *Cmd) bool {
	if cp.re != nil {
		looper(t.File, cp, cp.cmdc == 'x')
	} else {
		linelooper(t.File, cp)
	}
	return true
}

func X_cmd(t *wind.Text, cp *Cmd) bool {

	filelooper(t, cp, cp.cmdc == 'X')
	return true
}

var Run = func(w *wind.Window, s string, rdir []rune) {}

func runpipe(t *wind.Text, cmd rune, cr []rune, state int) {
	r := runes.SkipBlank(cr)
	if len(r) == 0 {
		editerror("no command specified for %c", cmd)
	}
	var w *wind.Window
	if state == Inserting {
		w = t.W
		t.Q0 = TheAddr.r.Pos
		t.Q1 = TheAddr.r.End
		if cmd == '<' || cmd == '|' {
			elogdelete(t.File, t.Q0, t.Q1)
		}
	}
	s := make([]rune, len(r)+1)
	s[0] = cmd
	copy(s[1:], r)
	var dir []rune
	dir = nil
	if t != nil {
		dir = wind.Dirname(t, nil)
	}
	if len(dir) == 1 && dir[0] == '.' { // sigh
		dir = nil
	}
	Editing = state
	if t != nil && t.W != nil {
		util.Incref(&t.W.Ref) // run will decref
	}
	Run(w, string(s), dir)
	if t != nil && t.W != nil {
		wind.Winunlock(t.W)
	}
	wind.TheRow.Lk.Unlock()
	<-Cedit
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
		q = &w.Editoutlk
	} else {
		q = &Editoutlk
	}
	q.Lock() // wait for file to close
	q.Unlock()
	wind.TheRow.Lk.Lock()
	Editing = Inactive
	if t != nil && t.W != nil {
		wind.Winlock(t.W, 'M')
	}
}

func pipe_cmd(t *wind.Text, cp *Cmd) bool {
	runpipe(t, cp.cmdc, cp.u.text.r, Inserting)
	return true
}

func Nlcount(t *wind.Text, q0 int, q1 int, pnr *int) int {
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
			t.File.Read(q0, buf[:nbuf])
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

func printposn(t *wind.Text, mode int) {
	if t != nil && t.File != nil && t.File.Name() != nil {
		alog.Printf("%s:", string(t.File.Name()))
	}
	var l1 int
	var l2 int
	var r1 int
	var r2 int

	switch mode {
	case PosnChars:
		alog.Printf("#%d", TheAddr.r.Pos)
		if TheAddr.r.End != TheAddr.r.Pos {
			alog.Printf(",#%d", TheAddr.r.End)
		}
		alog.Printf("\n")
		return

	default:
	case PosnLine:
		l1 = 1 + Nlcount(t, 0, TheAddr.r.Pos, nil)
		l2 = l1 + Nlcount(t, TheAddr.r.Pos, TheAddr.r.End, nil)
		// check if addr ends with '\n'
		if TheAddr.r.End > 0 && TheAddr.r.End > TheAddr.r.Pos && t.RuneAt(TheAddr.r.End-1) == '\n' {
			l2--
		}
		alog.Printf("%d", l1)
		if l2 != l1 {
			alog.Printf(",%d", l2)
		}
		alog.Printf("\n")
		return

	case PosnLineChars:
		l1 = 1 + Nlcount(t, 0, TheAddr.r.Pos, &r1)
		l2 = l1 + Nlcount(t, TheAddr.r.Pos, TheAddr.r.End, &r2)
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

func eq_cmd(t *wind.Text, cp *Cmd) bool {
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

func nl_cmd(t *wind.Text, cp *Cmd) bool {
	f := t.File
	if cp.addr == nil {
		var a Address
		// First put it on newline boundaries
		mkaddr(&a, f)
		TheAddr = lineaddr(0, a, -1)
		a = lineaddr(0, a, 1)
		TheAddr.r.End = a.r.End
		if TheAddr.r.Pos == t.Q0 && TheAddr.r.End == t.Q1 {
			mkaddr(&a, f)
			TheAddr = lineaddr(1, a, 1)
		}
	}
	wind.Textshow(t, TheAddr.r.Pos, TheAddr.r.End, true)
	return true
}

func fappend(f *wind.File, cp *Cmd, p int) bool {
	if len(cp.u.text.r) > 0 {
		eloginsert(f, p, cp.u.text.r)
	}
	f.Curtext.Q0 = p
	f.Curtext.Q1 = p
	return true
}

func pdisplay(f *wind.File) bool {
	p1 := TheAddr.r.Pos
	p2 := TheAddr.r.End
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
	f.Curtext.Q0 = TheAddr.r.Pos
	f.Curtext.Q1 = TheAddr.r.End
	return true
}

func pfilename(f *wind.File) {
	w := f.Curtext.W
	// same check for dirty as in settag, but we know ncache==0
	dirty := !w.IsDir && !w.IsScratch && f.Mod()
	ch := func(s string, b bool) byte {
		if b {
			return s[1]
		}
		return s[0]
	}
	alog.Printf("%c%c%c %s\n", ch(" '", dirty), '+', ch(" .", curtext != nil && curtext.File == f), string(f.Name()))
}

func loopcmd(f *wind.File, cp *Cmd, rp []runes.Range) {
	for i := 0; i < len(rp); i++ {
		f.Curtext.Q0 = rp[i].Pos
		f.Curtext.Q1 = rp[i].End
		cmdexec(f.Curtext, cp)
	}
}

func looper(f *wind.File, cp *Cmd, xy bool) {
	r := TheAddr.r
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
		if !regx.Match(f.Curtext, nil, p, r.End, &regx.Sel) { // no match, but y should still run
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

func linelooper(f *wind.File, cp *Cmd) {
	nest++
	var rp []runes.Range
	r := TheAddr.r
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
	w  []*wind.Window
}

var loopstruct Looper // only one; X and Y can't nest

func alllooper(w *wind.Window, v interface{}) {
	lp := v.(*Looper)
	cp := lp.cp
	//	if(w->isscratch || w->isdir)
	//		return;
	t := &w.Body
	// only use this window if it's the current window for the file
	if t.File.Curtext != t {
		return
	}
	//	if(w->nopen[QWevent] > 0)
	//		return;
	// no auto-execute on files without names
	if cp.re == nil && len(t.File.Name()) == 0 {
		return
	}
	if cp.re == nil || filematch(t.File, cp.re) == lp.XY {
		lp.w = append(lp.w, w)
	}
}

func alllocker(w *wind.Window, v interface{}) {
	if v.(bool) {
		util.Incref(&w.Ref)
	} else {
		wind.Winclose(w)
	}
}

func filelooper(t *wind.Text, cp *Cmd, XY bool) {
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
	wind.All(alllooper, &loopstruct)
	/*
	 * add a ref to all windows to keep safe windows accessed by X
	 * that would not otherwise have a ref to hold them up during
	 * the shenanigans.  note this with globalincref so that any
	 * newly created windows start with an extra reference.
	 */
	wind.All(alllocker, true)
	wind.GlobalIncref = 1

	/*
	 * Unlock the window running the X command.
	 * We'll need to lock and unlock each target window in turn.
	 */
	if t != nil && t.W != nil {
		wind.Winunlock(t.W)
	}

	for i := 0; i < len(loopstruct.w); i++ {
		targ := &loopstruct.w[i].Body
		if targ != nil && targ.W != nil {
			wind.Winlock(targ.W, cp.cmdc)
		}
		cmdexec(targ, cp.u.cmd)
		if targ != nil && targ.W != nil {
			wind.Winunlock(targ.W)
		}
	}

	if t != nil && t.W != nil {
		wind.Winlock(t.W, cp.cmdc)
	}

	wind.All(alllocker, false)
	wind.GlobalIncref = 0
	loopstruct.w = nil

	Glooping--
	nest--
}

func nextmatch(f *wind.File, r *String, p int, sign int) {
	if !regx.Compile(r.r) {
		editerror("bad regexp in command address")
	}
	if sign >= 0 {
		if !regx.Match(f.Curtext, nil, p, 0x7FFFFFFF, &regx.Sel) {
			editerror("no match for regexp")
		}
		if regx.Sel.R[0].Pos == regx.Sel.R[0].End && regx.Sel.R[0].Pos == p {
			p++
			if p > f.Len() {
				p = 0
			}
			if !regx.Match(f.Curtext, nil, p, 0x7FFFFFFF, &regx.Sel) {
				editerror("address")
			}
		}
	} else {
		if !regx.MatchBackward(f.Curtext, p, &regx.Sel) {
			editerror("no match for regexp")
		}
		if regx.Sel.R[0].Pos == regx.Sel.R[0].End && regx.Sel.R[0].End == p {
			p--
			if p < 0 {
				p = f.Len()
			}
			if !regx.MatchBackward(f.Curtext, p, &regx.Sel) {
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
				f.Curtext.Q0 = a1.r.Pos
				f.Curtext.Q1 = a1.r.End
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
	f *wind.File
	r *String
}

func alltofile(w *wind.Window, v interface{}) {
	tp := v.(*Tofile)
	if tp.f != nil {
		return
	}
	if w.IsScratch || w.IsDir {
		return
	}
	t := &w.Body
	// only use this window if it's the current window for the file
	if t.File.Curtext != t {
		return
	}
	//	if(w->nopen[QWevent] > 0)
	//		return;
	if runes.Equal(tp.r.r, t.File.Name()) {
		tp.f = t.File
	}
}

func tofile(r *String) *wind.File {
	var rr String
	rr.r = runes.SkipBlank(r.r)
	var t Tofile
	t.f = nil
	t.r = &rr
	wind.All(alltofile, &t)
	if t.f == nil {
		editerror("no such file\"%s\"", string(rr.r))
	}
	return t.f
}

func allmatchfile(w *wind.Window, v interface{}) {
	tp := v.(*Tofile)
	if w.IsScratch || w.IsDir {
		return
	}
	t := &w.Body
	// only use this window if it's the current window for the file
	if t.File.Curtext != t {
		return
	}
	//	if(w->nopen[QWevent] > 0)
	//		return;
	if filematch(w.Body.File, tp.r) {
		if tp.f != nil {
			editerror("too many files match \"%s\"", string(tp.r.r))
		}
		tp.f = w.Body.File
	}
}

func matchfile(r *String) *wind.File {
	var tf Tofile
	tf.f = nil
	tf.r = r
	wind.All(allmatchfile, &tf)

	if tf.f == nil {
		editerror("no file matches \"%s\"", string(r.r))
	}
	return tf.f
}

func filematch(f *wind.File, r *String) bool {
	// compile expr first so if we get an error, we haven't allocated anything
	if !regx.Compile(r.r) {
		editerror("bad regexp in file match")
	}
	w := f.Curtext.W
	// same check for dirty as in settag, but we know ncache==0
	dirty := !w.IsDir && !w.IsScratch && f.Mod()
	ch := func(s string, b bool) byte {
		if b {
			return s[1]
		}
		return s[0]
	}
	rbuf := []rune(fmt.Sprintf("%c%c%c %s\n", ch(" '", dirty), '+', ch(" .", curtext != nil && curtext.File == f), string(f.Name())))
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
				if f.Curtext.RuneAt(p) == '\n' {
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
				if f.Curtext.RuneAt(tmp9) == '\n' {
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
					c := f.Curtext.RuneAt(p - 1)
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
		for p > 0 && f.Curtext.RuneAt(p-1) != '\n' { // lines start after a newline
			p--
		}
		a.r.Pos = p
	}
	return a
}

type Filecheck struct {
	f *wind.File
	r []rune
}

func allfilecheck(w *wind.Window, v interface{}) {
	fp := v.(*Filecheck)
	f := w.Body.File
	if w.Body.File == fp.f {
		return
	}
	if runes.Equal(fp.r, f.Name()) {
		alog.Printf("warning: duplicate file name \"%s\"\n", string(fp.r))
	}
}

func cmdname(f *wind.File, str *String, set bool) []rune {
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
			newname := wind.Dirname(f.Curtext, runes.Clone(s))
			r = newname
		}
		var fc Filecheck
		fc.f = f
		fc.r = r
		wind.All(allfilecheck, &fc)
		if len(f.Name()) == 0 {
			set = true
		}
	}

	if set && !runes.Equal(r, f.Name()) {
		f.Mark()
		f.SetMod(true)
		f.Curtext.W.Dirty = true
		wind.Winsetname(f.Curtext.W, r)
	}
	return r
}

// editing

const (
	Inactive = 0 + iota
	Inserting
	Collecting
)

var Cedit = make(chan int)

var Editoutlk util.QLock // atomic flag
