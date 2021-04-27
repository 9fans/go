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
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
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

func clampaddr(w *Window) {
	if w.addr.Pos < 0 {
		w.addr.Pos = 0
	}
	if w.addr.End < 0 {
		w.addr.End = 0
	}
	if w.addr.Pos > w.body.Len() {
		w.addr.Pos = w.body.Len()
	}
	if w.addr.End > w.body.Len() {
		w.addr.End = w.body.Len()
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
	row.lk.Lock()
	for j := 0; j < len(row.col); j++ {
		c := row.col[j]
		for i := 0; i < len(c.w); i++ {
			w := c.w[i]
			winlock(w, 'E')
			ch := w.eventwait
			if ch != nil && w.eventtag == x.fcall.Oldtag {
				w.eventwait = nil
				ch <- false // flushed
				winunlock(w)
				goto out
			}
			winunlock(w)
		}
	}
out:
	row.lk.Unlock()
	var fc plan9.Fcall
	respond(x, &fc, "")
}

func xfidopen(x *Xfid) {
	w := x.f.w
	q := FILE(x.f.qid)
	var fc plan9.Fcall
	if w != nil {
		t := &w.body
		winlock(w, 'E')
		switch q {
		case QWaddr:
			tmp30 := nopen[wq{w, q}]
			nopen[wq{w, q}]++
			if tmp30 == 0 {
				w.addr = runes.Rng(0, 0)
				w.limit = runes.Rng(-1, -1)
			}
		case QWdata,
			QWxdata:
			nopen[wq{w, q}]++
		case QWevent:
			tmp31 := nopen[wq{w, q}]
			nopen[wq{w, q}]++
			if tmp31 == 0 {

				w.external = true
				if !w.isdir && w.col != nil {
					w.filemenu = false
					winsettag(w)
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
			if w.rdselfd != nil {
				winunlock(w)
				respond(x, &fc, Einuse)
				return
			}
			w.rdselfd = disk.TempFile() // TODO(rsc): who deletes this?
			if w.rdselfd == nil {       // TODO(rsc): impossible
				winunlock(w)
				respond(x, &fc, "can't create temp file")
				return
			}
			nopen[wq{w, q}]++
			q0 := t.q0
			q1 := t.q1
			r := bufs.AllocRunes()
			s := bufs.AllocRunes()
			for q0 < q1 {
				n := q1 - q0
				if n > bufs.Len/utf8.UTFMax {
					n = bufs.Len / utf8.UTFMax
				}
				t.file.Read(q0, r[:n])
				s := []byte(string(r[:n])) // TODO(rsc)
				if _, err := w.rdselfd.Write(s); err != nil {
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
			t.file.Mark()
			cut(t, t, nil, false, true, nil)
			w.wrselrange = runes.Rng(t.q1, t.q1)
			w.nomark = true
		case QWeditout:
			if editing == Inactive {
				winunlock(w)
				respond(x, &fc, Eperm)
				return
			}
			if !w.editoutlk.TryLock() {
				winunlock(w)
				respond(x, &fc, Einuse)
				return
			}
			w.wrselrange = runes.Rng(t.q1, t.q1)
		}
		winunlock(w)
	} else {
		switch q {
		case Qlog:
			xfidlogopen(x)
		case Qeditout:
			if !editoutlk.TryLock() {
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
			winclose(w)
		}
		respond(x, &fc, "")
		return
	}

	q := FILE(x.f.qid)
	x.f.open = false
	if w != nil {
		winlock(w, 'E')
		var t *Text
		switch q {
		case QWctl:
			if w.ctlfid != ^0 && w.ctlfid == x.f.fid {
				w.ctlfid = ^0
				w.ctllock.Unlock()
			}
		case QWdata,
			QWxdata:
			w.nomark = false
			fallthrough
		// fall through
		case QWaddr,
			QWevent: // BUG: do we need to shut down Xfid?
			nopen[wq{w, q}]--
			if nopen[wq{w, q}] == 0 {
				if q == QWdata || q == QWxdata {
					w.nomark = false
				}
				if q == QWevent && !w.isdir && w.col != nil {
					w.filemenu = true
					winsettag(w)
				}
				if q == QWevent {

					w.external = false
					w.dumpstr = ""
					w.dumpdir = ""
				}
			}
		case QWrdsel:
			w.rdselfd.Close()
			w.rdselfd = nil
		case QWwrsel:
			w.nomark = false
			t = &w.body
			// before: only did this if !w->noscroll, but that didn't seem right in practice
			textshow(t, util.Min(w.wrselrange.Pos, t.Len()), util.Min(w.wrselrange.End, t.Len()), true)
			textscrdraw(t)
		case QWeditout:
			w.editoutlk.Unlock()
		}
		winunlock(w)
		winclose(w)
	} else {
		switch q {
		case Qeditout:
			editoutlk.Unlock()
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

	winlock(w, 'F')
	if w.col == nil {
		winunlock(w)
		respond(x, &fc, Edel)
		return
	}
	defer winunlock(w)

	off := int64(x.fcall.Offset)
	var buf []byte
	switch q {
	case QWaddr:
		textcommit(&w.body, true)
		clampaddr(w)
		buf = []byte(fmt.Sprintf("%11d %11d ", w.addr.Pos, w.addr.End))
		goto Readbuf

	case QWbody:
		xfidutfread(x, &w.body, w.body.Len(), QWbody)

	case QWctl:
		buf = []byte(winctlprint(w, true))
		goto Readbuf

	case QWevent:
		xfideventread(x, w)

	case QWdata:
		// BUG: what should happen if q1 > q0?
		if w.addr.Pos > w.body.Len() {
			respond(x, &fc, Eaddr)
			break
		}
		w.addr.Pos += xfidruneread(x, &w.body, w.addr.Pos, w.body.Len())
		w.addr.End = w.addr.Pos

	case QWxdata:
		// BUG: what should happen if q1 > q0?
		if w.addr.Pos > w.body.Len() {
			respond(x, &fc, Eaddr)
			break
		}
		w.addr.Pos += xfidruneread(x, &w.body, w.addr.Pos, w.addr.End)

	case QWtag:
		xfidutfread(x, &w.tag, w.tag.Len(), QWtag)

	case QWrdsel:
		w.rdselfd.Seek(int64(off), 0)
		n := int(x.fcall.Count)
		if x.fcall.Count > bufs.Len {
			n = bufs.Len
		}
		b := make([]byte, bufs.Len) // TODO fbufalloc()
		n, err := w.rdselfd.Read(b[:n])
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

func shouldscroll(t *Text, q0 int, qid int) bool {
	if qid == Qcons {
		return true
	}
	return t.org <= q0 && q0 <= t.org+t.fr.NumChars
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
		winlock(w, c)
		if w.col == nil {
			winunlock(w)
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
		t := &w.body
		wincommit(w, t)
		eval := true
		var nb int
		a := addrpkg.Eval(false, t, w.limit, w.addr, r, 0, len(r), rgetc, &eval, &nb)
		if nb < len(r) {
			respond(x, &fc, Ebadaddr)
			break
		}
		if !eval {
			respond(x, &fc, Eaddr)
			break
		}
		w.addr = a
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	case Qeditout,
		QWeditout:
		r := fullrunewrite(x)
		var err error
		if w != nil {
			err = edittext(w, w.wrselrange.End, r)
		} else {
			err = edittext(nil, 0, r)
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
		a := w.addr
		t := &w.body
		wincommit(w, t)
		if a.Pos > t.Len() || a.End > t.Len() {
			respond(x, &fc, Eaddr)
			break
		}
		r := make([]rune, len(x.fcall.Data))
		_, nr, _ := runes.Convert(x.fcall.Data, r, true)
		r = r[:nr]
		if !w.nomark {
			file.Seq++
			t.file.Mark()
		}
		q0 := a.Pos
		if a.End > q0 {
			textdelete(t, q0, a.End, true)
			w.addr.End = q0
		}
		tq0 := t.q0
		tq1 := t.q1
		textinsert(t, q0, r, true)
		if tq0 >= q0 {
			tq0 += nr
		}
		if tq1 >= q0 {
			tq1 += nr
		}
		textsetselect(t, tq0, tq1)
		if shouldscroll(t, q0, qid) {
			textshow(t, q0+nr, q0+nr, false)
		}
		textscrdraw(t)
		winsettag(w)
		w.addr.Pos += nr
		w.addr.End = w.addr.Pos
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	case QWevent:
		xfideventwrite(x, w)

	case Qcons, QWerrors, QWbody, QWwrsel, QWtag:
		var t *Text
		switch qid {
		case Qcons:
			w = errorwin(x.f.mntdir, 'X')
			t = &w.body

		case QWerrors:
			w = errorwinforwin(w)
			t = &w.body

		case QWbody,
			QWwrsel:
			t = &w.body

		case QWtag:
			t = &w.tag
		}

		r := fullrunewrite(x)
		if len(r) > 0 {
			wincommit(w, t)
			var q0 int
			if qid == QWwrsel {
				q0 = w.wrselrange.End
				if q0 > t.Len() {
					q0 = t.Len()
				}
			} else {
				q0 = t.Len()
			}
			nr := len(r)
			if qid == QWtag {
				textinsert(t, q0, r, true)
			} else {
				if !w.nomark {
					file.Seq++
					t.file.Mark()
				}
				q0 = textbsinsert(t, q0, r, true, &nr)
				textsetselect(t, t.q0, t.q1) // insert could leave it somewhere else
				if qid != QWwrsel && shouldscroll(t, q0, qid) {
					textshow(t, q0+nr, q0+nr, true)
				}
				textscrdraw(t)
			}
			winsettag(w)
			if qid == QWwrsel {
				w.wrselrange.End += nr
			}
		}
		fc.Count = uint32(len(x.fcall.Data))
		respond(x, &fc, "")

	default:
		respond(x, &fc, fmt.Sprintf("unknown qid %d in write", qid))
	}
	if w != nil {
		// Note: Cannot defer above - w changes in errorwinforwin call.
		winunlock(w)
	}
}

func xfidctlwrite(x *Xfid, w *Window) {
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
	textcommit(&w.tag, true)
	p := string(x.fcall.Data)
	var err string
	for p != "" {
		if strings.HasPrefix(p, "lock") { // make window exclusive use
			w.ctllock.Lock()
			w.ctlfid = x.f.fid
			p = p[4:]
		} else if strings.HasPrefix(p, "unlock") { // release exclusive use
			w.ctlfid = ^0
			w.ctllock.Unlock()
			p = p[6:]
		} else if strings.HasPrefix(p, "clean") { // mark window 'clean', seq=0
			t := &w.body
			t.eq0 = ^0
			t.file.ResetLogs()
			t.file.SetMod(false)
			w.dirty = false
			settag = true
			p = p[5:]
		} else if strings.HasPrefix(p, "dirty") { // mark window 'dirty'
			t := &w.body
			// doesn't change sequence number, so "Put" won't appear.  it shouldn't.
			t.file.SetMod(true)
			w.dirty = true
			settag = true
			p = p[5:]
		} else if strings.HasPrefix(p, "show") { // show dot
			t := &w.body
			textshow(t, t.q0, t.q1, true)
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
			w.body.file.Mark()
			winsetname(w, r[:nr])
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
			fontx(&w.body, nil, nil, false, XXX, r)
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
			w.dumpstr = string(r)
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
			w.dumpdir = string(r)
		} else if strings.HasPrefix(p, "delete") { // delete for sure
			colcloseAndMouse(w.col, w, true)
			p = p[6:]
		} else if strings.HasPrefix(p, "del") { // delete, but check dirty
			if !winclean(w, true) {
				err = "file dirty"
				break
			}
			colcloseAndMouse(w.col, w, true)
			p = p[3:]
		} else if strings.HasPrefix(p, "get") { // get file
			get(&w.body, nil, nil, false, XXX, nil)
			p = p[3:]
		} else if strings.HasPrefix(p, "put") { // put file
			put(&w.body, nil, nil, XXX, XXX, nil)
			p = p[3:]
		} else if strings.HasPrefix(p, "dot=addr") { // set dot
			textcommit(&w.body, true)
			clampaddr(w)
			w.body.q0 = w.addr.Pos
			w.body.q1 = w.addr.End
			textsetselect(&w.body, w.body.q0, w.body.q1)
			settag = true
			p = p[8:]
		} else if strings.HasPrefix(p, "addr=dot") { // set addr
			w.addr.Pos = w.body.q0
			w.addr.End = w.body.q1
			p = p[8:]
		} else if strings.HasPrefix(p, "limit=addr") { // set limit
			textcommit(&w.body, true)
			clampaddr(w)
			w.limit.Pos = w.addr.Pos
			w.limit.End = w.addr.End
			p = p[10:]
		} else if strings.HasPrefix(p, "nomark") { // turn off automatic marking
			w.nomark = true
			p = p[6:]
		} else if strings.HasPrefix(p, "mark") { // mark file
			file.Seq++
			w.body.file.Mark()
			settag = true
			p = p[4:]
		} else if strings.HasPrefix(p, "nomenu") { // turn off automatic menu
			w.filemenu = false
			settag = true
			p = p[6:]
		} else if strings.HasPrefix(p, "menu") { // enable automatic menu
			w.filemenu = true
			settag = true
			p = p[4:]
		} else if strings.HasPrefix(p, "cleartag") { // wipe tag right of bar
			wincleartag(w)
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
		winsettag(w)
	}
	if scrdraw {
		textscrdraw(&w.body)
	}
}

func xfideventwrite(x *Xfid, w *Window) {
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
		w.owner = rune(p[0])
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
		var t *Text
		if 'a' <= c && c <= 'z' {
			t = &w.tag
		} else if 'A' <= c && c <= 'Z' {
			t = &w.body
		} else {
			goto Rescue
		}
		if q0 > t.Len() || q1 > t.Len() || q0 > q1 {
			goto Rescue
		}

		row.lk.Lock() // just like mousethread
		switch c {
		case 'x',
			'X':
			execute(t, q0, q1, true, nil)
		case 'l',
			'L':
			look3(t, q0, q1, true)
		default:
			row.lk.Unlock()
			goto Rescue
		}
		row.lk.Unlock()
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

func xfidutfread(x *Xfid, t *Text, q1 int, qid int) {
	w := t.w
	wincommit(w, t)
	off := int64(x.fcall.Offset)
	r := bufs.AllocRunes()
	b1 := make([]byte, bufs.Len) // fbufalloc()
	n := 0
	var q int
	var boff int64
	if qid == w.utflastqid && off >= int64(w.utflastboff) && w.utflastq <= q1 {
		boff = w.utflastboff
		q = w.utflastq
	} else {
		// BUG: stupid code: scan from beginning
		boff = 0
		q = 0
	}
	w.utflastqid = qid
	for q < q1 && n < int(x.fcall.Count) {
		/*
		 * Updating here avoids partial rune problem: we're always on a
		 * char boundary. The cost is we will usually do one more read
		 * than we really need, but that's better than being n^2.
		 */
		w.utflastboff = boff
		w.utflastq = q
		nr := q1 - q
		if nr > bufs.Len/utf8.UTFMax {
			nr = bufs.Len / utf8.UTFMax
		}
		t.file.Read(q, r[:nr])
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

func xfidruneread(x *Xfid, t *Text, q0 int, q1 int) int {
	w := t.w
	wincommit(w, t)
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
		t.file.Read(q, r[:nr])
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

func xfideventread(x *Xfid, w *Window) {
	x.flushed = false
	var fc plan9.Fcall
	if len(w.events) == 0 {
		c := make(chan bool, 1)
		w.eventtag = x.fcall.Tag
		w.eventwait = c
		winunlock(w)
		bigUnlock()
		ok := <-w.eventwait
		bigLock()
		winlock(w, 'F')
		if !ok {
			return
		}
		if len(w.events) == 0 {
			respond(x, &fc, "window shut down")
			return
		}
	}

	n := len(w.events)
	if n > int(x.fcall.Count) {
		n = int(x.fcall.Count)
	}
	fc.Count = uint32(n)
	fc.Data = w.events[:n]
	respond(x, &fc, "")
	m := copy(w.events[n:], w.events)
	w.events = w.events[:m]
}

func xfidindexread(x *Xfid) {
	row.lk.Lock()
	nmax := 0
	var i int
	var j int
	var w *Window
	var c *Column
	for j = 0; j < len(row.col); j++ {
		c = row.col[j]
		for i = 0; i < len(c.w); i++ {
			w = c.w[i]
			nmax += Ctlsize + w.tag.Len()*utf8.UTFMax + 1
		}
	}
	nmax++
	var buf bytes.Buffer
	r := bufs.AllocRunes()
	for j = 0; j < len(row.col); j++ {
		c = row.col[j]
		for i = 0; i < len(c.w); i++ {
			w = c.w[i]
			// only show the currently active window of a set
			if w.body.file.curtext != &w.body {
				continue
			}
			buf.WriteString(winctlprint(w, false))
			m := util.Min(bufs.RuneLen, w.tag.Len())
			w.tag.file.Read(0, r[:m])
			for i := 0; i < m && r[i] != '\n'; i++ {
				buf.WriteRune(r[i])
			}
			buf.WriteRune('\n')
		}
	}
	bufs.FreeRunes(r)
	row.lk.Unlock()
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
	w *Window
	q int
}

var nopen = make(map[wq]int)
