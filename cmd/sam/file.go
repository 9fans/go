// #include "sam.h"

/*
 * Structure of Undo list:
 * 	The Undo structure follows any associated data, so the list
 *	can be read backwards: read the structure, then read whatever
 *	data is associated (insert string, file name) and precedes it.
 *	The structure includes the previous value of the modify bit
 *	and a sequence number; successive Undo structures with the
 *	same sequence number represent simultaneous changes.
 */

package main

import (
	"os"
	"reflect"
	"unsafe"
)

type Undo struct {
	type_ int
	mod   bool
	seq   int
	p0    int
	n     int
}

type Merge struct {
	f    *File
	seq  int
	p0   int
	n    int
	nbuf int
	buf  [RBUFSIZE]rune
}

const (
	Maxmerge = 50
	Undosize = int(unsafe.Sizeof(Undo{}) / unsafe.Sizeof(rune(0)))
)

func undorunes(u *Undo) []rune {
	var r []rune
	h := (*reflect.SliceHeader)(unsafe.Pointer(&r))
	h.Data = uintptr(unsafe.Pointer(u))
	h.Len = Undosize
	h.Cap = Undosize
	return r
}

var merge Merge

func fileopen() *File {
	f := new(File)
	f.dot.f = f
	f.ndot.f = f
	f.seq = 0
	f.mod = false
	f.unread = true
	Strinit0(&f.name)
	return f
}

func fileisdirty(f *File) bool {
	return f.seq != f.cleanseq
}

func wrinsert(delta *Buffer, seq int, mod bool, p0 int, s []rune) {
	var u Undo
	u.type_ = Insert
	u.mod = mod
	u.seq = seq
	u.p0 = p0
	u.n = len(s)
	bufinsert(delta, delta.nc, s)
	bufinsert(delta, delta.nc, undorunes(&u))
}

func wrdelete(delta *Buffer, seq int, mod bool, p0 int, p1 int) {
	var u Undo
	u.type_ = Delete
	u.mod = mod
	u.seq = seq
	u.p0 = p0
	u.n = p1 - p0
	bufinsert(delta, delta.nc, undorunes(&u))
}

func flushmerge() {
	f := merge.f
	if f == nil {
		return
	}
	if merge.seq != f.seq {
		panic_("flushmerge seq mismatch")
	}
	if merge.n != 0 {
		wrdelete(&f.epsilon, f.seq, true, merge.p0, merge.p0+merge.n)
	}
	if merge.nbuf != 0 {
		wrinsert(&f.epsilon, f.seq, true, merge.p0+merge.n, merge.buf[:merge.nbuf])
	}
	merge.f = nil
	merge.n = 0
	merge.nbuf = 0
}

func mergeextend(f *File, p0 int) {
	mp0n := merge.p0 + merge.n
	if mp0n != p0 {
		bufread(&f.b, mp0n, merge.buf[merge.nbuf:merge.nbuf+p0-mp0n])
		merge.nbuf += p0 - mp0n
		merge.n = p0 - merge.p0
	}
}

/*
 * like fileundelete, but get the data from arguments
 */
func loginsert(f *File, p0 int, s []rune) {
	ns := len(s)
	if f.rescuing != 0 {
		return
	}
	if ns == 0 {
		return
	}
	if ns > STRSIZE {
		panic_("loginsert")
	}
	if f.seq < seq {
		filemark(f)
	}
	if p0 < f.hiposn {
		error_(Esequence)
	}

	if merge.f != f || p0-(merge.p0+merge.n) > Maxmerge || merge.nbuf+((p0+ns)-(merge.p0+merge.n)) >= RBUFSIZE { /* too far */ /* too long */
		flushmerge()
	}

	if ns >= RBUFSIZE {
		if !(merge.n == 0 && merge.nbuf == 0) || !(merge.f == nil) {
			panic_("loginsert bad merge state")
		}
		wrinsert(&f.epsilon, f.seq, true, p0, s)
	} else {
		if merge.f != f {
			merge.f = f
			merge.p0 = p0
			merge.seq = f.seq
		}
		mergeextend(f, p0)

		/* append string to merge */
		copy(merge.buf[merge.nbuf:], s)
		merge.nbuf += ns
	}

	f.hiposn = p0
	if !f.unread && !f.mod {
		state(f, Dirty)
	}
}

func logdelete(f *File, p0 int, p1 int) {
	if f.rescuing != 0 {
		return
	}
	if p0 == p1 {
		return
	}
	if f.seq < seq {
		filemark(f)
	}
	if p0 < f.hiposn {
		error_(Esequence)
	}

	if merge.f != f || p0-(merge.p0+merge.n) > Maxmerge || merge.nbuf+(p0-(merge.p0+merge.n)) >= RBUFSIZE { /* too far */ /* too long */
		flushmerge()
		merge.f = f
		merge.p0 = p0
		merge.seq = f.seq
	}

	mergeextend(f, p0)

	/* add to deletion */
	merge.n = p1 - merge.p0

	f.hiposn = p1
	if !f.unread && !f.mod {
		state(f, Dirty)
	}
}

/*
 * like fileunsetname, but get the data from arguments
 */
func logsetname(f *File, s *String) {
	if f.rescuing != 0 {
		return
	}

	if f.unread { /* This is setting initial file name */
		filesetname(f, s)
		return
	}

	if f.seq < seq {
		filemark(f)
	}

	/* undo a file name change by restoring old name */
	delta := &f.epsilon
	var u Undo
	u.type_ = Filename
	u.mod = true
	u.seq = f.seq
	u.p0 = 0 /* unused */
	u.n = len(s.s)
	if len(s.s) != 0 {
		bufinsert(delta, delta.nc, s.s)
	}
	bufinsert(delta, delta.nc, undorunes(&u))
	if !f.unread && !f.mod {
		state(f, Dirty)
	}
}

func fileuninsert(f *File, delta *Buffer, p0 int, ns int) {
	var u Undo
	/* undo an insertion by deleting */
	u.type_ = Delete
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = p0
	u.n = ns
	bufinsert(delta, delta.nc, undorunes(&u))
}

func fileundelete(f *File, delta *Buffer, p0 int, p1 int) {
	var u Undo
	/* undo a deletion by inserting */
	u.type_ = Insert
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = p0
	u.n = p1 - p0
	buf := fbufalloc()
	var n int
	for i := p0; i < p1; i += n {
		n = p1 - i
		if n > RBUFSIZE {
			n = RBUFSIZE
		}
		bufread(&f.b, i, buf[:n])
		bufinsert(delta, delta.nc, buf[:n])
	}
	fbuffree(buf)
	bufinsert(delta, delta.nc, undorunes(&u))

}

func filereadc(f *File, q int) rune {
	if q >= f.b.nc {
		return -1
	}
	var r [1]rune
	bufread(&f.b, q, r[:])
	return r[0]
}

func filesetname(f *File, s *String) {
	if !f.unread {
		fileunsetname(f, &f.delta)
	}
	Strduplstr(&f.name, s)
	sortname(f)
	f.unread = true
}

func fileunsetname(f *File, delta *Buffer) {
	var u Undo
	/* undo a file name change by restoring old name */
	u.type_ = Filename
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = 0 /* unused */
	var s String
	Strinit(&s)
	Strduplstr(&s, &f.name)
	fullname(&s)
	u.n = len(s.s)
	if len(s.s) != 0 {
		bufinsert(delta, delta.nc, s.s)
	}
	bufinsert(delta, delta.nc, undorunes(&u))
	Strclose(&s)
}

func fileunsetdot(f *File, delta *Buffer, dot Range) {
	var u Undo
	u.type_ = Dot
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = dot.p1
	u.n = dot.p2 - dot.p1
	bufinsert(delta, delta.nc, undorunes(&u))
}

func fileunsetmark(f *File, delta *Buffer, mark Range) {
	var u Undo
	u.type_ = Mark
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = mark.p1
	u.n = mark.p2 - mark.p1
	bufinsert(delta, delta.nc, undorunes(&u))
}

func fileload(f *File, p0 int, fd *os.File, nulls *bool) int {
	if f.seq > 0 {
		panic_("undo in file.load unimplemented")
	}
	return bufload(&f.b, p0, fd, nulls)
}

func fileupdate(f *File, notrans, toterm bool) bool {
	if f.rescuing != 0 {
		return false
	}

	flushmerge()

	/*
	 * fix the modification bit
	 * subtle point: don't save it away in the log.
	 *
	 * if another change is made, the correct f->mod
	 * state is saved  in the undo log by filemark
	 * when setting the dot and mark.
	 *
	 * if the change is undone, the correct state is
	 * saved from f in the fileun... routines.
	 */
	mod := f.mod
	f.mod = f.prevmod
	if f == cmd {
		notrans = true
	} else {
		fileunsetdot(f, &f.delta, f.prevdot)
		fileunsetmark(f, &f.delta, f.prevmark)
	}
	f.dot = f.ndot
	var p1 int
	var p2 int
	fileundo(f, false, !notrans, &p1, &p2, toterm)
	f.mod = mod

	if f.delta.nc == 0 {
		f.seq = 0
	}

	if f == cmd {
		return false
	}

	if f.mod {
		f.closeok = false
		quitok = false
	} else {
		f.closeok = true
	}
	return true
}

func prevseq(b *Buffer) int {
	up := b.nc
	if up == 0 {
		return 0
	}
	up -= Undosize
	var u Undo
	bufread(b, up, undorunes(&u))
	return u.seq
}

func undoseq(f *File, isundo bool) int {
	if isundo {
		return f.seq
	}

	return prevseq(&f.epsilon)
}

func fileundo(f *File, isundo, canredo bool, q0p *int, q1p *int, flag bool) {
	var stop int
	var delta *Buffer
	var epsilon *Buffer
	if isundo {
		/* undo; reverse delta onto epsilon, seq decreases */
		delta = &f.delta
		epsilon = &f.epsilon
		stop = f.seq
	} else {
		/* redo; reverse epsilon onto delta, seq increases */
		delta = &f.epsilon
		epsilon = &f.delta
		stop = 0 /* don't know yet */
	}

	raspstart(f)
	for delta.nc > 0 {
		/* rasp and buffer are in sync; sync with wire if needed */
		if needoutflush() {
			raspflush(f)
		}
		up := delta.nc - Undosize
		var u Undo
		bufread(delta, up, undorunes(&u))
		if isundo {
			if u.seq < stop {
				f.seq = u.seq
				raspdone(f, flag)
				return
			}
		} else {
			if stop == 0 {
				stop = u.seq
			}
			if u.seq > stop {
				raspdone(f, flag)
				return
			}
		}
		switch u.type_ {
		default:
			panic_("undo unknown u.type")

		case Delete:
			f.seq = u.seq
			if canredo {
				fileundelete(f, epsilon, u.p0, u.p0+u.n)
			}
			f.mod = u.mod
			bufdelete(&f.b, u.p0, u.p0+u.n)
			raspdelete(f, u.p0, u.p0+u.n, flag)
			*q0p = u.p0
			*q1p = u.p0

		case Insert:
			f.seq = u.seq
			if canredo {
				fileuninsert(f, epsilon, u.p0, u.n)
			}
			f.mod = u.mod
			up -= u.n
			buf := fbufalloc()
			var n int
			for i := 0; i < u.n; i += n {
				n = u.n - i
				if n > RBUFSIZE {
					n = RBUFSIZE
				}
				bufread(delta, up+i, buf[:n])
				bufinsert(&f.b, u.p0+i, buf[:n])
				raspinsert(f, u.p0+i, buf[:n], flag)
			}
			fbuffree(buf)
			*q0p = u.p0
			*q1p = u.p0 + u.n

		case Filename:
			f.seq = u.seq
			if canredo {
				fileunsetname(f, epsilon)
			}
			f.mod = u.mod
			up -= u.n

			Strinsure(&f.name, u.n)
			bufread(delta, up, f.name.s)
			fixname(&f.name)
			sortname(f)
		case Dot:
			f.seq = u.seq
			if canredo {
				fileunsetdot(f, epsilon, f.dot.r)
			}
			f.mod = u.mod
			f.dot.r.p1 = u.p0
			f.dot.r.p2 = u.p0 + u.n
		case Mark:
			f.seq = u.seq
			if canredo {
				fileunsetmark(f, epsilon, f.mark)
			}
			f.mod = u.mod
			f.mark.p1 = u.p0
			f.mark.p2 = u.p0 + u.n
		}
		bufdelete(delta, up, delta.nc)
	}
	if isundo {
		f.seq = 0
	}
	raspdone(f, flag)
}

func filereset(f *File) {
	bufreset(&f.delta)
	bufreset(&f.epsilon)
	f.seq = 0
}

func fileclose(f *File) {
	Strclose(&f.name)
	bufclose(&f.b)
	bufclose(&f.delta)
	bufclose(&f.epsilon)
	if f.rasp != nil {
		// listfree(f.rasp)
	}
	// free(f)
}

func filemark(f *File) {

	if f.unread {
		return
	}
	if f.epsilon.nc != 0 {
		bufdelete(&f.epsilon, 0, f.epsilon.nc)
	}

	if f != cmd {
		f.prevdot = f.dot.r
		f.prevmark = f.mark
		f.prevseq = f.seq
		f.prevmod = f.mod
	}

	f.ndot = f.dot
	f.seq = seq
	f.hiposn = 0
}
