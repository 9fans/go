// #include "sam.h"
// #include "parse.h"

package main

import "fmt"

var addr Address
var lastpat String
var patset bool
var menu *File

func address(ap *Addr, a Address, sign int) Address {
	f := a.f
	for {
		var a1, a2 Address
		switch ap.type_ {
		case 'l':
			a = lineaddr(ap.num, a, sign)

		case '#':
			a = charaddr(ap.num, a, sign)

		case '.':
			a = f.dot

		case '$':
			a.r.p2 = f.b.nc
			a.r.p1 = a.r.p2

		case '\'':
			a.r = f.mark

		case '?':
			sign = -sign
			if sign == 0 {
				sign = -1
			}
			fallthrough
		case '/':
			start := a.r.p2
			if sign < 0 {
				start = a.r.p1
			}
			nextmatch(f, ap.are, start, sign)
			a.r = sel.p[0]

		case '"':
			a = matchfile(ap.are).dot
			f = a.f
			if f.unread {
				load(f)
			}

		case '*':
			a.r.p1 = 0
			a.r.p2 = f.b.nc
			return a

		case ',',
			';':
			if ap.left != nil {
				a1 = address(ap.left, a, 0)
			} else {
				a1.f = a.f
				a1.r.p2 = 0
				a1.r.p1 = a1.r.p2
			}
			if ap.type_ == ';' {
				f = a1.f
				a = a1
				f.dot = a1
			}
			if ap.next != nil {
				a2 = address(ap.next, a, 0)
			} else {
				a2.f = a.f
				a2.r.p2 = f.b.nc
				a2.r.p1 = a2.r.p2
			}
			if a1.f != a2.f {
				error_(Eorder)
			}
			a.f = a1.f
			a.r.p1 = a1.r.p1
			a.r.p2 = a2.r.p2
			if a.r.p2 < a.r.p1 {
				error_(Eorder)
			}
			return a

		case '+',
			'-':
			sign = 1
			if ap.type_ == '-' {
				sign = -1
			}
			if ap.next == nil || ap.next.type_ == '+' || ap.next.type_ == '-' {
				a = lineaddr(1, a, sign)
			}
		default:
			panic_("address")
			return a
		}
		ap = ap.next
		if ap == nil { /* assign = */
			break
		}
	}
	return a
}

func nextmatch(f *File, r *String, p Posn, sign int) {
	compile(r)
	if sign >= 0 {
		if !execute(f, p, INFINITY) {
			error_(Esearch)
		}
		if sel.p[0].p1 == sel.p[0].p2 && sel.p[0].p1 == p {
			p++
			if p > f.b.nc {
				p = 0
			}
			if !execute(f, p, INFINITY) {
				panic_("address")
			}
		}
	} else {
		if !bexecute(f, p) {
			error_(Esearch)
		}
		if sel.p[0].p1 == sel.p[0].p2 && sel.p[0].p2 == p {
			p--
			if p < 0 {
				p = f.b.nc
			}
			if !bexecute(f, p) {
				panic_("address")
			}
		}
	}
}

func matchfile(r *String) *File {
	var match *File
	for _, f := range file {
		if f == cmd {
			continue
		}
		if filematch(f, r) {
			if match != nil {
				error_(Emanyfiles)
			}
			match = f
		}
	}
	if match == nil {
		error_(Efsearch)
	}
	return match
}

func filematch(f *File, r *String) bool {
	ch := func(s string, b bool) byte {
		if b {
			return s[1]
		}
		return s[0]
	}
	buf := fmt.Sprintf("%c%c%c %s\n", ch(" '", f.mod), ch("-+", f.rasp != nil), ch(" .", f == curfile), f.name)
	t := tmpcstr(buf)
	Strduplstr(&genstr, t)
	freetmpstr(t)
	/* A little dirty... */
	if menu == nil {
		menu = fileopen()
	}
	bufreset(&menu.b)
	bufinsert(&menu.b, 0, genstr.s)
	compile(r)
	return execute(menu, 0, menu.b.nc)
}

func charaddr(l Posn, addr Address, sign int) Address {
	if sign == 0 {
		addr.r.p2 = l
		addr.r.p1 = addr.r.p2
	} else if sign < 0 {
		addr.r.p1 -= l
		addr.r.p2 = addr.r.p1
	} else if sign > 0 {
		addr.r.p2 += l
		addr.r.p1 = addr.r.p2
	}
	if addr.r.p1 < 0 || addr.r.p2 > addr.f.b.nc {
		error_(Erange)
	}
	return addr
}

func lineaddr(l Posn, addr Address, sign int) Address {
	debug("lineaddr")
	f := addr.f
	var a Address
	a.f = f
	if sign >= 0 {
		debug("a1")
		var p Posn
		if l == 0 {
			debug("a2")
			if sign == 0 || addr.r.p2 == 0 {
				a.r.p2 = 0
				a.r.p1 = a.r.p2
				return a
			}
			a.r.p1 = addr.r.p2
			p = addr.r.p2 - 1
		} else {
			debug("a3")
			var n int
			if sign == 0 || addr.r.p2 == 0 {
				p = Posn(0)
				n = 1
			} else {
				p = addr.r.p2 - 1
				if filereadc(f, p) == '\n' {
					n = 1
				}
				p++
			}
			for n < l {
				if p >= f.b.nc {
					error_(Erange)
				}
				tmp3 := p
				p++
				if filereadc(f, tmp3) == '\n' {
					n++
				}
			}
			a.r.p1 = p
		}
		for p < f.b.nc {
			c := filereadc(f, p)
			p++
			if c == '\n' {
				break
			}
		}
		a.r.p2 = p
	} else {
		p := addr.r.p1
		if l == 0 {
			a.r.p2 = addr.r.p1
		} else {
			for n := 0; n < l; { /* always runs once */
				if p == 0 {
					n++
					if n != l {
						error_(Erange)
					}
				} else {
					c := filereadc(f, p-1)
					if c != '\n' || func() bool { n++; return n != l }() {
						p--
					}
				}
			}
			a.r.p2 = p
			if p > 0 {
				p--
			}
		}
		for p > 0 && filereadc(f, p-1) != '\n' { /* lines start after a newline */
			p--
		}
		a.r.p1 = p
	}
	return a
}
