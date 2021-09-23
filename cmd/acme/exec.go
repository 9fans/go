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
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/ui"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
)

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
	fn    func(et, t, argt *wind.Text, flag1, flag2 bool, s []rune)
	mark  bool
	flag1 bool
	flag2 bool
}

var exectab = [30]Exectab{
	Exectab{[]rune("Abort"), doabort, false, XXX, XXX},
	Exectab{[]rune("Cut"), ui.XCut, true, true, true},
	Exectab{[]rune("Del"), del, false, false, XXX},
	Exectab{[]rune("Delcol"), delcol, false, XXX, XXX},
	Exectab{[]rune("Delete"), del, false, true, XXX},
	Exectab{[]rune("Dump"), dump, false, true, XXX},
	Exectab{[]rune("Edit"), edit, false, XXX, XXX},
	Exectab{[]rune("Exit"), xexit, false, XXX, XXX},
	Exectab{[]rune("Font"), ui.Fontx, false, XXX, XXX},
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
	Exectab{[]rune("Paste"), ui.XPaste, true, true, XXX},
	Exectab{[]rune("Put"), put, false, XXX, XXX},
	Exectab{[]rune("Putall"), putall, false, XXX, XXX},
	Exectab{[]rune("Redo"), ui.XUndo, false, false, XXX},
	Exectab{[]rune("Send"), sendx, true, XXX, XXX},
	Exectab{[]rune("Snarf"), ui.XCut, false, true, false},
	Exectab{[]rune("Sort"), xsort, false, XXX, XXX},
	Exectab{[]rune("Tab"), tab, false, XXX, XXX},
	Exectab{[]rune("Undo"), ui.XUndo, false, true, XXX},
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

func execute(t *wind.Text, aq0 int, aq1 int, external bool, argt *wind.Text) {
	q0 := aq0
	q1 := aq1
	var c rune
	if q1 == q0 { // expand to find word (actually file name)
		// if in selection, choose selection
		if t.Q1 > t.Q0 && t.Q0 <= q0 && q0 <= t.Q1 {
			q0 = t.Q0
			q1 = t.Q1
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
	t.File.Read(q0, r)
	e := lookup(r)
	var a, aa *string
	var n int
	if !external && t.W != nil && t.W.External {
		f := 0
		if e != nil {
			f |= 1
		}
		if q0 != aq0 || q1 != aq1 {
			t.File.Read(aq0, r[:aq1-aq0])
			f |= 2
		}
		aa = getbytearg(argt, true, true, &a)
		if a != nil {
			if len(*a) > wind.EVENTSIZE { // too big; too bad
				alog.Printf("argument string too long\n")
				return
			}
			f |= 8
		}
		c = 'x'
		if t.What == wind.Body {
			c = 'X'
		}
		n = aq1 - aq0
		if n <= wind.EVENTSIZE {
			r := r
			if len(r) > n {
				r = r[:n]
			}
			wind.Winevent(t.W, "%c%d %d %d %d %s\n", c, aq0, aq1, f, n, string(r))
		} else {
			wind.Winevent(t.W, "%c%d %d %d 0 \n", c, aq0, aq1, f)
		}
		if q0 != aq0 || q1 != aq1 {
			n = q1 - q0
			t.File.Read(q0, r[:n])
			if n <= wind.EVENTSIZE {
				wind.Winevent(t.W, "%c%d %d 0 %d %s\n", c, q0, q1, n, string(r[:n]))
			} else {
				wind.Winevent(t.W, "%c%d %d 0 0 \n", c, q0, q1)
			}
		}
		if a != nil {
			wind.Winevent(t.W, "%c0 0 0 %d %s\n", c, utf8.RuneCountInString(*a), *a)
			if aa != nil {
				wind.Winevent(t.W, "%c0 0 0 %d %s\n", c, utf8.RuneCountInString(*aa), *aa)
			} else {
				wind.Winevent(t.W, "%c0 0 0 0 \n", c)
			}
		}
		return
	}
	if e != nil {
		if e.mark && wind.Seltext != nil {
			if wind.Seltext.What == wind.Body {
				file.Seq++
				wind.Seltext.W.Body.File.Mark()
			}
		}
		s := runes.SkipBlank(r[:q1-q0])
		s = runes.SkipNonBlank(s)
		s = runes.SkipBlank(s)
		e.fn(t, wind.Seltext, argt, e.flag1, e.flag2, s)
		return
	}

	b := string(r)
	dir := wind.Dirname(t, nil)
	if len(dir) == 1 && dir[0] == '.' { // sigh
		dir = nil
	}
	aa = getbytearg(argt, true, true, &a)
	if t.W != nil {
		util.Incref(&t.W.Ref)
	}
	run(t.W, b, dir, true, aa, a, false)
}

func getbytearg(argt *wind.Text, doaddr, dofile bool, bp **string) *string {
	*bp = nil
	var r []rune
	a := ui.Getarg(argt, doaddr, dofile, &r)
	if r == nil {
		return nil
	}
	b := string(r)
	*bp = &b
	return a
}

var doabort_n int

func doabort(_, _, _ *wind.Text, _, _ bool, _ []rune) {
	if doabort_n == 0 {
		doabort_n++
		alog.Printf("executing Abort again will call abort()\n")
		return
	}
	panic("abort")
}

func newcol(et, _, _ *wind.Text, _, _ bool, _ []rune) {

	c := wind.RowAdd(et.Row, nil, -1)
	ui.Clearmouse()
	if c != nil {
		w := ui.ColaddAndMouse(c, nil, nil, -1)
		wind.Winsettag(w)
		xfidlog(w, "new")
	}
}

func delcol(et, _, _ *wind.Text, _, _ bool, _ []rune) {

	c := et.Col
	if c == nil || !wind.Colclean(c) {
		return
	}
	for i := 0; i < len(c.W); i++ {
		w := c.W[i]
		if w.External {
			alog.Printf("can't delete column; %s is running an external command\n", string(w.Body.File.Name()))
			return
		}
	}
	wind.Rowclose(et.Col.Row, et.Col, true)
	ui.Clearmouse()
}

func del(et, _, _ *wind.Text, isDelete, _ bool, _ []rune) {
	if et.Col == nil || et.W == nil {
		return
	}
	if isDelete || len(et.W.Body.File.Text) > 1 || wind.Winclean(et.W, false) {
		ui.ColcloseAndMouse(et.Col, et.W, true)
	}
}

func xsort(et, _, _ *wind.Text, _, _ bool, _ []rune) {

	if et.Col != nil {
		ui.Clearmouse()
		wind.Colsort(et.Col)
	}
}

func getname(t *wind.Text, argt *wind.Text, arg []rune, isput bool) string {
	var r []rune
	ui.Getarg(argt, false, true, &r)
	promote := false
	if r == nil {
		promote = true
	} else if isput {
		// if are doing a Put, want to synthesize name even for non-existent file
		// best guess is that file name doesn't contain a slash
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
			return string(t.File.Name())
		}
		var dir []rune
		// prefix with directory name if necessary
		dir = nil
		if len(arg) > 0 && arg[0] != '/' {
			dir = wind.Dirname(t, nil)
			if len(dir) == 1 && dir[0] == '.' { // sigh
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

func zeroxx(et, t, _ *wind.Text, _, _ bool, _ []rune) {

	locked := false
	if t != nil && t.W != nil && t.W != et.W {
		locked = true
		c := 'M'
		if et.W != nil {
			c = et.W.Owner
		}
		wind.Winlock(t.W, c)
	}
	if t == nil {
		t = et
	}
	if t == nil || t.W == nil {
		return
	}
	t = &t.W.Body
	if t.W.IsDir {
		alog.Printf("%s is a directory; Zerox illegal\n", string(t.File.Name()))
	} else {
		nw := ui.ColaddAndMouse(t.W.Col, nil, t.W, -1)
		// ugly: fix locks so w->unlock works
		wind.Winlock1(nw, t.W.Owner)
		xfidlog(nw, "zerox")
	}
	if locked {
		wind.Winunlock(t.W)
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

func get(et, t, argt *wind.Text, flag1, _ bool, arg []rune) {
	if flag1 {
		if et == nil || et.W == nil {
			return
		}
	}
	if !et.W.IsDir && (et.W.Body.Len() > 0 && !wind.Winclean(et.W, true)) {
		return
	}
	w := et.W
	t = &w.Body
	name := getname(t, argt, arg, false)
	if name == "" {
		alog.Printf("no file name\n")
		return
	}
	if len(t.File.Text) > 1 {
		if info, err := os.Stat(name); err == nil && info.IsDir() {
			alog.Printf("%s is a directory; can't read with multiple windows on it\n", name)
			return
		}
	}
	addr := make([]TextAddr, len(t.File.Text))
	for i := 0; i < len(t.File.Text); i++ {
		a := &addr[i]
		u := t.File.Text[i]
		a.lorigin = nlcount(u, 0, u.Org, &a.rorigin)
		a.lq0 = nlcount(u, 0, u.Q0, &a.rq0)
		a.lq1 = nlcount(u, u.Q0, u.Q1, &a.rq1)
	}
	r := []rune(name)
	for i := 0; i < len(t.File.Text); i++ {
		u := t.File.Text[i]
		// second and subsequent calls with zero an already empty buffer, but OK
		wind.Textreset(u)
		wind.Windirfree(u.W)
	}
	samename := runes.Equal(r, t.File.Name())
	textload(t, 0, name, samename)
	var dirty bool
	if samename {
		t.File.SetMod(false)
		dirty = false
	} else {
		t.File.SetMod(true)
		dirty = true
	}
	for i := 0; i < len(t.File.Text); i++ {
		t.File.Text[i].W.Dirty = dirty
	}
	wind.Winsettag(w)
	t.File.Unread = false
	for i := 0; i < len(t.File.Text); i++ {
		u := t.File.Text[i]
		wind.Textsetselect(&u.W.Tag, u.W.Tag.Len(), u.W.Tag.Len())
		if samename {
			a := &addr[i]
			// Printf("%d %d %d %d %d %d\n", a->lorigin, a->rorigin, a->lq0, a->rq0, a->lq1, a->rq1);
			q0 := addrpkg.Advance(u, 0, a.lq0, a.rq0)
			q1 := addrpkg.Advance(u, q0, a.lq1, a.rq1)
			wind.Textsetselect(u, q0, q1)
			q0 = addrpkg.Advance(u, 0, a.lorigin, a.rorigin)
			wind.Textsetorigin(u, q0, false)
		}
		wind.Textscrdraw(u)
	}
	xfidlog(w, "get")
}

func checksha1(name string, f *wind.File, info os.FileInfo) {
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
	if out == f.SHA1 {
		f.Info = info
	}
}

func sameInfo(fi1, fi2 os.FileInfo) bool {
	return fi1 != nil && fi2 != nil && os.SameFile(fi1, fi2) && fi1.ModTime().Equal(fi2.ModTime()) && fi1.Size() == fi2.Size()
}

func putfile(f *wind.File, q0 int, q1 int, namer []rune) {
	w := f.Curtext.W
	name := string(namer)
	info, err := os.Stat(name)
	if err == nil && runes.Equal(namer, f.Name()) {
		if !sameInfo(info, f.Info) {
			checksha1(name, f, info)
		}
		if !sameInfo(info, f.Info) {
			if f.Unread {
				alog.Printf("%s not written; file already exists\n", name)
			} else {
				alog.Printf("%s modified since last read\n\twas %v; now %v\n", name, f.Info.ModTime().Format(timefmt), info.ModTime().Format(timefmt))
			}
			f.Info = info
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
			w.Dirty = true
			f.Unread = true
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
			f.Info = info
			h.Sum(f.SHA1[:0])
			f.SetMod(false)
			w.Dirty = false
			f.Unread = false
		}
		for i := 0; i < len(f.Text); i++ {
			f.Text[i].W.Putseq = f.Seq()
			f.Text[i].W.Dirty = w.Dirty
		}
	}
	bufs.FreeRunes(s)
	wind.Winsettag(w)
	return

Rescue2:
	bufs.FreeRunes(s)
	bufs.FreeRunes(r)
	// fall through
}

func trimspaces(et *wind.Text) {
	t := &et.W.Body
	f := t.File
	marked := 0

	if t.W != nil && et.W != t.W {
		// can this happen when t == &et->w->body?
		c := 'M'
		if et.W != nil {
			c = et.W.Owner
		}
		wind.Winlock(t.W, c)
	}

	r := bufs.AllocRunes()
	q0 := f.Len()
	delstart := q0 // end of current space run, or 0 if no active run; = q0 to delete spaces before EOF
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
					wind.Textdelete(t, q0+i, delstart, true)
				}
				if i == 0 {
					// keep run active into tail of next buffer
					if delstart > 0 {
						delstart = q0
					}
					break
				}
				delstart = 0
				if r[i-1] == '\n' {
					delstart = q0 + i - 1 // delete spaces before this newline
				}
			}
		}
	}
	bufs.FreeRunes(r)

	if t.W != nil && et.W != t.W {
		wind.Winunlock(t.W)
	}
}

func put(et, _, argt *wind.Text, _, _ bool, arg []rune) {

	if et == nil || et.W == nil || et.W.IsDir {
		return
	}
	w := et.W
	f := w.Body.File
	name := getname(&w.Body, argt, arg, true)
	if name == "" {
		alog.Printf("no file name\n")
		return
	}
	if w.Autoindent {
		trimspaces(et)
	}
	namer := []rune(name)
	putfile(f, 0, f.Len(), namer)
	xfidlog(w, "put")
}

func dump(_, _, argt *wind.Text, isdump, _ bool, arg []rune) {
	var name *string
	if len(arg) != 0 {
		s := string(arg)
		name = &s
	} else {
		getbytearg(argt, false, true, &name)
	}
	if isdump {
		rowdump(&wind.TheRow, name)
	} else {
		rowload(&wind.TheRow, name, false)
	}
}

func look(et, t, argt *wind.Text, _, _ bool, arg []rune) {
	if et != nil && et.W != nil {
		t = &et.W.Body
		if len(arg) > 0 {
			ui.Search(t, arg)
			return
		}
		var r []rune
		ui.Getarg(argt, false, false, &r)
		if r == nil {
			r = make([]rune, t.Q1-t.Q0)
			t.File.Read(t.Q0, r)
		}
		ui.Search(t, r)
	}
}

func sendx(et, t, _ *wind.Text, _, _ bool, _ []rune) {
	if et.W == nil {
		return
	}
	t = &et.W.Body
	if t.Q0 != t.Q1 {
		ui.XCut(t, t, nil, true, false, nil)
	}
	wind.Textsetselect(t, t.Len(), t.Len())
	ui.XPaste(t, t, nil, true, true, nil)
	if t.RuneAt(t.Len()-1) != '\n' {
		wind.Textinsert(t, t.Len(), []rune("\n"), true)
		wind.Textsetselect(t, t.Len(), t.Len())
	}
	t.IQ1 = t.Q1
	wind.Textshow(t, t.Q1, t.Q1, true)
}

func edit(et, _, argt *wind.Text, _, _ bool, arg []rune) {
	if et == nil {
		return
	}
	var r []rune
	ui.Getarg(argt, false, true, &r)
	file.Seq++
	if r != nil {
		editcmd(et, r)
	} else {
		editcmd(et, arg)
	}
}

func xexit(_, _, _ *wind.Text, _, _ bool, _ []rune) {
	if wind.Rowclean(&wind.TheRow) {
		cexit <- 0
		runtime.Goexit() // TODO(rsc)
	}
}

func putall(et, _, _ *wind.Text, _, _ bool, _ []rune) {
	for _, c := range wind.TheRow.Col {
		for _, w := range c.W {
			if w.IsScratch || w.IsDir || len(w.Body.File.Name()) == 0 {
				continue
			}
			if w.External {
				continue
			}
			a := string(w.Body.File.Name())
			_, e := os.Stat(a)
			if w.Body.File.Mod() || len(w.Body.Cache) != 0 {
				if e != nil {
					alog.Printf("no auto-Put of %s: %v\n", a, e)
				} else {
					wind.Wincommit(w, &w.Body)
					put(&w.Body, nil, nil, XXX, XXX, nil)
				}
			}
		}
	}
}

func id(et, _, _ *wind.Text, _, _ bool, _ []rune) {
	if et != nil && et.W != nil {
		alog.Printf("/mnt/acme/%d/\n", et.W.ID)
	}
}

func local(et, _, argt *wind.Text, _, _ bool, arg []rune) {
	var a *string
	aa := getbytearg(argt, true, true, &a)
	dir := wind.Dirname(et, nil)
	if len(dir) == 1 && dir[0] == '.' { // sigh
		dir = nil
	}
	run(nil, string(arg), dir, false, aa, a, false)
}

func xkill(_, _, argt *wind.Text, _, _ bool, arg []rune) {
	var r []rune
	ui.Getarg(argt, false, false, &r)
	if r != nil {
		xkill(nil, nil, nil, false, false, r)
	}
	// loop condition: *arg is not a blank
	for {
		a := runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			break
		}
		ckill <- runes.Clone(arg[:len(arg)-len(a)])
		arg = runes.SkipBlank(a)
	}
}

func incl(et, _, argt *wind.Text, _, _ bool, arg []rune) {
	if et == nil || et.W == nil {
		return
	}
	w := et.W
	n := 0
	var r []rune
	ui.Getarg(argt, false, true, &r)
	if r != nil {
		n++
		wind.Winaddincl(w, r)
	}
	// loop condition: len(arg) == 0 || arg[0] is not a blank
	for {
		a := runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			break
		}
		r = runes.Clone(arg[:len(arg)-len(a)])
		wind.Winaddincl(w, r)
		arg = runes.SkipBlank(a)
	}
	if n == 0 && len(w.Incl) > 0 {
		for n = len(w.Incl); ; {
			n--
			if n < 0 {
				break
			}
			alog.Printf("%s ", string(w.Incl[n]))
		}
		alog.Printf("\n")
	}
}

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
	if runes.Equal(s, []rune("ON")) {
		wind.GlobalAutoindent = true
		alog.Printf("Indent ON\n")
		return IGlobal
	}
	if runes.Equal(s, []rune("OFF")) {
		wind.GlobalAutoindent = false
		alog.Printf("Indent OFF\n")
		return IGlobal
	}
	if runes.Equal(s, []rune("on")) {
		return Ion
	}
	return Ioff
}

func fixindent(w *wind.Window, arg interface{}) {
	w.Autoindent = wind.GlobalAutoindent
}

func indent(et, _, argt *wind.Text, _, _ bool, arg []rune) {
	var w *wind.Window
	if et != nil && et.W != nil {
		w = et.W
	}
	autoindent := IError
	var r []rune
	ui.Getarg(argt, false, true, &r)
	if len(r) > 0 {
		autoindent = indentval(r)
	} else {
		a := runes.SkipNonBlank(arg)
		if len(a) != len(arg) {
			autoindent = indentval(arg[:len(arg)-len(a)])
		}
	}
	if autoindent == IGlobal {
		wind.All(fixindent, nil)
	} else if w != nil && autoindent >= 0 {
		w.Autoindent = autoindent == Ion
	}
}

func tab(et, _, argt *wind.Text, _, _ bool, arg []rune) {
	if et == nil || et.W == nil {
		return
	}
	w := et.W
	var r []rune
	ui.Getarg(argt, false, true, &r)
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
		if w.Body.Tabstop != tab {
			w.Body.Tabstop = tab
			ui.WinresizeAndMouse(w, w.R, false, true)
		}
	} else {
		alog.Printf("%s: Tab %d\n", string(w.Body.File.Name()), w.Body.Tabstop)
	}
}

func runproc(win *wind.Window, s string, rdir []rune, newns bool, argaddr, xarg *string, c *Command, cpid chan int, iseditcmd bool) {
	t := strings.TrimLeft(s, " \n\t")
	name := t
	if i := strings.IndexAny(name, " \n\t"); i >= 0 {
		name = name[:i]
	}
	if i := strings.LastIndex(name, "/"); i >= 0 {
		name = name[i+1:]
	}
	name += " " // add blank here for ease in waittask
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
		// end of args
		var filename string
		if win != nil {
			filename = string(win.Body.File.Name())
			if len(win.Incl) > 0 {
				incl = make([][]rune, len(win.Incl))
				for i := range win.Incl {
					incl[i] = runes.Clone(win.Incl[i])
				}
			}
			winid = win.ID
		} else if wind.Activewin != nil {
			winid = wind.Activewin.ID
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
		wind.Winclose(win)
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
		//static void *parg[2];
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
		// threadexec hasn't happened, so send a zero
	}

Fail:
	if cpid != nil { // TODO(rsc): is it always non-nil?
		cpid <- 0
	}
}

func runwaittask(c *Command, cpid chan int) {
	c.pid = <-cpid
	c.av = nil
	if c.pid != 0 { // successful exec
		ccommand <- c
	} else if c.iseditcmd {
		cedit <- 0
	}
}

func run(win *wind.Window, s string, rdir []rune, newns bool, argaddr, xarg *string, iseditcmd bool) {
	if s == "" {
		return
	}
	c := new(Command)
	cpid := make(chan int, 0)
	go runproc(win, s, rdir, newns, argaddr, xarg, c, cpid, iseditcmd)
	// mustn't block here because must be ready to answer mount() call in run()
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
