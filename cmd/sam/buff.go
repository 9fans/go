// #include "sam.h"

package main

import (
	iopkg "io"
	"os"
	"unicode/utf8"
)

func sizecache(b *Buffer, n int) {
	for cap(b.c) < n {
		b.c = append(b.c[:cap(b.c)], 0)
	}
	b.c = b.c[:n]
}

func addblock(b *Buffer, i int, n int) {
	if i > len(b.bl) {
		panic_("internal error: addblock")
	}
	b.bl = append(b.bl, nil)
	copy(b.bl[i+1:], b.bl[i:])
	b.bl[i] = disknewblock(disk, n)
}

func delblock(b *Buffer, i int) {
	if i >= len(b.bl) {
		panic_("internal error: delblock")
	}

	diskrelease(disk, b.bl[i])
	copy(b.bl[i:], b.bl[i+1:])
	b.bl = b.bl[:len(b.bl)-1]
}

/*
 * Move cache so b->cq <= q0 < b->cq+b->cnc.
 * If at very end, q0 will fall on end of cache block.
 */

func flush(b *Buffer) {
	if b.cdirty || len(b.c) == 0 {
		if len(b.c) == 0 {
			delblock(b, b.cbi)
		} else {
			diskwrite(disk, &b.bl[b.cbi], b.c)
		}
		b.cdirty = false
	}
}

func setcache(b *Buffer, q0 int) {
	if q0 > b.nc {
		panic_("internal error: setcache")
	}
	/*
	 * flush and reload if q0 is not in cache.
	 */
	if b.nc == 0 || (b.cq <= q0 && q0 < b.cq+len(b.c)) {
		return
	}
	/*
	 * if q0 is at end of file and end of cache, continue to grow this block
	 */
	if q0 == b.nc && q0 == b.cq+len(b.c) && len(b.c) <= Maxblock {
		return
	}
	flush(b)
	var q, i int
	/* find block */
	if q0 < b.cq {
		q = 0
		i = 0
	} else {
		q = b.cq
		i = b.cbi
	}
	blp := &b.bl[i]
	for q+(*blp).u.n <= q0 && q+(*blp).u.n < b.nc {
		q += (*blp).u.n
		i++
		if i >= len(b.bl) {
			panic_("block not found")
		}
		blp = &b.bl[i]
	}
	bl := *blp
	/* remember position */
	b.cbi = i
	b.cq = q
	sizecache(b, bl.u.n)
	/*read block*/
	diskread(disk, bl, b.c)
}

func bufinsert(b *Buffer, q0 int, s []rune) {
	n := len(s)
	if q0 > b.nc {
		panic_("internal error: bufinsert")
	}

	for n > 0 {
		setcache(b, q0)
		off := q0 - b.cq
		var m int
		if len(b.c)+n <= Maxblock {
			/* Everything fits in one block. */
			t := len(b.c) + n
			m = n
			if b.bl == nil { /* allocate */
				if len(b.c) != 0 {
					panic_("internal error: bufinsert1 cnc!=0")
				}
				addblock(b, 0, t)
				b.cbi = 0
			}
			sizecache(b, t)
			copy(b.c[off+m:], b.c[off:])
			copy(b.c[off:], s[:m])
		} else if q0 == b.cq || q0 == b.cq+len(b.c) {
			/*
			 * We must make a new block.  If q0 is at
			 * the very beginning or end of this block,
			 * just make a new block and fill it.
			 */
			if b.cdirty {
				flush(b)
			}
			m = min(n, Maxblock)
			var i int
			if b.bl == nil { /* allocate */
				if len(b.c) != 0 {
					panic_("internal error: bufinsert2 cnc!=0")
				}
				i = 0
			} else {
				i = b.cbi
				if q0 > b.cq {
					i++
				}
			}
			addblock(b, i, m)
			sizecache(b, m)
			copy(b.c, s[:m])
			b.cq = q0
			b.cbi = i
		} else {
			/*
			 * Split the block; cut off the right side and
			 * let go of it.
			 */
			m = len(b.c) - off
			if m > 0 {
				i := b.cbi + 1
				addblock(b, i, m)
				diskwrite(disk, &b.bl[i], b.c[off:])
				b.c = b.c[:off]
			}
			/*
			 * Now at end of block.  Take as much input
			 * as possible and tack it on end of block.
			 */
			m = min(n, Maxblock-len(b.c))
			n := len(b.c)
			sizecache(b, n+m)
			copy(b.c[n:], s)
		}

		b.nc += m
		q0 += m
		s = s[m:]
		n -= m
		b.cdirty = true
	}
}

func bufdelete(b *Buffer, q0 int, q1 int) {
	if !(q0 <= q1 && q0 <= b.nc) || !(q1 <= b.nc) {
		panic_("internal error: bufdelete")
	}
	for q1 > q0 {
		setcache(b, q0)
		off := q0 - b.cq
		var n int
		if q1 > b.cq+len(b.c) {
			n = len(b.c) - off
		} else {
			n = q1 - q0
		}
		m := len(b.c) - (off + n)
		if m > 0 {
			copy(b.c[off:], b.c[off+n:])
		}
		b.c = b.c[:len(b.c)-n]
		b.cdirty = true
		q1 -= n
		b.nc -= n
	}
}

func bufload(b *Buffer, q0 int, fd *os.File, nulls *bool) int {
	if q0 > b.nc {
		panic_("internal error: bufload")
	}
	p := make([]byte, Maxblock+utf8.UTFMax+1)
	r := make([]rune, Maxblock)
	m := 0
	n := 1
	q1 := q0
	/*
	 * At top of loop, may have m bytes left over from
	 * last pass, possibly representing a partial rune.
	 */
	for n > 0 {
		var err error
		n, err = fd.Read(p[m : m+Maxblock])
		if err != nil && err != iopkg.EOF {
			error_(Ebufload)
			break
		}
		m += n
		nb, nr, nulls1 := cvttorunes(p[:m], r, err == iopkg.EOF)
		if nulls1 {
			*nulls = true
		}
		copy(p, p[nb:m])
		m -= nb
		bufinsert(b, q1, r[:nr])
		q1 += nr
	}
	// free(p)
	// free(r)
	return q1 - q0
}

func bufread(b *Buffer, q0 int, s []rune) {
	n := len(s)
	if !(q0 <= b.nc) || !(q0+n <= b.nc) {
		panic_("bufread: internal error")
	}

	for n > 0 {
		setcache(b, q0)
		m := min(n, len(b.c)-(q0-b.cq))
		copy(s[:m], b.c[q0-b.cq:])
		q0 += m
		s = s[m:]
		n -= m
	}
}

func bufreset(b *Buffer) {
	b.nc = 0
	b.c = b.c[:0]
	b.cq = 0
	b.cdirty = false
	b.cbi = 0
	/* delete backwards to avoid n² behavior */
	// TODO(rsc): Is there a reason we leave one b.bl entry behind?
	for i := len(b.bl) - 1; ; {
		i--
		if i < 0 {
			break
		}
		delblock(b, i)
	}
}

func bufclose(b *Buffer) {
	bufreset(b)
	// free(b.c)
	b.c = nil
	// free(b.bl)
	b.bl = nil
}
