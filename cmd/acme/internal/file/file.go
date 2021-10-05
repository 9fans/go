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

package file

import (
	"fmt"
	"os"
	"reflect"
	"unsafe"

	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
)

type File struct {
	view    View
	b       disk.Buffer
	delta   disk.Buffer
	epsilon disk.Buffer
	name    []rune
	seq     int
	mod     bool
}

func (f *File) SetView(v View) { f.view = v }

type View interface {
	Insert(int, []rune)
	Delete(int, int)
}

const (
	typeEmpty    = 0
	typeNull     = '-'
	typeDelete   = 'd'
	typeInsert   = 'i'
	typeReplace  = 'r'
	typeFilename = 'f'
)

type undo struct {
	typ int
	mod bool
	seq int
	p0  int
	n   int
}

const undoSize = int(unsafe.Sizeof(undo{})) / runes.RuneSize

func undorunes(u *undo) []rune {
	var r []rune
	h := (*reflect.SliceHeader)(unsafe.Pointer(&r))
	h.Data = uintptr(unsafe.Pointer(u))
	h.Len = undoSize
	h.Cap = undoSize
	return r
}

func (f *File) Len() int { return f.b.Len() }

func (f *File) CanUndo() bool { return f.delta.Len() > 0 }

func (f *File) CanRedo() bool { return f.epsilon.Len() > 0 }

func (f *File) Mod() bool { return f.mod }

func (f *File) SetMod(b bool) { f.mod = b }

var Seq int

func (f *File) Seq() int { return f.seq }

func (f *File) SetSeq(seq int) { f.seq = seq }

func (f *File) Mark() {
	if f.epsilon.Len() != 0 {
		f.epsilon.Delete(0, f.epsilon.Len())
	}
	f.seq = Seq
}

func (f *File) Read(pos int, data []rune) { f.b.Read(pos, data) }

func (f *File) Insert(p0 int, s []rune) {
	if p0 > f.b.Len() {
		util.Fatal("internal error: fileinsert")
	}
	if f.seq > 0 {
		f.uninsert(&f.delta, p0, len(s))
	}
	f.b.Insert(p0, s)
	if len(s) != 0 {
		f.mod = true
	}
}

func (f *File) uninsert(delta *disk.Buffer, p0, ns int) {
	var u undo
	/* undo an insertion by deleting */
	u.typ = typeDelete
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = p0
	u.n = ns
	delta.Insert(delta.Len(), undorunes(&u))
}

func (f *File) Delete(p0, p1 int) {
	if !(p0 <= p1 && p0 <= f.b.Len()) || !(p1 <= f.b.Len()) {
		util.Fatal("internal error: filedelete")
	}
	if f.seq > 0 {
		f.undelete(&f.delta, p0, p1)
	}
	f.b.Delete(p0, p1)
	if p1 > p0 {
		f.mod = true
	}
}

func (f *File) undelete(delta *disk.Buffer, p0, p1 int) {
	var u undo
	/* undo a deletion by inserting */
	u.typ = typeInsert
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

func (f *File) Name() []rune { return f.name }

func (f *File) SetName(name []rune) {
	if f.seq > 0 {
		f.unsetname(&f.delta)
	}
	f.name = runes.Clone(name)
}

func (f *File) unsetname(delta *disk.Buffer) {
	var u undo
	/* undo a file name change by restoring old name */
	u.typ = typeFilename
	u.mod = f.mod
	u.seq = f.seq
	u.p0 = 0 /* unused */
	u.n = len(f.name)
	if len(f.name) != 0 {
		delta.Insert(delta.Len(), f.name)
	}
	delta.Insert(delta.Len(), undorunes(&u))
}

/* return sequence number of pending redo */
func (f *File) RedoSeq() int {
	delta := &f.epsilon
	if delta.Len() == 0 {
		return 0
	}
	var u undo
	delta.Read(delta.Len()-undoSize, undorunes(&u))
	return u.seq
}

func (f *File) Undo(isundo bool, q0p, q1p *int) {
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
		up := delta.Len() - undoSize
		var u undo
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
		var i int
		switch u.typ {
		default:
			fmt.Fprintf(os.Stderr, "undo: %#x\n", u.typ)
			panic("undo")

		case typeDelete:
			f.seq = u.seq
			f.undelete(epsilon, u.p0, u.p0+u.n)
			f.mod = u.mod
			f.b.Delete(u.p0, u.p0+u.n)
			f.view.Delete(u.p0, u.p0+u.n)
			*q0p = u.p0
			*q1p = u.p0

		case typeInsert:
			f.seq = u.seq
			f.uninsert(epsilon, u.p0, u.n)
			f.mod = u.mod
			up -= u.n
			for i = 0; i < u.n; i += n {
				n = u.n - i
				if n > bufs.RuneLen {
					n = bufs.RuneLen
				}
				delta.Read(up+i, buf[:n])
				f.b.Insert(u.p0+i, buf[:n])
				f.view.Insert(u.p0+i, buf[:n])
			}
			*q0p = u.p0
			*q1p = u.p0 + u.n

		case typeFilename:
			f.seq = u.seq
			f.unsetname(epsilon)
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

func (f *File) ResetLogs() {
	f.delta.Reset()
	f.epsilon.Reset()
	f.seq = 0
}

func (f *File) Truncate() { f.b.Reset() }

func (f *File) Close() {
	f.name = nil
	f.view = nil
	f.b.Close()
	f.delta.Close()
	f.epsilon.Close()
}
