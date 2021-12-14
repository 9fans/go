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
	"runtime"
	"strings"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
)

var linex = "\n"
var wordx = " \t\n"
var cmdtab []Cmdtab

type Cmdtab struct {
	cmdc    rune
	text    bool
	regexp  bool
	addr    bool
	defcmd  rune
	defaddr Defaddr
	count   uint8
	token   string
	fn      func(*wind.Text, *Cmd) bool
}

func init() { cmdtab = cmdtab1 } // break init cycle
var cmdtab1 = []Cmdtab{
	//	cmdc	text	regexp	addr	defcmd	defaddr	count	token	 fn
	Cmdtab{'\n', false, false, false, 0, aDot, 0, "", nl_cmd},
	Cmdtab{'a', true, false, false, 0, aDot, 0, "", a_cmd},
	Cmdtab{'b', false, false, false, 0, aNo, 0, linex, b_cmd},
	Cmdtab{'c', true, false, false, 0, aDot, 0, "", c_cmd},
	Cmdtab{'d', false, false, false, 0, aDot, 0, "", d_cmd},
	Cmdtab{'e', false, false, false, 0, aNo, 0, wordx, e_cmd},
	Cmdtab{'f', false, false, false, 0, aNo, 0, wordx, f_cmd},
	Cmdtab{'g', false, true, false, 'p', aDot, 0, "", g_cmd},
	Cmdtab{'i', true, false, false, 0, aDot, 0, "", i_cmd},
	Cmdtab{'m', false, false, true, 0, aDot, 0, "", m_cmd},
	Cmdtab{'p', false, false, false, 0, aDot, 0, "", p_cmd},
	Cmdtab{'r', false, false, false, 0, aDot, 0, wordx, e_cmd},
	Cmdtab{'s', false, true, false, 0, aDot, 1, "", s_cmd},
	Cmdtab{'t', false, false, true, 0, aDot, 0, "", m_cmd},
	Cmdtab{'u', false, false, false, 0, aNo, 2, "", u_cmd},
	Cmdtab{'v', false, true, false, 'p', aDot, 0, "", g_cmd},
	Cmdtab{'w', false, false, false, 0, aAll, 0, wordx, w_cmd},
	Cmdtab{'x', false, true, false, 'p', aDot, 0, "", x_cmd},
	Cmdtab{'y', false, true, false, 'p', aDot, 0, "", x_cmd},
	Cmdtab{'=', false, false, false, 0, aDot, 0, linex, eq_cmd},
	Cmdtab{'B', false, false, false, 0, aNo, 0, linex, B_cmd},
	Cmdtab{'D', false, false, false, 0, aNo, 0, linex, D_cmd},
	Cmdtab{'X', false, true, false, 'f', aNo, 0, "", X_cmd},
	Cmdtab{'Y', false, true, false, 'f', aNo, 0, "", X_cmd},
	Cmdtab{'<', false, false, false, 0, aDot, 0, linex, pipe_cmd},
	Cmdtab{'|', false, false, false, 0, aDot, 0, linex, pipe_cmd},
	Cmdtab{'>', false, false, false, 0, aDot, 0, linex, pipe_cmd},
	/* deliberately unimplemented:
	Cmdtab{'k', 0, 0, 0, 0, aDot, 0, "", k_cmd},
	Cmdtab{'n', 0, 0, 0, 0, aNo, 0, "", n_cmd},
	Cmdtab{'q', 0, 0, 0, 0, aNo, 0, "", q_cmd},
		Cmdtab{'!',	0,	0,	0,	0,	aNo,	0,	linex,	plan9_cmd},
	*/
}

var cmdstartp []rune
var cmdp int // index into cmdstartp
var editerrc chan string

var lastpat *String
var patset bool

var cmdlist []*Cmd
var addrlist []*Addr
var stringlist []*String
var curtext *wind.Text
var Editing int = Inactive

func editthread() {
	for {
		cmdp := parsecmd(0)
		if cmdp == nil {
			break
		}
		if !cmdexec(curtext, cmdp) {
			break
		}
		freecmd()
	}
	editerrc <- ""
}

func allelogterm(w *wind.Window, x interface{}) {
	if ef := elogfind(w.Body.File); ef != nil {
		elogterm(ef)
	}
}

func alleditinit(w *wind.Window, x interface{}) {
	wind.Textcommit(&w.Tag, true)
	wind.Textcommit(&w.Body, true)
}

func allupdate(w *wind.Window, x interface{}) {
	t := &w.Body
	if t.File.Curtext != t { // do curtext only
		return
	}
	f := elogfind(t.File)
	if f == nil {
		return
	}
	if f.elog.typ == elogNull {
		elogterm(f)
	} else if f.elog.typ != elogEmpty {
		elogapply(f)
		if f.editclean {
			f.SetMod(false)
			for i := 0; i < len(f.Text); i++ {
				f.Text[i].W.Dirty = false
			}
		}
	}
	wind.Textsetselect(t, t.Q0, t.Q1)
	wind.Textscrdraw(t)
	wind.Winsettag(w)
}

func editerror(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	freecmd()
	wind.All(allelogterm, nil) // truncate the edit logs
	editerrc <- s
	runtime.Goexit() // TODO(rsc)
}

func Editcmd(ct *wind.Text, r []rune) {
	if len(r) == 0 {
		return
	}
	if 2*len(r) > bufs.RuneLen { // TODO(rsc): why 2*len?
		alog.Printf("string too long\n")
		return
	}

	wind.All(alleditinit, nil)
	cmdstartp = make([]rune, len(r), len(r)+1)
	copy(cmdstartp, r)
	if r[len(r)-1] != '\n' {
		cmdstartp = append(cmdstartp, '\n')
	}
	cmdp = 0
	if ct.W == nil {
		curtext = nil
	} else {
		curtext = &ct.W.Body
	}
	resetxec()
	if editerrc == nil {
		editerrc = make(chan string)
		lastpat = allocstring(0)
	}
	go editthread()
	err := <-editerrc
	Editing = Inactive
	if err != "" {
		alog.Printf("Edit: %s\n", err)
	}

	// update everyone whose edit log has data
	wind.All(allupdate, nil)
}

func getch() rune {
	if cmdp >= len(cmdstartp) {
		return -1
	}
	r := cmdstartp[cmdp]
	cmdp++
	return r
}

func nextc() rune {
	if cmdp >= len(cmdstartp) {
		return -1
	}
	return cmdstartp[cmdp]
}

func ungetch() {
	cmdp--
	if cmdp < 0 {
		util.Fatal("ungetch")
	}
}

func getnum(signok int) int {
	n := 0
	sign := 1
	if signok > 1 && nextc() == '-' {
		sign = -1
		getch()
	}
	c := nextc()
	if c < '0' || '9' < c { // no number defaults to 1
		return sign
	}
	for {
		c = getch()
		if !('0' <= c) || !(c <= '9') {
			break
		}
		n = n*10 + int(c-'0')
	}
	ungetch()
	return sign * n
}

func cmdskipbl() rune {
	var c rune
	for {
		c = getch()
		if !(c == ' ') && !(c == '\t') {
			break
		}
	}
	if c >= 0 {
		ungetch()
	}
	return c
}

func allocstring(n int) *String {
	s := new(String)
	s.r = make([]rune, n, n+10)
	return s
}

func freestring(s *String) {
	s.r = nil
}

func newcmd() *Cmd {
	p := new(Cmd)
	cmdlist = append(cmdlist, p)
	return p
}

func newstring(n int) *String {
	p := allocstring(n)
	stringlist = append(stringlist, p)
	return p
}

func newaddr() *Addr {
	p := new(Addr)
	addrlist = append(addrlist, p)
	return p
}

func freecmd() {
	// free cmdlist[i]
	// free addrlist[i]
	// freestring stringlist[i]
}

func okdelim(c rune) {
	if c == '\\' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
		editerror("bad delimiter %c\n", c)
	}
}

func atnl() {
	cmdskipbl()
	c := getch()
	if c != '\n' {
		editerror("newline expected (saw %c)", c)
	}
}

func Straddc(s *String, c rune) {
	s.r = append(s.r, c)
}

func getrhs(s *String, delim, cmd rune) {
	for {
		c := getch()
		if !(c > 0 && c != delim) || !(c != '\n') {
			break
		}
		if c == '\\' {
			c = getch()
			if c <= 0 {
				util.Fatal("bad right hand side")
			}
			if c == '\n' {
				ungetch()
				c = '\\'
			} else if c == 'n' {
				c = '\n'
			} else if c != delim && (cmd == 's' || c != '\\') { // s does its own
				Straddc(s, '\\')
			}
		}
		Straddc(s, c)
	}
	ungetch() // let client read whether delimiter, '\n' or whatever
}

func collecttoken(end string) *String {
	s := newstring(0)
	var c rune
	for {
		c = nextc()
		if !(c == ' ') && !(c == '\t') {
			break
		}
		Straddc(s, getch()) // blanks significant for getname()
	}
	for {
		c = getch()
		if c <= 0 || strings.ContainsRune(end, c) {
			break
		}
		Straddc(s, c)
	}
	if c != '\n' {
		atnl()
	}
	return s
}

func collecttext() *String {
	s := newstring(0)
	if cmdskipbl() == '\n' {
		getch()
		i := 0
		for {
			begline := i
			var c rune
			for {
				c = getch()
				if !(c > 0) || !(c != '\n') {
					break
				}
				i++
				(func() { Straddc(s, c) }())
			}
			i++
			(func() { Straddc(s, '\n') }())
			if c < 0 {
				goto Return
			}
			if !(s.r[begline] != '.') && !(s.r[begline+1] != '\n') {
				break
			}
		}
		s.r = s.r[:len(s.r)-2]
	} else {
		delim := getch()
		okdelim(delim)
		getrhs(s, delim, 'a')
		if nextc() == delim {
			getch()
		}
		atnl()
	}
Return:
	return s
}

func cmdlookup(c rune) int {
	for i := 0; i < len(cmdtab); i++ {
		if cmdtab[i].cmdc == c {
			return i
		}
	}
	return -1
}

func parsecmd(nest int) *Cmd {
	var cmd Cmd
	cmd.u.cmd = nil
	cmd.next = cmd.u.cmd
	cmd.re = nil
	cmd.num = 0
	cmd.flag = false
	cmd.addr = compoundaddr()
	if cmdskipbl() == -1 {
		return nil
	}
	c := getch()
	if c == -1 {
		return nil
	}
	cmd.cmdc = c
	if cmd.cmdc == 'c' && nextc() == 'd' { // sleazy two-character case
		getch() // the 'd'
		cmd.cmdc = 'c' | 0x100
	}
	i := cmdlookup(cmd.cmdc)
	var cp *Cmd
	if i >= 0 {
		if cmd.cmdc == '\n' {
			goto Return // let nl_cmd work it all out
		}
		ct := &cmdtab[i]
		if ct.defaddr == aNo && cmd.addr != nil {
			editerror("command takes no address")
		}
		if ct.count != 0 {
			cmd.num = getnum(int(ct.count))
		}
		if ct.regexp {
			// x without pattern -> .*\n, indicated by cmd.re==0
			// X without pattern is all files
			if (ct.cmdc != 'x' && ct.cmdc != 'X') || func() bool { c = nextc(); return c != ' ' && c != '\t' && c != '\n' }() {
				cmdskipbl()
				c = getch()
				if c == '\n' || c < 0 {
					editerror("no address")
				}
				okdelim(c)
				cmd.re = getregexp(c)
				if ct.cmdc == 's' {
					cmd.u.text = newstring(0)
					getrhs(cmd.u.text, c, 's')
					if nextc() == c {
						getch()
						if nextc() == 'g' {
							getch()
							cmd.flag = true
						}
					}

				}
			}
		}
		if ct.addr {
			cmd.u.mtaddr = simpleaddr()
			if cmd.u.mtaddr == nil {
				editerror("bad address")
			}
		}
		if ct.defcmd != 0 {
			if cmdskipbl() == '\n' {
				getch()
				cmd.u.cmd = newcmd()
				cmd.u.cmd.cmdc = ct.defcmd
			} else {
				cmd.u.cmd = parsecmd(nest)
				if cmd.u.cmd == nil {
					util.Fatal("defcmd")
				}
			}
		} else if ct.text {
			cmd.u.text = collecttext()
		} else if ct.token != "" {
			cmd.u.text = collecttoken(ct.token)
		} else {
			atnl()
		}
	} else {
		var ncp *Cmd
		switch cmd.cmdc {
		case '{':
			cp = nil
			for {
				if cmdskipbl() == '\n' {
					getch()
				}
				ncp = parsecmd(nest + 1)
				if cp != nil {
					cp.next = ncp
				} else {
					cmd.u.cmd = ncp
				}
				cp = ncp
				if cp == nil {
					break
				}
			}
		case '}':
			atnl()
			if nest == 0 {
				editerror("right brace with no left brace")
			}
			return nil
		default:
			editerror("unknown command %c", cmd.cmdc)
		}
	}
Return:
	cp = newcmd()
	*cp = cmd
	return cp
}

func getregexp(delim rune) *String {
	buf := allocstring(0)
	var c rune
	for i := 0; ; i++ {
		c = getch()
		if c == '\\' {
			if nextc() == delim {
				c = getch()
			} else if nextc() == '\\' {
				Straddc(buf, c)
				c = getch()
			}
		} else if c == delim || c == '\n' {
			break
		}
		if i >= bufs.RuneLen {
			editerror("regular expression too long")
		}
		Straddc(buf, c)
	}
	if c != delim && c != 0 {
		ungetch()
	}
	if len(buf.r) > 0 {
		patset = true
		freestring(lastpat)
		lastpat = buf
	} else {
		freestring(buf)
	}
	if len(lastpat.r) == 0 {
		editerror("no regular expression defined")
	}
	r := newstring(len(lastpat.r))
	copy(r.r[:len(lastpat.r)], lastpat.r)
	return r
}

func simpleaddr() *Addr {
	var addr Addr
	addr.num = 0
	addr.next = nil
	addr.u.left = nil
	switch cmdskipbl() {
	case '#':
		addr.typ = getch()
		addr.num = getnum(1)
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		addr.num = getnum(1)
		addr.typ = 'l'
	case '/',
		'?',
		'"':
		addr.typ = getch()
		addr.u.re = getregexp(addr.typ)
	case '.',
		'$',
		'+',
		'-',
		'\'':
		addr.typ = getch()
	default:
		return nil
	}
	addr.next = simpleaddr()
	if addr.next != nil {
		var nap *Addr
		switch addr.next.typ {
		case '.',
			'$',
			'\'':
			if addr.typ == '"' {
				break
			}
			fallthrough
		// fall through
		case '"':
			editerror("bad address syntax")
		case 'l',
			'#':
			if addr.typ == '"' {
				break
			}
			fallthrough
		// fall through
		case '/',
			'?':
			if addr.typ != '+' && addr.typ != '-' {
				// insert the missing '+'
				nap = newaddr()
				nap.typ = '+'
				nap.next = addr.next
				addr.next = nap
			}
		case '+',
			'-':
			break
		default:
			util.Fatal("simpleaddr")
		}
	}
	ap := newaddr()
	*ap = addr
	return ap
}

func compoundaddr() *Addr {
	var addr Addr
	addr.u.left = simpleaddr()
	addr.typ = cmdskipbl()
	if addr.typ != ',' && addr.typ != ';' {
		return addr.u.left
	}
	getch()
	addr.next = compoundaddr()
	next := addr.next
	if next != nil && (next.typ == ',' || next.typ == ';') && next.u.left == nil {
		editerror("bad address syntax")
	}
	ap := newaddr()
	*ap = addr
	return ap
}
