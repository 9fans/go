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

package main

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"

	addrpkg "9fans.net/go/cmd/acme/internal/addr"
	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/disk"
	editpkg "9fans.net/go/cmd/acme/internal/edit"
	"9fans.net/go/cmd/acme/internal/exec"
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/ui"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/plan9"
)

const (
	Ctlsize = 5 * 12
)

var Edel string = "deleted window"
var Ebadctl string = "ill-formed control message"
var Ebadaddr string = "bad address syntax"
var Eaddr string = "address out of range"
var Einuse string = "already in use"
var Ebadevent string = "bad event syntax"

// extern var Eperm [unknown]C.char

func clampaddr(w *wind.Window) {
	if w.Addr.Pos < 0 {
		w.Addr.Pos = 0
	}
	if w.Addr.End < 0 {
		w.Addr.End = 0
	}
	if w.Addr.Pos > w.Body.Len() {
		w.Addr.Pos = w.Body.Len()
	}
	if w.Addr.End > w.Body.Len() {
		w.Addr.End = w.Body.Len()
	}
}

func xfidctl(x *Xfid) {
	for {
		f := <-x.c
		bigLock()
		f(x)
		adraw.Display.Flush()
		bigUnlock()
		cxfidfree <- x
	}
}

func xfidflush(x *Xfid) {
	xfidlogflush(x)

	// search windows for matching tag
	bigUnlock()
	wind.TheRow.Lk.Lock()
	bigLock()
	for j := 0; j < len(wind.TheRow.Col); j++ {
		c := wind.TheRow.Col[j]
		for i := 0; i < len(c.W); i++ {
			w := c.W[i]
			wind.Winlock(w, 'E')
			ch := w.Eventwait
			if ch != nil && w.Eventtag == x.fcall.Oldtag {
				w.Eventwait = nil
				ch <- false // flushed
				wind.Winunlock(w)
				goto out
			}
			wind.Winunlock(w)
		}
	}
out:
	wind.TheRow.Lk.Unlock()
	var fc plan9.Fcall
	respond(x, &fc, "")
}

func xfidopen(x *Xfid) {
	w := x.f.w
	q := FILE(x.f.qid)
	var fc plan9.Fcall
	if w != nil {
		t := &w.Body
		wind.Winlock(w, 'E')
		switch q {
		case QWaddr:
			tmp30 := nopen[wq{w, q}]
			nopen[wq{w, q}]++
			if tmp30 == 0 {
				w.Addr = runes.Rng(0, 0)
				w.Limit = runes.Rng(-1, -1)
			}
		case QWdata,
			QWxdata:
			nopen[wq{w, q}]++
		case QWevent:
			tmp31 := nopen[wq{w, q}]
			nopen[wq{w, q}]++
			if tmp31 == 0 {

				w.External = true
				if !w.IsDir && w.Col != nil {
					w.Filemenu = false
					wind.Winsettag(w)
				}
			}
		/*
		 * Use a temporary file.
		 * A pipe would be the obvious, but we can't afford the
		 * broken pipe notification.  Using the code to read QWbody
		 * is n², which should probably also be fixed.  Even then,
		 * though, we'd need to squirrel away the data in case it's
		 * modified during the operation, e.g. by |sort
		 */
		case QWrdsel:
			if w.Rdselfd != nil {
				wind.Winunlock(w)
				respond(x, &fc, Einuse)
				return
			}
			w.Rdselfd = disk.TempFile() // TODO(rsc): who deletes this?
			if w.Rdselfd == nil {       // TODO(rsc): impossible
				wind.Winunlock(w)
				respond(x, &fc, "can't create temp file")
				return
			}
			nopen[wq{w, q}]++
			q0 := t.Q0
			q1 := t.Q1
			r := bufs.AllocRunes()
			s := bufs.AllocRunes()
			for q0 < q1 {
				n := q1 - q0
				if n > bufs.Len/utf8.UTFMax {
					n = bufs.Len / utf8.UTFMax
				}
				t.File.Read(q0, r[:n])
				s := []byte(string(r[:n])) // TODO(rsc)
				if _, err := w.Rdselfd.Write(s); err != nil {
					alog.Printf("can't write temp file for pipe command %v\n", err)
					break
				}
				q0 += n
			}
			bufs.FreeRunes(s)
			bufs.FreeRunes(r)
		case QWwrsel:
			nopen[wq{w, q}]++
			file.Seq++
			t.File.Mark()
			ui.XCut(t, t, nil, false, true, nil)
			w.Wrselrange = runes.Rng(t.Q1, t.Q1)
			w.Nomark = true
		case QWeditout:
			if editpkg.Editing == editpkg.Inactive {
				wind.Winunlock(w)
				respond(x, &fc, Eperm)
				return
			}
			if !w.Editoutlk.TryLock() {
				wind.Winunlock(w)
				respond(x, &fc, Einuse)
				return
			}
			w.Wrselrange = runes.Rng(t.Q1, t.Q1)
		}
		wind.Winunlock(w)
	} else {
		switch q {
		case Qlog:
			xfidlogopen(x)
		case Qeditout:
			if !editpkg.Editoutlk.TryLock() {
				respond(x, &fc, Einuse)
				return
			}
		}
	}
	fc.Qid = x.f.qid
	fc.Iounit = uint32(messagesize - plan9.IOHDRSZ)
	x.f.open = true
	respond(x, &fc, "")
}

func xfidclose(x *Xfid) {
	w := x.f.w
	x.f.busy = false
	x.f.w = nil
	var fc plan9.Fcall
	if !x.f.open {
		if w != nil {
			wind.Winclose(w)
		}
		respond(x, &fc, "")
		return
	}

	q := FILE(x.f.qid)
	x.f.open = false
	if w != nil {
		wind.Winlock(w, 'E')
		var t *wind.Text
		switch q {
		case QWctl:
			if w.Ctlfid != ^0 && w.Ctlfid == x.f.fid {
				w.Ctlfid = ^0
				w.Ctllock.Unlock()
			}
		case QWdata,
			QWxdata:
			w.Nomark = false
			fallthrough
		// fall through
		case QWaddr,
			QWevent: // BUG: do we need to shut down Xfid?
			nopen[wq{w, q}]--
			if nopen[wq{w, q}] == 0 {
				if q == QWdata || q == QWxdata {
					w.Nomark = false
				}
				if q == QWevent && !w.IsDir && w.Col != nil {
					w.Filemenu = true
					wind.Winsettag(w)
				}
				if q == QWevent {

					w.External = false
					w.Dumpstr = ""
					w.Dumpdir = ""
				}
			}
		case QWrdsel:
			w.Rdselfd.Close()
			w.Rdselfd = nil
		case QWwrsel:
			w.Nomark = false
			t = &w.Body
			// before: only did this if !w->noscroll, but that didn't seem right in practice
			wind.Textshow(t, util.Min(w.Wrselrange.Pos, t.Len()), util.Min(w.Wrselrange.End, t.Len()), true)
			wind.Textscrdraw(t)
		case QWeditout:
			w.Editoutlk.Unlock()
		}
		wind.Winunlock(w)
		wind.Winclose(w)
	} else {
		switch q {
		case Qeditout:
			editpkg.Editoutlk.Unlock()
		}
	}
	respond(x, &fc, "")
}

func xfidread(x *Xfid) {
	q := FILE(x.f.qid)
	w := x.f.w
	var fc plan9.Fcall
	if w == nil {
		fc.Count = 0
		switch q {
		case Qcons,
			Qlabel:
			break
		case Qindex:
			xfidindexread(x)
			return
		case Qlog:
			xfidlogread(x)
			return
		default:
			alog.Printf("unknown qid %d\n", q)
		}
		respond(x, &fc, "")
		return
	}

	wind.Winlock(w, 'F')
	if w.Col == nil {
		wind.Winunlock(w)
		respond(x, &fc, Edel)
		return
	}
	defer wind.Winunlock(w)

	off := int64(x.fcall.Offset)
	var buf []byte
	switch q {
	case QWaddr:
		wind.Textcommit(&w.Body, true)
		clampaddr(w)
		buf = []byte(fmt.Sprintf("%11d %11d ", w.Addr.Pos, w.Addr.End))
		goto Readbuf

	case QWbody:
		xfidutfread(x, &w.Body, w.Body.Len(), QWbody)

	case QWctl:
		buf = []byte(wind.Winctlprint(w, true))
		goto Readbuf

	case QWevent:
		xfideventread(x, w)

	case QWdata:
		// BUG: what should happen if q1 > q0?
		if w.Addr.Pos > w.Body.Len() {
			respond(x, &fc, Eaddr)
			break
		}
		w.Addr.Pos += xfidruneread(x, &w.Body, w.Addr.Pos, w.Body.Len())
		w.Addr.End = w.Addr.Pos

	case QWxdata:
		// BUG: what should happen if q1 > q0?
		if w.Addr.Pos > w.Body.Len() {
			respond(x, &fc, Eaddr)
			break
		}
		w.Addr.Pos += xfidruneread(x, &w.Body, w.Addr.Pos, w.Addr.End)

	case QWtag:
		xfidutfread(x, &w.Tag, w.Tag.Len(), QWtag)

	case QWrdsel:
		w.Rdselfd.Seek(int64(off), 0)
		n := int(x.fcall.Count)
		if x.fcall.Count > bufs.Len {
			n = bufs.Len
		}
		b := make([]byte, bufs.Len) // TODO fbufalloc()
		n, err := w.Rdselfd.Read(b[:n])
		if err != nil && err != io.EOF {
			respond(x, &fc, "I/O error in temp file")
			break
		}
		fc.Count = uint32(n)
		fc.Data = b[:n]
		respond(x, &fc, "")
		// fbuffree(b)

	default:
		respond(x, &fc, fmt.Sprintf("unknown qid %d in read", q))
	}
	return

Readbuf:
	if off > int64(len(buf)) {
		off = int64(len(buf))
	}
	fc.Data = buf[off:]
	if int64(len(fc.Data)) > int64(x.fcall.Count) {
		fc.Data = fc.Data[:x.fcall.Count]
	}
	fc.Count = uint32(len(fc.Data))
	respond(x, &fc, "")
}

func shouldscroll(t *wind.Text, q0 int, qid int) bool {
	if qid == Qcons {
		return true
	}
	return t.Org <= q0 && q0 <= t.Org+t.Fr.NumChars
}

func fullrunewrite(x *Xfid) []rune {
	q := len(x.f.rpart)
	cnt := len(x.fcall.Data)
	if q > 0 {
		x.fcall.Data = x.fcall.Data[:cnt+q]
		copy(x.fcall.Data[q:], x.fcall.Data)
		copy(x.fcall.Data, x.f.rpart[:q])
		x.f.rpart = x.f.rpart[:0]
	}
	r := make([]rune, cnt)
	nb, nr, _ := runes.Convert(x.fcall.Data, r, false)
	r = r[:nr]
	// approach end of buffer
	for utf8.FullRune(x.fcall.Data[nb:cnt]) {
		ch, w := utf8.DecodeRune(x.fcall.Data[nb:])
		nb += w
		if ch != 0 {
			r = append(r, ch)
		}
	}
	if nb < cnt {
		if cap(x.f.rpart) < utf8.UTFMax {
			x.f.rpart = make([]byte, 0, utf8.UTFMax)
		}
		x.f.rpart = append(x.f.rpart, x.fcall.Data[nb:]...)
	}
	return r
}

func xfidwrite(x *Xfid) {
	qid := FILE(x.f.qid)
	w := x.f.w
	var fc plan9.Fcall
	if w != nil {
		c := 'F'
		if qid == QWtag || qid == QWbody {
			c = 'E'
		}
		wind.Winlock(w, c)
		if w.Col == nil {
			wind.Winunlock(w)
			respond(x, &fc, Edel)
			return
		}
	}
	switch qid {
	case Qlabel:
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	case QWaddr:
		r := []rune(string(x.fcall.Data))
		t := &w.Body
		wind.Wincommit(w, t)
		eval := true
		var nb int
		a := addrpkg.Eval(false, t, w.Limit, w.Addr, r, 0, len(r), rgetc, &eval, &nb)
		if nb < len(r) {
			respond(x, &fc, Ebadaddr)
			break
		}
		if !eval {
			respond(x, &fc, Eaddr)
			break
		}
		w.Addr = a
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	case Qeditout,
		QWeditout:
		r := fullrunewrite(x)
		var err error
		if w != nil {
			err = editpkg.Edittext(w, w.Wrselrange.End, r)
		} else {
			err = editpkg.Edittext(nil, 0, r)
		}
		if err != nil {
			respond(x, &fc, err.Error())
			break
		}
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	case QWctl:
		xfidctlwrite(x, w)

	case QWdata:
		a := w.Addr
		t := &w.Body
		wind.Wincommit(w, t)
		if a.Pos > t.Len() || a.End > t.Len() {
			respond(x, &fc, Eaddr)
			break
		}
		r := make([]rune, len(x.fcall.Data))
		_, nr, _ := runes.Convert(x.fcall.Data, r, true)
		r = r[:nr]
		if !w.Nomark {
			file.Seq++
			t.File.Mark()
		}
		q0 := a.Pos
		if a.End > q0 {
			wind.Textdelete(t, q0, a.End, true)
			w.Addr.End = q0
		}
		tq0 := t.Q0
		tq1 := t.Q1
		wind.Textinsert(t, q0, r, true)
		if tq0 >= q0 {
			tq0 += nr
		}
		if tq1 >= q0 {
			tq1 += nr
		}
		wind.Textsetselect(t, tq0, tq1)
		if shouldscroll(t, q0, qid) {
			wind.Textshow(t, q0+nr, q0+nr, false)
		}
		wind.Textscrdraw(t)
		wind.Winsettag(w)
		w.Addr.Pos += nr
		w.Addr.End = w.Addr.Pos
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	case QWevent:
		xfideventwrite(x, w)

	case Qcons, QWerrors, QWbody, QWwrsel, QWtag:
		var t *wind.Text
		switch qid {
		case Qcons:
			w = errorwin(x.f.mntdir, 'X')
			t = &w.Body

		case QWerrors:
			w = errorwinforwin(w)
			t = &w.Body

		case QWbody,
			QWwrsel:
			t = &w.Body

		case QWtag:
			t = &w.Tag
		}

		r := fullrunewrite(x)
		if len(r) > 0 {
			wind.Wincommit(w, t)
			var q0 int
			if qid == QWwrsel {
				q0 = w.Wrselrange.End
				if q0 > t.Len() {
					q0 = t.Len()
				}
			} else {
				q0 = t.Len()
			}
			nr := len(r)
			if qid == QWtag {
				wind.Textinsert(t, q0, r, true)
			} else {
				if !w.Nomark {
					file.Seq++
					t.File.Mark()
				}
				q0 = wind.Textbsinsert(t, q0, r, true, &nr)
				wind.Textsetselect(t, t.Q0, t.Q1) // insert could leave it somewhere else
				if qid != QWwrsel && shouldscroll(t, q0, qid) {
					wind.Textshow(t, q0+nr, q0+nr, true)
				}
				wind.Textscrdraw(t)
			}
			wind.Winsettag(w)
			if qid == QWwrsel {
				w.Wrselrange.End += nr
			}
		}
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	default:
		respond(x, &fc, fmt.Sprintf("unknown qid %d in write", qid))
	}
	if w != nil {
		// Note: Cannot defer above - w changes in errorwinforwin call.
		wind.Winunlock(w)
	}
}

func xfidctlwrite(x *Xfid, w *wind.Window) {
	scrdraw := false
	settag := false
	isfbuf := true
	var r []rune
	if int(x.fcall.Count) < bufs.RuneLen {
		r = bufs.AllocRunes()
	} else {
		isfbuf = false
		r = make([]rune, x.fcall.Count*utf8.UTFMax)
	}
	wind.Textcommit(&w.Tag, true)
	p := string(x.fcall.Data)
	var err string
	for p != "" {
		if strings.HasPrefix(p, "lock") { // make window exclusive use
			w.Ctllock.Lock()
			w.Ctlfid = x.f.fid
			p = p[4:]
		} else if strings.HasPrefix(p, "unlock") { // release exclusive use
			w.Ctlfid = ^0
			w.Ctllock.Unlock()
			p = p[6:]
		} else if strings.HasPrefix(p, "clean") { // mark window 'clean', seq=0
			t := &w.Body
			t.Eq0 = ^0
			t.File.ResetLogs()
			t.File.SetMod(false)
			w.Dirty = false
			settag = true
			p = p[5:]
		} else if strings.HasPrefix(p, "dirty") { // mark window 'dirty'
			t := &w.Body
			// doesn't change sequence number, so "Put" won't appear.  it shouldn't.
			t.File.SetMod(true)
			w.Dirty = true
			settag = true
			p = p[5:]
		} else if strings.HasPrefix(p, "show") { // show dot
			t := &w.Body
			wind.Textshow(t, t.Q0, t.Q1, true)
			p = p[4:]
		} else if strings.HasPrefix(p, "name ") { // set file name
			pp := p[5:]
			p = p[5:]
			i := strings.Index(pp, "\n")
			if i <= 0 {
				err = Ebadctl
				break
			}
			pp = pp[:i]
			p = p[i+1:]
			r := make([]rune, len(pp))
			_, nr, nulls := runes.Convert([]byte(pp), r, true)
			if nulls {
				err = "nulls in file name"
				break
			}
			r = r[:nr]
			for i := 0; i < nr; i++ {
				if r[i] <= ' ' {
					err = "bad character in file name"
					goto out // TODO(rsc): still set name?
				}
			}
		out:
			file.Seq++
			w.Body.File.Mark()
			wind.Winsetname(w, r[:nr])
		} else if strings.HasPrefix(p, "font ") { // execute font command
			pp := p[5:]
			p = p[5:]
			i := strings.Index(pp, "\n")
			if i <= 0 {
				err = Ebadctl
				break
			}
			pp = pp[:i]
			p = p[i+1:]
			r := make([]rune, len(pp))
			_, nr, nulls := runes.Convert([]byte(pp), r, true)
			if nulls {
				err = "nulls in font string"
				break
			}
			r = r[:nr]
			ui.Fontx(&w.Body, nil, nil, false, exec.XXX, r)
		} else if strings.HasPrefix(p, "dump ") { // set dump string
			pp := p[5:]
			p = p[5:]
			i := strings.Index(pp, "\n")
			if i <= 0 {
				err = Ebadctl
				break
			}
			pp = pp[:i]
			p = p[i+1:]
			r := make([]rune, len(pp))
			_, nr, nulls := runes.Convert([]byte(pp), r, true)
			if nulls {
				err = "nulls in dump string"
				break
			}
			r = r[:nr]
			w.Dumpstr = string(r)
		} else if strings.HasPrefix(p, "dumpdir ") { // set dump directory
			pp := p[8:]
			p = p[8:]
			i := strings.Index(pp, "\n")
			if i <= 0 {
				err = Ebadctl
				break
			}
			pp = pp[:i]
			p = p[i+1:]
			r := make([]rune, len(pp))
			_, nr, nulls := runes.Convert([]byte(pp), r, true)
			if nulls {
				err = "nulls in dump string"
				break
			}
			r = r[:nr]
			w.Dumpdir = string(r)
		} else if strings.HasPrefix(p, "delete") { // delete for sure
			ui.ColcloseAndMouse(w.Col, w, true)
			p = p[6:]
		} else if strings.HasPrefix(p, "del") { // delete, but check dirty
			if !wind.Winclean(w, true) {
				err = "file dirty"
				break
			}
			ui.ColcloseAndMouse(w.Col, w, true)
			p = p[3:]
		} else if strings.HasPrefix(p, "get") { // get file
			exec.Get(&w.Body, nil, nil, false, exec.XXX, nil)
			p = p[3:]
		} else if strings.HasPrefix(p, "put") { // put file
			exec.Put(&w.Body, nil, nil, exec.XXX, exec.XXX, nil)
			p = p[3:]
		} else if strings.HasPrefix(p, "dot=addr") { // set dot
			wind.Textcommit(&w.Body, true)
			clampaddr(w)
			w.Body.Q0 = w.Addr.Pos
			w.Body.Q1 = w.Addr.End
			wind.Textsetselect(&w.Body, w.Body.Q0, w.Body.Q1)
			settag = true
			p = p[8:]
		} else if strings.HasPrefix(p, "addr=dot") { // set addr
			w.Addr.Pos = w.Body.Q0
			w.Addr.End = w.Body.Q1
			p = p[8:]
		} else if strings.HasPrefix(p, "limit=addr") { // set limit
			wind.Textcommit(&w.Body, true)
			clampaddr(w)
			w.Limit.Pos = w.Addr.Pos
			w.Limit.End = w.Addr.End
			p = p[10:]
		} else if strings.HasPrefix(p, "nomark") { // turn off automatic marking
			w.Nomark = true
			p = p[6:]
		} else if strings.HasPrefix(p, "mark") { // mark file
			file.Seq++
			w.Body.File.Mark()
			settag = true
			p = p[4:]
		} else if strings.HasPrefix(p, "nomenu") { // turn off automatic menu
			w.Filemenu = false
			settag = true
			p = p[6:]
		} else if strings.HasPrefix(p, "menu") { // enable automatic menu
			w.Filemenu = true
			settag = true
			p = p[4:]
		} else if strings.HasPrefix(p, "cleartag") { // wipe tag right of bar
			wind.Wincleartatg(w)
			settag = true
			p = p[8:]
		} else {
			err = Ebadctl
			break
		}
		for p != "" && p[0] == '\n' {
			p = p[1:]
		}
	}

	if isfbuf {
		bufs.FreeRunes(r)
	}
	n := len(x.fcall.Data)
	if err != "" {
		n = 0
	}
	var fc plan9.Fcall
	fc.Count = uint32(n)
	respond(x, &fc, err)
	if settag {
		wind.Winsettag(w)
	}
	if scrdraw {
		wind.Textscrdraw(&w.Body)
	}
}

func xfideventwrite(x *Xfid, w *wind.Window) {
	isfbuf := true
	var r []rune
	if len(x.fcall.Data) < bufs.RuneLen {
		r = bufs.AllocRunes()
	} else {
		isfbuf = false
		r = make([]rune, len(x.fcall.Data)*utf8.UTFMax)
	}
	var err string
	p := x.fcall.Data
	for len(p) > 0 {
		// Parse event.
		w.Owner = rune(p[0])
		p = p[1:]
		if len(p) == 0 {
			goto Rescue
		}
		c := p[0]
		p = p[1:]
		for len(p) > 0 && p[0] == ' ' {
			p = p[1:]
		}
		q0, i := strtoul(p)
		if i == 0 {
			goto Rescue
		}
		p = p[i:]
		for len(p) > 0 && p[0] == ' ' {
			p = p[1:]
		}
		q1, i := strtoul(p)
		if i == 0 {
			goto Rescue
		}
		p = p[i:]
		for len(p) > 0 && p[0] == ' ' {
			p = p[1:]
		}
		if len(p) == 0 || p[0] != '\n' {
			goto Rescue
		}
		p = p[1:]

		// Apply event.
		var t *wind.Text
		if 'a' <= c && c <= 'z' {
			t = &w.Tag
		} else if 'A' <= c && c <= 'Z' {
			t = &w.Body
		} else {
			goto Rescue
		}
		if q0 > t.Len() || q1 > t.Len() || q0 > q1 {
			goto Rescue
		}

		o := w.Owner
		wind.Winunlock(w)
		bigUnlock()
		wind.TheRow.Lk.Lock() // just like mousethread
		bigLock()
		wind.Winlock(w, o)
		switch c {
		case 'x',
			'X':
			exec.Execute(t, q0, q1, true, nil)
		case 'l',
			'L':
			ui.Look3(t, q0, q1, true)
		default:
			wind.TheRow.Lk.Unlock()
			goto Rescue
		}
		wind.TheRow.Lk.Unlock()
	}
	goto Out

Rescue:
	err = Ebadevent
	goto Out

Out:
	if isfbuf {
		bufs.FreeRunes(r)
	}
	n := len(x.fcall.Data)
	if err != "" {
		n = 0
	}
	var fc plan9.Fcall
	fc.Count = uint32(n)
	respond(x, &fc, err)
}

func strtoul(p []byte) (value, width int) {
	i := 0
	for i < len(p) && '0' <= p[i] && p[i] <= '9' {
		value = value*10 + int(p[i]) - '0'
		i++
	}
	return value, i
}

func xfidutfread(x *Xfid, t *wind.Text, q1 int, qid int) {
	w := t.W
	wind.Wincommit(w, t)
	off := int64(x.fcall.Offset)
	r := bufs.AllocRunes()
	b1 := make([]byte, bufs.Len) // fbufalloc()
	n := 0
	var q int
	var boff int64
	if qid == w.Utflastqid && off >= int64(w.Utflastboff) && w.Utflastq <= q1 {
		boff = w.Utflastboff
		q = w.Utflastq
	} else {
		// BUG: stupid code: scan from beginning
		boff = 0
		q = 0
	}
	w.Utflastqid = qid
	for q < q1 && n < int(x.fcall.Count) {
		/*
		 * Updating here avoids partial rune problem: we're always on a
		 * char boundary. The cost is we will usually do one more read
		 * than we really need, but that's better than being n^2.
		 */
		w.Utflastboff = boff
		w.Utflastq = q
		nr := q1 - q
		if nr > bufs.Len/utf8.UTFMax {
			nr = bufs.Len / utf8.UTFMax
		}
		t.File.Read(q, r[:nr])
		b := []byte(string(r[:nr]))
		if boff >= off {
			m := len(b)
			if boff+int64(m) > off+int64(x.fcall.Count) {
				m = int(off + int64(x.fcall.Count) - boff)
			}
			copy(b1[n:], b[:m])
			n += m
		} else if boff+int64(len(b)) > off {
			if n != 0 {
				util.Fatal("bad count in utfrune")
			}
			m := int(int64(len(b)) - (off - boff))
			if m > int(x.fcall.Count) {
				m = int(x.fcall.Count)
			}
			copy(b1[:m], b[off-boff:])
			n += m
		}
		boff += int64(len(b))
		q += nr
	}
	bufs.FreeRunes(r)
	var fc plan9.Fcall
	fc.Count = uint32(n)
	fc.Data = b1[:n]
	respond(x, &fc, "")
	// TODO fbuffree(b1)
}

func xfidruneread(x *Xfid, t *wind.Text, q0 int, q1 int) int {
	w := t.W
	wind.Wincommit(w, t)
	r := bufs.AllocRunes()
	// b := fbufalloc()
	b1 := make([]byte, bufs.Len) // fbufalloc()
	n := 0
	q := q0
	boff := 0
	for q < q1 && n < int(x.fcall.Count) {
		nr := q1 - q
		if nr > bufs.Len/utf8.UTFMax {
			nr = bufs.Len / utf8.UTFMax
		}
		t.File.Read(q, r[:nr])
		b := []byte(string(r[:nr]))
		nb := len(b)
		m := nb
		if boff+m > int(x.fcall.Count) {
			i := int(x.fcall.Count) - boff
			// copy whole runes only
			m = 0
			nr = 0
			for m < i {
				_, rw := utf8.DecodeRune(b[m:])
				if m+rw > i {
					break
				}
				m += rw
				nr++
			}
			if m == 0 {
				break
			}
		}
		copy(b1[n:], b[:m])
		n += m
		boff += nb
		q += nr
	}
	bufs.FreeRunes(r)
	var fc plan9.Fcall
	fc.Count = uint32(n)
	fc.Data = b1[:n]
	respond(x, &fc, "")
	return q - q0
}

func xfideventread(x *Xfid, w *wind.Window) {
	x.flushed = false
	var fc plan9.Fcall
	if len(w.Events) == 0 {
		c := make(chan bool, 1)
		w.Eventtag = x.fcall.Tag
		w.Eventwait = c
		wind.Winunlock(w)
		bigUnlock()
		ok := <-w.Eventwait
		bigLock()
		wind.Winlock(w, 'F')
		if !ok {
			return
		}
		if len(w.Events) == 0 {
			respond(x, &fc, "window shut down")
			return
		}
	}

	n := len(w.Events)
	if n > int(x.fcall.Count) {
		n = int(x.fcall.Count)
	}
	fc.Count = uint32(n)
	fc.Data = w.Events[:n]
	respond(x, &fc, "")
	m := copy(w.Events[n:], w.Events)
	w.Events = w.Events[:m]
}

func xfidindexread(x *Xfid) {
	wind.TheRow.Lk.Lock()
	nmax := 0
	var i int
	var j int
	var w *wind.Window
	var c *wind.Column
	for j = 0; j < len(wind.TheRow.Col); j++ {
		c = wind.TheRow.Col[j]
		for i = 0; i < len(c.W); i++ {
			w = c.W[i]
			nmax += Ctlsize + w.Tag.Len()*utf8.UTFMax + 1
		}
	}
	nmax++
	var buf bytes.Buffer
	r := bufs.AllocRunes()
	for j = 0; j < len(wind.TheRow.Col); j++ {
		c = wind.TheRow.Col[j]
		for i = 0; i < len(c.W); i++ {
			w = c.W[i]
			// only show the currently active window of a set
			if w.Body.File.Curtext != &w.Body {
				continue
			}
			buf.WriteString(wind.Winctlprint(w, false))
			m := util.Min(bufs.RuneLen, w.Tag.Len())
			w.Tag.File.Read(0, r[:m])
			for i := 0; i < m && r[i] != '\n'; i++ {
				buf.WriteRune(r[i])
			}
			buf.WriteRune('\n')
		}
	}
	bufs.FreeRunes(r)
	wind.TheRow.Lk.Unlock()
	off := int(x.fcall.Offset)
	cnt := int(x.fcall.Count)
	n := buf.Len()
	if off > n {
		off = n
	}
	if off+cnt > n {
		cnt = n - off
	}
	var fc plan9.Fcall
	fc.Count = uint32(cnt)
	fc.Data = buf.Bytes()[off : off+cnt]
	respond(x, &fc, "")
}

type wq struct {
	w *wind.Window
	q int
}

var nopen = make(map[wq]int)
