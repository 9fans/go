// #include "sam.h"

package main

import (
	iopkg "io"
	"reflect"
	"unsafe"
)

var blist *Block

func diskinit() *Disk {
	d := new(Disk)
	d.fd = tempdisk()
	return d
}

func ntosize(n int, ip *int) int {
	if n > Maxblock {
		panic_("internal error: ntosize")
	}
	size := n
	if size&(Blockincr-1) != 0 {
		size += Blockincr - (size & (Blockincr - 1))
	}
	/* last bucket holds blocks of exactly Maxblock */
	if ip != nil {
		*ip = size / Blockincr
	}
	return size * RUNESIZE
}

func disknewblock(d *Disk, n int) *Block {
	var i int
	size := ntosize(n, &i)
	b := d.free[i]
	if b != nil {
		d.free[i] = b.u.next
	} else {
		/* allocate in chunks to reduce malloc overhead */
		if blist == nil {
			bb := make([]Block, 100)
			for j := 0; j < 100-1; j++ {
				bb[j].u.next = &bb[j+1]
			}
			blist = &bb[0]
		}
		b = blist
		blist = b.u.next
		b.addr = d.addr
		if d.addr+int64(size) < d.addr {
			panic_("temp file overflow")
		}
		d.addr += int64(size)
	}
	b.u.n = n
	return b
}

func diskrelease(d *Disk, b *Block) {
	var i int
	ntosize(b.u.n, &i)
	b.u.next = d.free[i]
	d.free[i] = b
}

func runedata(r []rune) []byte {
	var b []byte
	h := (*reflect.SliceHeader)(unsafe.Pointer(&b))
	h.Data = uintptr(unsafe.Pointer(&r[0]))
	h.Len = RUNESIZE * len(r)
	h.Cap = RUNESIZE * cap(r)
	return b
}

func diskwrite(d *Disk, bp **Block, r []rune) {
	n := len(r)
	b := *bp
	size := ntosize(b.u.n, nil)
	nsize := ntosize(n, nil)
	if size != nsize {
		diskrelease(d, b)
		b = disknewblock(d, n)
		*bp = b
	}
	if nw, err := d.fd.WriteAt(runedata(r), b.addr); nw != n*RUNESIZE || err != nil {
		if err == nil {
			err = iopkg.ErrShortWrite
		}
		panic_("writing temp file: %v", err)
	}
	b.u.n = n
}

func diskread(d *Disk, b *Block, r []rune) {
	n := len(r)
	if n > b.u.n {
		panic_("internal error: diskread")
	}

	ntosize(b.u.n, nil) /* called only for sanity check on Maxblock */
	if nr, err := d.fd.ReadAt(runedata(r), b.addr); nr != n*RUNESIZE || err != nil {
		panic_("read error from temp file")
	}
}
