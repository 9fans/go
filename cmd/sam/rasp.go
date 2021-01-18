package main

/*
 * GROWDATASIZE must be big enough that all errors go out as Hgrowdata's,
 * so they will be scrolled into visibility in the ~~sam~~ window (yuck!).
 */
const GROWDATASIZE = 50 /* if size is <= this, send data with grow */

var growpos Posn
var grown Posn
var shrinkpos Posn
var shrunk Posn

/*
 * rasp routines inform the terminal of changes to the file.
 *
 * a rasp is a list of spans within the file, and an indication
 * of whether the terminal knows about the span.
 *
 * optimize by coalescing multiple updates to the same span
 * if it is not known by the terminal.
 *
 * other possible optimizations: flush terminal's rasp by cut everything,
 * insert everything if rasp gets too large.
 */

/*
 * only called for initial load of file
 */
func raspload(f *File) {
	if f.rasp == nil {
		return
	}
	grown = f.b.nc
	growpos = 0
	if f.b.nc != 0 {
		rgrow(f.rasp, 0, f.b.nc)
	}
	raspdone(f, true)
}

func raspstart(f *File) {
	if f.rasp == nil {
		return
	}
	grown = 0
	shrunk = 0
	outbuffered = true
}

func raspdone(f *File, toterm bool) {
	if f.dot.r.p1 > f.b.nc {
		f.dot.r.p1 = f.b.nc
	}
	if f.dot.r.p2 > f.b.nc {
		f.dot.r.p2 = f.b.nc
	}
	if f.mark.p1 > f.b.nc {
		f.mark.p1 = f.b.nc
	}
	if f.mark.p2 > f.b.nc {
		f.mark.p2 = f.b.nc
	}
	if f.rasp == nil {
		return
	}
	if grown != 0 {
		outTsll(Hgrow, f.tag, growpos, grown)
	} else if shrunk != 0 {
		outTsll(Hcut, f.tag, shrinkpos, shrunk)
	}
	if toterm {
		outTs(Hcheck0, f.tag)
	}
	outflush()
	outbuffered = false
	if f == cmd {
		cmdpt += cmdptadv
		cmdptadv = 0
	}
}

func raspflush(f *File) {
	if grown != 0 {
		outTsll(Hgrow, f.tag, growpos, grown)
		grown = 0
	} else if shrunk != 0 {
		outTsll(Hcut, f.tag, shrinkpos, shrunk)
		shrunk = 0
	}
	outflush()
}

func raspdelete(f *File, p1 int, p2 int, toterm bool) {
	n := p2 - p1
	if n == 0 {
		return
	}

	if p2 <= f.dot.r.p1 {
		f.dot.r.p1 -= n
		f.dot.r.p2 -= n
	}
	if p2 <= f.mark.p1 {
		f.mark.p1 -= n
		f.mark.p2 -= n
	}

	if f.rasp == nil {
		return
	}

	if f == cmd && p1 < cmdpt {
		if p2 <= cmdpt {
			cmdpt -= n
		} else {
			cmdpt = p1
		}
	}
	if toterm {
		if grown != 0 {
			outTsll(Hgrow, f.tag, growpos, grown)
			grown = 0
		} else if shrunk != 0 && shrinkpos != p1 && shrinkpos != p2 {
			outTsll(Hcut, f.tag, shrinkpos, shrunk)
			shrunk = 0
		}
		if shrunk == 0 || shrinkpos == p2 {
			shrinkpos = p1
		}
		shrunk += n
	}
	rcut(f.rasp, p1, p2)
}

func raspinsert(f *File, p1 int, buf []rune, toterm bool) {
	n := len(buf)
	if n == 0 {
		return
	}

	if p1 < f.dot.r.p1 {
		f.dot.r.p1 += n
		f.dot.r.p2 += n
	}
	if p1 < f.mark.p1 {
		f.mark.p1 += n
		f.mark.p2 += n
	}

	if f.rasp == nil {
		return
	}
	if f == cmd && p1 < cmdpt {
		cmdpt += n
	}
	if toterm {
		if shrunk != 0 {
			outTsll(Hcut, f.tag, shrinkpos, shrunk)
			shrunk = 0
		}
		if n > GROWDATASIZE || !rterm(f.rasp, p1) {
			rgrow(f.rasp, p1, n)
			if grown != 0 && growpos+grown != p1 && growpos != p1 {
				outTsll(Hgrow, f.tag, growpos, grown)
				grown = 0
			}
			if grown == 0 {
				growpos = p1
			}
			grown += n
		} else {
			if grown != 0 {
				outTsll(Hgrow, f.tag, growpos, grown)
				grown = 0
			}
			rgrow(f.rasp, p1, n)
			r := rdata(f.rasp, p1, n)
			if r.p1 != p1 || r.p2 != p1+n {
				panic_("rdata in toterminal")
			}
			outTsllS(Hgrowdata, f.tag, p1, n, tmprstr(buf[:n]))
		}
	} else {
		rgrow(f.rasp, p1, n)
		r := rdata(f.rasp, p1, n)
		if r.p1 != p1 || r.p2 != p1+n {
			panic_("rdata in toterminal")
		}
	}
}

type PosnList []Posn

const M = 0x80000000

func (l *PosnList) T(i int) bool { return (*l)[i]&M != 0 }
func (l *PosnList) L(i int) Posn { return (*l)[i] &^ M }

func (l *PosnList) ins(i int, n Posn) {
	*l = append(*l, Posn(0))
	copy((*l)[i+1:], (*l)[i:])
	(*l)[i] = n
}

func (l *PosnList) del(i int) {
	copy((*l)[i:], (*l)[i+1:])
	*l = (*l)[:len(*l)-1]
}

// #define	(*r)[i]	r->posnptr[i]
// #define	T(i)	((*r)[i]&M)	/* in terminal */
// #define	L(i)	((*r)[i]&~M)	/* length of this piece */

func rcut(r *PosnList, p1 Posn, p2 Posn) {
	if p1 == p2 {
		panic_("rcut 0")
	}
	p := 0
	i := 0
	for i < len(*r) && p+r.L(i) <= p1 {
		p += r.L(i)
		i++
	}
	if i == len(*r) {
		panic_("rcut 1")
	}
	var x Posn
	if p < p1 { /* chop this piece */
		if p+r.L(i) < p2 {
			x = p1 - p
			p += r.L(i)
		} else {
			x = r.L(i) - (p2 - p1)
			p = p2
		}
		if r.T(i) {
			(*r)[i] = x | M
		} else {
			(*r)[i] = x
		}
		i++
	}
	for i < len(*r) && p+r.L(i) <= p2 {
		p += r.L(i)
		r.del(i)
	}
	if p < p2 {
		if i == len(*r) {
			panic_("rcut 2")
		}
		x = r.L(i) - (p2 - p)
		if r.T(i) {
			(*r)[i] = x | M
		} else {
			(*r)[i] = x
		}
	}
	/* can we merge i and i-1 ? */
	if i > 0 && i < len(*r) && r.T(i-1) == r.T(i) {
		x = r.L(i-1) + r.L(i)
		r.del(i)
		i--
		if r.T(i) {
			(*r)[i] = x | M
		} else {
			(*r)[i] = x
		}
	}
}

func rgrow(r *PosnList, p1 Posn, n Posn) {
	if n == 0 {
		panic_("rgrow 0")
	}
	p := 0
	i := 0
	for ; i < len(*r) && p+r.L(i) <= p1; func() { p += r.L(i); i++ }() {
	}
	if i == len(*r) { /* stick on end of file */
		if p != p1 {
			panic_("rgrow 1")
		}
		if i > 0 && !r.T(i-1) {
			(*r)[i-1] += n
		} else {
			r.ins(i, n)
		}
	} else if !r.T(i) {
		(*r)[i] += n
	} else if p == p1 && i > 0 && !r.T(i-1) { /* special case; simplifies life */
		(*r)[i-1] += n
	} else if p == p1 {
		r.ins(i, n)
	} else { /* must break piece in terminal */
		r.ins(i+1, (r.L(i)-(p1-p))|M)
		r.ins(i+1, n)
		(*r)[i] = (p1 - p) | M
	}
}

func rterm(r *PosnList, p1 Posn) bool {
	p := 0
	i := 0
	for ; i < len(*r) && p+r.L(i) <= p1; func() { p += r.L(i); i++ }() {
	}
	if i == len(*r) && (i == 0 || !r.T(i-1)) {
		return false
	}

	// TODO(rsc): Use of uninitialized or stale data?
	// The original C code does return T(i) even when i == r->nused (len(*r)).
	// Most (all?) of the time, the backing store has capacity (i < r->nalloc).
	// If no entries have been deleted from the rasp, then the spare capacity
	// is zeroed and T(i) returns false.
	// But if entries have been deleted, then the spare capacity may hold
	// a stale entry, and the stale entry may have the M bit set, causing
	// T(i) to return true . This does happen in practice.
	// (On my system, B /etc/passwd followed by B /etc/group triggers
	// the stale true in the second B command.)
	// It is difficult to believe that accessing those stale entries is intended,
	// but the C version has been stable for a long time, so assume it is correct.
	if i == len(*r) {
		if i == cap(*r) {
			// Never initialized, assume C version had extra backing store
			// (which would definitely have been zeroed if it existed).
			return false
		}
		// Read stale (deleted) entry.
		rr := (*r)[:i+1]
		return rr.T(i)
	}

	return r.T(i)
}

func rdata(r *PosnList, p1 Posn, n Posn) Range {
	if n == 0 {
		panic_("rdata 0")
	}
	p := 0
	i := 0
	for ; i < len(*r) && p+r.L(i) <= p1; func() { p += r.L(i); i++ }() {
	}
	if i == len(*r) {
		panic_("rdata 1")
	}
	var rg Range
	if r.T(i) {
		n -= r.L(i) - (p1 - p)
		if n <= 0 {
			rg.p2 = p1
			rg.p1 = rg.p2
			return rg
		}
		p += r.L(i)
		i++
		p1 = p
	}
	if r.T(i) || i == len(*r) {
		panic_("rdata 2")
	}
	if p+r.L(i) < p1+n {
		n = r.L(i) - (p1 - p)
	}
	rg.p1 = p1
	rg.p2 = p1 + n
	if p != p1 {
		r.ins(i+1, r.L(i)-(p1-p))
		(*r)[i] = p1 - p
		i++
	}
	if r.L(i) != n {
		r.ins(i+1, r.L(i)-n)
		(*r)[i] = n
	}
	(*r)[i] |= M
	/* now i is set; can we merge? */
	if i < len(*r)-1 && r.T(i+1) {
		n += r.L(i + 1)
		(*r)[i] = n | M
		r.del(i + 1)
	}
	if i > 0 && r.T(i-1) {
		(*r)[i] = (n + r.L(i-1)) | M
		r.del(i - 1)
	}
	return rg
}
