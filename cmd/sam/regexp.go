package main

import "unicode/utf8"

var sel Rangeset
var lastregexp String

/*
 * Machine Information
 */

type Inst struct {
	type_ int

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

type Ilist struct {
	inst   *Inst
	se     Rangeset
	startp Posn
}

var tl []Ilist /* This list, next list */

const NLIST = 127

var nl []Ilist
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

func regerror(e Err) {
	Strzero(&lastregexp)
	error_(e)
}

func regerror_c(e Err, c rune) {
	Strzero(&lastregexp)
	error_c(e, c)
}

func newinst(t int) *Inst {
	if progp >= NPROG {
		regerror(Etoolong)
	}
	p := &program[progp]
	progp++
	*p = Inst{}
	p.type_ = t
	return p
}

func realcompile(s []rune) *Inst {
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
			operand(int(token))
		}
	}
	/* Close with a low priority operator */
	evaluntil(START)
	/* Force END */
	operand(END)
	evaluntil(START)
	if nbra != 0 {
		regerror(Eleftpar)
	}
	andp-- /* points to first and only operand */
	return andstack[andp].first
}

func compile(s *String) {
	if Strcmp(s, &lastregexp) == 0 {
		return
	}
	for _, c := range class {
		// free(c)
		_ = c
	}
	class = class[:0]
	progp = 0
	backwards = false
	startinst = realcompile(s.s)
	optimize(0)
	oprogp := progp
	backwards = true
	bstartinst = realcompile(s.s)
	optimize(oprogp)
	Strduplstr(&lastregexp, s)
}

func operand(t int) {
	if lastwasand {
		operator(CAT) /* catenate is implicit */
	}
	i := newinst(t)
	if t == CCLASS {
		if negateclass {
			i.type_ = NCCLASS /* UGH */
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
			regerror(Erightpar)
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

func cant(s string) {
	panic_("regexp: can't happen: " + s)
}

func pushand(f *Inst, l *Inst) {
	if andp >= len(andstack) {
		cant("operand stack overflow")
	}
	a := &andstack[andp]
	andp++
	a.first = f
	a.last = l
}

func pushator(t int) {
	if atorp >= NSTACK {
		cant("operator stack overflow")
	}
	atorstack[atorp] = t
	atorp++
	if cursubid >= NSUBEXP {
		subidstack[subidp] = -1
		subidp++
	} else {
		subidstack[subidp] = cursubid
		subidp++
	}
}

func popand(op rune) *Node {
	if andp <= 0 {
		if op != 0 {
			regerror_c(Emissop, op)
		} else {
			regerror(Ebadregexp)
		}
	}
	andp--
	return &andstack[andp]
}

func popator() int {
	if atorp <= 0 {
		cant("operator stack underflow")
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
			panic_("unknown regexp operator")
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
			if backwards && op2.first.type_ != END {
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
	for i := start; program[i].type_ != END; i++ {
		inst := &program[i]
		target := inst.next
		for target.type_ == NOP {
			target = target.next
		}
		inst.next = target
	}
}

// #ifdef	DEBUG
func dumpstack() {
	dprint("operators\n")
	for ip := 0; ip < atorp; ip++ {
		dprint("0%o\n", atorstack[ip])
	}
	dprint("operands\n")
	for stk := 0; stk < andp; stk++ {
		dprint("0%o\t0%o\n", andstack[stk].first.type_, andstack[stk].last.type_)
	}
}

func dump() {
	l := 0
	for {
		p := &program[l]
		dprint("%p:\t0%o\t%p\t%p\n", p, p.type_, p.next, p.right)
		if p.type_ == 0 {
			break
		}
	}
}

// #endif

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
		regerror(Ebadclass)
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
			goto Error
		}
		if exprp[0] == '-' {
			exprp = exprp[1:] /* eat '-' */
			c2 := nextrec()
			if c2 == ']' {
				goto Error
			}
			classp = append(classp, utf8.MaxRune, c1, c2)
		} else {
			classp = append(classp, c1&^QUOTED)
		}
	}
	class = append(class, classp)
	return

Error:
	// free(classp)
	regerror(Ebadclass)
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
			if sep.p[0].p1 < p.se.p[0].p1 {
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

func execute(f *File, startp Posn, eof Posn) bool {
	flag := 0
	p := startp
	nnl := 0
	wrapped := 0
	startchar := rune(0)
	if startinst.type_ < OPERATOR {
		startchar = rune(startinst.type_)
	}

	list[1][0].inst = nil
	list[0][0].inst = list[1][0].inst
	sel.p[0].p1 = -1
	/* Execute machine once for each character */
	for ; ; p++ {
	doloop:
		c := filereadc(f, p)
		if p >= eof || c < 0 {
			tmp21 := wrapped
			wrapped++
			switch tmp21 {
			case 0, /* let loop run one more click */
				2:
				break
			case 1: /* expired; wrap to beginning */
				if sel.p[0].p1 >= 0 || eof != INFINITY {
					goto Return
				}
				list[1][0].inst = nil
				list[0][0].inst = list[1][0].inst
				p = 0
				goto doloop
			default:
				goto Return
			}
		} else if ((wrapped != 0 && p >= startp) || sel.p[0].p1 > 0) && nnl == 0 {
			break
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
		if sel.p[0].p1 < 0 && (wrapped == 0 || p < startp || startp == eof) {
			/* Add first instruction to this list */
			sempty.p[0].p1 = p
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
			prev := inst
			if inst == nil {
				break
			}
		Switchstmt:
			if inst == nil {
				debug("%#x led to nil", prev.type_)
			}
			prev = inst
			switch inst.type_ {
			default: /* regular character */
				if inst.type_ == int(c) {
					goto Addinst
				}
			case LBRA:
				if inst.subid >= 0 {
					tl[tlp].se.p[inst.subid].p1 = p
				}
				inst = inst.next
				goto Switchstmt
			case RBRA:
				if inst.subid >= 0 {
					tl[tlp].se.p[inst.subid].p2 = p
				}
				inst = inst.next
				goto Switchstmt
			case ANY:
				if c != '\n' {
					goto Addinst
				}
			case BOL:
				if p == 0 || filereadc(f, p-1) == '\n' {
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
				if inst.next == nil {
					debug("OR no left")
				}
				if inst.right == nil {
					debug("OR no right")
				}
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
				tl[tlp].se.p[0].p2 = p
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
	return sel.p[0].p1 >= 0

Overflow:
	error_(Eoverflow)
	panic("unreachable")
}

func newmatch(sp *Rangeset) {
	if sel.p[0].p1 < 0 || sp.p[0].p1 < sel.p[0].p1 || (sp.p[0].p1 == sel.p[0].p1 && sp.p[0].p2 > sel.p[0].p2) {
		for i := 0; i < NSUBEXP; i++ {
			sel.p[i] = sp.p[i]
		}
	}
}

func bexecute(f *File, startp Posn) bool {
	flag := 0
	p := startp
	nnl := 0
	wrapped := 0
	startchar := rune(0)
	if bstartinst.type_ < OPERATOR {
		startchar = rune(bstartinst.type_)
	}

	list[1][0].inst = nil
	list[0][0].inst = list[1][0].inst
	sel.p[0].p1 = -1
	/* Execute machine once for each character, including terminal NUL */
	for ; ; p-- {
	doloop:
		c := filereadc(f, p-1)
		if c == -1 {
			tmp23 := wrapped
			wrapped++
			switch tmp23 {
			case 0, /* let loop run one more click */
				2:
				break
			case 1: /* expired; wrap to end */
				if sel.p[0].p1 >= 0 {
					goto Return
				}
				list[1][0].inst = nil
				list[0][0].inst = list[1][0].inst
				p = f.b.nc
				goto doloop
			case 3:
				fallthrough
			default:
				goto Return
			}
		} else if ((wrapped != 0 && p <= startp) || sel.p[0].p1 > 0) && nnl == 0 {
			break
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
		if sel.p[0].p1 < 0 && (wrapped == 0 || p > startp) {
			/* Add first instruction to this list */
			/* the minus is so the optimizations in addinst work */
			sempty.p[0].p1 = -p
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
			}
		Switchstmt:
			switch inst.type_ {
			default: /* regular character */
				if inst.type_ == int(c) {
					goto Addinst
				}
			case LBRA:
				if inst.subid >= 0 {
					tl[tlp].se.p[inst.subid].p1 = p
				}
				inst = inst.next
				goto Switchstmt
			case RBRA:
				if inst.subid >= 0 {
					tl[tlp].se.p[inst.subid].p2 = p
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
				if p == f.b.nc || filereadc(f, p) == '\n' {
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
				if addinst(tl[tlp:], inst.right, &tl[tlp].se) != 0 {
					ntl++
					if ntl >= NLIST {
						goto Overflow
					}
				}
				/* efficiency: advance and re-evaluate */
				inst = inst.next
				goto Switchstmt
			case END: /* Match! */
				tl[tlp].se.p[0].p1 = -tl[tlp].se.p[0].p1 /* minus sign */
				tl[tlp].se.p[0].p2 = p
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
	return sel.p[0].p1 >= 0

Overflow:
	error_(Eoverflow)
	panic("unreachable")

}

func bnewmatch(sp *Rangeset) {
	if sel.p[0].p1 < 0 || sp.p[0].p1 > sel.p[0].p2 || (sp.p[0].p1 == sel.p[0].p2 && sp.p[0].p2 < sel.p[0].p1) {
		for i := 0; i < NSUBEXP; i++ { /* note the reversal; p1<=p2 */
			sel.p[i].p1 = sp.p[i].p2
			sel.p[i].p2 = sp.p[i].p1
		}
	}
}
