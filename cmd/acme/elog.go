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
// #include "edit.h"

package main

import (
	"fmt"
	"os"
	"reflect"
	"unsafe"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
)

var Wsequence = "warning: changes out of sequence\n"
var warned = false

/*
 * Log of changes made by editing commands.  Three reasons for this:
 * 1) We want addresses in commands to apply to old file, not file-in-change.
 * 2) It's difficult to track changes correctly as things move, e.g. ,x m$
 * 3) This gives an opportunity to optimize by merging adjacent changes.
 * It's a little bit like the Undo/Redo log in Files, but Point 3) argues for a
 * separate implementation.  To do this well, we use Replace as well as
 * Insert and Delete
 */

type Buflog struct {
	typ int
	q0  int
	nd  int
	nr  int
}

const (
	Buflogsize = int(unsafe.Sizeof(Buflog{})) / runes.RuneSize
)

/*
 * Minstring shouldn't be very big or we will do lots of I/O for small changes.
 * Maxstring is RBUFSIZE so we can fbufalloc() once and not realloc elog.r.
 */

const (
	Minstring = 16
	Maxstring = bufs.RuneLen
)

type elogFile struct {
	*File
	elogbuf   *disk.Buffer
	elog      Elog
	editclean bool
}

var elogs = make(map[*File]*elogFile)

func eloginit(f *File) *elogFile {
	if ef := elogs[f]; ef != nil {
		return ef
	}
	ef := &elogFile{File: f}
	ef.elog.typ = elogNull
	ef.elogbuf = new(disk.Buffer)
	ef.elog.r = bufs.AllocRunes()
	elogs[f] = ef
	return ef
}

func elogreset(f *elogFile) {
	f.elog.typ = elogNull
	f.elog.nd = 0
	f.elog.r = f.elog.r[:0]
}

func elogfind(f *File) *elogFile {
	return elogs[f]
}

func elogterm(f *elogFile) {
	elogreset(f)
	f.elogbuf.Reset()
	f.elog.typ = elogEmpty
	bufs.FreeRunes(f.elog.r)
	f.elog.r = nil
	warned = false
	delete(elogs, f.File)
}

func elogflush(f *elogFile) {
	var b Buflog
	b.typ = f.elog.typ
	b.q0 = f.elog.q0
	b.nd = f.elog.nd
	b.nr = len(f.elog.r)
	switch f.elog.typ {
	default:
		alog.Printf("unknown elog type %#x\n", f.elog.typ)
	case elogNull:
		break
	case elogInsert,
		elogReplace:
		if len(f.elog.r) > 0 {
			f.elogbuf.Insert(f.elogbuf.Len(), f.elog.r)
		}
		fallthrough
	/* fall through */
	case elogDelete:
		f.elogbuf.Insert(f.elogbuf.Len(), buflogrunes(&b))
	}
	elogreset(f)
}

func buflogrunes(b *Buflog) []rune {
	var r []rune
	h := (*reflect.SliceHeader)(unsafe.Pointer(&r))
	h.Data = uintptr(unsafe.Pointer(b))
	h.Len = Buflogsize
	h.Cap = Buflogsize
	return r
}

func elogreplace(ff *File, q0 int, q1 int, r []rune) {
	if q0 == q1 && len(r) == 0 {
		return
	}
	f := eloginit(ff)
	if f.elog.typ != elogNull && q0 < f.elog.q0 {
		if !warned {
			warned = true
			alog.Printf(Wsequence)
		}
		elogflush(f)
	}
	/* try to merge with previous */
	gap := q0 - (f.elog.q0 + f.elog.nd) /* gap between previous and this */
	if f.elog.typ == elogReplace && len(f.elog.r)+gap+len(r) < Maxstring {
		if gap < Minstring {
			if gap > 0 {
				n := len(f.elog.r)
				f.Read(f.elog.q0+f.elog.nd, f.elog.r[n:n+gap])
				f.elog.r = f.elog.r[:n+gap]
			}
			f.elog.nd += gap + q1 - q0
			f.elog.r = append(f.elog.r, r...)
			return
		}
	}
	elogflush(f)
	f.elog.typ = elogReplace
	f.elog.q0 = q0
	f.elog.nd = q1 - q0
	if len(r) > bufs.RuneLen {
		editerror("internal error: replacement string too large(%d)", len(r))
	}
	f.elog.r = f.elog.r[:len(r)]
	copy(f.elog.r, r)
}

func eloginsert(ff *File, q0 int, r []rune) {
	if len(r) == 0 {
		return
	}
	f := eloginit(ff)
	if f.elog.typ != elogNull && q0 < f.elog.q0 {
		if !warned {
			warned = true
			alog.Printf(Wsequence)
		}
		elogflush(f)
	}
	/* try to merge with previous */
	if f.elog.typ == elogInsert && q0 == f.elog.q0 && len(f.elog.r)+len(r) < Maxstring {
		f.elog.r = append(f.elog.r, r...)
		return
	}
	for len(r) > 0 {
		elogflush(f)
		f.elog.typ = elogInsert
		f.elog.q0 = q0
		n := len(r)
		if n > bufs.RuneLen {
			n = bufs.RuneLen
		}
		f.elog.r = append(f.elog.r, r[:n]...)
		r = r[n:]
	}
}

func elogdelete(ff *File, q0 int, q1 int) {
	if q0 == q1 {
		return
	}
	f := eloginit(ff)
	if f.elog.typ != elogNull && q0 < f.elog.q0+f.elog.nd {
		if !warned {
			warned = true
			alog.Printf(Wsequence)
		}
		elogflush(f)
	}
	/* try to merge with previous */
	if f.elog.typ == elogDelete && f.elog.q0+f.elog.nd == q0 {
		f.elog.nd += q1 - q0
		return
	}
	elogflush(f)
	f.elog.typ = elogDelete
	f.elog.q0 = q0
	f.elog.nd = q1 - q0
}

func elogapply(f *elogFile) {
	const tracelog = false

	elogflush(f)
	log := f.elogbuf
	t := f.curtext

	buf := bufs.AllocRunes()
	mod := false

	owner := rune(0)
	if t.w != nil {
		owner = t.w.owner
		if owner == 0 {
			t.w.owner = 'E'
		}
	}

	/*
	 * The edit commands have already updated the selection in t->q0, t->q1,
	 * but using coordinates relative to the unmodified buffer.  As we apply the log,
	 * we have to update the coordinates to be relative to the modified buffer.
	 * Textinsert and textdelete will do this for us; our only work is to apply the
	 * convention that an insertion at t->q0==t->q1 is intended to select the
	 * inserted text.
	 */

	/*
	 * We constrain the addresses in here (with textconstrain()) because
	 * overlapping changes will generate bogus addresses.   We will warn
	 * about changes out of sequence but proceed anyway; here we must
	 * keep things in range.
	 */

	for log.Len() > 0 {
		up := log.Len() - Buflogsize
		var b Buflog
		log.Read(up, buflogrunes(&b))
		var tq1 int
		var tq0 int
		var n int
		var i int
		switch b.typ {
		default:
			fmt.Fprintf(os.Stderr, "elogapply: %#x\n", b.typ)
			panic("elogapply")

		case elogReplace:
			if tracelog {
				alog.Printf("elog replace %d %d (%d %d)\n", b.q0, b.q0+b.nd, t.q0, t.q1)
			}
			if !mod {
				mod = true
				f.Mark()
			}
			textconstrain(t, b.q0, b.q0+b.nd, &tq0, &tq1)
			textdelete(t, tq0, tq1, true)
			up -= b.nr
			for i = 0; i < b.nr; i += n {
				n = b.nr - i
				if n > bufs.RuneLen {
					n = bufs.RuneLen
				}
				log.Read(up+i, buf[:n])
				textinsert(t, tq0+i, buf[:n], true)
			}
			if t.q0 == b.q0 && t.q1 == b.q0 {
				t.q1 += b.nr
			}

		case elogDelete:
			if tracelog {
				alog.Printf("elog delete %d %d (%d %d)\n", b.q0, b.q0+b.nd, t.q0, t.q1)
			}
			if !mod {
				mod = true
				f.Mark()
			}
			textconstrain(t, b.q0, b.q0+b.nd, &tq0, &tq1)
			textdelete(t, tq0, tq1, true)

		case elogInsert:
			if tracelog {
				alog.Printf("elog insert %d %d (%d %d)\n", b.q0, b.q0+b.nr, t.q0, t.q1)
			}
			if !mod {
				mod = true
				f.Mark()
			}
			textconstrain(t, b.q0, b.q0, &tq0, &tq1)
			up -= b.nr
			for i = 0; i < b.nr; i += n {
				n = b.nr - i
				if n > bufs.RuneLen {
					n = bufs.RuneLen
				}
				log.Read(up+i, buf[:n])
				textinsert(t, tq0+i, buf[:n], true)
			}
			if t.q0 == b.q0 && t.q1 == b.q0 {
				t.q1 += b.nr
			}

			/*		case Filename:
					f->seq = u.seq;
					fileunsetname(f, epsilon);
					f->mod = u.mod;
					up -= u.n;
					free(f->name);
					if(u.n == 0)
						f->name = nil;
					else
						f->name = runemalloc(u.n);
					bufread(delta, up, f->name, u.n);
					f->nname = u.n;
					break;
			*/
		}
		log.Delete(up, log.Len())
	}
	bufs.FreeRunes(buf)
	elogterm(f)

	/*
	 * Bad addresses will cause bufload to crash, so double check.
	 * If changes were out of order, we expect problems so don't complain further.
	 */
	if t.q0 > f.Len() || t.q1 > f.Len() || t.q0 > t.q1 {
		if !warned {
			alog.Printf("elogapply: can't happen %d %d %d\n", t.q0, t.q1, f.Len())
		}
		t.q1 = util.Min(t.q1, f.Len())
		t.q0 = util.Min(t.q0, t.q1)
	}

	if t.w != nil {
		t.w.owner = owner
	}
}

const (
	elogEmpty    = 0
	elogNull     = '-'
	elogDelete   = 'd'
	elogInsert   = 'i'
	elogReplace  = 'r'
	elogFilename = 'f'
)
