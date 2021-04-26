// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <regexp.h>
// #include <9pclient.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"time"

	addrpkg "9fans.net/go/cmd/acme/internal/addr"
	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/plan9"
	"9fans.net/go/plan9/client"
	"9fans.net/go/plumb"
)

var plumbsendfid *client.Fid
var plumbeditfid *client.Fid

var nuntitled int

func plumbthread() {
	/*
	 * Loop so that if plumber is restarted, acme need not be.
	 */
	for {
		/*
		 * Connect to plumber.
		 */
		// TODO(rsc): plumbunmount()
		var fid *client.Fid
		for {
			var err error
			fid, err = plumb.Open("edit", plan9.OREAD|plan9.OCEXEC)
			if err == nil {
				break
			}
			time.Sleep(2 * time.Second)
		}
		big.Lock() // TODO still racy
		plumbeditfid = fid
		plumbsendfid, _ = plumb.Open("send", plan9.OWRITE|plan9.OCEXEC)
		big.Unlock()

		/*
		 * Relay messages.
		 */
		bedit := bufio.NewReader(fid)
		for {
			m := new(plumb.Message)
			err := m.Recv(bedit)
			if err != nil {
				break
			}
			cplumb <- m
		}

		/*
		 * Lost connection.
		 */
		big.Lock() // TODO still racy
		fid = plumbsendfid
		plumbsendfid = nil
		big.Unlock()
		fid.Close()

		big.Lock() // TODO still racy
		fid = plumbeditfid
		plumbeditfid = nil
		big.Unlock()
		fid.Close()
	}
}

func startplumbing() {
	go plumbthread()
}

func look3(t *Text, q0, q1 int, external bool) {
	ct := seltext
	if ct == nil {
		seltext = t
	}
	var e Expand
	expanded := expand(t, q0, q1, &e)
	var n int
	var c rune
	var r []rune
	if !external && t.w != nil && t.w.external {
		// send alphanumeric expansion to external client
		if !expanded {
			return
		}
		f := 0
		if (e.arg != nil && t.w != nil) || (len(e.name) > 0 && lookfile(e.name) != nil) {
			f = 1 // acme can do it without loading a file
		}
		if q0 != e.q0 || q1 != e.q1 {
			f |= 2 // second (post-expand) message follows
		}
		if len(e.name) != 0 {
			f |= 4 // it's a file name
		}
		c = 'l'
		if t.what == Body {
			c = 'L'
		}
		n = q1 - q0
		if n <= EVENTSIZE {
			r := make([]rune, n)
			t.file.Read(q0, r)
			winevent(t.w, "%c%d %d %d %d %s\n", c, q0, q1, f, n, string(r))
		} else {
			winevent(t.w, "%c%d %d %d 0 \n", c, q0, q1, f)
		}
		if q0 == e.q0 && q1 == e.q1 {
			return
		}
		if len(e.name) != 0 {
			n = len(e.name)
			if e.a1 > e.a0 {
				n += 1 + (e.a1 - e.a0)
			}
			r = make([]rune, n)
			copy(r, e.name)
			if e.a1 > e.a0 {
				r[len(e.name)] = ':'
				at := e.arg.(*Text)
				at.file.Read(e.a0, r[len(e.name)+1:])
			}
		} else {
			n = e.q1 - e.q0
			r = make([]rune, n)
			t.file.Read(e.q0, r)
		}
		f &^= 2
		if n <= EVENTSIZE {
			r := r
			if len(r) > n {
				r = r[:n]
			}
			winevent(t.w, "%c%d %d %d %d %s\n", c, e.q0, e.q1, f, n, string(r))
		} else {
			winevent(t.w, "%c%d %d %d 0 \n", c, e.q0, e.q1, f)
		}
		return
	}
	if plumbsendfid != nil {
		// send whitespace-delimited word to plumber
		m := new(plumb.Message)
		m.Src = "acme"
		dir := dirname(t, nil)
		if len(dir) == 1 && dir[0] == '.' { // sigh
			dir = nil
		}
		if len(dir) == 0 {
			m.Dir = wdir
		} else {
			m.Dir = string(dir)
		}
		m.Type = "text"
		if q1 == q0 {
			if t.q1 > t.q0 && t.q0 <= q0 && q0 <= t.q1 {
				q0 = t.q0
				q1 = t.q1
			} else {
				p := q0
				for q0 > 0 && func() bool { c = tgetc(t, q0-1); return c != ' ' }() && c != '\t' && c != '\n' {
					q0--
				}
				for q1 < t.Len() && func() bool { c = tgetc(t, q1); return c != ' ' }() && c != '\t' && c != '\n' {
					q1++
				}
				if q1 == q0 {
					return
				}
				m.Attr = &plumb.Attribute{Name: "click", Value: fmt.Sprint(p - q0)}
			}
		}
		r = make([]rune, q1-q0)
		t.file.Read(q0, r)
		m.Data = []byte(string(r))
		if len(m.Data) < messagesize-1024 && m.Send(plumbsendfid) == nil {
			return
		}
		// plumber failed to match; fall through
	}

	// interpret alphanumeric string ourselves
	if !expanded {
		return
	}
	if e.name != nil || e.arg != nil {
		openfile(t, &e)
	} else {
		if t.w == nil {
			return
		}
		ct = &t.w.body
		if t.w != ct.w {
			winlock(ct.w, 'M')
		}
		if t == ct {
			textsetselect(ct, e.q1, e.q1)
		}
		r = make([]rune, e.q1-e.q0)
		t.file.Read(e.q0, r)
		if search(ct, r) && e.jump {
			display.MoveCursor(ct.fr.PointOf(ct.fr.P0).Add(draw.Pt(4, ct.fr.Font.Height-4)))
		}
		if t.w != ct.w {
			winunlock(ct.w)
		}
	}
}

func plumbgetc(a interface{}, n int) rune {
	r := a.([]rune)
	if n > len(r) {
		return 0
	}
	return r[n]
}

func plumblook(m *plumb.Message) {
	if len(m.Data) >= bufs.Len {
		alog.Printf("insanely long file name (%d bytes) in plumb message (%.32s...)\n", len(m.Data), m.Data)
		return
	}
	var e Expand
	e.q0 = 0
	e.q1 = 0
	if len(m.Data) == 0 {
		return
	}
	e.arg = nil
	e.bname = string(m.Data)
	e.name = []rune(e.bname)
	e.jump = true
	e.a0 = 0
	e.a1 = 0
	addr := m.LookupAttr("addr")
	if addr != "" {
		r := []rune(addr)
		e.a1 = len(r)
		e.arg = r
		e.agetc = plumbgetc
	}
	display.Top()
	openfile(nil, &e)
}

func plumbshow(m *plumb.Message) {
	display.Top()
	w := makenewwindow(nil)
	name := m.LookupAttr("filename")
	if name == "" {
		nuntitled++
		name = fmt.Sprintf("Untitled-%d", nuntitled)
	}
	if name[0] != '/' && m.Dir != "" {
		name = fmt.Sprintf("%s/%s", m.Dir, name)
	}
	var rb [256]rune
	_, nr, _ := runes.Convert([]byte(name), rb[:], true)
	rs := runes.CleanPath(rb[:nr])
	winsetname(w, rs)
	r := make([]rune, len(m.Data))
	_, nr, _ = runes.Convert(m.Data, r, true)
	textinsert(&w.body, 0, r[:nr], true)
	w.body.file.SetMod(false)
	w.dirty = false
	winsettag(w)
	textscrdraw(&w.body)
	textsetselect(&w.tag, w.tag.Len(), w.tag.Len())
	xfidlog(w, "new")
}

func search(ct *Text, r []rune) bool {
	if len(r) == 0 || len(r) > ct.Len() {
		return false
	}
	if 2*len(r) > bufs.RuneLen {
		alog.Printf("string too long\n") // TODO(rsc): why???????
		return false
	}
	maxn := util.Max(2*len(r), bufs.RuneLen)
	s := bufs.AllocRunes()
	b := s[:0]
	around := 0
	q := ct.q1
	for {
		if q >= ct.Len() {
			q = 0
			around = 1
			b = b[:0]
		}
		if len(b) > 0 {
			i := runes.IndexRune(b, r[0])
			if i < 0 {
				q += len(b)
				b = b[:0]
				if around != 0 && q >= ct.q1 {
					break
				}
				continue
			}
			q += i
			b = b[i:]
		}
		// reload if buffer covers neither string nor rest of file
		if len(b) < len(r) && len(b) != ct.Len()-q {
			nb := ct.Len() - q
			if nb >= maxn {
				nb = maxn - 1
			}
			ct.file.Read(q, s[:nb])
			b = s[:nb]
		}
		// this runeeq is fishy but the null at b[nb] makes it safe // TODO(rsc): NUL done gone
		if len(b) >= len(r) && runes.Equal(b[:len(r)], r) {
			if ct.w != nil {
				textshow(ct, q, q+len(r), true)
				winsettag(ct.w)
			} else {
				ct.q0 = q
				ct.q1 = q + len(r)
			}
			seltext = ct
			bufs.FreeRunes(s)
			return true
		}
		b = b[1:]
		q++
		if around != 0 && q >= ct.q1 {
			break
		}
	}
	bufs.FreeRunes(s)
	return false
}

// Runestr wrapper for cleanname

var includefile_Lslash = [2]rune{'/', 0}

func includefile(dir []rune, file []rune) []rune {
	a := fmt.Sprintf("%s/%s", string(dir), string(file))
	if _, err := os.Stat(a); err != nil {
		return nil
	}
	return []rune(path.Clean(a))
}

var objdir []rune

var (
	Lsysinclude           = []rune("/sys/include")
	Lusrinclude           = []rune("/usr/include")
	Lusrlocalinclude      = []rune("/usr/local/include")
	Lusrlocalplan9include = []rune("/usr/local/plan9/include")
)

func includename(t *Text, r []rune) []rune {
	var i int
	if objdir == nil && objtype != "" {
		buf := fmt.Sprintf("/%s/include", objtype)
		objdir = []rune(buf)
	}

	w := t.w
	if len(r) == 0 || r[0] == '/' || w == nil {
		return r
	}
	if len(r) > 2 && r[0] == '.' && r[1] == '/' {
		return r
	}
	var file []rune
	file = nil
	for i = 0; i < len(w.incl) && file == nil; i++ {
		file = includefile(w.incl[i], r)
	}

	if file == nil {
		file = includefile(Lsysinclude, r)
	}
	if file == nil {
		file = includefile(Lusrlocalplan9include, r)
	}
	if file == nil {
		file = includefile(Lusrlocalinclude, r)
	}
	if file == nil {
		file = includefile(Lusrinclude, r)
	}
	if file == nil && objdir != nil {
		file = includefile(objdir, r)
	}
	if file == nil {
		return r
	}
	return file
}

func dirname(t *Text, r []rune) []rune {
	if t == nil || t.w == nil {
		goto Rescue
	}
	{
		nt := t.w.tag.Len()
		if nt == 0 {
			goto Rescue
		}
		if len(r) >= 1 && r[0] == '/' {
			goto Rescue
		}
		b, i := parsetag(t.w, len(r))
		slash := -1
		for i--; i >= 0; i-- {
			if b[i] == '/' {
				slash = i
				break
			}
		}
		if slash < 0 {
			goto Rescue
		}
		b = append(b[:slash+1], r...)
		return runes.CleanPath(b)
	}

Rescue:
	tmp := r
	if len(r) >= 1 {
		return runes.CleanPath(tmp)
	}
	return tmp
}

func texthas(t *Text, q0 int, r []rune) bool {
	if int(q0) < 0 {
		return false
	}
	for i := 0; i < len(r); i++ {
		if q0+i >= t.Len() || t.RuneAt(q0+i) != r[i] {
			return false
		}
	}
	return true
}

func hasPrefix(r []rune, s []rune) bool {
	if len(r) < len(s) {
		return false
	}
	for i := 0; i < len(s); i++ {
		if r[i] != s[i] {
			return false
		}
	}
	return true
}

var (
	Lhttpcss  = []rune("http://")
	Lhttpscss = []rune("https://")
)

func expandfile(t *Text, q0 int, q1 int, e *Expand) bool {
	amax := q1
	var c rune
	if q1 == q0 {
		colon := -1
		for q1 < t.Len() {
			c = t.RuneAt(q1)
			if !runes.IsFilename(c) {
				break
			}
			if c == ':' && !texthas(t, q1-4, Lhttpcss) && !texthas(t, q1-5, Lhttpscss) {
				colon = q1
				break
			}
			q1++
		}
		for q0 > 0 {
			c = t.RuneAt(q0 - 1)
			if !runes.IsFilename(c) && !runes.IsAddr(c) && !runes.IsRegx(c) {
				break
			}
			q0--
			if colon < 0 && c == ':' && !texthas(t, q0-4, Lhttpcss) && !texthas(t, q0-5, Lhttpscss) {
				colon = q0
			}
		}
		/*
		 * if it looks like it might begin file: , consume address chars after :
		 * otherwise terminate expansion at :
		 */
		if colon >= 0 {
			q1 = colon
			if colon < t.Len()-1 && runes.IsAddr(t.RuneAt(colon+1)) {
				q1 = colon + 1
				for q1 < t.Len() && runes.IsAddr(t.RuneAt(q1)) {
					q1++
				}
			}
		}
		if q1 > q0 {
			if colon >= 0 { // stop at white space
				for amax = colon + 1; amax < t.Len(); amax++ {
					c = t.RuneAt(amax)
					if c == ' ' || c == '\t' || c == '\n' {
						break
					}
				}
			} else {
				amax = t.Len()
			}
		}
	}
	amin := amax
	e.q0 = q0
	e.q1 = q1
	n := q1 - q0
	if n == 0 {
		return false
	}
	// see if it's a file name
	r := make([]rune, n)
	t.file.Read(q0, r)
	// is it a URL? look for http:// and https:// prefix
	if hasPrefix(r, Lhttpcss) || hasPrefix(r, Lhttpscss) {
		// Avoid capturing end-of-sentence punctuation.
		if r[n-1] == '.' {
			e.q1--
			n--
		}
		e.name = r
		e.arg = t
		e.a0 = e.q1
		e.a1 = e.q1
		return true
	}
	// first, does it have bad chars?
	nname := -1
	var i int
	for i = 0; i < n; i++ {
		c = r[i]
		if c == ':' && nname < 0 {
			if q0+i+1 < t.Len() && (i == n-1 || runes.IsAddr(t.RuneAt(q0+i+1))) {
				amin = q0 + i
			} else {
				return false
			}
			nname = i
		}
	}
	if nname == -1 {
		nname = n
	}
	for i = 0; i < nname; i++ {
		if !runes.IsFilename(r[i]) && r[i] != ' ' {
			return false
		}
	}
	/*
	 * See if it's a file name in <>, and turn that into an include
	 * file name if so.  Should probably do it for "" too, but that's not
	 * restrictive enough syntax and checking for a #include earlier on the
	 * line would be silly.
	 */
	if q0 > 0 && t.RuneAt(q0-1) == '<' && q1 < t.Len() && t.RuneAt(q1) == '>' {
		rs := includename(t, r[:nname])
		r = rs
		nname = len(rs)
	} else if amin == q0 {
		goto Isfile
	} else {
		rs := dirname(t, r[:nname])
		r = rs
		nname = len(rs)
	}
	e.bname = string(r[:nname])
	// if it's already a window name, it's a file
	{
		w := lookfile(r[:nname])
		if w != nil {
			goto Isfile
		}
		// if it's the name of a file, it's a file
		if ismtpt(e.bname) {
			e.bname = ""
			return false
		}
		if _, err := os.Stat(e.bname); err != nil {
			e.bname = ""
			return false
		}
	}

Isfile:
	e.name = r[:nname]
	e.arg = t
	e.a0 = amin + 1
	eval := false
	addrpkg.Eval(true, nil, runes.Rng(-1, -1), runes.Rng(0, 0), t, e.a0, amax, tgetc, &eval, (*int)(&e.a1))
	return true
}

func expand(t *Text, q0 int, q1 int, e *Expand) bool {
	*e = Expand{}
	e.agetc = tgetc
	// if in selection, choose selection
	e.jump = true
	if q1 == q0 && t.q1 > t.q0 && t.q0 <= q0 && q0 <= t.q1 {
		q0 = t.q0
		q1 = t.q1
		if t.what == Tag {
			e.jump = false
		}
	}

	if expandfile(t, q0, q1, e) {
		return true
	}

	if q0 == q1 {
		for q1 < t.Len() && runes.IsAlphaNum(t.RuneAt(q1)) {
			q1++
		}
		for q0 > 0 && runes.IsAlphaNum(t.RuneAt(q0-1)) {
			q0--
		}
	}
	e.q0 = q0
	e.q1 = q1
	return q1 > q0
}

func lookfile(s []rune) *Window {
	// avoid terminal slash on directories
	if len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	for _, c := range row.col {
		for _, w := range c.w {
			t := &w.body
			k := len(t.file.Name())
			if k > 1 && t.file.Name()[k-1] == '/' {
				k--
			}
			if runes.Equal(t.file.Name()[:k], s) {
				w = w.body.file.curtext.w
				if w.col != nil { // protect against race deleting w
					return w
				}
			}
		}
	}
	return nil
}

func lookid(id int) *Window {
	for _, c := range row.col {
		for _, w := range c.w {
			if w.id == id {
				return w
			}
		}
	}
	return nil
}

func openfile(t *Text, e *Expand) *Window {
	var r runes.Range
	r.Pos = 0
	r.End = 0
	var w *Window
	if len(e.name) == 0 {
		w = t.w
		if w == nil {
			return nil
		}
	} else {
		w = lookfile(e.name)
		if w == nil && e.name[0] != '/' {
			/*
			 * Unrooted path in new window.
			 * This can happen if we type a pwd-relative path
			 * in the topmost tag or the column tags.
			 * Most of the time plumber takes care of these,
			 * but plumber might not be running or might not
			 * be configured to accept plumbed directories.
			 * Make the name a full path, just like we would if
			 * opening via the plumber.
			 */
			rs := []rune(path.Join(string(wdir), string(e.name)))
			e.name = rs
			w = lookfile(e.name)
		}
	}
	if w != nil {
		t = &w.body
		if !t.col.safe && t.fr.MaxLines == 0 { // window is obscured by full-column window
			colgrow(t.col, t.col.w[0], 1)
		}
	} else {
		var ow *Window
		if t != nil {
			ow = t.w
		}
		w = makenewwindow(t)
		t = &w.body
		winsetname(w, e.name)
		if textload(t, 0, e.bname, true) >= 0 {
			t.file.unread = false
		}
		t.file.SetMod(false)
		t.w.dirty = false
		winsettag(t.w)
		textsetselect(&t.w.tag, t.w.tag.Len(), t.w.tag.Len())
		if ow != nil {
			for i := len(ow.incl); ; {
				i--
				if i < 0 {
					break
				}
				rp := runes.Clone(ow.incl[i])
				winaddincl(w, rp)
			}
			w.autoindent = ow.autoindent
		} else {
			w.autoindent = globalautoindent
		}
		xfidlog(w, "new")
	}
	var eval bool
	if e.a1 == e.a0 {
		eval = false
	} else {
		eval = true
		var dummy int
		r = addrpkg.Eval(true, t, runes.Rng(-1, -1), runes.Rng(t.q0, t.q1), e.arg, e.a0, e.a1, e.agetc, &eval, &dummy)
		if r.Pos > r.End {
			eval = false
			alog.Printf("addresses out of order\n")
		}
		if !eval {
			e.jump = false // don't jump if invalid address
		}
	}
	if !eval {
		r.Pos = t.q0
		r.End = t.q1
	}
	textshow(t, r.Pos, r.End, true)
	winsettag(t.w)
	seltext = t
	if e.jump {
		display.MoveCursor(t.fr.PointOf(t.fr.P0).Add(draw.Pt(4, font.Height-4)))
	}
	return w
}

func new_(et, t, argt *Text, flag1, flag2 bool, arg []rune) {
	var a []rune
	getarg(argt, false, true, &a)
	if a != nil {
		new_(et, t, nil, flag1, flag2, a)
		if len(arg) == 0 {
			return
		}
	}
	// loop condition: *arg is not a blank
	for ndone := 0; ; ndone++ {
		a = runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			if ndone == 0 && et.col != nil {
				w := coladd(et.col, nil, nil, -1)
				winsettag(w)
				xfidlog(w, "new")
			}
			break
		}
		nf := len(arg) - len(a)
		f := runes.Clone(arg[:nf])
		rs := dirname(et, f)
		var e Expand
		e.name = rs
		e.bname = string(rs)
		e.jump = true
		openfile(et, &e)
		arg = runes.SkipBlank(a)
	}
}
