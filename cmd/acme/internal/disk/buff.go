package disk

import "9fans.net/go/cmd/acme/internal/util"

type Buffer struct {
	nc     int
	c      []rune // cnc was len(c), cmax was cap(c)
	cq     int
	cdirty bool
	cbi    int
	bl     []*block // nbl was len(bl) == cap(bl)
}

var blist *block

func (b *Buffer) Len() int { return b.nc }

func (b *Buffer) resizeCache(n int) {
	for cap(b.c) < n {
		b.c = append(b.c[:cap(b.c)], 0)
	}
	b.c = b.c[:n]
}

func (b *Buffer) insertBlock(i, n int) {
	if i > len(b.bl) {
		util.Fatal("internal error: addblock")
	}
	b.bl = append(b.bl, nil)
	copy(b.bl[i+1:], b.bl[i:])
	b.bl[i] = disk.allocBlock(n)
}

func (b *Buffer) deleteBlock(i int) {
	if i >= len(b.bl) {
		util.Fatal("internal error: delblock")
	}

	disk.freeBlock(b.bl[i])
	copy(b.bl[i:], b.bl[i+1:])
	b.bl = b.bl[:len(b.bl)-1]
}

/*
 * Move cache so b->cq <= q0 < b->cq+b->cnc.
 * If at very end, q0 will fall on end of cache block.
 */

func (b *Buffer) flushCache() {
	if b.cdirty || len(b.c) == 0 {
		if len(b.c) == 0 {
			b.deleteBlock(b.cbi)
		} else {
			disk.write(&b.bl[b.cbi], b.c)
		}
		b.cdirty = false
	}
}

func (b *Buffer) setCache(q0 int) {
	if q0 > b.Len() {
		util.Fatal("internal error: setcache")
	}
	/*
	 * flush and reload if q0 is not in cache.
	 */
	if b.Len() == 0 || (b.cq <= q0 && q0 < b.cq+len(b.c)) {
		return
	}
	/*
	 * if q0 is at end of file and end of cache, continue to grow this block
	 */
	if q0 == b.Len() && q0 == b.cq+len(b.c) && len(b.c) < maxblock { // TODO(rsc): sam says <= Maxblock; which is right?
		return
	}
	b.flushCache()
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
	for q+(*blp).u.n <= q0 && q+(*blp).u.n < b.Len() {
		q += (*blp).u.n
		i++
		if i >= len(b.bl) {
			util.Fatal("block not found")
		}
		blp = &b.bl[i]
	}
	bl := *blp
	/* remember position */
	b.cbi = i
	b.cq = q
	b.resizeCache(bl.u.n)
	/*read block*/
	disk.read(bl, b.c)
}

func (b *Buffer) Read(q0 int, s []rune) {
	n := len(s)
	if !(q0 <= b.Len()) || !(q0+n <= b.Len()) {
		util.Fatal("bufread: internal error")
	}

	for n > 0 {
		b.setCache(q0)
		m := util.Min(n, len(b.c)-(q0-b.cq))
		copy(s[:m], b.c[q0-b.cq:])
		q0 += m
		s = s[m:]
		n -= m
	}
}

func (b *Buffer) Insert(q0 int, s []rune) {
	n := len(s)
	if q0 > b.Len() {
		util.Fatal("internal error: bufinsert")
	}

	for n > 0 {
		b.setCache(q0)
		off := q0 - b.cq
		var m int
		if len(b.c)+n <= maxblock {
			/* Everything fits in one block. */
			t := len(b.c) + n
			m = n
			if b.bl == nil { /* allocate */
				if len(b.c) != 0 {
					util.Fatal("internal error: bufinsert1 cnc!=0")
				}
				b.insertBlock(0, t)
				b.cbi = 0
			}
			b.resizeCache(t)
			copy(b.c[off+m:], b.c[off:])
			copy(b.c[off:], s[:m])
		} else if q0 == b.cq || q0 == b.cq+len(b.c) {
			/*
			 * We must make a new block.  If q0 is at
			 * the very beginning or end of this block,
			 * just make a new block and fill it.
			 */
			if b.cdirty {
				b.flushCache()
			}
			m = util.Min(n, maxblock)
			var i int
			if b.bl == nil { /* allocate */
				if len(b.c) != 0 {
					util.Fatal("internal error: bufinsert2 cnc!=0")
				}
				i = 0
			} else {
				i = b.cbi
				if q0 > b.cq {
					i++
				}
			}
			b.insertBlock(i, m)
			b.resizeCache(m)
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
				b.insertBlock(i, m)
				disk.write(&b.bl[i], b.c[off:])
				b.c = b.c[:off]
			}
			/*
			 * Now at end of block.  Take as much input
			 * as possible and tack it on end of block.
			 */
			m = util.Min(n, maxblock-len(b.c))
			n := len(b.c)
			b.resizeCache(n + m)
			copy(b.c[n:], s)
		}

		b.nc += m
		q0 += m
		s = s[m:]
		n -= m
		b.cdirty = true
	}
}

func (b *Buffer) Delete(q0, q1 int) {
	if !(q0 <= q1 && q0 <= b.Len()) || !(q1 <= b.Len()) {
		util.Fatal("internal error: bufdelete")
	}
	for q1 > q0 {
		b.setCache(q0)
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

func (b *Buffer) Reset() {
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
		b.deleteBlock(i)
	}
}

func (b *Buffer) Close() {
	b.Reset()
	// free(b.c)
	b.c = nil
	// free(b.bl)
	b.bl = nil
}
