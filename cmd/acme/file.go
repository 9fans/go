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
	"fmt"
	"io"
	"os"
	"reflect"
	"unsafe"

	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
)

type Undo struct {
	typ int
	mod bool
	seq int
	p0  int
	n   int
}

const Undosize = int(unsafe.Sizeof(Undo{})) / runes.RuneSize

func fileaddtext(f *File, t *Text) *File {
	if f == nil {
		f = new(File)
		f.unread = true
	}
	f.text = append(f.text, t)
	f.curtext = t
	return f
}

func filedeltext(f *File, t *Text) {
	var i int
	for i = 0; i < len(f.text); i++ {
		if f.text[i] == t {
			goto Found
		}
	}
	util.Fatal("can't find text in filedeltext")

Found:
	copy(f.text[i:], f.text[i+1:])
	f.text = f.text[:len(f.text)-1]
	if len(f.text) == 0 {
		fileclose(f)
		return
	}
	if f.curtext == t {
		f.curtext = f.text[0]
	}
}

func fileinsert(f *File, p0 int, s []rune) {
	if p0 > f.b.Len() {
		util.Fatal("internal error: fileinsert")
	}
	if f.seq > 0 {
		fileuninsert(f, &f.delta, p0, len(s))
	}
	f.b.Insert(p0, s)
	if len(s) != 0 {
		f.mod = true
	}
}

func undorunes(u *Undo) []rune {
	var r []rune
	h := (*reflect.SliceHeader)(unsafe.Pointer(&r))
	h.Data = uintptr(unsafe.Pointer(u))
	h.Len = Undosize
	h.Cap = Undosize
	return r
}

func fileuninsert(f *File, delta *disk.Buffer, p0 int, ns int) {
	var u Undo
	/* undo an insertion by deleting */
	u.typ = Delete
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = p0
	u.n = ns
	delta.Insert(delta.Len(), undorunes(&u))
}

func filedelete(f *File, p0 int, p1 int) {
	if !(p0 <= p1 && p0 <= f.b.Len()) || !(p1 <= f.b.Len()) {
		util.Fatal("internal error: filedelete")
	}
	if f.seq > 0 {
		fileundelete(f, &f.delta, p0, p1)
	}
	f.b.Delete(p0, p1)
	if p1 > p0 {
		f.mod = true
	}
}

func fileundelete(f *File, delta *disk.Buffer, p0 int, p1 int) {
	var u Undo
	/* undo a deletion by inserting */
	u.typ = Insert
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = p0
	u.n = p1 - p0
	buf := bufs.AllocRunes()
	var n int
	for i := p0; i < p1; i += n {
		n = p1 - i
		if n > bufs.RuneLen {
			n = bufs.RuneLen
		}
		f.b.Read(i, buf[:n])
		delta.Insert(delta.Len(), buf[:n])
	}
	bufs.FreeRunes(buf)
	delta.Insert(delta.Len(), undorunes(&u))
}

func filesetname(f *File, name []rune) {
	if f.seq > 0 {
		fileunsetname(f, &f.delta)
	}
	f.name = runes.Clone(name)
	f.unread = true
}

func fileunsetname(f *File, delta *disk.Buffer) {
	var u Undo
	/* undo a file name change by restoring old name */
	u.typ = Filename
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = 0 /* unused */
	u.n = len(f.name)
	if len(f.name) != 0 {
		delta.Insert(delta.Len(), f.name)
	}
	delta.Insert(delta.Len(), undorunes(&u))
}

func fileload(f *File, p0 int, fd *os.File, nulls *bool, h io.Writer) int {
	if f.seq > 0 {
		util.Fatal("undo in file.load unimplemented")
	}
	return bufload(&f.b, p0, fd, nulls, h)
}

/* return sequence number of pending redo */
func fileredoseq(f *File) int {
	delta := &f.epsilon
	if delta.Len() == 0 {
		return 0
	}
	var u Undo
	delta.Read(delta.Len()-Undosize, undorunes(&u))
	return u.seq
}

func fileundo(f *File, isundo bool, q0p *int, q1p *int) {
	var stop int
	var delta *disk.Buffer
	var epsilon *disk.Buffer
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

	buf := bufs.AllocRunes()
	for delta.Len() > 0 {
		up := delta.Len() - Undosize
		var u Undo
		delta.Read(up, undorunes(&u))
		if isundo {
			if u.seq < stop {
				f.seq = u.seq
				goto Return
			}
		} else {
			if stop == 0 {
				stop = u.seq
			}
			if u.seq > stop {
				goto Return
			}
		}
		var n int
		var j int
		var i int
		switch u.typ {
		default:
			fmt.Fprintf(os.Stderr, "undo: %#x\n", u.typ)
			panic("undo")

		case Delete:
			f.seq = u.seq
			fileundelete(f, epsilon, u.p0, u.p0+u.n)
			f.mod = u.mod
			f.b.Delete(u.p0, u.p0+u.n)
			for j = 0; j < len(f.text); j++ {
				textdelete(f.text[j], u.p0, u.p0+u.n, false)
			}
			*q0p = u.p0
			*q1p = u.p0

		case Insert:
			f.seq = u.seq
			fileuninsert(f, epsilon, u.p0, u.n)
			f.mod = u.mod
			up -= u.n
			for i = 0; i < u.n; i += n {
				n = u.n - i
				if n > bufs.RuneLen {
					n = bufs.RuneLen
				}
				delta.Read(up+i, buf[:n])
				f.b.Insert(u.p0+i, buf[:n])
				for j = 0; j < len(f.text); j++ {
					textinsert(f.text[j], u.p0+i, buf[:n], false)
				}
			}
			*q0p = u.p0
			*q1p = u.p0 + u.n

		case Filename:
			f.seq = u.seq
			fileunsetname(f, epsilon)
			f.mod = u.mod
			up -= u.n
			if u.n == 0 {
				f.name = nil
			} else {
				f.name = make([]rune, u.n)
			}
			delta.Read(up, f.name)
		}
		delta.Delete(up, delta.Len())
	}
	if isundo {
		f.seq = 0
	}
Return:
	bufs.FreeRunes(buf)
}

func filereset(f *File) {
	f.delta.Reset()
	f.epsilon.Reset()
	f.seq = 0
}

func fileclose(f *File) {
	f.name = nil
	f.text = nil
	f.b.Close()
	f.delta.Close()
	f.epsilon.Close()
	elogclose(f)
}

func filemark(f *File) {
	if f.epsilon.Len() != 0 {
		f.epsilon.Delete(0, f.epsilon.Len())
	}
	f.seq = seq
}
