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

package ui

import (
	"fmt"
	"os"
	"path"

	"9fans.net/go/cmd/acme/internal/addr"
	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
	"9fans.net/go/plan9/client"
	"9fans.net/go/plumb"
)

func Look3(t *wind.Text, q0, q1 int, external bool) {
	ct := wind.Seltext
	if ct == nil {
		wind.Seltext = t
	}
	var e Expand
	expanded := Expand_(t, q0, q1, &e)
	var n int
	var c rune
	var r []rune
	if !external && t.W != nil && t.W.External {
		// send alphanumeric expansion to external client
		if !expanded {
			return
		}
		f := 0
		if (e.Arg != nil && t.W != nil) || (len(e.Name) > 0 && LookFile(e.Name) != nil) {
			f = 1 // acme can do it without loading a file
		}
		if q0 != e.Q0 || q1 != e.Q1 {
			f |= 2 // second (post-expand) message follows
		}
		if len(e.Name) != 0 {
			f |= 4 // it's a file name
		}
		c = 'l'
		if t.What == wind.Body {
			c = 'L'
		}
		n = q1 - q0
		if n <= wind.EVENTSIZE {
			r := make([]rune, n)
			t.File.Read(q0, r)
			wind.Winevent(t.W, "%c%d %d %d %d %s\n", c, q0, q1, f, n, string(r))
		} else {
			wind.Winevent(t.W, "%c%d %d %d 0 \n", c, q0, q1, f)
		}
		if q0 == e.Q0 && q1 == e.Q1 {
			return
		}
		if len(e.Name) != 0 {
			n = len(e.Name)
			if e.A1 > e.A0 {
				n += 1 + (e.A1 - e.A0)
			}
			r = make([]rune, n)
			copy(r, e.Name)
			if e.A1 > e.A0 {
				r[len(e.Name)] = ':'
				at := e.Arg.(*wind.Text)
				at.File.Read(e.A0, r[len(e.Name)+1:])
			}
		} else {
			n = e.Q1 - e.Q0
			r = make([]rune, n)
			t.File.Read(e.Q0, r)
		}
		f &^= 2
		if n <= wind.EVENTSIZE {
			r := r
			if len(r) > n {
				r = r[:n]
			}
			wind.Winevent(t.W, "%c%d %d %d %d %s\n", c, e.Q0, e.Q1, f, n, string(r))
		} else {
			wind.Winevent(t.W, "%c%d %d %d 0 \n", c, e.Q0, e.Q1, f)
		}
		return
	}
	if Plumbsendfid != nil {
		// send whitespace-delimited word to plumber
		m := new(plumb.Message)
		m.Src = "acme"
		dir := wind.Dirname(t, nil)
		if len(dir) == 1 && dir[0] == '.' { // sigh
			dir = nil
		}
		if len(dir) == 0 {
			m.Dir = Wdir
		} else {
			m.Dir = string(dir)
		}
		m.Type = "text"
		if q1 == q0 {
			if t.Q1 > t.Q0 && t.Q0 <= q0 && q0 <= t.Q1 {
				q0 = t.Q0
				q1 = t.Q1
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
		t.File.Read(q0, r)
		m.Data = []byte(string(r))
		if len(m.Data) < 7*1024 && m.Send(Plumbsendfid) == nil {
			return
		}
		// plumber failed to match; fall through
	}

	// interpret alphanumeric string ourselves
	if !expanded {
		return
	}
	if e.Name != nil || e.Arg != nil {
		Openfile(t, &e)
	} else {
		if t.W == nil {
			return
		}
		ct = &t.W.Body
		if t.W != ct.W {
			wind.Winlock(ct.W, 'M')
		}
		if t == ct {
			wind.Textsetselect(ct, e.Q1, e.Q1)
		}
		r = make([]rune, e.Q1-e.Q0)
		t.File.Read(e.Q0, r)
		if Search(ct, r) && e.Jump {
			adraw.Display.MoveCursor(ct.Fr.PointOf(ct.Fr.P0).Add(draw.Pt(4, ct.Fr.Font.Height-4)))
		}
		if t.W != ct.W {
			wind.Winunlock(ct.W)
		}
	}
}

func Search(ct *wind.Text, r []rune) bool {
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
	q := ct.Q1
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
				if around != 0 && q >= ct.Q1 {
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
			ct.File.Read(q, s[:nb])
			b = s[:nb]
		}
		// this runeeq is fishy but the null at b[nb] makes it safe // TODO(rsc): NUL done gone
		if len(b) >= len(r) && runes.Equal(b[:len(r)], r) {
			if ct.W != nil {
				wind.Textshow(ct, q, q+len(r), true)
				wind.Winsettag(ct.W)
			} else {
				ct.Q0 = q
				ct.Q1 = q + len(r)
			}
			wind.Seltext = ct
			bufs.FreeRunes(s)
			return true
		}
		b = b[1:]
		q++
		if around != 0 && q >= ct.Q1 {
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

func includename(t *wind.Text, r []rune) []rune {
	var i int
	if objdir == nil && Objtype != "" {
		buf := fmt.Sprintf("/%s/include", Objtype)
		objdir = []rune(buf)
	}

	w := t.W
	if len(r) == 0 || r[0] == '/' || w == nil {
		return r
	}
	if len(r) > 2 && r[0] == '.' && r[1] == '/' {
		return r
	}
	var file []rune
	file = nil
	for i = 0; i < len(w.Incl) && file == nil; i++ {
		file = includefile(w.Incl[i], r)
	}

	if file == nil {
		file = includefile([]rune("/sys/include"), r)
	}
	if file == nil {
		file = includefile([]rune("/usr/local/plan9/include"), r)
	}
	if file == nil {
		file = includefile([]rune("/usr/local/include"), r)
	}
	if file == nil {
		file = includefile([]rune("/usr/include"), r)
	}
	if file == nil && objdir != nil {
		file = includefile(objdir, r)
	}
	if file == nil {
		return r
	}
	return file
}

func texthas(t *wind.Text, q0 int, r []rune) bool {
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

func expandfile(t *wind.Text, q0 int, q1 int, e *Expand) bool {
	amax := q1
	var c rune
	if q1 == q0 {
		colon := -1
		for q1 < t.Len() {
			c = t.RuneAt(q1)
			if !runes.IsFilename(c) {
				break
			}
			if c == ':' && !texthas(t, q1-4, []rune("http://")) && !texthas(t, q1-5, []rune("https://")) {
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
			if colon < 0 && c == ':' && !texthas(t, q0-4, []rune("http://")) && !texthas(t, q0-5, []rune("https://")) {
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
	e.Q0 = q0
	e.Q1 = q1
	n := q1 - q0
	if n == 0 {
		return false
	}
	// see if it's a file name
	r := make([]rune, n)
	t.File.Read(q0, r)
	// is it a URL? look for http:// and https:// prefix
	if hasPrefix(r, []rune("http://")) || hasPrefix(r, []rune("https://")) {
		// Avoid capturing end-of-sentence punctuation.
		if r[n-1] == '.' {
			e.Q1--
			n--
		}
		e.Name = r
		e.Arg = t
		e.A0 = e.Q1
		e.A1 = e.Q1
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
		rs := wind.Dirname(t, r[:nname])
		r = rs
		nname = len(rs)
	}
	e.Bname = string(r[:nname])
	// if it's already a window name, it's a file
	{
		w := LookFile(r[:nname])
		if w != nil {
			goto Isfile
		}
		// if it's the name of a file, it's a file
		if Ismtpt(e.Bname) {
			e.Bname = ""
			return false
		}
		if _, err := os.Stat(e.Bname); err != nil {
			e.Bname = ""
			return false
		}
	}

Isfile:
	e.Name = r[:nname]
	e.Arg = t
	e.A0 = amin + 1
	eval := false
	addr.Eval(true, nil, runes.Rng(-1, -1), runes.Rng(0, 0), t, e.A0, amax, tgetc, &eval, (*int)(&e.A1))
	return true
}

func Expand_(t *wind.Text, q0 int, q1 int, e *Expand) bool {
	*e = Expand{}
	e.Agetc = tgetc
	// if in selection, choose selection
	e.Jump = true
	if q1 == q0 && t.Q1 > t.Q0 && t.Q0 <= q0 && q0 <= t.Q1 {
		q0 = t.Q0
		q1 = t.Q1
		if t.What == wind.Tag {
			e.Jump = false
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
	e.Q0 = q0
	e.Q1 = q1
	return q1 > q0
}

func LookFile(s []rune) *wind.Window {
	// avoid terminal slash on directories
	if len(s) > 0 && s[len(s)-1] == '/' {
		s = s[:len(s)-1]
	}
	for _, c := range wind.TheRow.Col {
		for _, w := range c.W {
			t := &w.Body
			k := len(t.File.Name())
			if k > 1 && t.File.Name()[k-1] == '/' {
				k--
			}
			if runes.Equal(t.File.Name()[:k], s) {
				w = w.Body.File.Curtext.W
				if w.Col != nil { // protect against race deleting w
					return w
				}
			}
		}
	}
	return nil
}

func LookID(id int) *wind.Window {
	for _, c := range wind.TheRow.Col {
		for _, w := range c.W {
			if w.ID == id {
				return w
			}
		}
	}
	return nil
}

type Expand struct {
	Q0    int
	Q1    int
	Name  []rune
	Bname string
	Jump  bool
	Arg   interface{}
	Agetc func(interface{}, int) rune
	A0    int
	A1    int
}

func tgetc(a interface{}, n int) rune {
	t := a.(*wind.Text)
	if n >= t.Len() {
		return 0
	}
	return t.RuneAt(n)
}

var Ismtpt func(string) bool

var Plumbsendfid *client.Fid

var Wdir = "."

var Objtype string

func Openfile(t *wind.Text, e *Expand) *wind.Window {
	var r runes.Range
	r.Pos = 0
	r.End = 0
	var w *wind.Window
	if len(e.Name) == 0 {
		w = t.W
		if w == nil {
			return nil
		}
	} else {
		w = LookFile(e.Name)
		if w == nil && e.Name[0] != '/' {
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
			rs := []rune(path.Join(string(Wdir), string(e.Name)))
			e.Name = rs
			w = LookFile(e.Name)
		}
	}
	if w != nil {
		t = &w.Body
		if !t.Col.Safe && t.Fr.MaxLines == 0 { // window is obscured by full-column window
			wind.Colgrow(t.Col, t.Col.W[0], 1)
		}
	} else {
		var ow *wind.Window
		if t != nil {
			ow = t.W
		}
		w = Makenewwindow(t)
		t = &w.Body
		wind.Winsetname(w, e.Name)
		if Textload(t, 0, e.Bname, true) >= 0 {
			t.File.Unread = false
		}
		t.File.SetMod(false)
		t.W.Dirty = false
		wind.Winsettag(t.W)
		wind.Textsetselect(&t.W.Tag, t.W.Tag.Len(), t.W.Tag.Len())
		if ow != nil {
			for i := len(ow.Incl); ; {
				i--
				if i < 0 {
					break
				}
				rp := runes.Clone(ow.Incl[i])
				wind.Winaddincl(w, rp)
			}
			w.Autoindent = ow.Autoindent
		} else {
			w.Autoindent = wind.GlobalAutoindent
		}
		OnNewWindow(w)
	}
	var eval bool
	if e.A1 == e.A0 {
		eval = false
	} else {
		eval = true
		var dummy int
		r = addr.Eval(true, t, runes.Rng(-1, -1), runes.Rng(t.Q0, t.Q1), e.Arg, e.A0, e.A1, e.Agetc, &eval, &dummy)
		if r.Pos > r.End {
			eval = false
			alog.Printf("addresses out of order\n")
		}
		if !eval {
			e.Jump = false // don't jump if invalid address
		}
	}
	if !eval {
		r.Pos = t.Q0
		r.End = t.Q1
	}
	wind.Textshow(t, r.Pos, r.End, true)
	wind.Winsettag(t.W)
	wind.Seltext = t
	if e.Jump {
		adraw.Display.MoveCursor(t.Fr.PointOf(t.Fr.P0).Add(draw.Pt(4, adraw.Font.Height-4)))
	}
	return w
}

var Textload func(*wind.Text, int, string, bool) int

var OnNewWindow func(*wind.Window)
