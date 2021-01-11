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
		plumbeditfid = fid
		plumbsendfid, _ = plumb.Open("send", plan9.OWRITE|plan9.OCEXEC)

		/*
		 * Relay messages.
		 */
		bedit := bufio.NewReader(plumbeditfid)
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
		fid = plumbsendfid
		plumbsendfid = nil
		fid.Close()

		fid = plumbeditfid
		plumbeditfid = nil
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
	if !external && t.w != nil && t.w.nopen[QWevent] > 0 {
		/* send alphanumeric expansion to external client */
		if !expanded {
			return
		}
		f := 0
		if (e.arg != nil && t.w != nil) || (len(e.name) > 0 && lookfile(e.name) != nil) {
			f = 1 /* acme can do it without loading a file */
		}
		if q0 != e.q0 || q1 != e.q1 {
			f |= 2 /* second (post-expand) message follows */
		}
		if len(e.name) != 0 {
			f |= 4 /* it's a file name */
		}
		c = 'l'
		if t.what == Body {
			c = 'L'
		}
		n = q1 - q0
		if n <= EVENTSIZE {
			r := make([]rune, n)
			bufread(&t.file.b, q0, r)
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
				bufread(&at.file.b, e.a0, r[len(e.name)+1:])
			}
		} else {
			n = e.q1 - e.q0
			r = make([]rune, n)
			bufread(&t.file.b, e.q0, r)
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
		/* send whitespace-delimited word to plumber */
		m := new(plumb.Message)
		m.Src = "acme"
		dir := dirname(t, nil)
		if len(dir.r) == 1 && dir.r[0] == '.' { /* sigh */
			dir.r = nil
		}
		if len(dir.r) == 0 {
			m.Dir = estrdup(wdir)
		} else {
			m.Dir = runetobyte(dir.r)
		}
		m.Type = estrdup("text")
		if q1 == q0 {
			if t.q1 > t.q0 && t.q0 <= q0 && q0 <= t.q1 {
				q0 = t.q0
				q1 = t.q1
			} else {
				p := q0
				for q0 > 0 && func() bool { c = tgetc(t, q0-1); return c != ' ' }() && c != '\t' && c != '\n' {
					q0--
				}
				for q1 < t.file.b.nc && func() bool { c = tgetc(t, q1); return c != ' ' }() && c != '\t' && c != '\n' {
					q1++
				}
				if q1 == q0 {
					return
				}
				m.Attr = &plumb.Attribute{Name: "click", Value: fmt.Sprint(p - q0)}
			}
		}
		r = make([]rune, q1-q0)
		bufread(&t.file.b, q0, r)
		m.Data = []byte(runetobyte(r))
		if len(m.Data) < messagesize-1024 && m.Send(plumbsendfid) == nil {
			return
		}
		/* plumber failed to match; fall through */
	}

	/* interpret alphanumeric string ourselves */
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
		bufread(&t.file.b, e.q0, r)
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
	if len(m.Data) >= BUFSIZE {
		warning(nil, "insanely long file name (%d bytes) in plumb message (%.32s...)\n", len(m.Data), m.Data)
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
	e.name = bytetorune(e.bname)
	e.jump = true
	e.a0 = 0
	e.a1 = 0
	addr := m.LookupAttr("addr")
	if addr != "" {
		r := bytetorune(addr)
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
	_, nr, _ := cvttorunes([]byte(name), rb[:], true)
	rs := cleanrname(runestr(rb[:nr]))
	winsetname(w, rs.r)
	r := make([]rune, len(m.Data))
	_, nr, _ = cvttorunes(m.Data, r, true)
	textinsert(&w.body, 0, r[:nr], true)
	w.body.file.mod = false
	w.dirty = false
	winsettag(w)
	textscrdraw(&w.body)
	textsetselect(&w.tag, w.tag.file.b.nc, w.tag.file.b.nc)
	xfidlog(w, "new")
}

func search(ct *Text, r []rune) bool {
	if len(r) == 0 || len(r) > ct.file.b.nc {
		return false
	}
	if 2*len(r) > RBUFSIZE {
		warning(nil, "string too long\n") // TODO(rsc): why???????
		return false
	}
	maxn := max(2*len(r), RBUFSIZE)
	s := fbufalloc()
	b := s[:0]
	around := 0
	q := ct.q1
	for {
		if q >= ct.file.b.nc {
			q = 0
			around = 1
			b = b[:0]
		}
		if len(b) > 0 {
			i := indexRune(b, r[0])
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
		/* reload if buffer covers neither string nor rest of file */
		if len(b) < len(r) && len(b) != ct.file.b.nc-q {
			nb := ct.file.b.nc - q
			if nb >= maxn {
				nb = maxn - 1
			}
			bufread(&ct.file.b, q, s[:nb])
			b = s[:nb]
		}
		/* this runeeq is fishy but the null at b[nb] makes it safe */ // TODO(rsc): NUL done gone
		if len(b) >= len(r) && runeeq(b[:len(r)], r) {
			if ct.w != nil {
				textshow(ct, q, q+len(r), true)
				winsettag(ct.w)
			} else {
				ct.q0 = q
				ct.q1 = q + len(r)
			}
			seltext = ct
			fbuffree(s)
			return true
		}
		b = b[1:]
		q++
		if around != 0 && q >= ct.q1 {
			break
		}
	}
	fbuffree(s)
	return false
}

var isfilec_Lx = []rune(".-+/:@")

func isfilec(r rune) bool {
	if isalnum(r) {
		return true
	}
	if indexRune(isfilec_Lx, r) >= 0 {
		return true
	}
	return false
}

func indexRune(rs []rune, c rune) int {
	for i, r := range rs {
		if r == c {
			return i
		}
	}
	return -1
}

func runesIndex(r, s []rune) int {
	if len(s) == 0 {
		return 0
	}
	c := s[0]
	for i, ri := range r {
		if len(r)-i < len(s) {
			break
		}
		if ri == c && runeeq(r[i:], s) {
			return i
		}
	}
	return -1
}

/* Runestr wrapper for cleanname */
func cleanrname(rs Runestr) Runestr {
	s := runetobyte(rs.r)
	s = path.Clean(s)
	r := rs.r[:cap(rs.r)]
	_, nr, _ := cvttorunes([]byte(s), r, true)
	rs.r = r[:nr]
	return rs
}

var includefile_Lslash = [2]rune{'/', 0}

func includefile(dir []rune, file []rune) Runestr {
	a := fmt.Sprintf("%s/%s", string(dir), string(file))
	if _, err := os.Stat(a); err != nil {
		return runestr(nil)
	}
	return Runestr{[]rune(path.Clean(a))}
}

var objdir []rune

var (
	Lsysinclude           = []rune("/sys/include")
	Lusrinclude           = []rune("/usr/include")
	Lusrlocalinclude      = []rune("/usr/local/include")
	Lusrlocalplan9include = []rune("/usr/local/plan9/include")
)

func includename(t *Text, r []rune) Runestr {
	var i int
	if objdir == nil && objtype != "" {
		buf := fmt.Sprintf("/%s/include", objtype)
		objdir = bytetorune(buf)
	}

	w := t.w
	if len(r) == 0 || r[0] == '/' || w == nil {
		return runestr(r)
	}
	if len(r) > 2 && r[0] == '.' && r[1] == '/' {
		return runestr(r)
	}
	var file Runestr
	file.r = nil
	for i = 0; i < len(w.incl) && file.r == nil; i++ {
		file = includefile(w.incl[i], r)
	}

	if file.r == nil {
		file = includefile(Lsysinclude, r)
	}
	if file.r == nil {
		file = includefile(Lusrlocalplan9include, r)
	}
	if file.r == nil {
		file = includefile(Lusrlocalinclude, r)
	}
	if file.r == nil {
		file = includefile(Lusrinclude, r)
	}
	if file.r == nil && objdir != nil {
		file = includefile(objdir, r)
	}
	if file.r == nil {
		return runestr(r)
	}
	return file
}

func dirname(t *Text, r []rune) Runestr {
	if t == nil || t.w == nil {
		goto Rescue
	}
	{
		nt := t.w.tag.file.b.nc
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
		return cleanrname(runestr(b))
	}

Rescue:
	tmp := runestr(r)
	if len(r) >= 1 {
		return cleanrname(tmp)
	}
	return tmp
}

func texthas(t *Text, q0 int, r []rune) bool {
	if int(q0) < 0 {
		return false
	}
	for i := 0; i < len(r); i++ {
		if q0+i >= t.file.b.nc || textreadc(t, q0+i) != r[i] {
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
		for q1 < t.file.b.nc {
			c = textreadc(t, q1)
			if !isfilec(c) {
				break
			}
			if c == ':' && !texthas(t, q1-4, Lhttpcss) && !texthas(t, q1-5, Lhttpscss) {
				colon = q1
				break
			}
			q1++
		}
		for q0 > 0 {
			c = textreadc(t, q0-1)
			if !isfilec(c) && !isaddrc(c) && !isregexc(c) {
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
			if colon < t.file.b.nc-1 && isaddrc(textreadc(t, colon+1)) {
				q1 = colon + 1
				for q1 < t.file.b.nc && isaddrc(textreadc(t, q1)) {
					q1++
				}
			}
		}
		if q1 > q0 {
			if colon >= 0 { /* stop at white space */
				for amax = colon + 1; amax < t.file.b.nc; amax++ {
					c = textreadc(t, amax)
					if c == ' ' || c == '\t' || c == '\n' {
						break
					}
				}
			} else {
				amax = t.file.b.nc
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
	/* see if it's a file name */
	r := make([]rune, n)
	bufread(&t.file.b, q0, r)
	/* is it a URL? look for http:// and https:// prefix */
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
	/* first, does it have bad chars? */
	nname := -1
	var i int
	for i = 0; i < n; i++ {
		c = r[i]
		if c == ':' && nname < 0 {
			if q0+i+1 < t.file.b.nc && (i == n-1 || isaddrc(textreadc(t, q0+i+1))) {
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
		if !isfilec(r[i]) && r[i] != ' ' {
			return false
		}
	}
	/*
	 * See if it's a file name in <>, and turn that into an include
	 * file name if so.  Should probably do it for "" too, but that's not
	 * restrictive enough syntax and checking for a #include earlier on the
	 * line would be silly.
	 */
	if q0 > 0 && textreadc(t, q0-1) == '<' && q1 < t.file.b.nc && textreadc(t, q1) == '>' {
		rs := includename(t, r[:nname])
		r = rs.r
		nname = len(rs.r)
	} else if amin == q0 {
		goto Isfile
	} else {
		rs := dirname(t, r[:nname])
		r = rs.r
		nname = len(rs.r)
	}
	e.bname = runetobyte(r[:nname])
	/* if it's already a window name, it's a file */
	{
		w := lookfile(r[:nname])
		if w != nil {
			goto Isfile
		}
		/* if it's the name of a file, it's a file */
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
	address(true, nil, range_(-1, -1), range_(0, 0), t, e.a0, amax, tgetc, &eval, (*int)(&e.a1))
	return true
}

func expand(t *Text, q0 int, q1 int, e *Expand) bool {
	*e = Expand{}
	e.agetc = tgetc
	/* if in selection, choose selection */
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
		for q1 < t.file.b.nc && isalnum(textreadc(t, q1)) {
			q1++
		}
		for q0 > 0 && isalnum(textreadc(t, q0-1)) {
			q0--
		}
	}
	e.q0 = q0
	e.q1 = q1
	return q1 > q0
}

func lookfile(s []rune) *Window {
	/* avoid terminal slash on directories */
	if len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	for _, c := range row.col {
		for _, w := range c.w {
			t := &w.body
			k := len(t.file.name)
			if k > 1 && t.file.name[k-1] == '/' {
				k--
			}
			if runeeq(t.file.name[:k], s) {
				w = w.body.file.curtext.w
				if w.col != nil { /* protect against race deleting w */
					return w
				}
			}
		}
	}
	return nil
}

func lookid(id int, dump bool) *Window {
	for _, c := range row.col {
		for _, w := range c.w {
			if dump && w.dumpid == id {
				return w
			}
			if !dump && w.id == id {
				return w
			}
		}
	}
	return nil
}

func openfile(t *Text, e *Expand) *Window {
	var r Range
	r.q0 = 0
	r.q1 = 0
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
			rs := runestr([]rune(path.Join(string(wdir), string(e.name))))
			e.name = rs.r
			w = lookfile(e.name)
		}
	}
	if w != nil {
		t = &w.body
		if !t.col.safe && t.fr.MaxLines == 0 { /* window is obscured by full-column window */
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
		t.file.mod = false
		t.w.dirty = false
		winsettag(t.w)
		textsetselect(&t.w.tag, t.w.tag.file.b.nc, t.w.tag.file.b.nc)
		if ow != nil {
			for i := len(ow.incl); ; {
				i--
				if i < 0 {
					break
				}
				rp := runestrdup(ow.incl[i])
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
		r = address(true, t, range_(-1, -1), range_(t.q0, t.q1), e.arg.(*Text), e.a0, e.a1, e.agetc, &eval, &dummy)
		if r.q0 > r.q1 {
			eval = false
			warning(nil, "addresses out of order\n")
		}
		if !eval {
			e.jump = false /* don't jump if invalid address */
		}
	}
	if !eval {
		r.q0 = t.q0
		r.q1 = t.q1
	}
	textshow(t, r.q0, r.q1, true)
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
	/* loop condition: *arg is not a blank */
	for ndone := 0; ; ndone++ {
		a = findbl(arg)
		if len(a) == len(arg) {
			if ndone == 0 && et.col != nil {
				w := coladd(et.col, nil, nil, -1)
				winsettag(w)
				xfidlog(w, "new")
			}
			break
		}
		nf := len(a) - len(arg)
		f := runestrdup(arg[:nf])
		rs := dirname(et, f)
		var e Expand
		e.name = rs.r
		e.bname = runetobyte(rs.r)
		e.jump = true
		openfile(et, &e)
		arg = skipbl(a)
	}
}
