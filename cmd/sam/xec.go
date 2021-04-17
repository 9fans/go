package main

import (
	"fmt"
	"os"
	"strings"
)

var Glooping int
var nest int

func resetxec() {
	nest = 0
	Glooping = nest
}

func cmdexec(f *File, cp *Cmd) bool {
	if f != nil && f.unread {
		load(f)
	}
	if f == nil && (cp.addr == nil || cp.addr.type_ != '"') && !strings.ContainsRune("bBnqUXY!", cp.cmdc) && cp.cmdc != 'c'|0x100 && (cp.cmdc != 'D' || cp.ctext == nil) {
		error_(Enofile)
	}
	i := lookup(cp.cmdc)
	if i >= 0 && cmdtab[i].defaddr != aNo {
		ap := cp.addr
		if ap == nil && cp.cmdc != '\n' {
			ap = newaddr()
			cp.addr = ap
			ap.type_ = '.'
			if cmdtab[i].defaddr == aAll {
				ap.type_ = '*'
			}
		} else if ap != nil && ap.type_ == '"' && ap.next == nil && cp.cmdc != '\n' {
			ap.next = newaddr()
			ap.next.type_ = '.'
			if cmdtab[i].defaddr == aAll {
				ap.next.type_ = '*'
			}
		}
		if cp.addr != nil { /* may be false for '\n' (only) */
			if f != nil {
				addr = address(ap, f.dot, 0)
			} else { /* a " */
				addr = address(ap, Address{}, 0)
			}
			f = addr.f
		}
	}
	current(f)
	switch cp.cmdc {
	case '{':
		var a Address
		if cp.addr != nil {
			a = address(cp.addr, f.dot, 0)
		} else {
			a = f.dot
		}
		for cp = cp.ccmd; cp != nil; cp = cp.next {
			a.f.dot = a
			cmdexec(a.f, cp)
		}
	default:
		return cmdtab[i].fn(f, cp)
	}
	return true
}

func a_cmd(f *File, cp *Cmd) bool {
	return fappend(f, cp, addr.r.p2)
}

func b_cmd(f *File, cp *Cmd) bool {
	debug("%c ctext=%q\n", cp.cmdc, string(cp.ctext.s))
	if cp.cmdc == 'b' {
		f = tofile(cp.ctext)
	} else {
		f = getfile(cp.ctext)
	}
	if f.unread {
		load(f)
	} else if nest == 0 {
		filename(f)
	}
	return true
}

func c_cmd(f *File, cp *Cmd) bool {
	logdelete(f, addr.r.p1, addr.r.p2)
	f.ndot.r.p2 = addr.r.p2
	f.ndot.r.p1 = f.ndot.r.p2
	return fappend(f, cp, addr.r.p2)
}

func d_cmd(f *File, cp *Cmd) bool {
	logdelete(f, addr.r.p1, addr.r.p2)
	f.ndot.r.p2 = addr.r.p1
	f.ndot.r.p1 = f.ndot.r.p2
	return true
}

func D_cmd(f *File, cp *Cmd) bool {
	closefiles(f, cp.ctext)
	return true
}

func e_cmd(f *File, cp *Cmd) bool {
	if getname(f, cp.ctext, cp.cmdc == 'e') == 0 {
		error_(Enoname)
	}
	edit(f, cp.cmdc)
	return true
}

func f_cmd(f *File, cp *Cmd) bool {
	getname(f, cp.ctext, true)
	filename(f)
	return true
}

func g_cmd(f *File, cp *Cmd) bool {
	if f != addr.f {
		panic_("g_cmd f!=addr.f")
	}
	compile(cp.re)
	if execute(f, addr.r.p1, addr.r.p2) != (cp.cmdc == 'v') {
		f.dot = addr
		return cmdexec(f, cp.ccmd)
	}
	return true
}

func i_cmd(f *File, cp *Cmd) bool {
	return fappend(f, cp, addr.r.p1)
}

func k_cmd(f *File, cp *Cmd) bool {
	f.mark = addr.r
	return true
}

func m_cmd(f *File, cp *Cmd) bool {
	addr2 := address(cp.caddr, f.dot, 0)
	if cp.cmdc == 'm' {
		move(f, addr2)
	} else {
		fcopy(f, addr2)
	}
	return true
}

func n_cmd(f *File, cp *Cmd) bool {
	for _, f := range file {
		if f == cmd {
			continue
		}
		Strduplstr(&genstr, &f.name)
		filename(f)
	}
	return true
}

func p_cmd(f *File, cp *Cmd) bool {
	return display(f)
}

func q_cmd(f *File, cp *Cmd) bool {
	trytoquit()
	if downloaded {
		outT0(Hexit)
		return true
	}
	return false
}

func s_cmd(f *File, cp *Cmd) bool {
	didsub := 0
	delta := 0

	n := cp.num
	op := -1
	compile(cp.re)
	for p1 := addr.r.p1; p1 <= addr.r.p2 && execute(f, p1, addr.r.p2); {
		if sel.p[0].p1 == sel.p[0].p2 { /* empty match? */
			if sel.p[0].p1 == op {
				p1++
				continue
			}
			p1 = sel.p[0].p2 + 1
		} else {
			p1 = sel.p[0].p2
		}
		op = sel.p[0].p2
		n--
		if n > 0 {
			continue
		}
		Strzero(&genstr)
		for i := 0; i < len(cp.ctext.s); i++ { // i reassigned below
			c := cp.ctext.s[i]
			if c == '\\' && i+1 < len(cp.ctext.s) {
				i++
				c = cp.ctext.s[i]
				if '1' <= c && c <= '9' {
					j := c - '0'
					if sel.p[j].p2-sel.p[j].p1 > BLOCKSIZE {
						error_(Elongtag)
					}
					bufread(&f.b, sel.p[j].p1, genbuf[:sel.p[j].p2-sel.p[j].p1])
					Strinsert(&genstr, tmprstr(genbuf[:(sel.p[j].p2-sel.p[j].p1)]), len(genstr.s))
				} else {
					Straddc(&genstr, c)
				}
			} else if c != '&' {
				Straddc(&genstr, c)
			} else {
				if sel.p[0].p2-sel.p[0].p1 > BLOCKSIZE {
					error_(Elongrhs)
				}
				bufread(&f.b, sel.p[0].p1, genbuf[:sel.p[0].p2-sel.p[0].p1])
				Strinsert(&genstr, tmprstr(genbuf[:sel.p[0].p2-sel.p[0].p1]), len(genstr.s))
			}
		}
		if sel.p[0].p1 != sel.p[0].p2 {
			logdelete(f, sel.p[0].p1, sel.p[0].p2)
			delta -= sel.p[0].p2 - sel.p[0].p1
		}
		if len(genstr.s) > 0 {
			loginsert(f, sel.p[0].p2, genstr.s)
			delta += len(genstr.s)
		}
		didsub = 1
		if !cp.flag {
			break
		}
	}
	if didsub == 0 && nest == 0 {
		error_(Enosub)
	}
	f.ndot.r.p1 = addr.r.p1
	f.ndot.r.p2 = addr.r.p2 + delta
	return true
}

func u_cmd(f *File, cp *Cmd) bool {
	n := cp.num
	if n >= 0 {
		for {
			tmp35 := n
			n--
			if !(tmp35 != 0) || !(undo(true) != 0) {
				break
			}
		}
	} else {
		for {
			tmp36 := n
			n++
			if !(tmp36 != 0) || !(undo(false) != 0) {
				break
			}
		}
	}
	return true
}

func w_cmd(f *File, cp *Cmd) bool {
	fseq := f.seq
	if getname(f, cp.ctext, false) == 0 {
		error_(Enoname)
	}
	if fseq == seq {
		error_s(Ewseq, genc)
	}
	writef(f)
	return true
}

func x_cmd(f *File, cp *Cmd) bool {
	if cp.re != nil {
		looper(f, cp, cp.cmdc == 'x')
	} else {
		linelooper(f, cp)
	}
	return true
}

func X_cmd(f *File, cp *Cmd) bool {
	filelooper(cp, cp.cmdc == 'X')
	return true
}

func plan9_cmd(f *File, cp *Cmd) bool {
	plan9(f, cp.cmdc, cp.ctext, nest > 0)
	return true
}

func eq_cmd(f *File, cp *Cmd) bool {
	var charsonly bool
	switch len(cp.ctext.s) {
	case 0:
		charsonly = false
	case 1:
		if cp.ctext.s[0] == '#' {
			charsonly = true
			break
		}
		fallthrough
	default:
		error_(Enewline)
	}
	printposn(f, charsonly)
	return true
}

func nl_cmd(f *File, cp *Cmd) bool {
	if cp.addr == nil {
		/* First put it on newline boundaries */
		addr = lineaddr(Posn(0), f.dot, -1)
		a := lineaddr(Posn(0), f.dot, 1)
		addr.r.p2 = a.r.p2
		if addr.r.p1 == f.dot.r.p1 && addr.r.p2 == f.dot.r.p2 {
			addr = lineaddr(Posn(1), f.dot, 1)
		}
		display(f)
	} else if downloaded {
		moveto(f, addr.r)
	} else {
		display(f)
	}
	return true
}

func cd_cmd(f *File, cp *Cmd) bool {
	cd(cp.ctext)
	return true
}

func fappend(f *File, cp *Cmd, p Posn) bool {
	if len(cp.ctext.s) > 0 && cp.ctext.s[len(cp.ctext.s)-1] == 0 {
		// TODO(rsc): Where did the NUL come from?
		cp.ctext.s = cp.ctext.s[:len(cp.ctext.s)-1]
	}
	if len(cp.ctext.s) > 0 {
		loginsert(f, p, cp.ctext.s)
	}
	f.ndot.r.p1 = p
	f.ndot.r.p2 = p + len(cp.ctext.s)
	return true
}

func display(f *File) bool {
	p1 := addr.r.p1
	p2 := addr.r.p2
	if p2 > f.b.nc {
		fmt.Fprintf(os.Stderr, "bad display addr p1=%d p2=%d f->b.nc=%d\n", p1, p2, f.b.nc) /*ZZZ should never happen, can remove */
		p2 = f.b.nc
	}
	for p1 < p2 {
		np := p2 - p1
		if np > BLOCKSIZE-1 {
			np = BLOCKSIZE - 1
		}
		text := genbuf[:np]
		bufread(&f.b, p1, text)
		if downloaded {
			termwrite(string(text))
		} else {
			Write(os.Stdout, []byte(string(text))) // TODO(rsc)
		}
		// free(c)
		p1 += np
	}
	f.dot = addr
	return true
}

func looper(f *File, cp *Cmd, xy bool) {
	r := addr.r
	op := r.p1
	if xy {
		op = -1
	}
	nest++
	compile(cp.re)
	for p := r.p1; p <= r.p2; {
		if !execute(f, p, r.p2) { /* no match, but y should still run */
			if xy || op > r.p2 {
				break
			}
			f.dot.r.p1 = op
			f.dot.r.p2 = r.p2
			p = r.p2 + 1 /* exit next loop */
		} else {
			if sel.p[0].p1 == sel.p[0].p2 { /* empty match? */
				if sel.p[0].p1 == op {
					p++
					continue
				}
				p = sel.p[0].p2 + 1
			} else {
				p = sel.p[0].p2
			}
			if xy {
				f.dot.r = sel.p[0]
			} else {
				f.dot.r.p1 = op
				f.dot.r.p2 = sel.p[0].p1
			}
		}
		op = sel.p[0].p2
		cmdexec(f, cp.ccmd)
		compile(cp.re)
	}
	nest--
}

func linelooper(f *File, cp *Cmd) {
	nest++
	r := addr.r
	var a3 Address
	a3.f = f
	a3.r.p2 = r.p1
	a3.r.p1 = a3.r.p2
	for p := r.p1; p < r.p2; p = a3.r.p2 {
		a3.r.p1 = a3.r.p2
		var a Address
		var linesel Range
		/*pjw		if(p!=r.p1 || (linesel = lineaddr((Posn)0, a3, 1)).r.p2==p)*/
		if p != r.p1 || func() bool { a = lineaddr(Posn(0), a3, 1); linesel = a.r; return linesel.p2 == p }() {
			a = lineaddr(Posn(1), a3, 1)
			linesel = a.r
		}
		if linesel.p1 >= r.p2 {
			break
		}
		if linesel.p2 >= r.p2 {
			linesel.p2 = r.p2
		}
		if linesel.p2 > linesel.p1 {
			if linesel.p1 >= a3.r.p2 && linesel.p2 > a3.r.p2 {
				f.dot.r = linesel
				cmdexec(f, cp.ccmd)
				a3.r = linesel
				continue
			}
		}
		break
	}
	nest--
}

func filelooper(cp *Cmd, XY bool) {
	tmp38 := Glooping
	Glooping++
	if tmp38 != 0 {
		error_(EnestXY)
	}
	nest++
	settempfile()
	cur := curfile
	for _, f := range tempfile {
		if f == cmd {
			continue
		}
		if cp.re == nil || filematch(f, cp.re) == XY {
			cmdexec(f, cp.ccmd)
		}
	}
	if cur != nil && whichmenu(cur) >= 0 { /* check that cur is still a file */
		current(cur)
	}
	Glooping--
	nest--
}
