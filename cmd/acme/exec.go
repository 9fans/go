// #include <u.h>
// #include <libc.h>
// #include <bio.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <plumb.h>
// #include <libsec.h>
// #include <9pclient.h>
// #include "dat.h"
// #include "fns.h"

package main

import (
	"bufio"
	"crypto/sha1"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"unicode/utf8"

	addrpkg "9fans.net/go/cmd/acme/internal/addr"
	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

var snarfbuf disk.Buffer

/*
 * These functions get called as:
 *
 *	fn(et, t, argt, flag1, flag2, arg);
 *
 * Where the arguments are:
 *
 *	et: the Text* in which the executing event (click) occurred
 *	t: the Text* containing the current selection (Edit, Cut, Snarf, Paste)
 *	argt: the Text* containing the argument for a 2-1 click.
 *	flag1: from Exectab entry
 * 	flag2: from Exectab entry
 *	arg: the command line remainder (e.g., "x" if executing "Dump x")
 */

type Exectab struct {
	name  []rune
	fn    func(et, t, argt *Text, flag1, flag2 bool, s []rune)
	mark  bool
	flag1 bool
	flag2 bool
}

var exectab = [30]Exectab{
	Exectab{[]rune("Abort"), doabort, false, XXX, XXX},
	Exectab{[]rune("Cut"), cut, true, true, true},
	Exectab{[]rune("Del"), del, false, false, XXX},
	Exectab{[]rune("Delcol"), delcol, false, XXX, XXX},
	Exectab{[]rune("Delete"), del, false, true, XXX},
	Exectab{[]rune("Dump"), dump, false, true, XXX},
	Exectab{[]rune("Edit"), edit, false, XXX, XXX},
	Exectab{[]rune("Exit"), xexit, false, XXX, XXX},
	Exectab{[]rune("Font"), fontx, false, XXX, XXX},
	Exectab{[]rune("Get"), get, false, true, XXX},
	Exectab{[]rune("ID"), id, false, XXX, XXX},
	Exectab{[]rune("Incl"), incl, false, XXX, XXX},
	Exectab{[]rune("Indent"), indent, false, XXX, XXX},
	Exectab{[]rune("Kill"), xkill, false, XXX, XXX},
	Exectab{[]rune("Load"), dump, false, false, XXX},
	Exectab{[]rune("Local"), local, false, XXX, XXX},
	Exectab{[]rune("Look"), look, false, XXX, XXX},
	Exectab{[]rune("New"), new_, false, XXX, XXX},
	Exectab{[]rune("Newcol"), newcol, false, XXX, XXX},
	Exectab{[]rune("Paste"), paste, true, true, XXX},
	Exectab{[]rune("Put"), put, false, XXX, XXX},
	Exectab{[]rune("Putall"), putall, false, XXX, XXX},
	Exectab{[]rune("Redo"), undo, false, false, XXX},
	Exectab{[]rune("Send"), sendx, true, XXX, XXX},
	Exectab{[]rune("Snarf"), cut, false, true, false},
	Exectab{[]rune("Sort"), xsort, false, XXX, XXX},
	Exectab{[]rune("Tab"), tab, false, XXX, XXX},
	Exectab{[]rune("Undo"), undo, false, true, XXX},
	Exectab{[]rune("Zerox"), zeroxx, false, XXX, XXX},
}

func lookup(r []rune) *Exectab {
	r = runes.SkipBlank(r)
	if len(r) == 0 {
		return nil
	}
	r = r[:len(r)-len(runes.SkipNonBlank(r))]
	for i := range exectab {
		e := &exectab[i]
		if runes.Equal(r, e.name) {
			return e
		}
	}
	return nil
}

func isexecc(c rune) bool {
	if runes.IsFilename(c) {
		return true
	}
	return c == '<' || c == '|' || c == '>'
}

func execute(t *Text, aq0 int, aq1 int, external bool, argt *Text) {
	q0 := aq0
	q1 := aq1
	var c rune
	if q1 == q0 { /* expand to find word (actually file name) */
		/* if in selection, choose selection */
		if t.q1 > t.q0 && t.q0 <= q0 && q0 <= t.q1 {
			q0 = t.q0
			q1 = t.q1
		} else {
			for q1 < t.Len() && func() bool { c = t.RuneAt(q1); return isexecc(c) }() && c != ':' {
				q1++
			}
			for q0 > 0 && func() bool { c = t.RuneAt(q0 - 1); return isexecc(c) }() && c != ':' {
				q0--
			}
			if q1 == q0 {
				return
			}
		}
	}
	r := make([]rune, q1-q0)
	t.file.Read(q0, r)
	e := lookup(r)
	var a, aa *string
	var n int
	if !external && t.w != nil && t.w.nopen[QWevent] > 0 {
		f := 0
		if e != nil {
			f |= 1
		}
		if q0 != aq0 || q1 != aq1 {
			t.file.Read(aq0, r[:aq1-aq0])
			f |= 2
		}
		aa = getbytearg(argt, true, true, &a)
		if a != nil {
			if len(*a) > EVENTSIZE { /* too big; too bad */
				alog.Printf("argument string too long\n")
				return
			}
			f |= 8
		}
		c = 'x'
		if t.what == Body {
			c = 'X'
		}
		n = aq1 - aq0
		if n <= EVENTSIZE {
			r := r
			if len(r) > n {
				r = r[:n]
			}
			winevent(t.w, "%c%d %d %d %d %s\n", c, aq0, aq1, f, n, string(r))
		} else {
			winevent(t.w, "%c%d %d %d 0 \n", c, aq0, aq1, f)
		}
		if q0 != aq0 || q1 != aq1 {
			n = q1 - q0
			t.file.Read(q0, r[:n])
			if n <= EVENTSIZE {
				winevent(t.w, "%c%d %d 0 %d %s\n", c, q0, q1, n, string(r[:n]))
			} else {
				winevent(t.w, "%c%d %d 0 0 \n", c, q0, q1)
			}
		}
		if a != nil {
			winevent(t.w, "%c0 0 0 %d %s\n", c, utf8.RuneCountInString(*a), *a)
			if aa != nil {
				winevent(t.w, "%c0 0 0 %d %s\n", c, utf8.RuneCountInString(*aa), *aa)
			} else {
				winevent(t.w, "%c0 0 0 0 \n", c)
			}
		}
		return
	}
	if e != nil {
		if e.mark && seltext != nil {
			if seltext.what == Body {
				file.Seq++
				seltext.w.body.file.Mark()
			}
		}
		s := runes.SkipBlank(r[:q1-q0])
		s = runes.SkipNonBlank(s)
		s = runes.SkipBlank(s)
		e.fn(t, seltext, argt, e.flag1, e.flag2, s)
		return
	}

	b := string(r)
	dir := dirname(t, nil)
	if len(dir) == 1 && dir[0] == '.' { /* sigh */
		dir = nil
	}
	aa = getbytearg(argt, true, true, &a)
	if t.w != nil {
		util.Incref(&t.w.ref)
	}
	run(t.w, b, dir, true, aa, a, false)
}

func printarg(argt *Text, q0 int, q1 int) *string {
	if argt.what != Body || argt.file.Name() == nil {
		return nil
	}
	var buf string
	if q0 == q1 {
		buf = fmt.Sprintf("%s:#%d", string(argt.file.Name()), q0)
	} else {
		buf = fmt.Sprintf("%s:#%d,#%d", string(argt.file.Name()), q0, q1)
	}
	return &buf
}

func getarg(argt *Text, doaddr, dofile bool, rp *[]rune) *string {
	*rp = nil
	if argt == nil {
		return nil
	}
	textcommit(argt, true)
	var e Expand
	var a *string
	if expand(argt, argt.q0, argt.q1, &e) {
		if len(e.name) > 0 && dofile {
			if doaddr {
				a = printarg(argt, e.q0, e.q1)
			}
			*rp = e.name
			return a
		}
	} else {
		e.q0 = argt.q0
		e.q1 = argt.q1
	}
	n := e.q1 - e.q0
	*rp = make([]rune, n)
	argt.file.Read(e.q0, *rp)
	if doaddr {
		a = printarg(argt, e.q0, e.q1)
	}
	return a
}

func getbytearg(argt *Text, doaddr, dofile bool, bp **string) *string {
	*bp = nil
	var r []rune
	a := getarg(argt, doaddr, dofile, &r)
	if r == nil {
		return nil
	}
	b := string(r)
	*bp = &b
	return a
}

var doabort_n int

func doabort(_, _, _ *Text, _, _ bool, _ []rune) {
	if doabort_n == 0 {
		doabort_n++
		alog.Printf("executing Abort again will call abort()\n")
		return
	}
	panic("abort")
}

func newcol(et, _, _ *Text, _, _ bool, _ []rune) {

	c := rowadd(et.row, nil, -1)
	if c != nil {
		w := coladd(c, nil, nil, -1)
		winsettag(w)
		xfidlog(w, "new")
	}
}

func delcol(et, _, _ *Text, _, _ bool, _ []rune) {

	c := et.col
	if c == nil || !colclean(c) {
		return
	}
	for i := 0; i < len(c.w); i++ {
		w := c.w[i]
		if w.nopen[QWevent]+w.nopen[QWaddr]+w.nopen[QWdata]+w.nopen[QWxdata] > 0 {
			alog.Printf("can't delete column; %s is running an external command\n", string(w.body.file.Name()))
			return
		}
	}
	rowclose(et.col.row, et.col, true)
}

func del(et, _, _ *Text, isDelete, _ bool, _ []rune) {
	if et.col == nil || et.w == nil {
		return
	}
	if isDelete || len(et.w.body.file.text) > 1 || winclean(et.w, false) {
		colclose(et.col, et.w, true)
	}
}

func xsort(et, _, _ *Text, _, _ bool, _ []rune) {

	if et.col != nil {
		colsort(et.col)
	}
}

func seqof(w *Window, isundo bool) int {
	/* if it's undo, see who changed with us */
	if isundo {
		return w.body.file.Seq()
	}
	/* if it's redo, see who we'll be sync'ed up with */
	return w.body.file.RedoSeq()
}

func undo(et, _, _ *Text, isundo, _ bool, _ []rune) {
	if et == nil || et.w == nil {
		return
	}
	seq := seqof(et.w, isundo)
	if seq == 0 {
		/* nothing to undo */
		return
	}
	/*
	 * Undo the executing window first. Its display will update. other windows
	 * in the same file will not call show() and jump to a different location in the file.
	 * Simultaneous changes to other files will be chaotic, however.
	 */
	winundo(et.w, isundo)
	for i := 0; i < len(row.col); i++ {
		c := row.col[i]
		for j := 0; j < len(c.w); j++ {
			w := c.w[j]
			if w == et.w {
				continue
			}
			if seqof(w, isundo) == seq {
				winundo(w, isundo)
			}
		}
	}
}

func getname(t *Text, argt *Text, arg []rune, isput bool) string {
	var r []rune
	getarg(argt, false, true, &r)
	promote := false
	if r == nil {
		promote = true
	} else if isput {
		/* if are doing a Put, want to synthesize name even for non-existent file */
		/* best guess is that file name doesn't contain a slash */
		promote = true
		for i := 0; i < len(r); i++ {
			if r[i] == '/' {
				promote = false
				break
			}
		}
		if promote {
			t = argt
			arg = r
		}
	}
	if promote {
		if len(arg) == 0 {
			return string(t.file.Name())
		}
		var dir []rune
		/* prefix with directory name if necessary */
		dir = nil
		if len(arg) > 0 && arg[0] != '/' {
			dir = dirname(t, nil)
			if len(dir) == 1 && dir[0] == '.' { /* sigh */
				dir = nil
			}
		}
		if dir != nil {
			r = make([]rune, len(dir)+1+len(arg))
			r = append(r, dir...)
			if len(r) > 0 && r[len(r)-1] != '/' && len(arg) > 0 && arg[0] != '/' {
				r = append(r, '/')
			}
			r = append(r, arg...)
		} else {
			r = arg
		}
	}
	return string(r)
}

func zeroxx(et, t, _ *Text, _, _ bool, _ []rune) {

	locked := false
	if t != nil && t.w != nil && t.w != et.w {
		locked = true
		c := 'M'
		if et.w != nil {
			c = et.w.owner
		}
		winlock(t.w, c)
	}
	if t == nil {
		t = et
	}
	if t == nil || t.w == nil {
		return
	}
	t = &t.w.body
	if t.w.isdir {
		alog.Printf("%s is a directory; Zerox illegal\n", string(t.file.Name()))
	} else {
		nw := coladd(t.w.col, nil, t.w, -1)
		/* ugly: fix locks so w->unlock works */
		winlock1(nw, t.w.owner)
		xfidlog(nw, "zerox")
	}
	if locked {
		winunlock(t.w)
	}
}

type TextAddr struct {
	lorigin int
	rorigin int
	lq0     int
	rq0     int
	lq1     int
	rq1     int
}

func get(et, t, argt *Text, flag1, _ bool, arg []rune) {
	if flag1 {
		if et == nil || et.w == nil {
			return
		}
	}
	if !et.w.isdir && (et.w.body.Len() > 0 && !winclean(et.w, true)) {
		return
	}
	w := et.w
	t = &w.body
	name := getname(t, argt, arg, false)
	if name == "" {
		alog.Printf("no file name\n")
		return
	}
	if len(t.file.text) > 1 {
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			alog.Printf("%s is a directory; can't read with multiple windows on it\n", name)
			return
		}
	}
	addr := make([]TextAddr, len(t.file.text))
	for i := 0; i < len(t.file.text); i++ {
		a := &addr[i]
		u := t.file.text[i]
		a.lorigin = nlcount(u, 0, u.org, &a.rorigin)
		a.lq0 = nlcount(u, 0, u.q0, &a.rq0)
		a.lq1 = nlcount(u, u.q0, u.q1, &a.rq1)
	}
	r := []rune(name)
	for i := 0; i < len(t.file.text); i++ {
		u := t.file.text[i]
		/* second and subsequent calls with zero an already empty buffer, but OK */
		textreset(u)
		windirfree(u.w)
	}
	samename := runes.Equal(r, t.file.Name())
	textload(t, 0, name, samename)
	var dirty bool
	if samename {
		t.file.SetMod(false)
		dirty = false
	} else {
		t.file.SetMod(true)
		dirty = true
	}
	for i := 0; i < len(t.file.text); i++ {
		t.file.text[i].w.dirty = dirty
	}
	winsettag(w)
	t.file.unread = false
	for i := 0; i < len(t.file.text); i++ {
		u := t.file.text[i]
		textsetselect(&u.w.tag, u.w.tag.Len(), u.w.tag.Len())
		if samename {
			a := &addr[i]
			// Printf("%d %d %d %d %d %d\n", a->lorigin, a->rorigin, a->lq0, a->rq0, a->lq1, a->rq1);
			q0 := addrpkg.Advance(u, 0, a.lq0, a.rq0)
			q1 := addrpkg.Advance(u, q0, a.lq1, a.rq1)
			textsetselect(u, q0, q1)
			q0 = addrpkg.Advance(u, 0, a.lorigin, a.rorigin)
			textsetorigin(u, q0, false)
		}
		textscrdraw(u)
	}
	xfidlog(w, "get")
}

func checksha1(name string, f *File, info os.FileInfo) {
	fd, err := os.Open(name)
	if err != nil {
		return
	}
	buf := make([]byte, bufs.Len)
	h := sha1.New()
	for {
		n, err := fd.Read(buf)
		h.Write(buf[:n])
		if err != nil {
			break
		}
	}
	fd.Close()
	var out [20]uint8
	h.Sum(out[:0])
	if out == f.sha1 {
		f.info = info
	}
}

func sameInfo(fi1, fi2 os.FileInfo) bool {
	return fi1 != nil && fi2 != nil && os.SameFile(fi1, fi2) && fi1.ModTime().Equal(fi2.ModTime()) && fi1.Size() == fi2.Size()
}

func putfile(f *File, q0 int, q1 int, namer []rune) {
	w := f.curtext.w
	name := string(namer)
	info, err := os.Stat(name)
	if err == nil && runes.Equal(namer, f.Name()) {
		if !sameInfo(info, f.info) {
			checksha1(name, f, info)
		}
		if !sameInfo(info, f.info) {
			if f.unread {
				alog.Printf("%s not written; file already exists\n", name)
			} else {
				alog.Printf("%s modified since last read\n\twas %v; now %v\n", name, f.info.ModTime().Format(timefmt), info.ModTime().Format(timefmt))
			}
			f.info = info
			return
		}
	}
	fd, err := os.Create(name)
	if err != nil {
		alog.Printf("can't create file %s: %v\n", name, err)
		return
	}
	defer fd.Close() // for Rescue case

	// Use bio in order to force the writes to be large and
	// block-aligned (bio's default is 8K). This is not strictly
	// necessary; it works around some buggy underlying
	// file systems that mishandle unaligned writes.
	// https://codereview.appspot.com/89550043/
	b := bufio.NewWriter(fd)
	r := bufs.AllocRunes()
	s := bufs.AllocRunes()
	info, err = fd.Stat()
	h := sha1.New()
	isAppend := err == nil && info.Size() > 0 && info.Mode()&os.ModeAppend != 0
	if isAppend {
		alog.Printf("%s not written; file is append only\n", name)
		goto Rescue2
	}
	{
		var n int
		for q := q0; q < q1; q += n {
			n = q1 - q
			if n > bufs.Len/utf8.UTFMax {
				n = bufs.Len / utf8.UTFMax
			}
			f.Read(q, r[:n])
			buf := []byte(string(r[:n])) // TODO(rsc)
			h.Write(buf)
			if _, err := b.Write(buf); err != nil { // TODO(rsc): avoid alloc
				alog.Printf("can't write file %s: %v\n", name, err)
				goto Rescue2
			}
		}
	}
	if err := b.Flush(); err != nil {
		alog.Printf("can't write file %s: %v\n", name, err)
		goto Rescue2
	}
	if err := fd.Close(); err != nil {
		alog.Printf("can't write file %s: %v\n", name, err)
		goto Rescue2 // flush or close failed
	}
	if runes.Equal(namer, f.Name()) {
		if q0 != 0 || q1 != f.Len() {
			f.SetMod(true)
			w.dirty = true
			f.unread = true
		} else {
			// In case the file is on NFS, reopen the fd
			// before dirfstat to cause the attribute cache
			// to be updated (otherwise the mtime in the
			// dirfstat below will be stale and not match
			// what NFS sees).  The file is already written,
			// so this should be a no-op when not on NFS.
			// Opening for OWRITE (but no truncation)
			// in case we don't have read permission.
			// (The create above worked, so we probably
			// still have write permission.)
			if fd, err := os.OpenFile(name, os.O_WRONLY, 0); err == nil {
				if info1, err := fd.Stat(); err == nil {
					info = info1
				}
				fd.Close()
			}
			f.info = info
			h.Sum(f.sha1[:0])
			f.SetMod(false)
			w.dirty = false
			f.unread = false
		}
		for i := 0; i < len(f.text); i++ {
			f.text[i].w.putseq = f.Seq()
			f.text[i].w.dirty = w.dirty
		}
	}
	bufs.FreeRunes(s)
	winsettag(w)
	return

Rescue2:
	bufs.FreeRunes(s)
	bufs.FreeRunes(r)
	/* fall through */
}

func trimspaces(et *Text) {
	t := &et.w.body
	f := t.file
	marked := 0

	if t.w != nil && et.w != t.w {
		/* can this happen when t == &et->w->body? */
		c := 'M'
		if et.w != nil {
			c = et.w.owner
		}
		winlock(t.w, c)
	}

	r := bufs.AllocRunes()
	q0 := f.Len()
	delstart := q0 /* end of current space run, or 0 if no active run; = q0 to delete spaces before EOF */
	for q0 > 0 {
		n := bufs.RuneLen
		if n > q0 {
			n = q0
		}
		q0 -= n
		f.Read(q0, r[:n])
		for i := n; ; i-- {
			if i == 0 || (r[i-1] != ' ' && r[i-1] != '\t') {
				// Found non-space or start of buffer. Delete active space run.
				if q0+i < delstart {
					if marked == 0 {
						marked = 1
						file.Seq++
						f.Mark()
					}
					textdelete(t, q0+i, delstart, true)
				}
				if i == 0 {
					/* keep run active into tail of next buffer */
					if delstart > 0 {
						delstart = q0
					}
					break
				}
				delstart = 0
				if r[i-1] == '\n' {
					delstart = q0 + i - 1 /* delete spaces before this newline */
				}
			}
		}
	}
	bufs.FreeRunes(r)

	if t.w != nil && et.w != t.w {
		winunlock(t.w)
	}
}

func put(et, _, argt *Text, _, _ bool, arg []rune) {

	if et == nil || et.w == nil || et.w.isdir {
		return
	}
	w := et.w
	f := w.body.file
	name := getname(&w.body, argt, arg, true)
	if name == "" {
		alog.Printf("no file name\n")
		return
	}
	if w.autoindent {
		trimspaces(et)
	}
	namer := []rune(name)
	putfile(f, 0, f.Len(), namer)
	xfidlog(w, "put")
}

func dump(_, _, argt *Text, isdump, _ bool, arg []rune) {
	var name *string
	if len(arg) != 0 {
		s := string(arg)
		name = &s
	} else {
		getbytearg(argt, false, true, &name)
	}
	if isdump {
		rowdump(&row, name)
	} else {
		rowload(&row, name, false)
	}
}

func cut(et, t, _ *Text, dosnarf, docut bool, _ []rune) {

	/*
	 * if not executing a mouse chord (et != t) and snarfing (dosnarf)
	 * and executed Cut or Snarf in window tag (et->w != nil),
	 * then use the window body selection or the tag selection
	 * or do nothing at all.
	 */
	if et != t && dosnarf && et.w != nil {
		if et.w.body.q1 > et.w.body.q0 {
			t = &et.w.body
			if docut {
				t.file.Mark() /* seq has been incremented by execute */
			}
		} else if et.w.tag.q1 > et.w.tag.q0 {
			t = &et.w.tag
		} else {
			t = nil
		}
	}
	if t == nil { /* no selection */
		return
	}

	locked := false
	if t.w != nil && et.w != t.w {
		locked = true
		c := 'M'
		if et.w != nil {
			c = et.w.owner
		}
		winlock(t.w, c)
	}
	if t.q0 == t.q1 {
		if locked {
			winunlock(t.w)
		}
		return
	}
	if dosnarf {
		q0 := t.q0
		q1 := t.q1
		snarfbuf.Delete(0, snarfbuf.Len())
		r := bufs.AllocRunes()
		for q0 < q1 {
			n := q1 - q0
			if n > bufs.RuneLen {
				n = bufs.RuneLen
			}
			t.file.Read(q0, r[:n])
			snarfbuf.Insert(snarfbuf.Len(), r[:n])
			q0 += n
		}
		bufs.FreeRunes(r)
		acmeputsnarf()
	}
	if docut {
		textdelete(t, t.q0, t.q1, true)
		textsetselect(t, t.q0, t.q0)
		if t.w != nil {
			textscrdraw(t)
			winsettag(t.w)
		}
	} else if dosnarf { /* Snarf command */
		argtext = t
	}
	if locked {
		winunlock(t.w)
	}
}

func paste(et, t, _ *Text, selectall, tobody bool, _ []rune) {

	/* if(tobody), use body of executing window  (Paste or Send command) */
	if tobody && et != nil && et.w != nil {
		t = &et.w.body
		t.file.Mark() /* seq has been incremented by execute */
	}
	if t == nil {
		return
	}

	acmegetsnarf()
	if t == nil || snarfbuf.Len() == 0 {
		return
	}
	if t.w != nil && et.w != t.w {
		c := 'M'
		if et.w != nil {
			c = et.w.owner
		}
		winlock(t.w, c)
	}
	cut(t, t, nil, false, true, nil)
	q := 0
	q0 := t.q0
	q1 := t.q0 + snarfbuf.Len()
	r := bufs.AllocRunes()
	for q0 < q1 {
		n := q1 - q0
		if n > bufs.RuneLen {
			n = bufs.RuneLen
		}
		snarfbuf.Read(q, r[:n])
		textinsert(t, q0, r[:n], true)
		q += n
		q0 += n
	}
	bufs.FreeRunes(r)
	if selectall {
		textsetselect(t, t.q0, q1)
	} else {
		textsetselect(t, q1, q1)
	}
	if t.w != nil {
		textscrdraw(t)
		winsettag(t.w)
	}
	if t.w != nil && et.w != t.w {
		winunlock(t.w)
	}
}

func look(et, t, argt *Text, _, _ bool, arg []rune) {
	if et != nil && et.w != nil {
		t = &et.w.body
		if len(arg) > 0 {
			search(t, arg)
			return
		}
		var r []rune
		getarg(argt, false, false, &r)
		if r == nil {
			r = make([]rune, t.q1-t.q0)
			t.file.Read(t.q0, r)
		}
		search(t, r)
	}
}

var Lnl = []rune("\n")

func sendx(et, t, _ *Text, _, _ bool, _ []rune) {
	if et.w == nil {
		return
	}
	t = &et.w.body
	if t.q0 != t.q1 {
		cut(t, t, nil, true, false, nil)
	}
	textsetselect(t, t.Len(), t.Len())
	paste(t, t, nil, true, true, nil)
	if t.RuneAt(t.Len()-1) != '\n' {
		textinsert(t, t.Len(), Lnl, true)
		textsetselect(t, t.Len(), t.Len())
	}
	t.iq1 = t.q1
	textshow(t, t.q1, t.q1, true)
}

func edit(et, _, argt *Text, _, _ bool, arg []rune) {
	if et == nil {
		return
	}
	var r []rune
	getarg(argt, false, true, &r)
	file.Seq++
	if r != nil {
		editcmd(et, r)
	} else {
		editcmd(et, arg)
	}
}

func xexit(_, _, _ *Text, _, _ bool, _ []rune) {
	if rowclean(&row) {
		cexit <- 0
		runtime.Goexit() // TODO(rsc)
	}
}

func putall(et, _, _ *Text, _, _ bool, _ []rune) {
	for _, c := range row.col {
		for _, w := range c.w {
			if w.isscratch || w.isdir || len(w.body.file.Name()) == 0 {
				continue
			}
			if w.nopen[QWevent] > 0 {
				continue
			}
			a := string(w.body.file.Name())
			_, e := os.Stat(a)
			if w.body.file.Mod() || len(w.body.cache) != 0 {
				if e != nil {
					alog.Printf("no auto-Put of %s: %v\n", a, e)
				} else {
					wincommit(w, &w.body)
					put(&w.body, nil, nil, XXX, XXX, nil)
				}
			}
		}
	}
}

func id(et, _, _ *Text, _, _ bool, _ []rune) {
	if et != nil && et.w != nil {
		alog.Printf("/mnt/acme/%d/\n", et.w.id)
	}
}

func local(et, _, argt *Text, _, _ bool, arg []rune) {
	var a *string
	aa := getbytearg(argt, true, true, &a)
	dir := dirname(et, nil)
	if len(dir) == 1 && dir[0] == '.' { /* sigh */
		dir = nil
	}
	run(nil, string(arg), dir, false, aa, a, false)
}

func xkill(_, _, argt *Text, _, _ bool, arg []rune) {
	var r []rune
	getarg(argt, false, false, &r)
	if r != nil {
		xkill(nil, nil, nil, false, false, r)
	}
	/* loop condition: *arg is not a blank */
	for {
		a := runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			break
		}
		ckill <- runes.Clone(arg[:len(arg)-len(a)])
		arg = runes.SkipBlank(a)
	}
}

var Lfix = []rune("fix")
var Lvar = []rune("var")

func fontx(et, t, argt *Text, _, _ bool, arg []rune) {
	if et == nil || et.w == nil {
		return
	}
	t = &et.w.body
	var flag []rune
	var file []rune
	/* loop condition: *arg is not a blank */
	var r []rune
	for {
		a := runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			break
		}
		r = runes.Clone(arg[:len(arg)-len(a)])
		if runes.Equal(r, Lfix) || runes.Equal(r, Lvar) {
			flag = r
		} else {
			file = r
		}
		arg = runes.SkipBlank(a)
	}
	getarg(argt, false, true, &r)
	if r != nil {
		if runes.Equal(r, Lfix) || runes.Equal(r, Lvar) {
			flag = r
		} else {
			file = r
		}
	}
	fix := true
	var newfont *Reffont
	if flag != nil {
		fix = runes.Equal(flag, Lfix)
	} else if file == nil {
		newfont = rfget(false, false, false, "")
		if newfont != nil {
			fix = newfont.f.Name == t.fr.Font.Name
		}
	}
	var aa string
	if file != nil {
		newfont = rfget(fix, flag != nil, false, string(file))
	} else {
		newfont = rfget(fix, false, false, "")
	}
	if newfont != nil {
		display.ScreenImage.Draw(t.w.r, textcols[frame.BACK], nil, draw.ZP)
		rfclose(t.reffont)
		t.reffont = newfont
		t.fr.Font = newfont.f
		t.fr.InitTick()
		if t.w.isdir {
			t.all.Min.X++ /* force recolumnation; disgusting! */
			for i := 0; i < len(t.w.dlp); i++ {
				dp := t.w.dlp[i]
				aa = string(dp.r)
				dp.wid = newfont.f.StringWidth(aa)
			}
		}
		/* avoid shrinking of window due to quantization */
		colgrow(t.w.col, t.w, -1)
	}
}

func incl(et, _, argt *Text, _, _ bool, arg []rune) {
	if et == nil || et.w == nil {
		return
	}
	w := et.w
	n := 0
	var r []rune
	getarg(argt, false, true, &r)
	if r != nil {
		n++
		winaddincl(w, r)
	}
	/* loop condition: len(arg) == 0 || arg[0] is not a blank */
	for {
		a := runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			break
		}
		r = runes.Clone(arg[:len(arg)-len(a)])
		winaddincl(w, r)
		arg = runes.SkipBlank(a)
	}
	if n == 0 && len(w.incl) > 0 {
		for n = len(w.incl); ; {
			n--
			if n < 0 {
				break
			}
			alog.Printf("%s ", string(w.incl[n]))
		}
		alog.Printf("\n")
	}
}

var LON = []rune("ON")
var LOFF = []rune("OFF")
var Lon = []rune("on")

const (
	IGlobal = -2
	IError  = -1
	Ioff    = 0
	Ion     = 1
)

func indentval(s []rune) int {
	if len(s) < 2 {
		return IError
	}
	if runes.Equal(s, LON) {
		globalautoindent = true
		alog.Printf("Indent ON\n")
		return IGlobal
	}
	if runes.Equal(s, LOFF) {
		globalautoindent = false
		alog.Printf("Indent OFF\n")
		return IGlobal
	}
	if runes.Equal(s, Lon) {
		return Ion
	}
	return Ioff
}

func fixindent(w *Window, arg interface{}) {
	w.autoindent = globalautoindent
}

func indent(et, _, argt *Text, _, _ bool, arg []rune) {
	var w *Window
	if et != nil && et.w != nil {
		w = et.w
	}
	autoindent := IError
	var r []rune
	getarg(argt, false, true, &r)
	if len(r) > 0 {
		autoindent = indentval(r)
	} else {
		a := runes.SkipNonBlank(arg)
		if len(a) != len(arg) {
			autoindent = indentval(arg[:len(arg)-len(a)])
		}
	}
	if autoindent == IGlobal {
		allwindows(fixindent, nil)
	} else if w != nil && autoindent >= 0 {
		w.autoindent = autoindent == Ion
	}
}

func tab(et, _, argt *Text, _, _ bool, arg []rune) {
	if et == nil || et.w == nil {
		return
	}
	w := et.w
	var r []rune
	getarg(argt, false, true, &r)
	tab := 0
	if len(r) > 0 {
		p := string(r)
		if '0' <= p[0] && p[0] <= '9' {
			tab, _ = strconv.Atoi(p)
		}
	} else {
		a := runes.SkipNonBlank(arg)
		if len(a) != len(arg) {
			p := string(arg[:len(arg)-len(a)])
			if '0' <= p[0] && p[0] <= '9' {
				tab, _ = strconv.Atoi(p)
			}
		}
	}
	if tab > 0 {
		if w.body.tabstop != tab {
			w.body.tabstop = tab
			winresize(w, w.r, false, true)
		}
	} else {
		alog.Printf("%s: Tab %d\n", string(w.body.file.Name()), w.body.tabstop)
	}
}

func runproc(win *Window, s string, rdir []rune, newns bool, argaddr, xarg *string, c *Command, cpid chan int, iseditcmd bool) {
	t := strings.TrimLeft(s, " \n\t")
	name := t
	if i := strings.IndexAny(name, " \n\t"); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name += " " /* add blank here for ease in waittask */
	c.name = []rune(name)
	pipechar := '\x00'
	if len(t) > 0 && (t[0] == '<' || t[0] == '|' || t[0] == '>') {
		pipechar = rune(t[0])
		t = t[1:]
	}
	c.iseditcmd = iseditcmd
	c.text = s
	var sfd [3]*os.File
	if newns {
		var incl [][]rune
		var winid int
		/* end of args */
		var filename string
		if win != nil {
			filename = string(win.body.file.Name())
			if len(win.incl) > 0 {
				incl = make([][]rune, len(win.incl))
				for i := range win.incl {
					incl[i] = runes.Clone(win.incl[i])
				}
			}
			winid = win.id
		} else if activewin != nil {
			winid = activewin.id
		}

		os.Setenv("winid", fmt.Sprint(winid)) // TODO(rsc)

		if filename != "" {
			os.Setenv("%", filename)       // TODO(rsc)
			os.Setenv("samfile", filename) // TODO(rsc)
		}

		var err error
		c.md = fsysmount(rdir, incl)

		fs, err := client.MountServiceAname("acme", fmt.Sprint(c.md.id))
		if err != nil {
			fmt.Fprintf(os.Stderr, "child: can't mount acme: %v\n", err)
			fsysdelid(c.md)
			c.md = nil
			return // TODO(rsc): goto Fail?
		}
		if winid > 0 && (pipechar == '|' || pipechar == '>') {
			sfd[0], _ = fsopenfd(fs, fmt.Sprintf("%d/rdsel", winid), plan9.OREAD)
		} else {
			sfd[0], _ = os.Open(os.DevNull)
		}
		if (winid > 0 || iseditcmd) && (pipechar == '|' || pipechar == '<') {
			var buf string
			if iseditcmd {
				if winid > 0 {
					buf = fmt.Sprintf("%d/editout", winid)
				} else {
					buf = "editout"
				}
			} else {
				buf = fmt.Sprintf("%d/wrsel", winid)
			}
			sfd[1], _ = fsopenfd(fs, buf, plan9.OWRITE)
			sfd[2], _ = fsopenfd(fs, "cons", plan9.OWRITE)
		} else {
			sfd[1], _ = fsopenfd(fs, "cons", plan9.OWRITE)
			sfd[2] = sfd[1]
		}
		// fsunmount(fs) // TODO(rsc): implement
	} else {
		// TODO(rsc): This is "Local foo".
		// Interpret the command since a subshell will not be Local.
		// Can look for 'Local cd' and 'Local x=y'.
		alog.Printf("Local not implemented")
		goto Fail
	}
	if win != nil {
		winclose(win)
	}
	defer sfd[0].Close()
	defer sfd[1].Close()
	defer sfd[2].Close()

	if argaddr != nil {
		os.Setenv("acmeaddr", *argaddr) // TODO(rsc)
	}
	if acmeshell != "" {
		goto Hard
	}
	{
		inarg := false
		for _, r := range t {
			if r == ' ' || r == '\t' {
				continue
			}
			if r < ' ' {
				goto Hard
			}
			if strings.ContainsRune("#;&|^$=`'{}()<>[]*?^~`/", r) {
				goto Hard
			}
			inarg = true
		}
		if !inarg {
			goto Fail
		}

		var av []string
		ai := 0
		inarg = false
		for i, r := range t {
			if r == ' ' || r == '\t' {
				if inarg {
					inarg = false
					av[len(av)-1] = t[ai:i]
				}
				continue
			}
			if !inarg {
				inarg = true
				av = append(av, t[i:])
			}
		}
		if xarg != nil {
			av = append(av, *xarg)
		}
		c.av = av

		var dir string
		if rdir != nil {
			dir = string(rdir)
		}
		cmd := exec.Command(av[0], av[1:]...)
		cmd.Stdin = sfd[0]
		cmd.Stdout = sfd[1]
		cmd.Stderr = sfd[2]
		cmd.Dir = dir
		err := cmd.Start()
		if err == nil {
			if cpid != nil {
				cpid <- cmd.Process.Pid // TODO(rsc): send cmd.Process
			}
			return
		}
		goto Fail
	}

Hard:
	{
		if xarg != nil {
			t += " '" + *xarg + "'" // BUG: what if quote in *xarg? TODO(rsc)
			c.text = t
		}
		var dir string
		if rdir != nil {
			dir = string(rdir)
		}
		shell := acmeshell
		if shell == "" {
			shell = "rc"
		}
		/*static void *parg[2]; */
		cmd := exec.Command(shell, "-c", t)
		cmd.Dir = dir
		cmd.Stdin = sfd[0]
		cmd.Stdout = sfd[1]
		cmd.Stderr = sfd[2]
		err := cmd.Start()
		if err == nil {
			if cpid != nil {
				cpid <- cmd.Process.Pid // TODO(rsc): send cmd.Process
			}
			return
		}
		alog.Printf("exec %s: %v\n", shell, err)
		/* threadexec hasn't happened, so send a zero */
	}

Fail:
	if cpid != nil { // TODO(rsc): is it always non-nil?
		cpid <- 0
	}
}

func runwaittask(c *Command, cpid chan int) {
	c.pid = <-cpid
	c.av = nil
	if c.pid != 0 { /* successful exec */
		ccommand <- c
	} else if c.iseditcmd {
		cedit <- 0
	}
}

func run(win *Window, s string, rdir []rune, newns bool, argaddr, xarg *string, iseditcmd bool) {
	if s == "" {
		return
	}
	c := new(Command)
	cpid := make(chan int, 0)
	go runproc(win, s, rdir, newns, argaddr, xarg, c, cpid, iseditcmd)
	/* mustn't block here because must be ready to answer mount() call in run() */
	go runwaittask(c, cpid)
}

func fsopenfd(fs *client.Fsys, name string, mode uint8) (*os.File, error) {
	fd, err := fs.Open(name, mode)
	if err != nil {
		return nil, err
	}
	r, w, err := os.Pipe()
	if err != nil {
		fd.Close()
		return nil, err
	}
	if mode == plan9.OREAD {
		go func() {
			io.Copy(w, fd)
			w.Close()
			fd.Close()
		}()
		return r, nil
	}
	go func() {
		io.Copy(fd, r)
		r.Close()
		fd.Close()
	}()
	return w, nil
}
