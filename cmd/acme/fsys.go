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
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/plan9"
)

func QID(w, q int) uint64  { return uint64(w<<8 | q) }
func WIN(q plan9.Qid) int  { return int(q.Path>>8) & 0xFFFFFF }
func FILE(q plan9.Qid) int { return int(q.Path & 0xFF) }

var sfdR, sfdW *os.File

const (
	Nhash = 16
	DEBUG = 0
)

var fids [Nhash]*Fid

var fcall [plan9.Tmax]func(*Xfid, *Fid) *Xfid

func initfcall() {
	fcall[plan9.Tflush] = fsysflush
	fcall[plan9.Tversion] = fsysversion
	fcall[plan9.Tauth] = fsysauth
	fcall[plan9.Tattach] = fsysattach
	fcall[plan9.Twalk] = fsyswalk
	fcall[plan9.Topen] = fsysopen
	fcall[plan9.Tcreate] = fsyscreate
	fcall[plan9.Tread] = fsysread
	fcall[plan9.Twrite] = fsyswrite
	fcall[plan9.Tclunk] = fsysclunk
	fcall[plan9.Tremove] = fsysremove
	fcall[plan9.Tstat] = fsysstat
	fcall[plan9.Twstat] = fsyswstat
}

var Eperm string = "permission denied"
var Eexist string = "file does not exist"
var Enotdir string = "not a directory"

var dirtab = [11]Dirtab{
	Dirtab{".", plan9.QTDIR, Qdir, 0500 | plan9.DMDIR},
	Dirtab{"acme", plan9.QTDIR, Qacme, 0500 | plan9.DMDIR},
	Dirtab{"cons", plan9.QTFILE, Qcons, 0600},
	Dirtab{"consctl", plan9.QTFILE, Qconsctl, 0000},
	Dirtab{"draw", plan9.QTDIR, Qdraw, 0000 | plan9.DMDIR}, // to suppress graphics progs started in acme
	Dirtab{"editout", plan9.QTFILE, Qeditout, 0200},
	Dirtab{"index", plan9.QTFILE, Qindex, 0400},
	Dirtab{"label", plan9.QTFILE, Qlabel, 0600},
	Dirtab{"log", plan9.QTFILE, Qlog, 0400},
	Dirtab{"new", plan9.QTDIR, Qnew, 0500 | plan9.DMDIR},
}

var dirtabw = [13]Dirtab{
	Dirtab{".", plan9.QTDIR, Qdir, 0500 | plan9.DMDIR},
	Dirtab{"addr", plan9.QTFILE, QWaddr, 0600},
	Dirtab{"body", plan9.QTAPPEND, QWbody, 0600 | plan9.DMAPPEND},
	Dirtab{"ctl", plan9.QTFILE, QWctl, 0600},
	Dirtab{"data", plan9.QTFILE, QWdata, 0600},
	Dirtab{"editout", plan9.QTFILE, QWeditout, 0200},
	Dirtab{"errors", plan9.QTFILE, QWerrors, 0200},
	Dirtab{"event", plan9.QTFILE, QWevent, 0600},
	Dirtab{"rdsel", plan9.QTFILE, QWrdsel, 0400},
	Dirtab{"wrsel", plan9.QTFILE, QWwrsel, 0200},
	Dirtab{"tag", plan9.QTAPPEND, QWtag, 0600 | plan9.DMAPPEND},
	Dirtab{"xdata", plan9.QTFILE, QWxdata, 0600},
}

type Mnt struct {
	lk sync.Mutex
	id int
	md *Mntdir
}

var mnt Mnt

var user string = "Wile E. Coyote"
var closing bool
var messagesize int = 8192 + plan9.IOHDRSZ // good start

func fsysinit() {
	initfcall()
	r1, w1, err := os.Pipe()
	if err != nil {
		util.Fatal("can't create pipe")
	}
	r2, w2, err := os.Pipe()
	if err != nil {
		util.Fatal("can't create pipe")
	}
	sfdR, sfdW = r1, w2
	if err := post9pservice(r2, w1, "acme", mtpt); err != nil {
		util.Fatal("can't post service")
	}
	u := os.Getenv("USER")
	if u != "" {
		user = u
	}
	go fsysproc()
}

func fsysproc() {
	var x *Xfid
	for {
		if x == nil {
			cxfidalloc <- nil
			x = <-cxfidalloc
		}
		fc, err := plan9.ReadFcall(sfdR)
		if err != nil {
			if closing {
				break
			}
			util.Fatal("i/o error on server channel")
		}
		if DEBUG != 0 {
			fmt.Fprintf(os.Stderr, "-> %v\n", fc)
		}
		x.fcall = fc
		if int(x.fcall.Type) >= len(fcall) || fcall[x.fcall.Type] == nil {
			var t plan9.Fcall
			x = respond(x, &t, "bad fcall type")
		} else {
			var f *Fid
			var t plan9.Fcall
			switch x.fcall.Type {
			case plan9.Tversion, plan9.Tauth, plan9.Tflush:
				f = nil
			case plan9.Tattach:
				f = newfid(int(x.fcall.Fid))
			default:
				f = newfid(int(x.fcall.Fid))
				if !f.busy {
					x.f = f
					x = respond(x, &t, "fid not in use")
					continue
				}
			}
			x.f = f
			x = fcall[x.fcall.Type](x, f)
		}
	}
}

func fsysaddid(dir []rune, incl [][]rune) *Mntdir {
	mnt.lk.Lock()
	mnt.id++
	id := mnt.id
	m := new(Mntdir)
	m.id = id
	m.dir = dir
	m.ref = 1 // one for Command, one will be incremented in attach
	m.next = mnt.md
	m.incl = incl
	mnt.md = m
	mnt.lk.Unlock()
	return m
}

func fsysincid(m *Mntdir) {
	mnt.lk.Lock()
	m.ref++
	mnt.lk.Unlock()
}

func fsysdelid(idm *Mntdir) {
	if idm == nil {
		return
	}
	mnt.lk.Lock()
	idm.ref--
	if idm.ref > 0 {
		mnt.lk.Unlock()
		return
	}
	var prev *Mntdir
	for m := mnt.md; m != nil; m = m.next {
		if m == idm {
			if prev != nil {
				prev.next = m.next
			} else {
				mnt.md = m.next
			}
			mnt.lk.Unlock()
			return
		}
		prev = m
	}
	mnt.lk.Unlock()
	cerr <- []byte(fmt.Sprintf("fsysdelid: can't find id %d\n", idm.id))
}

/*
 * Called only in exec.c:/^run(), from a different FD group
 */
func fsysmount(dir []rune, incl [][]rune) *Mntdir {
	return fsysaddid(dir, incl)
}

func fsysclose() {
	closing = true
	/*
		 * apparently this is not kosher on openbsd.
		 * perhaps because fsysproc is reading from sfd right now,
		 * the close hangs indefinitely.
		close(sfd);
	*/
}

func respond(x *Xfid, t *plan9.Fcall, err string) *Xfid {
	if err != "" {
		t.Type = plan9.Rerror
		t.Ename = err
	} else {
		t.Type = x.fcall.Type + 1
	}
	t.Fid = x.fcall.Fid
	t.Tag = x.fcall.Tag
	if DEBUG != 0 {
		fmt.Fprintf(os.Stderr, "<- %v\n", t)
	}
	if err := plan9.WriteFcall(sfdW, t); err != nil {
		util.Fatal("write error in respond")
	}
	return x
}

func fsysversion(x *Xfid, f *Fid) *Xfid {
	var t plan9.Fcall
	if x.fcall.Msize < 256 {
		return respond(x, &t, "version: message size too small")
	}
	messagesize = int(x.fcall.Msize)
	t.Msize = uint32(messagesize)
	if !strings.HasPrefix(x.fcall.Version, "9P2000") {
		return respond(x, &t, "unrecognized 9P version")
	}
	t.Version = "9P2000"
	return respond(x, &t, "")
}

func fsysauth(x *Xfid, f *Fid) *Xfid {
	var t plan9.Fcall
	return respond(x, &t, "acme: authentication not required")
}

func fsysflush(x *Xfid, f *Fid) *Xfid {
	x.c <- xfidflush
	return nil
}

func fsysattach(x *Xfid, f *Fid) *Xfid {
	var t plan9.Fcall
	if x.fcall.Uname != user {
		return respond(x, &t, Eperm)
	}
	f.busy = true
	f.open = false
	f.qid.Path = Qdir
	f.qid.Type = plan9.QTDIR
	f.qid.Vers = 0
	f.dir = dirtab[:]
	f.rpart = f.rpart[:0]
	f.w = nil
	t.Qid = f.qid
	f.mntdir = nil
	id, _ := strconv.Atoi(x.fcall.Aname)
	mnt.lk.Lock()
	var m *Mntdir
	for m = mnt.md; m != nil; m = m.next {
		if m.id == id {
			f.mntdir = m
			m.ref++
			break
		}
	}
	if m == nil && len(x.fcall.Aname) > 0 {
		cerr <- []byte(fmt.Sprintf("unknown id '%s' in attach", x.fcall.Aname))
	}
	mnt.lk.Unlock()
	return respond(x, &t, "")
}

func fsyswalk(x *Xfid, f *Fid) *Xfid {
	var nf *Fid
	var w *wind.Window
	var t plan9.Fcall
	if f.open {
		return respond(x, &t, "walk of open file")
	}
	if x.fcall.Fid != x.fcall.Newfid {
		nf = newfid(int(x.fcall.Newfid))
		if nf.busy {
			return respond(x, &t, "newfid already in use")
		}
		nf.busy = true
		nf.open = false
		nf.mntdir = f.mntdir
		if f.mntdir != nil {
			f.mntdir.ref++
		}
		nf.dir = f.dir
		nf.qid = f.qid
		nf.w = f.w
		nf.rpart = nf.rpart[:0] // not open, so must be zero
		if nf.w != nil {
			util.Incref(&nf.w.Ref)
		}
		f = nf // walk f
	}

	t.Wqid = nil
	var err string
	var dir []Dirtab
	id := WIN(f.qid)
	q := f.qid
	if len(x.fcall.Wname) > 0 {
		var i int
		for i = 0; i < len(x.fcall.Wname); i++ {
			if q.Type&plan9.QTDIR == 0 {
				err = Enotdir
				break
			}
			var typ uint8
			var path_ int

			if x.fcall.Wname[i] == ".." {
				typ = plan9.QTDIR
				path_ = Qdir
				id = 0
				if w != nil {
					wind.Winclose(w)
					w = nil
				}
				goto Accept
			}

			// is it a numeric name?
			for j := 0; j < len(x.fcall.Wname[i]); j++ {
				c := x.fcall.Wname[i][j]
				if c < '0' || '9' < c {
					goto Regular
				}
			}
			// yes: it's a directory
			if w != nil { // name has form 27/23; get out before losing w
				break
			}
			id, _ = strconv.Atoi(x.fcall.Wname[i])
			wind.TheRow.Lk.Lock()
			w = lookid(id)
			if w == nil {
				wind.TheRow.Lk.Unlock()
				break
			}
			util.Incref(&w.Ref) // we'll drop reference at end if there's an error
			path_ = Qdir
			typ = plan9.QTDIR
			wind.TheRow.Lk.Unlock()
			dir = dirtabw[:]
			goto Accept

		Regular:
			if x.fcall.Wname[i] == "new" {
				if w != nil {
					util.Fatal("w set in walk to new")
				}
				cnewwindow <- nil // signal newwindowthread
				w = <-cnewwindow  // receive new window
				util.Incref(&w.Ref)
				typ = plan9.QTDIR
				path_ = Qdir
				id = w.ID
				dir = dirtabw[:]
				goto Accept
			}
			{
				var d []Dirtab

				if id == 0 {
					d = dirtab[:]
				} else {
					d = dirtabw[:]
				}
				d = d[1:] // skip '.'
				for ; len(d) > 0; d = d[1:] {
					if x.fcall.Wname[i] == d[0].name {
						path_ = int(d[0].qid) // TODO(rsc)
						typ = d[0].typ
						dir = d
						goto Accept
					}
				}

				break // file not found
			}

		Accept:
			if i == plan9.MAXWELEM {
				err = "name too long"
				break
			}
			q.Type = typ
			q.Vers = 0
			q.Path = QID(id, path_)
			t.Wqid = append(t.Wqid, q)
			continue
		}

		if i == 0 && err == "" {
			err = Eexist
		}
	}

	if err != "" || len(t.Wqid) < len(x.fcall.Wname) {
		if nf != nil {
			nf.busy = false
			fsysdelid(nf.mntdir)
		}
	} else if len(t.Wqid) == len(x.fcall.Wname) {
		if w != nil {
			f.w = w
			w = nil // don't drop the reference
		}
		if dir != nil {
			f.dir = dir
		}
		f.qid = q
	}

	if w != nil {
		wind.Winclose(w)
	}

	return respond(x, &t, err)
}

func fsysopen(x *Xfid, f *Fid) *Xfid {
	// can't truncate anything, so just disregard
	x.fcall.Mode &^= plan9.OTRUNC | plan9.OCEXEC
	// can't execute or remove anything
	if x.fcall.Mode == plan9.OEXEC || x.fcall.Mode&plan9.ORCLOSE != 0 {
		goto Deny
	}
	{
		var m int
		switch x.fcall.Mode {
		default:
			goto Deny
		case plan9.OREAD:
			m = 0400
		case plan9.OWRITE:
			m = 0200
		case plan9.ORDWR:
			m = 0600
		}
		if (f.dir[0].perm & ^(plan9.DMDIR|plan9.DMAPPEND))&m != m {
			goto Deny
		}

		x.c <- xfidopen
		return nil
	}

Deny:
	var t plan9.Fcall
	return respond(x, &t, Eperm)
}

func fsyscreate(x *Xfid, f *Fid) *Xfid {
	var t plan9.Fcall
	return respond(x, &t, Eperm)
}

func fsysread(x *Xfid, f *Fid) *Xfid {
	if f.qid.Type&plan9.QTDIR != 0 {
		var t plan9.Fcall
		if FILE(f.qid) == Qacme { // empty dir
			t.Data = nil
			t.Count = 0
			respond(x, &t, "")
			return x
		}
		o := int64(x.fcall.Offset)
		e := int64(x.fcall.Offset) + int64(x.fcall.Count)
		clock := getclock()
		b := make([]byte, messagesize)
		id := WIN(f.qid)
		n := 0
		var d []Dirtab
		if id > 0 {
			d = dirtabw[:]
		} else {
			d = dirtab[:]
		}
		d = d[1:] // first entry is '.'
		var w int
		var i int
		for i = 0; len(d) > 0 && int64(i) < e; i += w {
			buf, err := dostat(WIN(x.f.qid), &d[0], clock)
			if err != nil {
				break
			}
			if n+len(buf) > int(x.fcall.Count) {
				break
			}
			copy(b[n:], buf)
			w = len(buf)
			if w <= 2 {
				break
			}
			if int64(i) >= o {
				n += w
			}
			d = d[1:]
		}
		if id == 0 {
			wind.TheRow.Lk.Lock()
			nids := 0
			var ids []int
			var j int
			var k int
			for j = 0; j < len(wind.TheRow.Col); j++ {
				c := wind.TheRow.Col[j]
				for k = 0; k < len(c.W); k++ {
					ids = append(ids, c.W[k].ID)
				}
			}
			wind.TheRow.Lk.Unlock()
			sort.Ints(ids)
			j = 0
			var dt Dirtab
			var nn int
			for ; j < nids && int64(i) < e; i += nn {
				k = ids[j]
				dt.name = fmt.Sprintf("%d", k)
				dt.qid = int(QID(k, Qdir)) // TODO(rsc)
				dt.typ = plan9.QTDIR
				dt.perm = plan9.DMDIR | 0700
				buf, err := dostat(k, &dt, clock)
				if err != nil || len(b) > int(x.fcall.Count)-n {
					break
				}
				copy(b[n:], buf)
				nn = len(buf)
				if int64(i) >= o {
					n += nn
				}
				j++
			}
		}
		t.Data = b[:n]
		t.Count = uint32(n)
		respond(x, &t, "")
		return x
	}
	x.c <- xfidread
	return nil
}

func fsyswrite(x *Xfid, f *Fid) *Xfid {
	x.c <- xfidwrite
	return nil
}

func fsysclunk(x *Xfid, f *Fid) *Xfid {
	fsysdelid(f.mntdir)
	x.c <- xfidclose
	return nil
}

func fsysremove(x *Xfid, f *Fid) *Xfid {
	var t plan9.Fcall
	return respond(x, &t, Eperm)
}

func fsysstat(x *Xfid, f *Fid) *Xfid {
	var t plan9.Fcall
	var err error
	t.Stat, err = dostat(WIN(x.f.qid), &f.dir[0], getclock())
	if err != nil {
		return respond(x, &t, err.Error())
	}
	if len(t.Stat) > messagesize-plan9.IOHDRSZ {
		return respond(x, &t, "stat too long")
	}
	return respond(x, &t, "")
}

func fsyswstat(x *Xfid, f *Fid) *Xfid {
	var t plan9.Fcall
	return respond(x, &t, Eperm)
}

func newfid(fid int) *Fid {
	var ff *Fid
	fh := &fids[fid&(Nhash-1)]
	var f *Fid
	for f = *fh; f != nil; f = f.next {
		if f.fid == fid {
			return f
		} else if ff == nil && !f.busy {
			ff = f
		}
	}
	if ff != nil {
		ff.fid = fid
		return ff
	}
	f = new(Fid)
	f.fid = fid
	f.next = *fh
	*fh = f
	return f
}

func getclock() int {
	return int(time.Now().Unix())
}

func dostat(id int, dir *Dirtab, clock int) ([]byte, error) {
	var d plan9.Dir
	d.Qid.Path = QID(id, dir.qid)
	d.Qid.Vers = 0
	d.Qid.Type = dir.typ
	d.Mode = plan9.Perm(dir.perm)
	d.Length = 0 // would be nice to do better
	d.Name = dir.name
	d.Uid = user
	d.Gid = user
	d.Muid = user
	d.Atime = uint32(clock)
	d.Mtime = uint32(clock)
	return d.Bytes()
}
