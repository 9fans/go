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
// #include "fns.h"

package main

import (
	"fmt"
	"runtime"
	"unicode/utf8"
)

// var sel Rangeset - in ecmd.go
var lastregexp []rune

// #undef class
// #define class regxclass /* some systems declare "class" in system headers */

/*
 * Machine Information
 */

type Inst struct {
	typ rune

	// former union
	subid  int
	rclass int
	right  *Inst

	// former union
	next *Inst
}

const NPROG = 1024

var program [NPROG]Inst
var progp int
var startinst *Inst  /* First inst. of program; might not be program[0] */
var bstartinst *Inst /* same for backwards machine */
var rechan chan *Inst

type Ilist struct {
	inst   *Inst
	se     Rangeset
	startp int
}

const NLIST = 127

var tl []Ilist
var nl []Ilist               /* This list, next list */
var list [2][NLIST + 1]Ilist /* +1 for trailing null */
var sempty Rangeset

/*
 * Actions and Tokens
 *
 *	0x10000xx are operators, value == precedence
 *	0x20000xx are tokens, i.e. operands for operators
 */
const (
	OPERATOR = 0x1000000    /* Bit set in all operators */
	START    = OPERATOR + 0 /* Start, used for marker on stack */
	RBRA     = OPERATOR + 1 /* Right bracket,  */
	LBRA     = OPERATOR + 2 /* Left bracket,  */
	OR       = OPERATOR + 3 /* Alternation, | */
	CAT      = OPERATOR + 4 /* Concatentation, implicit operator */
	STAR     = OPERATOR + 5 /* Closure, * */
	PLUS     = OPERATOR + 6 /* a+ == aa* */
	QUEST    = OPERATOR + 7 /* a? == a|nothing, i.e. 0 or 1 a's */
	ANY      = 0x2000000    /* Any character but newline, . */
	NOP      = ANY + 1      /* No operation, internal use only */
	BOL      = ANY + 2      /* Beginning of line, ^ */
	EOL      = ANY + 3      /* End of line, $ */
	CCLASS   = ANY + 4      /* Character class, [] */
	NCCLASS  = ANY + 5      /* Negated character class, [^] */
	END      = ANY + 0x77   /* Terminate: match found */

	ISATOR = OPERATOR
	ISAND  = ANY

	QUOTED = 0x4000000 /* Bit set for \-ed lex characters */
)

/*
 * Parser Information
 */

type Node struct {
	first *Inst
	last  *Inst
}

const NSTACK = 20

var andstack [NSTACK]Node
var andp int
var atorstack [NSTACK]int
var atorp int
var lastwasand bool /* Last token was operand */
var cursubid int
var subidstack [NSTACK]int
var subidp int
var backwards bool
var nbra int
var exprp []rune  /* pointer to next character in source expression */
const DCLASS = 10 /* allocation increment */
var class [][]rune
var negateclass bool

func rxinit() {
	rechan = make(chan *Inst)
}

func regerror(e string) {
	lastregexp = lastregexp[:0]
	warning(nil, "regexp: %s\n", e)
	rechan <- nil
	runtime.Goexit() // TODO(rsc)
}

func newinst(t rune) *Inst {
	if progp >= NPROG {
		regerror("expression too long")
	}
	p := &program[progp]
	progp++
	*p = Inst{}
	p.typ = t
	return p
}

func realcompile(s []rune) {
	startlex(s)
	atorp = 0
	andp = 0
	subidp = 0
	cursubid = 0
	lastwasand = false
	/* Start with a low priority operator to prime parser */
	pushator(START - 1)
	for {
		token := lex()
		if token == END {
			break
		}
		if token&ISATOR == OPERATOR {
			operator(int(token))
		} else {
			operand(token)
		}
	}
	/* Close with a low priority operator */
	evaluntil(START)
	/* Force END */
	operand(END)
	evaluntil(START)
	if nbra != 0 {
		regerror("unmatched `('")
	}
	andp-- /* points to first and only operand */
	rechan <- andstack[andp].first
}

func rxcompile(r []rune) bool {
	if runesEqual(lastregexp, r) {
		return true
	}
	lastregexp = lastregexp[:0]
	for _, c := range class {
		// free(c)
		_ = c
	}
	class = class[:0]
	progp = 0
	backwards = false
	bstartinst = nil
	go realcompile(r)
	startinst = <-rechan
	if startinst == nil {
		return false
	}
	optimize(0)
	oprogp := progp
	backwards = true
	go realcompile(r)
	bstartinst = <-rechan
	if bstartinst == nil {
		return false
	}
	optimize(oprogp)
	lastregexp = append(lastregexp[:0], r...)
	return true
}

func operand(t rune) {
	if lastwasand {
		operator(CAT) /* catenate is implicit */
	}
	i := newinst(t)
	if t == CCLASS {
		if negateclass {
			i.typ = NCCLASS /* UGH */
		}
		i.rclass = len(class) - 1 /* UGH */
	}
	pushand(i, i)
	lastwasand = true
}

func operator(t int) {
	if t == RBRA {
		nbra--
		if nbra < 0 {
			regerror("unmatched `)'")
		}
	}
	if t == LBRA {
		/*
		 *		if(++cursubid >= NSUBEXP)
		 *			regerror(Esubexp);
		 */
		cursubid++ /* silently ignored */
		nbra++
		if lastwasand {
			operator(CAT)
		}
	} else {
		evaluntil(t)
	}
	if t != RBRA {
		pushator(t)
	}
	lastwasand = false
	if t == STAR || t == QUEST || t == PLUS || t == RBRA {
		lastwasand = true /* these look like operands */
	}
}

func pushand(f *Inst, l *Inst) {
	if andp >= len(andstack) {
		error_("operand stack overflow")
	}
	a := &andstack[andp]
	andp++
	a.first = f
	a.last = l
}

func pushator(t int) {
	if atorp >= NSTACK {
		error_("operator stack overflow")
	}
	atorstack[atorp] = t
	atorp++
	if cursubid >= NRange {
		subidstack[subidp] = -1
		subidp++
	} else {
		subidstack[subidp] = cursubid
		subidp++
	}
}

func popand(op int) *Node {
	if andp <= 0 {
		if op != 0 {
			regerror(fmt.Sprintf("missing operand for %c", op))
		} else {
			regerror("malformed regexp")
		}
	}
	andp--
	return &andstack[andp]
}

func popator() int {
	if atorp <= 0 {
		error_("operator stack underflow")
	}
	subidp--
	atorp--
	return atorstack[atorp]
}

func evaluntil(pri int) {
	for pri == RBRA || atorstack[atorp-1] >= pri {
		var inst2 *Inst
		var inst1 *Inst
		var t *Node
		var op2 *Node
		var op1 *Node
		switch popator() {
		case LBRA:
			op1 = popand('(')
			inst2 = newinst(RBRA)
			inst2.subid = subidstack[subidp]
			op1.last.next = inst2
			inst1 = newinst(LBRA)
			inst1.subid = subidstack[subidp]
			inst1.next = op1.first
			pushand(inst1, inst2)
			return /* must have been RBRA */
		default:
			error_("unknown regexp operator")
		case OR:
			op2 = popand('|')
			op1 = popand('|')
			inst2 = newinst(NOP)
			op2.last.next = inst2
			op1.last.next = inst2
			inst1 = newinst(OR)
			inst1.right = op1.first
			inst1.next = op2.first
			pushand(inst1, inst2)
		case CAT:
			op2 = popand(0)
			op1 = popand(0)
			if backwards && op2.first.typ != END {
				t = op1
				op1 = op2
				op2 = t
			}
			op1.last.next = op2.first
			pushand(op1.first, op2.last)
		case STAR:
			op2 = popand('*')
			inst1 = newinst(OR)
			op2.last.next = inst1
			inst1.right = op2.first
			pushand(inst1, inst1)
		case PLUS:
			op2 = popand('+')
			inst1 = newinst(OR)
			op2.last.next = inst1
			inst1.right = op2.first
			pushand(op2.first, inst1)
		case QUEST:
			op2 = popand('?')
			inst1 = newinst(OR)
			inst2 = newinst(NOP)
			inst1.next = inst2
			inst1.right = op2.first
			op2.last.next = inst2
			pushand(inst1, inst2)
		}
	}
}

func optimize(start int) {
	for i := start; program[i].typ != END; i++ {
		inst := &program[i]
		target := inst.next
		for target.typ == NOP {
			target = target.next
		}
		inst.next = target
	}
}

func startlex(s []rune) {
	exprp = s
	nbra = 0
}

func lex() rune {
	if len(exprp) == 0 {
		return END
	}

	c := exprp[0]
	exprp = exprp[1:]
	switch c {
	case '\\':
		if len(exprp) > 0 {
			c = exprp[0]
			exprp = exprp[1:]
			if c == 'n' {
				c = '\n'
			}
		}
	case '*':
		c = STAR
	case '?':
		c = QUEST
	case '+':
		c = PLUS
	case '|':
		c = OR
	case '.':
		c = ANY
	case '(':
		c = LBRA
	case ')':
		c = RBRA
	case '^':
		c = BOL
	case '$':
		c = EOL
	case '[':
		c = CCLASS
		bldcclass()
	}
	return c
}

func nextrec() rune {
	if len(exprp) == 0 || (len(exprp) == 1 && exprp[0] == '\\') {
		regerror("malformed `[]'")
	}
	if exprp[0] == '\\' {
		exprp = exprp[1:]
		if exprp[0] == 'n' {
			exprp = exprp[1:]
			return '\n'
		}
		c := exprp[0]
		exprp = exprp[1:]
		return c | QUOTED
	}
	c := exprp[0]
	exprp = exprp[1:]
	return c
}

func bldcclass() {
	var classp []rune
	/* we have already seen the '[' */
	if exprp[0] == '^' { /* don't match newline in negate case */
		classp = append(classp, '\n')
		negateclass = true
		exprp = exprp[1:]
	} else {
		negateclass = false
	}
	for {
		c1 := nextrec()
		if c1 == ']' {
			break
		}
		if c1 == '-' {
			regerror("malformed `[]'")
		}
		if exprp[0] == '-' {
			exprp = exprp[1:] /* eat '-' */
			c2 := nextrec()
			if c2 == ']' {
				regerror("malformed `[]'")
			}
			classp = append(classp, utf8.MaxRune, c1, c2)
		} else {
			classp = append(classp, c1&^QUOTED)
		}
	}
	class = append(class, classp)
}

func classmatch(classno int, c rune, negate bool) bool {
	p := class[classno]
	for len(p) > 0 {
		if p[0] == utf8.MaxRune {
			if p[1] <= c && c <= p[2] {
				return !negate
			}
			p = p[3:]
		} else {
			r := p[0]
			p = p[1:]
			if r == c {
				return !negate
			}
		}
	}
	return negate
}

/*
 * Note optimization in addinst:
 * 	*l must be pending when addinst called; if *l has been looked
 *		at already, the optimization is a bug.
 */
func addinst(l []Ilist, inst *Inst, sep *Rangeset) int {
	i := 0
	p := &l[i]
	for p.inst != nil {
		if p.inst == inst {
			if sep.r[0].q0 < p.se.r[0].q1 {
				p.se = *sep /* this would be bug */
			}
			return 0 /* It's already there */
		}
		i++
		p = &l[i]
	}
	p.inst = inst
	p.se = *sep
	l[i+1].inst = nil
	return 1
}

func rxnull() bool {
	return startinst == nil || bstartinst == nil
}

/* either t!=nil or r!=nil, and we match the string in the appropriate place */
func rxexecute(t *Text, r []rune, startp int, eof int, rp *Rangeset) bool {
	flag := 0
	p := startp
	startchar := rune(0)
	wrapped := 0
	nnl := 0
	if startinst.typ < OPERATOR {
		startchar = startinst.typ
	}
	list[1][0].inst = nil
	list[0][0].inst = list[1][0].inst
	sel.r[0].q0 = -1
	var nc int
	if t != nil {
		nc = t.file.b.nc
	} else {
		nc = len(r)
	}
	/* Execute machine once for each character */
	for ; ; p++ {
	doloop:
		var c rune
		if p >= eof || p >= nc {
			tmp22 := wrapped
			wrapped++
			switch tmp22 {
			case 0, /* let loop run one more click */
				2:
				break
			case 1: /* expired; wrap to beginning */
				if sel.r[0].q0 >= 0 || eof != Infinity {
					goto Return
				}
				list[1][0].inst = nil
				list[0][0].inst = list[1][0].inst
				p = 0
				goto doloop
			default:
				goto Return
			}
			c = 0
		} else {
			if ((wrapped != 0 && p >= startp) || sel.r[0].q0 > 0) && nnl == 0 {
				break
			}
			if t != nil {
				c = textreadc(t, p)
			} else {
				c = r[p]
			}
		}
		/* fast check for first char */
		if startchar != 0 && nnl == 0 && c != startchar {
			continue
		}
		tl = list[flag][:]
		flag ^= 1
		nl = list[flag][:]
		nl[0].inst = nil
		ntl := nnl
		nnl = 0
		if sel.r[0].q0 < 0 && (wrapped == 0 || p < startp || startp == eof) {
			/* Add first instruction to this list */
			sempty.r[0].q0 = p
			if addinst(tl, startinst, &sempty) != 0 {
				ntl++
				if ntl >= NLIST {
					goto Overflow
				}
			}
		}
		/* Execute machine until this list is empty */
		for tlp := 0; ; tlp++ {
			inst := tl[tlp].inst
			if inst == nil {
				break
			} /* assignment = */
		Switchstmt:
			switch inst.typ {
			default: /* regular character */
				if inst.typ == c {
					goto Addinst
				}
			case LBRA:
				if inst.subid >= 0 {
					tl[tlp].se.r[inst.subid].q0 = p
				}
				inst = inst.next
				goto Switchstmt
			case RBRA:
				if inst.subid >= 0 {
					tl[tlp].se.r[inst.subid].q1 = p
				}
				inst = inst.next
				goto Switchstmt
			case ANY:
				if c != '\n' {
					goto Addinst
				}
			case BOL:
				if p == 0 || (t != nil && textreadc(t, p-1) == '\n') || (r != nil && r[p-1] == '\n') {
					inst = inst.next
					goto Switchstmt
				}
			case EOL:
				if c == '\n' {
					inst = inst.next
					goto Switchstmt
				}
			case CCLASS:
				if c >= 0 && classmatch(inst.rclass, c, false) {
					goto Addinst
				}
			case NCCLASS:
				if c >= 0 && classmatch(inst.rclass, c, true) {
					goto Addinst
				}
			/* evaluate right choice later */
			case OR:
				if addinst(tl, inst.right, &tl[tlp].se) != 0 {
					ntl++
					if ntl >= NLIST {
						goto Overflow
					}
				}
				/* efficiency: advance and re-evaluate */
				inst = inst.next
				goto Switchstmt
			case END: /* Match! */
				tl[tlp].se.r[0].q1 = p
				newmatch(&tl[tlp].se)
			}
			continue

		Addinst:
			if addinst(nl, inst.next, &tl[tlp].se) != 0 {
				nnl++
				if nnl >= NLIST {
					goto Overflow
				}
			}

		}
	}
Return:
	*rp = sel
	return sel.r[0].q0 >= 0

Overflow:
	warning(nil, "regexp list overflow\n")
	sel.r[0].q0 = -1
	goto Return
}

func newmatch(sp *Rangeset) {
	if sel.r[0].q0 < 0 || sp.r[0].q0 < sel.r[0].q0 || (sp.r[0].q0 == sel.r[0].q0 && sp.r[0].q1 > sel.r[0].q1) {
		sel = *sp
	}
}

func rxbexecute(t *Text, startp int, rp *Rangeset) bool {
	flag := 0
	nnl := 0
	wrapped := 0
	p := startp
	startchar := rune(0)
	if bstartinst.typ < OPERATOR {
		startchar = bstartinst.typ
	}
	list[1][0].inst = nil
	list[0][0].inst = list[1][0].inst
	sel.r[0].q0 = -1
	/* Execute machine once for each character, including terminal NUL */
	for ; ; p-- {
	doloop:
		var c rune
		if p <= 0 {
			tmp23 := wrapped
			wrapped++
			switch tmp23 {
			case 0, /* let loop run one more click */
				2:
				break
			case 1: /* expired; wrap to end */
				if sel.r[0].q0 >= 0 {
					goto Return
				}
				list[1][0].inst = nil
				list[0][0].inst = list[1][0].inst
				p = t.file.b.nc
				goto doloop
			case 3:
				fallthrough
			default:
				goto Return
			}
			c = 0
		} else {
			if ((wrapped != 0 && p <= startp) || sel.r[0].q0 > 0) && nnl == 0 {
				break
			}
			c = textreadc(t, p-1)
		}
		/* fast check for first char */
		if startchar != 0 && nnl == 0 && c != startchar {
			continue
		}
		tl = list[flag][:]
		flag ^= 1
		nl = list[flag][:]
		nl[0].inst = nil
		ntl := nnl
		nnl = 0
		if sel.r[0].q0 < 0 && (wrapped == 0 || p > startp) {
			/* Add first instruction to this list */
			/* the minus is so the optimizations in addinst work */
			sempty.r[0].q0 = -p
			if addinst(tl, bstartinst, &sempty) != 0 {
				ntl++
				if ntl >= NLIST {
					goto Overflow
				}
			}
		}
		/* Execute machine until this list is empty */
		for tlp := 0; ; tlp++ {
			inst := tl[tlp].inst
			if inst == nil {
				break
			} /* assignment = */
		Switchstmt:
			switch inst.typ {
			default: /* regular character */
				if inst.typ == c {
					goto Addinst
				}
			case LBRA:
				if inst.subid >= 0 {
					tl[tlp].se.r[inst.subid].q0 = p
				}
				inst = inst.next
				goto Switchstmt
			case RBRA:
				if inst.subid >= 0 {
					tl[tlp].se.r[inst.subid].q1 = p
				}
				inst = inst.next
				goto Switchstmt
			case ANY:
				if c != '\n' {
					goto Addinst
				}
			case BOL:
				if c == '\n' || p == 0 {
					inst = inst.next
					goto Switchstmt
				}
			case EOL:
				if p < t.file.b.nc && textreadc(t, p) == '\n' {
					inst = inst.next
					goto Switchstmt
				}
			case CCLASS:
				if c > 0 && classmatch(inst.rclass, c, false) {
					goto Addinst
				}
			case NCCLASS:
				if c > 0 && classmatch(inst.rclass, c, true) {
					goto Addinst
				}
			/* evaluate right choice later */
			case OR:
				if addinst(tl, inst.right, &tl[tlp].se) != 0 {
					ntl++
					if ntl >= NLIST {
						goto Overflow
					}
				}
				/* efficiency: advance and re-evaluate */
				inst = inst.next
				goto Switchstmt
			case END: /* Match! */
				tl[tlp].se.r[0].q0 = -tl[tlp].se.r[0].q0 /* minus sign */
				tl[tlp].se.r[0].q1 = p
				bnewmatch(&tl[tlp].se)
			}
			continue

		Addinst:
			if addinst(nl, inst.next, &tl[tlp].se) != 0 {
				nnl++
				if nnl >= NLIST {
					goto Overflow
				}
			}

		}
	}
Return:
	*rp = sel
	return sel.r[0].q0 >= 0

Overflow:
	warning(nil, "regexp list overflow\n")
	sel.r[0].q0 = -1
	goto Return
}

func bnewmatch(sp *Rangeset) {
	if sel.r[0].q0 < 0 || sp.r[0].q0 > sel.r[0].q1 || (sp.r[0].q0 == sel.r[0].q1 && sp.r[0].q1 < sel.r[0].q0) {
		for i := 0; i < NRange; i++ { /* note the reversal; q0<=q1 */
			sel.r[i].q0 = sp.r[i].q1
			sel.r[i].q1 = sp.r[i].q0
		}
	}
}

func runesEqual(x, y []rune) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}
