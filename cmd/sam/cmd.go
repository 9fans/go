// #include "sam.h"
// #include "parse.h"

package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

var linex = "\n"
var wordx = " \t\n"

var cmdtab []Cmdtab = nil

func init() { cmdtab = cmdtab1 } // break init loop

var cmdtab1 = []Cmdtab{
	/*	cmdc	text	regexp	addr	defcmd	defaddr	count	token	 fn	*/
	{'\n', false, false, false, 0, aDot, 0, "", nl_cmd},
	{'a', true, false, false, 0, aDot, 0, "", a_cmd},
	{'b', false, false, false, 0, aNo, 0, linex, b_cmd},
	{'B', false, false, false, 0, aNo, 0, linex, b_cmd},
	{'c', true, false, false, 0, aDot, 0, "", c_cmd},
	{'d', false, false, false, 0, aDot, 0, "", d_cmd},
	{'D', false, false, false, 0, aNo, 0, linex, D_cmd},
	{'e', false, false, false, 0, aNo, 0, wordx, e_cmd},
	{'f', false, false, false, 0, aNo, 0, wordx, f_cmd},
	{'g', false, true, false, 'p', aDot, 0, "", g_cmd},
	{'i', true, false, false, 0, aDot, 0, "", i_cmd},
	{'k', false, false, false, 0, aDot, 0, "", k_cmd},
	{'m', false, false, true, 0, aDot, 0, "", m_cmd},
	{'n', false, false, false, 0, aNo, 0, "", n_cmd},
	{'p', false, false, false, 0, aDot, 0, "", p_cmd},
	{'q', false, false, false, 0, aNo, 0, "", q_cmd},
	{'r', false, false, false, 0, aDot, 0, wordx, e_cmd},
	{'s', false, true, false, 0, aDot, 1, "", s_cmd},
	{'t', false, false, true, 0, aDot, 0, "", m_cmd},
	{'u', false, false, false, 0, aNo, 2, "", u_cmd},
	{'v', false, true, false, 'p', aDot, 0, "", g_cmd},
	{'w', false, false, false, 0, aAll, 0, wordx, w_cmd},
	{'x', false, true, false, 'p', aDot, 0, "", x_cmd},
	{'y', false, true, false, 'p', aDot, 0, "", x_cmd},
	{'X', false, true, false, 'f', aNo, 0, "", X_cmd},
	{'Y', false, true, false, 'f', aNo, 0, "", X_cmd},
	{'!', false, false, false, 0, aNo, 0, linex, plan9_cmd},
	{'>', false, false, false, 0, aDot, 0, linex, plan9_cmd},
	{'<', false, false, false, 0, aDot, 0, linex, plan9_cmd},
	{'|', false, false, false, 0, aDot, 0, linex, plan9_cmd},
	{'=', false, false, false, 0, aDot, 0, linex, eq_cmd},
	{'c' | 0x100, false, false, false, 0, aNo, 0, wordx, cd_cmd},
}

var line = make([]rune, 0, BLOCKSIZE)
var linep int // index in line

var termline [BLOCKSIZE]rune
var terminp int  // write index in termline
var termoutp int // read index in termline

var cmdlist []*Cmd
var addrlist []*Addr
var relist []*String
var stringlist []*String

var eof bool

func resetcmd() {
	linep = 0
	line = line[:0]
	termoutp = 0
	terminp = 0
	freecmd()
}

func inputc() rune {
Again:
	nbuf := 0
	var r rune
	if downloaded {
		for termoutp == terminp {
			cmdupdate()
			if patset {
				tellpat()
			}
			for termlocked > 0 {
				outT0(Hunlock)
				termlocked--
			}
			if !rcv() {
				return -1
			}
		}
		r = termline[termoutp]
		termoutp++
		if termoutp == terminp {
			termoutp = 0
			terminp = 0
		}
	} else {
		var buf [utf8.UTFMax]byte
		for {
			n, err := os.Stdin.Read(buf[nbuf : nbuf+1])
			if err != nil || n <= 0 {
				return -1
			}
			nbuf += n
			if utf8.FullRune(buf[:nbuf]) {
				break
			}
		}
		r, _ = utf8.DecodeRune(buf[:])
	}
	if r == 0 {
		warn(Wnulls)
		goto Again
	}
	return r
}

var Dflag bool

func debug(format string, args ...interface{}) {
	if !Dflag {
		return
	}
	s := fmt.Sprintf(format, args...)
	if !strings.HasSuffix(s, "\n") {
		s += "\n"
	}
	os.Stderr.WriteString(s)
}

func inputline() int {
	/*
	 * Could set linep = line and i = 0 here and just
	 * error(Etoolong) below, but this way we keep
	 * old input buffer history around for a while.
	 * This is useful only for debugging.
	 */
	i := linep
	for {
		c := inputc()
		if c <= 0 {
			return -1
		}
		if i >= cap(line) {
			if linep == 0 {
				error_(Etoolong)
			}
			i = copy(line[0:], line[linep:])
			linep = 0
		}
		line = append(line, c)
		if c == '\n' {
			break
		}
	}
	debug("input: %q\n", string(line[linep:]))
	return 1
}

func getch() rune {
	if eof {
		return -1
	}
	if linep == len(line) && inputline() < 0 {
		eof = true
		return -1
	}
	r := line[linep]
	debug("getch %d %q\n", linep, r)
	linep++
	return r
}

func nextc() rune {
	if linep >= len(line) {
		return -1
	}
	return line[linep]
}

func ungetch() {
	linep--
	if linep < 0 {
		panic_("ungetch")
	}
	// debug("ungetch %d %q\n", linep, line[linep])
}

func getnum(signok int) Posn {
	n := 0
	sign := 1
	if signok > 1 && nextc() == '-' {
		sign = -1
		getch()
	}
	c := nextc()
	if c < '0' || '9' < c { /* no number defaults to 1 */
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

func skipbl() rune {
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

func termcommand() {
	for p := cmdpt; p < cmd.b.nc; p++ {
		if terminp >= len(termline) {
			cmdpt = cmd.b.nc
			error_(Etoolong)
		}
		termline[terminp] = filereadc(cmd, p)
		terminp++
	}
	cmdpt = cmd.b.nc
}

func cmdloop() {
	for {
		if !downloaded && curfile != nil && curfile.unread {
			load(curfile)
		}
		cmdp := parsecmd(0)
		if cmdp == nil {
			if downloaded {
				rescue()
				os.Exit(1) // "eof"
			}
			break
		}
		ocurfile := curfile
		loaded := curfile != nil && !curfile.unread
		if !cmdexec(curfile, cmdp) {
			break
		}
		freecmd()
		cmdupdate()
		update()
		if downloaded && curfile != nil && (ocurfile != curfile || (!loaded && !curfile.unread)) {
			outTs(Hcurrent, curfile.tag)
		}
		/* don't allow type ahead on files that aren't bound */
		if downloaded && curfile != nil && curfile.rasp == nil {
			terminp = termoutp
		}
	}
}

func newcmd() *Cmd {
	p := new(Cmd)
	cmdlist = append(cmdlist, p)
	return p
}

func newaddr() *Addr {
	p := new(Addr)
	addrlist = append(addrlist, p)
	return p
}

func newre() *String {
	p := new(String)
	relist = append(relist, p)
	return p
}

func newstring() *String {
	p := new(String)
	stringlist = append(stringlist, p)
	return p
}

func freecmd() {
	for _, c := range cmdlist {
		// free(c)
		_ = c
	}
	cmdlist = cmdlist[:0]
	for _, a := range addrlist {
		// free(a)
		_ = a
	}
	addrlist = addrlist[:0]
	for _, s := range relist {
		Strclose(s)
		// free(s)
	}
	relist = relist[:0]
	for _, s := range stringlist {
		Strclose(s)
		// free(s)
	}
	stringlist = stringlist[:0]
}

func lookup(c rune) int {
	for i := range cmdtab {
		if cmdtab[i].cmdc == c {
			return i
		}
	}
	return -1
}

func okdelim(c rune) {
	if c == '\\' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') {
		error_c(Edelim, c)
	}
}

func atnl() {
	skipbl()
	if c := getch(); c != '\n' {
		error_(Enewline)
	}
}

func getrhs(s *String, delim rune, cmd int) {
	for {
		c := getch()
		if !(c > 0 && c != delim) || !(c != '\n') {
			break
		}
		if c == '\\' {
			c = getch()
			if c <= 0 {
				error_(Ebadrhs)
			}
			if c == '\n' {
				ungetch()
				c = '\\'
			} else if c == 'n' {
				c = '\n'
			} else if c != delim && (cmd == 's' || c != '\\') { /* s does its own */
				Straddc(s, '\\')
			}
		}
		Straddc(s, c)
	}
	ungetch() /* let client read whether delimeter, '\n' or whatever */
}

func collecttoken(end string) *String {
	s := newstring()
	var c rune
	for {
		c = nextc()
		if !(c == ' ') && !(c == '\t') {
			break
		}
		Straddc(s, getch()) /* blanks significant for getname() */
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
	s := newstring()
	if skipbl() == '\n' {
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
				Straddc(s, c)
			}
			i++
			Straddc(s, '\n')
			if c < 0 {
				goto Return
			}
			if !(s.s[begline] != '.') && !(s.s[begline+1] != '\n') {
				break
			}
		}
		Strdelete(s, len(s.s)-2, len(s.s))
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

func parsecmd(nest int) *Cmd {
	var cmd Cmd
	cmd.ccmd = nil
	cmd.next = cmd.ccmd
	cmd.re = nil
	cmd.num = 0
	cmd.flag = false
	cmd.addr = compoundaddr()
	if skipbl() == -1 {
		return nil
	}
	c := getch()
	if c == -1 {
		return nil
	}
	cmd.cmdc = c
	if cmd.cmdc == 'c' && nextc() == 'd' { /* sleazy two-character case */
		getch() /* the 'd' */
		cmd.cmdc = 'c' | 0x100
	}
	i := lookup(cmd.cmdc)
	var cp *Cmd
	if i >= 0 {
		if cmd.cmdc == '\n' {
			goto Return /* let nl_cmd work it all out */
		}
		ct := &cmdtab[i]
		if ct.defaddr == aNo && cmd.addr != nil {
			error_(Enoaddr)
		}
		if ct.count != 0 {
			cmd.num = getnum(int(ct.count))
		}
		if ct.regexp {
			/* x without pattern -> .*\n, indicated by cmd.re==0 */
			/* X without pattern is all files */
			if (ct.cmdc != 'x' && ct.cmdc != 'X') || func() bool { c = nextc(); return c != ' ' && c != '\t' && c != '\n' }() {
				skipbl()
				c = getch()
				if c == '\n' || c < 0 {
					error_(Enopattern)
				}
				okdelim(c)
				cmd.re = getregexp(c)
				if ct.cmdc == 's' {
					cmd.ctext = newstring()
					getrhs(cmd.ctext, c, 's')
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
			cmd.caddr = simpleaddr()
			if cmd.caddr == nil {
				error_(Eaddress)
			}
		}
		if ct.defcmd != 0 {
			if skipbl() == '\n' {
				getch()
				cmd.ccmd = newcmd()
				cmd.ccmd.cmdc = ct.defcmd
			} else {
				cmd.ccmd = parsecmd(nest)
				if cmd.ccmd == nil {
					panic_("defcmd")
				}
			}
		} else if ct.text {
			cmd.ctext = collecttext()
		} else if ct.token != "" {
			cmd.ctext = collecttoken(ct.token)
		} else {
			atnl()
		}
	} else {
		var ncp *Cmd
		switch cmd.cmdc {
		case '{':
			cp = nil
			for {
				if skipbl() == '\n' {
					getch()
				}
				ncp = parsecmd(nest + 1)
				if cp != nil {
					cp.next = ncp
				} else {
					cmd.ccmd = ncp
				}
				cp = ncp
				if cp == nil {
					break
				}
			}
		case '}':
			atnl()
			if nest == 0 {
				error_(Enolbrace)
			}
			return nil
		default:
			error_c(Eunk, cmd.cmdc)
		}
	}
Return:
	cp = newcmd()
	*cp = cmd
	return cp /* BUGGERED */
}

func getregexp(delim rune) *String {
	r := newre()
	var c rune
	for Strzero(&genstr); ; Straddc(&genstr, c) {
		c = getch()
		if c == '\\' {
			if nextc() == delim {
				c = getch()
			} else if nextc() == '\\' {
				Straddc(&genstr, c)
				c = getch()
			}
		} else if c == delim || c == '\n' {
			break
		}
	}
	if c != delim && c != 0 {
		ungetch()
	}
	if len(genstr.s) > 0 {
		patset = true
		Strduplstr(&lastpat, &genstr)
	}
	if len(lastpat.s) <= 0 {
		error_(Epattern)
	}
	Strduplstr(r, &lastpat)
	return r
}

func simpleaddr() *Addr {
	var addr Addr
	addr.next = nil
	addr.left = nil
	addr.num = 0
	switch skipbl() {
	case '#':
		addr.type_ = getch()
		addr.num = getnum(1)
	case '0',
		'1',
		'2',
		'3',
		'4',
		'5',
		'6',
		'7',
		'8',
		'9':
		addr.num = getnum(1)
		addr.type_ = 'l'
	case '/',
		'?',
		'"':
		addr.type_ = getch()
		addr.are = getregexp(addr.type_)
	case '.',
		'$',
		'+',
		'-',
		'\'':
		addr.type_ = getch()
	default:
		return nil
	}
	addr.next = simpleaddr()
	if addr.next != nil {
		var nap *Addr
		switch addr.next.type_ {
		case '.',
			'$',
			'\'':
			if addr.type_ == '"' {
				break
			}
			fallthrough
		/* fall through */
		case '"':
			error_(Eaddress)
		case 'l',
			'#':
			if addr.type_ == '"' {
				break
			}
			fallthrough
		/* fall through */
		case '/',
			'?':
			if addr.type_ != '+' && addr.type_ != '-' {
				/* insert the missing '+' */
				nap = newaddr()
				nap.type_ = '+'
				nap.next = addr.next
				addr.next = nap
			}
		case '+',
			'-':
			break
		default:
			panic_("simpleaddr")
		}
	}
	ap := newaddr()
	*ap = addr
	return ap
}

func compoundaddr() *Addr {
	var addr Addr
	addr.left = simpleaddr()
	addr.type_ = skipbl()
	if addr.type_ != ',' && addr.type_ != ';' {
		return addr.left
	}
	getch()
	addr.next = compoundaddr()
	next := addr.next
	if next != nil && (next.type_ == ',' || next.type_ == ';') && next.left == nil {
		error_(Eaddress)
	}
	ap := newaddr()
	*ap = addr
	return ap
}
