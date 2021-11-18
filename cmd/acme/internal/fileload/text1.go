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
// #include <complete.h>
// #include "dat.h"
// #include "fns.h"

package fileload

import (
	"crypto/sha1"
	"hash"
	"io"
	"os"
	"sort"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/complete"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
)

var Ismtpt = func(string) bool { return false }

func Textload(t *wind.Text, q0 int, file string, setqid bool) int {
	if len(t.Cache) > 0 || t.Len() != 0 || t.W == nil || t != &t.W.Body {
		util.Fatal("text.load")
	}
	if t.W.IsDir && len(t.File.Name()) == 0 {
		alog.Printf("empty directory name")
		return -1
	}
	if Ismtpt(file) {
		alog.Printf("will not open self mount point %s\n", file)
		return -1
	}
	f, err := os.Open(file)
	if err != nil {
		alog.Printf("can't open %s: %v\n", file, err)
		return -1
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		alog.Printf("can't fstat %s: %v\n", file, err)
		return -1
	}
	nulls := false
	var h hash.Hash
	var rp []rune
	var i int
	var n int
	var q1 int
	if info.IsDir() {
		// this is checked in get() but it's possible the file changed underfoot
		if len(t.File.Text) > 1 {
			alog.Printf("%s is a directory; can't read with multiple windows on it\n", file)
			return -1
		}
		t.W.IsDir = true
		t.W.Filemenu = false
		if len(t.File.Name()) > 0 && t.File.Name()[len(t.File.Name())-1] != '/' {
			rp := make([]rune, len(t.File.Name())+1)
			copy(rp, t.File.Name())
			rp[len(t.File.Name())] = '/'
			wind.Winsetname(t.W, rp)
		}
		var dlp []*wind.Dirlist
		for {
			// TODO(rsc): sort order here should be before /, not after
			// Can let ReadDir(-1) do it.
			dirs, err := f.ReadDir(100)
			for _, dir := range dirs {
				dl := new(wind.Dirlist)
				name := dir.Name()
				if dir.IsDir() {
					name += "/"
				}
				dl.R = []rune(name)
				dl.Wid = t.Fr.Font.StringWidth(name)
				dlp = append(dlp, dl)
			}
			if err != nil {
				break
			}
		}
		sort.Slice(dlp, func(i, j int) bool {
			return runes.Compare(dlp[i].R, dlp[j].R) < 0
		})
		t.W.Dlp = dlp
		wind.Textcolumnate(t, dlp)
		q1 = t.Len()
	} else {
		t.W.IsDir = false
		t.W.Filemenu = true
		if q0 == 0 {
			h = sha1.New()
		}
		q1 = q0 + fileload(t.File, q0, f, &nulls, h)
	}
	if setqid {
		if h != nil {
			h.Sum(t.File.SHA1[:0])
			h = nil
		} else {
			t.File.SHA1 = [20]byte{}
		}
		t.File.Info = info
	}
	f.Close()
	rp = bufs.AllocRunes()
	for q := q0; q < q1; q += n {
		n = q1 - q
		if n > bufs.RuneLen {
			n = bufs.RuneLen
		}
		t.File.Read(q, rp[:n])
		if q < t.Org {
			t.Org += n
		} else if q <= t.Org+t.Fr.NumChars {
			t.Fr.Insert(rp[:n], q-t.Org)
		}
		if t.Fr.LastLineFull {
			break
		}
	}
	bufs.FreeRunes(rp)
	for i = 0; i < len(t.File.Text); i++ {
		u := t.File.Text[i]
		if u != t {
			if u.Org > u.Len() { // will be 0 because of reset(), but safety first
				u.Org = 0
			}
			wind.Textresize(u, u.All, true)
			wind.Textbacknl(u, u.Org, 0) // go to beginning of line
		}
		wind.Textsetselect(u, q0, q0)
	}
	if nulls {
		alog.Printf("%s: NUL bytes elided\n", file)
	}
	return q1 - q0
}

func Textcomplete(t *wind.Text) []rune {
	// control-f: filename completion; works back to white space or /
	if t.Q0 < t.Len() && t.RuneAt(t.Q0) > ' ' { // must be at end of word
		return nil
	}
	nstr := wind.Textfilewidth(t, t.Q0, true)
	str := make([]rune, 0, nstr)
	npath := wind.Textfilewidth(t, t.Q0-nstr, false)
	path_ := make([]rune, 0, npath)

	for q := t.Q0 - nstr; q < t.Q0; q++ {
		str = append(str, t.RuneAt(q))
	}
	for q := t.Q0 - nstr - npath; q < t.Q0-nstr; q++ {
		path_ = append(path_, t.RuneAt(q))
	}
	var dir []rune
	// is path rooted? if not, we need to make it relative to window path
	if npath > 0 && path_[0] == '/' {
		dir = path_
	} else {
		dir = wind.Dirname(t, nil)
		if len(dir) == 0 {
			dir = []rune{'.'}
		}
		dir = append(dir, '/')
		dir = append(dir, path_...)
		dir = runes.CleanPath(dir)
	}

	c, err := complete.Complete(string(dir), string(str))
	if err != nil {
		alog.Printf("error attempting completion: %v\n", err)
		return nil
	}

	if !c.Progress {
		sep := ""
		if len(dir) > 0 && dir[len(dir)-1] != '/' {
			sep = "/"
		}
		more := ""
		if c.NumMatch == 0 {
			more = ": no matches in:"
		}
		alog.Printf("%s%s%s*%s\n", string(dir), sep, string(str), more)
		for i := 0; i < len(c.Files); i++ {
			alog.Printf(" %s\n", c.Files[i])
		}
	}

	var rp []rune
	if c.Progress {
		rp = []rune(c.Text)
	}

	return rp
}

func fileload(f *wind.File, p0 int, fd *os.File, nulls *bool, h io.Writer) int {
	if f.Seq() > 0 {
		util.Fatal("undo in file.load unimplemented")
	}
	return fileload1(f, p0, fd, nulls, h)
}
