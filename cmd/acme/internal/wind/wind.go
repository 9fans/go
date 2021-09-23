package wind

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"unsafe"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/bufs"
	"9fans.net/go/cmd/acme/internal/file"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

type Window struct {
	lk          sync.Mutex
	Ref         uint32
	Tag         Text
	Body        Text
	R           draw.Rectangle
	IsDir       bool
	IsScratch   bool
	Filemenu    bool
	Dirty       bool
	Autoindent  bool
	Showdel     bool
	ID          int
	Addr        runes.Range
	Limit       runes.Range
	Nomark      bool
	Wrselrange  runes.Range
	Rdselfd     *os.File
	Col         *Column
	Eventtag    uint16
	Eventwait   chan bool
	Events      []byte
	Owner       rune
	Maxlines    int
	Dlp         []*Dirlist
	Putseq      int
	Incl        [][]rune
	reffont     *adraw.RefFont
	Ctllock     sync.Mutex
	Ctlfid      int
	Dumpstr     string
	Dumpdir     string
	dumpid      int
	Utflastqid  int
	Utflastboff int64
	Utflastq    int
	Tagsafe     bool
	Tagexpand   bool
	Taglines    int
	tagtop      draw.Rectangle
	Editoutlk   util.QLock
	External    bool
}

// Text.what

const (
	Columntag = iota
	Rowtag
	Tag
	Body
)

type Dirlist struct {
	R   []rune
	Wid int
}

var GlobalIncref int

// extern var wdir [unknown]C.char /* must use extern because no dimension given */
var GlobalAutoindent bool

var Activewin *Window

var winid int

func Init(w *Window, clone *Window, r draw.Rectangle) {
	w.Tag.W = w
	w.Taglines = 1
	w.Tagexpand = true
	w.Body.W = w
	winid++
	w.ID = winid
	util.Incref(&w.Ref)
	if GlobalIncref != 0 {
		util.Incref(&w.Ref)
	}
	w.Ctlfid = ^0
	w.Utflastqid = -1
	r1 := r

	w.tagtop = r
	w.tagtop.Max.Y = r.Min.Y + adraw.Font.Height
	r1.Max.Y = r1.Min.Y + w.Taglines*adraw.Font.Height

	util.Incref(&adraw.RefFont1.Ref)
	f := fileaddtext(nil, &w.Tag)
	textinit(&w.Tag, f, r1, &adraw.RefFont1, adraw.TagCols[:])
	w.Tag.What = Tag
	// tag is a copy of the contents, not a tracked image
	if clone != nil {
		Textdelete(&w.Tag, 0, w.Tag.Len(), true)
		nc := clone.Tag.Len()
		rp := make([]rune, nc)
		clone.Tag.File.Read(0, rp)
		Textinsert(&w.Tag, 0, rp, true)
		w.Tag.File.ResetLogs()
		Textsetselect(&w.Tag, nc, nc)
	}
	r1 = r
	r1.Min.Y += w.Taglines*adraw.Font.Height + 1
	if r1.Max.Y < r1.Min.Y {
		r1.Max.Y = r1.Min.Y
	}
	f = nil
	var rf *adraw.RefFont
	if clone != nil {
		f = clone.Body.File
		w.Body.Org = clone.Body.Org
		w.IsScratch = clone.IsScratch
		rf = adraw.FindFont(false, false, false, clone.Body.Reffont.F.Name)
	} else {
		rf = adraw.FindFont(false, false, false, "")
	}
	f = fileaddtext(f, &w.Body)
	w.Body.What = Body
	textinit(&w.Body, f, r1, rf, adraw.TextCols[:])
	r1.Min.Y -= 1
	r1.Max.Y = r1.Min.Y + 1
	adraw.Display.ScreenImage.Draw(r1, adraw.TagCols[frame.BORD], nil, draw.ZP)
	Textscrdraw(&w.Body)
	w.R = r
	var br draw.Rectangle
	br.Min = w.Tag.ScrollR.Min
	br.Max.X = br.Min.X + adraw.Button.R.Dx()
	br.Max.Y = br.Min.Y + adraw.Button.R.Dy()
	adraw.Display.ScreenImage.Draw(br, adraw.Button, nil, adraw.Button.R.Min)
	w.Filemenu = true
	w.Maxlines = w.Body.Fr.MaxLines
	w.Autoindent = GlobalAutoindent
	if clone != nil {
		w.Dirty = clone.Dirty
		w.Autoindent = clone.Autoindent
		Textsetselect(&w.Body, clone.Body.Q0, clone.Body.Q1)
		Winsettag(w)
	}
}

/*
 * Draw the appropriate button.
 */
func windrawbutton(w *Window) {
	b := adraw.Button
	if !w.IsDir && !w.IsScratch && (w.Body.File.Mod() || len(w.Body.Cache) != 0) {
		b = adraw.ModButton
	}
	var br draw.Rectangle
	br.Min = w.Tag.ScrollR.Min
	br.Max.X = br.Min.X + b.R.Dx()
	br.Max.Y = br.Min.Y + b.R.Dy()
	adraw.Display.ScreenImage.Draw(br, b, nil, b.R.Min)
}

func Delrunepos(w *Window) int {
	_, i := parsetag(w, 0)
	i += 2
	if i >= w.Tag.Len() {
		return -1
	}
	return i
}

/*
 * Compute number of tag lines required
 * to display entire tag text.
 */
func wintaglines(w *Window, r draw.Rectangle) int {
	if !w.Tagexpand && !w.Showdel {
		return 1
	}
	w.Showdel = false
	w.Tag.Fr.NoRedraw = 1
	Textresize(&w.Tag, r, true)
	w.Tag.Fr.NoRedraw = 0
	w.Tagsafe = false
	var n int

	if !w.Tagexpand {
		// use just as many lines as needed to show the Del
		n = Delrunepos(w)
		if n < 0 {
			return 1
		}
		p := w.Tag.Fr.PointOf(n).Sub(w.Tag.Fr.R.Min)
		return 1 + p.Y/w.Tag.Fr.Font.Height
	}

	// can't use more than we have
	if w.Tag.Fr.NumLines >= w.Tag.Fr.MaxLines {
		return w.Tag.Fr.MaxLines
	}

	// if tag ends with \n, include empty line at end for typing
	n = w.Tag.Fr.NumLines
	if w.Tag.Len() > 0 {
		var rune_ [1]rune
		w.Tag.File.Read(w.Tag.Len()-1, rune_[:])
		if rune_[0] == '\n' {
			n++
		}
	}
	if n == 0 {
		n = 1
	}
	return n
}

func Winresize(w *Window, r draw.Rectangle, safe, keepextra bool) int {
	// tagtop is first line of tag
	w.tagtop = r
	w.tagtop.Max.Y = r.Min.Y + adraw.Font.Height

	r1 := r
	r1.Max.Y = util.Min(r.Max.Y, r1.Min.Y+w.Taglines*adraw.Font.Height)

	// If needed, recompute number of lines in tag.
	if !safe || !w.Tagsafe || !(w.Tag.All == r1) {
		w.Taglines = wintaglines(w, r)
		r1.Max.Y = util.Min(r.Max.Y, r1.Min.Y+w.Taglines*adraw.Font.Height)
	}

	// If needed, resize & redraw tag.
	y := r1.Max.Y
	if !safe || !w.Tagsafe || !(w.Tag.All == r1) {
		Textresize(&w.Tag, r1, true)
		y = w.Tag.Fr.R.Max.Y
		windrawbutton(w)
		w.Tagsafe = true
	}

	// If needed, resize & redraw body.
	r1 = r
	r1.Min.Y = y
	if !safe || !(w.Body.All == r1) {
		oy := y
		if y+1+w.Body.Fr.Font.Height <= r.Max.Y { // room for one line
			r1.Min.Y = y
			r1.Max.Y = y + 1
			adraw.Display.ScreenImage.Draw(r1, adraw.TagCols[frame.BORD], nil, draw.ZP)
			y++
			r1.Min.Y = util.Min(y, r.Max.Y)
			r1.Max.Y = r.Max.Y
		} else {
			r1.Min.Y = y
			r1.Max.Y = y
		}
		y = Textresize(&w.Body, r1, keepextra)
		w.R = r
		w.R.Max.Y = y
		Textscrdraw(&w.Body)
		w.Body.All.Min.Y = oy
	}
	w.Maxlines = util.Min(w.Body.Fr.NumLines, util.Max(w.Maxlines, w.Body.Fr.MaxLines))
	return w.R.Max.Y
}

func Winclean(w *Window, conservative bool) bool {
	if w.IsScratch || w.IsDir { // don't whine if it's a guide file, error window, etc.
		return true
	}
	if !conservative && w.External {
		return true
	}
	if w.Dirty {
		if len(w.Body.File.Name()) != 0 {
			alog.Printf("%s modified\n", string(w.Body.File.Name()))
		} else {
			if w.Body.Len() < 100 { // don't whine if it's too small
				return true
			}
			alog.Printf("unnamed file modified\n")
		}
		w.Dirty = false
		return false
	}
	return true
}

func Winlock(w *Window, owner rune) {
	f := w.Body.File
	for i := 0; i < len(f.Text); i++ {
		Winlock1(f.Text[i].W, owner)
	}
}

func Winlock1(w *Window, owner rune) {
	util.Incref(&w.Ref)
	w.lk.Lock()
	w.Owner = owner
}

func Winunlock(w *Window) {
	/*
	 * subtle: loop runs backwards to avoid tripping over
	 * winclose indirectly editing f->text and freeing f
	 * on the last iteration of the loop.
	 */
	f := w.Body.File
	for i := len(f.Text) - 1; i >= 0; i-- {
		w = f.Text[i].W
		w.Owner = 0
		w.lk.Unlock()
		Winclose(w)
	}
}

func Windirfree(w *Window) {
	w.Dlp = nil
}

var OnWinclose func(*Window)

func Winclose(w *Window) {
	if util.Decref(&w.Ref) == 0 {
		if OnWinclose != nil {
			OnWinclose(w)
		}
		Windirfree(w)
		textclose(&w.Tag)
		textclose(&w.Body)
		if Activewin == w {
			Activewin = nil
		}
	}
}

func windelete(w *Window) {
	c := w.Eventwait
	if c != nil {
		w.Events = nil
		w.Eventwait = nil
		c <- true // wake him up
	}
}

func Winundo(w *Window, isundo bool) {
	w.Utflastqid = -1
	body := &w.Body
	body.File.Undo(isundo, &body.Q0, &body.Q1)
	Textshow(body, body.Q0, body.Q1, true)
	f := body.File
	for i := 0; i < len(f.Text); i++ {
		v := f.Text[i].W
		v.Dirty = (f.Seq() != v.Putseq)
		if v != w {
			v.Body.Q0 = v.Body.Fr.P0 + v.Body.Org
			v.Body.Q1 = v.Body.Fr.P1 + v.Body.Org
		}
	}
	Winsettag(w)
}

func Winsetname(w *Window, name []rune) {
	t := &w.Body
	if runes.Equal(t.File.Name(), name) {
		return
	}
	w.IsScratch = false
	if len(name) >= 6 && runes.Equal([]rune("/guide"), name[len(name)-6:]) {
		w.IsScratch = true
	} else if len(name) >= 7 && runes.Equal([]rune("+Errors"), name[len(name)-7:]) {
		w.IsScratch = true
	}
	t.File.SetName(name)
	for i := 0; i < len(t.File.Text); i++ {
		v := t.File.Text[i].W
		Winsettag(v)
		v.IsScratch = w.IsScratch
	}
}

func Wincleartatg(w *Window) {
	// w must be committed
	n := w.Tag.Len()
	r, i := parsetag(w, 0)
	for ; i < n; i++ {
		if r[i] == '|' {
			break
		}
	}
	if i == n {
		return
	}
	i++
	Textdelete(&w.Tag, i, n, true)
	w.Tag.File.SetMod(false)
	if w.Tag.Q0 > i {
		w.Tag.Q0 = i
	}
	if w.Tag.Q1 > i {
		w.Tag.Q1 = i
	}
	Textsetselect(&w.Tag, w.Tag.Q0, w.Tag.Q1)
}

func parsetag(w *Window, extra int) ([]rune, int) {
	r := make([]rune, w.Tag.Len(), w.Tag.Len()+extra+1)
	w.Tag.File.Read(0, r)

	/*
	 * " |" or "\t|" ends left half of tag
	 * If we find " Del Snarf" in the left half of the tag
	 * (before the pipe), that ends the file name.
	 */
	pipe := runes.Index(r, []rune(" |"))
	p := runes.Index(r, []rune("\t|"))
	if p >= 0 && (pipe < 0 || p < pipe) {
		pipe = p
	}
	p = runes.Index(r, []rune(" Del Snarf"))
	var i int
	if p >= 0 && (pipe < 0 || p < pipe) {
		i = p
	} else {
		for i = 0; i < w.Tag.Len(); i++ {
			if r[i] == ' ' || r[i] == '\t' {
				break
			}
		}
	}
	return r, i
}

func Winsettag(w *Window) {
	f := w.Body.File
	for i := 0; i < len(f.Text); i++ {
		v := f.Text[i].W
		if v.Col.Safe || v.Body.Fr.MaxLines > 0 {
			winsettag1(v)
		}
	}
}

func winsettag1(w *Window) {

	// there are races that get us here with stuff in the tag cache, so we take extra care to sync it
	if len(w.Tag.Cache) != 0 || w.Tag.File.Mod() {
		Wincommit(w, &w.Tag) // check file name; also guarantees we can modify tag contents
	}
	old, ii := parsetag(w, 0)
	if !runes.Equal(old[:ii], w.Body.File.Name()) {
		Textdelete(&w.Tag, 0, ii, true)
		Textinsert(&w.Tag, 0, w.Body.File.Name(), true)
		old = make([]rune, w.Tag.Len())
		w.Tag.File.Read(0, old)
	}

	// compute the text for the whole tag, replacing current only if it differs
	new_ := make([]rune, 0, len(w.Body.File.Name())+100)
	new_ = append(new_, w.Body.File.Name()...)
	new_ = append(new_, []rune(" Del Snarf")...)
	if w.Filemenu {
		if w.Body.Needundo || w.Body.File.CanUndo() || len(w.Body.Cache) != 0 {
			new_ = append(new_, []rune(" Undo")...)
		}
		if w.Body.File.CanRedo() {
			new_ = append(new_, []rune(" Redo")...)
		}
		dirty := len(w.Body.File.Name()) != 0 && (len(w.Body.Cache) != 0 || w.Body.File.Seq() != w.Putseq)
		if !w.IsDir && dirty {
			new_ = append(new_, []rune(" Put")...)
		}
	}
	if w.IsDir {
		new_ = append(new_, []rune(" Get")...)
	}
	new_ = append(new_, []rune(" |")...)
	r := runes.IndexRune(old, '|')
	var k int
	if r >= 0 {
		k = r + 1
	} else {
		k = len(old)
		if w.Body.File.Seq() == 0 {
			new_ = append(new_, []rune(" Look ")...)
		}
	}

	// replace tag if the new one is different
	resize := 0
	var n int
	if !runes.Equal(new_, old[:k]) {
		resize = 1
		n = k
		if n > len(new_) {
			n = len(new_)
		}
		var j int
		for j = 0; j < n; j++ {
			if old[j] != new_[j] {
				break
			}
		}
		q0 := w.Tag.Q0
		q1 := w.Tag.Q1
		Textdelete(&w.Tag, j, k, true)
		Textinsert(&w.Tag, j, new_[j:], true)
		// try to preserve user selection
		r = runes.IndexRune(old, '|')
		if r >= 0 {
			bar := r
			if q0 > bar {
				bar = runes.IndexRune(new_, '|') - bar
				w.Tag.Q0 = q0 + bar
				w.Tag.Q1 = q1 + bar
			}
		}
	}
	w.Tag.File.SetMod(false)
	n = w.Tag.Len() + len(w.Tag.Cache)
	if w.Tag.Q0 > n {
		w.Tag.Q0 = n
	}
	if w.Tag.Q1 > n {
		w.Tag.Q1 = n
	}
	Textsetselect(&w.Tag, w.Tag.Q0, w.Tag.Q1)
	windrawbutton(w)
	if resize != 0 {
		w.Tagsafe = false
		Winresize(w, w.R, true, true)
	}
}

func Wincommit(w *Window, t *Text) {
	Textcommit(t, true)
	f := t.File
	var i int
	if len(f.Text) > 1 {
		for i = 0; i < len(f.Text); i++ {
			Textcommit(f.Text[i], false) // no-op for t
		}
	}
	if t.What == Body {
		return
	}
	r, i := parsetag(w, 0)
	if !runes.Equal(r[:i], w.Body.File.Name()) {
		file.Seq++
		w.Body.File.Mark()
		w.Body.File.SetMod(true)
		w.Dirty = true
		Winsetname(w, r[:i])
		Winsettag(w)
	}
}

func Winaddincl(w *Window, r []rune) {
	a := string(r)
	info, err := os.Stat(a)
	if err != nil {
		if !strings.HasPrefix(a, "/") {
			rs := Dirname(&w.Body, r)
			r = rs
			a = string(r)
			info, err = os.Stat(a)
		}
		if err != nil {
			alog.Printf("%s: %v", a, err)
			return
		}
	}
	if !info.IsDir() {
		alog.Printf("%s: not a directory\n", a)
		return
	}
	w.Incl = append(w.Incl, nil)
	copy(w.Incl[1:], w.Incl)
	w.Incl[0] = runes.Clone(r)
}

func Winctlprint(w *Window, fonts bool) string {
	isdir := 0
	if w.IsDir {
		isdir = 1
	}
	dirty := 0
	if w.Dirty {
		dirty = 1
	}
	base := fmt.Sprintf("%11d %11d %11d %11d %11d ", w.ID, w.Tag.Len(), w.Body.Len(), isdir, dirty)
	if fonts {
		base += fmt.Sprintf("%11d %q %11d ", w.Body.Fr.R.Dx(), w.Body.Reffont.F.Name, w.Body.Fr.MaxTab)
	}
	return base
}

// fbufalloc() guarantees room off end of BUFSIZE
const (
	BUFSIZE   = 8192
	RUNESIZE  = int(unsafe.Sizeof(rune(0)))
	RBUFSIZE  = bufs.Len / runes.RuneSize
	EVENTSIZE = 256
)

func Winevent(w *Window, format string, args ...interface{}) {
	if !w.External {
		return
	}
	if w.Owner == 0 {
		util.Fatal("no window owner")
	}
	b := fmt.Sprintf(format, args...)
	w.Events = append(w.Events, byte(w.Owner))
	w.Events = append(w.Events, b...)
	c := w.Eventwait
	if c != nil {
		w.Eventwait = nil
		c <- true
	}
}
