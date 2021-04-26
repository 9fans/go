package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/regx"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var fontcache []*Reffont
var wdir = "."
var reffonts [2]*Reffont
var snarffd = -1
var mainpid int
var swapscrollbuttons bool = false
var mtpt string

var mainthread sync.Mutex

const (
	NSnarf = 1000
)

var snarfrune [NSnarf + 1]rune

var fontnames = []string{
	"/lib/font/bit/lucsans/euro.8.font",
	"/lib/font/bit/lucm/unicode.9.font",
}

var command *Command

func derror(d *draw.Display, errorstr string) {
	util.Fatal(errorstr)
}

func main() {
	bigLock()
	log.SetFlags(0)
	log.SetPrefix("acme: ")

	ncol := -1
	loadfile := ""
	winsize := ""

	flag.Bool("D", false, "") // ignored
	flag.BoolVar(&globalautoindent, "a", globalautoindent, "autoindent")
	flag.BoolVar(&bartflag, "b", bartflag, "bartflag")
	flag.IntVar(&ncol, "c", ncol, "set number of `columns`")
	flag.StringVar(&fontnames[0], "f", fontnames[0], "font")
	flag.StringVar(&fontnames[1], "F", fontnames[1], "font")
	flag.StringVar(&loadfile, "l", loadfile, "loadfile")
	flag.StringVar(&mtpt, "m", mtpt, "mtpt")
	flag.BoolVar(&swapscrollbuttons, "r", swapscrollbuttons, "swapscrollbuttons")
	flag.StringVar(&winsize, "W", winsize, "set window `size`")
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: acme [options] [files...]\n")
		os.Exit(2)
	}
	flag.Parse()

	alog.Init(func(msg string) { warning(nil, "%s", msg) })

	cputype = os.Getenv("cputype")
	objtype = os.Getenv("objtype")
	home = os.Getenv("HOME")
	acmeshell = os.Getenv("acmeshell")
	p := os.Getenv("tabstop")
	if p != "" {
		maxtab, _ = strconv.Atoi(p)
	}
	if maxtab == 0 {
		maxtab = 4
	}
	if loadfile != "" {
		rowloadfonts(loadfile)
	}
	os.Setenv("font", fontnames[0])
	/*
		snarffd = syscall.Open("/dev/snarf", syscall.O_RDONLY|OCEXEC, 0)
		if(cputype){
			sprint(buf, "/acme/bin/%s", cputype);
			bind(buf, "/bin", MBEFORE);
		}
		bind("/acme/bin", "/bin", MBEFORE);
	*/
	wdir, _ = os.Getwd()

	/*
		if(geninitdraw(nil, derror, fontnames[0], "acme", nil, Refnone) < 0){
			fprint(2, "acme: can't open display: %r\n");
			threadexitsall("geninitdraw");
		}
	*/
	ch := make(chan error)
	d, err := draw.Init(ch, fontnames[0], "acme", winsize)
	if err != nil {
		log.Fatal(err)
	}
	go func() {
		for err := range ch {
			bigLock()
			derror(d, err.Error())
			bigUnlock()
		}
	}()

	display = d
	font = d.Font
	//assert(font);

	reffont.f = font
	reffonts[0] = &reffont
	util.Incref(&reffont.ref) // one to hold up 'font' variable
	util.Incref(&reffont.ref) // one to hold up reffonts[0]
	fontcache = make([]*Reffont, 1)
	fontcache[0] = &reffont

	iconinit()
	// TODO timerinit()
	regx.Init()

	mousectl = display.InitMouse()
	if mousectl == nil {
		log.Fatal("can't initialize mouse")
	}
	mouse = &mousectl.Mouse
	keyboardctl = display.InitKeyboard()
	if keyboardctl == nil {
		log.Fatal("can't initialize keyboard")
	}
	mainpid = os.Getpid()
	startplumbing()

	fsysinit()

	const WPERCOL = 8
	disk.Init()
	if loadfile == "" || !rowload(&row, &loadfile, true) {
		rowinit(&row, display.ScreenImage.Clipr)
		argc := flag.NArg()
		argv := flag.Args()
		if ncol < 0 {
			if argc == 0 {
				ncol = 2
			} else {
				ncol = (argc + (WPERCOL - 1)) / WPERCOL
				if ncol < 2 {
					ncol = 2
				}
			}
		}
		if ncol == 0 {
			ncol = 2
		}
		var c *Column
		var i int
		for i = 0; i < ncol; i++ {
			c = rowadd(&row, nil, -1)
			if c == nil && i == 0 {
				util.Fatal("initializing columns")
			}
		}
		c = row.col[len(row.col)-1]
		if argc == 0 {
			readfile(c, wdir)
		} else {
			for i = 0; i < argc; i++ {
				j := strings.LastIndex(argv[i], "/")
				if j >= 0 && argv[i][j:] == "/guide" || i/WPERCOL >= len(row.col) {
					readfile(c, argv[i])
				} else {
					readfile(row.col[i/WPERCOL], argv[i])
				}
			}
		}
	}
	display.Flush()

	acmeerrorinit()
	go keyboardthread()
	go mousethread()
	go waitthread()
	go xfidallocthread()
	go newwindowthread()
	// threadnotify(shutdown, 1)
	bigUnlock()
	<-cexit
	bigLock()
	killprocs()
	os.Exit(0)
}

func readfile(c *Column, s string) {
	w := coladd(c, nil, nil, -1)
	var rb []rune
	if !strings.HasPrefix(s, "/") {
		rb = []rune(wdir + "/" + s)
	} else {
		rb = []rune(s)
	}
	rs := runes.CleanPath(rb)
	winsetname(w, rs)
	textload(&w.body, 0, s, true)
	w.body.file.SetMod(false)
	w.dirty = false
	winsettag(w)
	winresize(w, w.r, false, true)
	textscrdraw(&w.body)
	textsetselect(&w.tag, w.tag.Len(), w.tag.Len())
	xfidlog(w, "new")
}

var ignotes = []string{
	"sys: write on closed pipe",
	"sys: ttin",
	"sys: ttou",
	"sys: tstp",
}

var oknotes = []string{
	"delete",
	"hangup",
	"kill",
	"exit",
}

var dumping bool

func shutdown(v *[0]byte, msg string) bool {
	for _, ig := range ignotes {
		if strings.HasPrefix(msg, ig) {
			return true
		}
	}

	killprocs()
	if !dumping && msg != "kill" && msg != "exit" {
		dumping = true
		rowdump(&row, nil)
	}
	for _, ok := range oknotes {
		if strings.HasPrefix(msg, ok) {
			os.Exit(0)
		}
	}
	print("acme: %s\n", msg)
	return false
}

/*
void
shutdownthread(void *v)
{
	char *msg;
	Channel *c;

	USED(v);

	threadsetname("shutdown");
	c = threadnotechan();
	while((msg = recvp(c)) != nil)
		shutdown(nil, msg);
}
*/

func killprocs() {
	fsysclose()
	//	if(display)
	//		flushimage(display, 1);

	for c := command; c != nil; c = c.next {
		// TODO postnote(PNGROUP, c.pid, "hangup")
		_ = c
	}
}

var errorfd *os.File
var erroutfd *os.File

func acmeerrorproc() {
	buf := make([]byte, 8192)
	for {
		n, err := errorfd.Read(buf)
		if err != nil {
			break
		}
		s := make([]byte, n)
		copy(s, buf)
		cerr <- s
	}
}

func acmeerrorinit() {
	r, w, err := os.Pipe()
	if err != nil {
		log.Fatal(err)
	}
	errorfd = r
	erroutfd = w
	go acmeerrorproc()
}

/*
void
plumbproc(void *v)
{
	Plumbmsg *m;

	USED(v);
	threadsetname("plumbproc");
	for(;;){
		m = threadplumbrecv(plumbeditfd);
		if(m == nil)
			threadexits(nil);
		sendp(cplumb, m);
	}
}
*/

func keyboardthread() {
	bigLock()
	defer bigUnlock()

	var timerc <-chan time.Time
	var r rune
	var timer *time.Timer
	typetext = nil
	for {
		var t *Text
		bigUnlock()
		select {
		case <-timerc:
			bigLock()
			timer = nil
			timerc = nil
			t = typetext
			if t != nil && t.what == Tag {
				winlock(t.w, 'K')
				wincommit(t.w, t)
				winunlock(t.w)
				display.Flush()
			}

		case r = <-keyboardctl.C:
			bigLock()
		Loop:
			typetext = rowtype(&row, r, mouse.Point)
			t = typetext
			if t != nil && t.col != nil && (!(r == draw.KeyDown || r == draw.KeyLeft) && !(r == draw.KeyRight)) { // scrolling doesn't change activecol
				activecol = t.col
			}
			if t != nil && t.w != nil {
				t.w.body.file.curtext = &t.w.body
			}
			if timer != nil {
				timer.Stop()
				timer = nil
			}
			if t != nil && t.what == Tag {
				timer = time.NewTimer(500 * time.Millisecond)
				timerc = timer.C
			} else {
				timer = nil
				timerc = nil
			}
			select {
			default:
				// non-blocking
			case r = <-keyboardctl.C:
				goto Loop
			}
			display.Flush()
		}
	}
}

func mousethread() {
	bigLock()
	defer bigUnlock()

	for {
		row.lk.Lock()
		flushwarnings()
		row.lk.Unlock()

		display.Flush()

		bigUnlock()
		select {
		case <-mousectl.Resize:
			bigLock()
			if err := display.Attach(draw.RefNone); err != nil {
				util.Fatal("attach to window: " + err.Error())
			}
			display.ScreenImage.Draw(display.ScreenImage.R, display.White, nil, draw.ZP)
			iconinit()
			scrlresize()
			rowresize(&row, display.ScreenImage.Clipr)

		case pm := <-cplumb:
			bigLock()
			if pm.Type == "text" {
				act := pm.LookupAttr("action")
				if act == "" || act == "showfile" {
					plumblook(pm)
				} else if act == "showdata" {
					plumbshow(pm)
				}
			}

		case <-cwarn:
			bigLock()
			// ok

		/*
		 * Make a copy so decisions are consistent; mousectl changes
		 * underfoot.  Can't just receive into m because this introduces
		 * another race; see /sys/src/libdraw/mouse.c.
		 */
		case m := <-mousectl.C:
			bigLock()
			mousectl.Mouse = m
			row.lk.Lock()
			t := rowwhich(&row, m.Point)

			if (t != mousetext && t != nil && t.w != nil) && (mousetext == nil || mousetext.w == nil || t.w.id != mousetext.w.id) {
				xfidlog(t.w, "focus")
			}

			if t != mousetext && mousetext != nil && mousetext.w != nil {
				winlock(mousetext.w, 'M')
				mousetext.eq0 = ^0
				wincommit(mousetext.w, mousetext)
				winunlock(mousetext.w)
			}
			mousetext = t
			var but int
			var w *Window
			if t == nil {
				goto Continue
			}
			w = t.w
			if t == nil || m.Buttons == 0 { // TODO(rsc): just checked t above
				goto Continue
			}
			but = 0
			if m.Buttons == 1 {
				but = 1
			} else if m.Buttons == 2 {
				but = 2
			} else if m.Buttons == 4 {
				but = 3
			}
			barttext = t
			if t.what == Body && m.Point.In(t.scrollr) {
				if but != 0 {
					if swapscrollbuttons {
						if but == 1 {
							but = 3
						} else if but == 3 {
							but = 1
						}
					}
					winlock(w, 'M')
					t.eq0 = ^0
					textscroll(t, but)
					winunlock(w)
				}
				goto Continue
			}
			// scroll buttons, wheels, etc.
			if w != nil && m.Buttons&(8|16) != 0 {
				var ch rune
				if m.Buttons&8 != 0 {
					ch = Kscrolloneup
				} else {
					ch = Kscrollonedown
				}
				winlock(w, 'M')
				t.eq0 = ^0
				texttype(t, ch)
				winunlock(w)
				goto Continue
			}
			if m.Point.In(t.scrollr) {
				if but != 0 {
					if t.what == Columntag {
						rowdragcol(&row, t.col, but)
					} else if t.what == Tag {
						coldragwin(t.col, t.w, but)
						if t.w != nil {
							barttext = &t.w.body
						}
					}
					if t.col != nil {
						activecol = t.col
					}
				}
				goto Continue
			}
			if m.Buttons != 0 {
				if w != nil {
					winlock(w, 'M')
				}
				t.eq0 = ^0
				if w != nil {
					wincommit(w, t)
				} else {
					textcommit(t, true)
				}
				if m.Buttons&1 != 0 {
					textselect(t)
					if w != nil {
						winsettag(w)
					}
					argtext = t
					seltext = t
					if t.col != nil {
						activecol = t.col // button 1 only
					}
					if t.w != nil && t == &t.w.body {
						activewin = t.w
					}
				} else if m.Buttons&2 != 0 {
					var argt *Text
					var q0, q1 int
					if textselect2(t, &q0, &q1, &argt) != 0 {
						execute(t, q0, q1, false, argt)
					}
				} else if m.Buttons&4 != 0 {
					var q0, q1 int
					if textselect3(t, &q0, &q1) {
						look3(t, q0, q1, false)
					}
				}
				if w != nil {
					winunlock(w)
				}
				goto Continue
			}
		Continue:
			row.lk.Unlock()
		}
	}
}

/*
 * There is a race between process exiting and our finding out it was ever created.
 * This structure keeps a list of processes that have exited we haven't heard of.
 */

type Pid struct {
	pid  int
	msg  string
	next *Pid
}

func waitthread() {
	var pids *Pid

	bigLock()
	defer bigUnlock()

	for {
		var c *Command
		bigUnlock()
		select {
		case errb := <-cerr:
			bigLock()
			row.lk.Lock()
			alog.Printf("%s", errb)
			display.Flush()
			row.lk.Unlock()

		case cmd := <-ckill:
			bigLock()
			found := false
			for c = command; c != nil; c = c.next {
				// -1 for blank
				if runes.Equal(c.name[:len(c.name)-1], cmd) {
					/* TODO postnote
					if postnote(PNGROUP, c.pid, "kill") < 0 {
						Printf("kill %S: %r\n", cmd)
					}
					*/
					found = true
				}
			}
			if !found {
				alog.Printf("Kill: no process %s\n", string(cmd))
			}

		case w := <-cwait:
			bigLock()
			pid := w.pid
			var c, lc *Command
			for c = command; c != nil; c = c.next {
				if c.pid == pid {
					if lc != nil {
						lc.next = c.next
					} else {
						command = c.next
					}
					break
				}
				lc = c
			}
			row.lk.Lock()
			t := &row.tag
			textcommit(t, true)
			if c == nil {
				p := new(Pid)
				p.pid = pid
				p.msg = w.msg
				p.next = pids
				pids = p
			} else {
				if search(t, c.name) {
					textdelete(t, t.q0, t.q1, true)
					textsetselect(t, 0, 0)
				}
				if w.msg[0] != 0 {
					warning(c.md, "%s: exit %s\n", string(c.name[:len(c.name)-1]), w.msg)
				}
				display.Flush()
			}
			row.lk.Unlock()
			goto Freecmd

		case c = <-ccommand:
			bigLock()
			// has this command already exited?
			var lastp *Pid
			for p := pids; p != nil; p = p.next {
				if p.pid == c.pid {
					if p.msg[0] != 0 {
						warning(c.md, "%s\n", p.msg)
					}
					if lastp == nil {
						pids = p.next
					} else {
						lastp.next = p.next
					}
					goto Freecmd
				}
				lastp = p
			}
			c.next = command
			command = c
			row.lk.Lock()
			t := &row.tag
			textcommit(t, true)
			textinsert(t, 0, c.name, true)
			textsetselect(t, 0, 0)
			display.Flush()
			row.lk.Unlock()
		}
		continue

	Freecmd:
		if c != nil {
			if c.iseditcmd {
				cedit <- 0
			}
			fsysdelid(c.md)
		}
	}
}

func xfidallocthread() {
	var xfree *Xfid
	for {
		// TODO(rsc): split cxfidalloc into two channels
		select {
		case <-cxfidalloc:
			x := xfree
			if x != nil {
				xfree = x.next
			} else {
				x = new(Xfid)
				x.c = make(chan func(*Xfid))
				x.arg = x
				go xfidctl(x)
			}
			cxfidalloc <- x

		case x := <-cxfidfree:
			x.next = xfree
			xfree = x
		}
	}
}

// this thread, in the main proc, allows fsysproc to get a window made without doing graphics
func newwindowthread() {
	for {
		// only fsysproc is talking to us, so synchronization is trivial
		// TODO(rsc): split cnewwindow into two channels
		<-cnewwindow
		bigLock()
		w := makenewwindow(nil)
		winsettag(w)
		xfidlog(w, "new")
		bigUnlock()
		cnewwindow <- w
	}
}

var nfix int

func rfget(fix, save, setfont bool, name string) *Reffont {
	var r *Reffont
	fixi := 0
	if fix {
		fixi = 1
		if nfix++; nfix > 1 {
			panic("fixi")
		}
	}
	if name == "" {
		name = fontnames[fixi]
		r = reffonts[fixi]
	}
	if r == nil {
		for _, r = range fontcache {
			if r.f.Name == name {
				goto Found
			}
		}
		f, err := display.OpenFont(name)
		if err != nil {
			alog.Printf("can't open font file %s: %v\n", name, err)
			return nil
		}
		r = new(Reffont)
		r.f = f
		fontcache = append(fontcache, r)
	}
Found:
	if save {
		util.Incref(&r.ref)
		if reffonts[fixi] != nil {
			rfclose(reffonts[fixi])
		}
		reffonts[fixi] = r
		if name != fontnames[fixi] {
			fontnames[fixi] = name
		}
	}
	if setfont {
		reffont.f = r.f
		util.Incref(&r.ref)
		rfclose(reffonts[0])
		font = r.f
		reffonts[0] = r
		util.Incref(&r.ref)
		iconinit()
	}
	util.Incref(&r.ref)
	return r
}

func rfclose(r *Reffont) {
	if util.Decref(&r.ref) == 0 {
		for i := range fontcache {
			if fontcache[i] == r {
				copy(fontcache[i:], fontcache[i+1:])
				fontcache = fontcache[:len(fontcache)-1]
				goto Found
			}
		}
		alog.Printf("internal error: can't find font in cache\n")
	Found:
		r.f.Free()
	}
}

var boxcursor = draw.Cursor{
	Point: draw.Point{-7, -7},
	White: [...]uint8{
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xF8, 0x1F, 0xF8, 0x1F, 0xF8, 0x1F,
		0xF8, 0x1F, 0xF8, 0x1F, 0xF8, 0x1F, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF, 0xFF,
	},
	Black: [...]uint8{
		0x00, 0x00, 0x7F, 0xFE, 0x7F, 0xFE, 0x7F, 0xFE,
		0x70, 0x0E, 0x70, 0x0E, 0x70, 0x0E, 0x70, 0x0E,
		0x70, 0x0E, 0x70, 0x0E, 0x70, 0x0E, 0x70, 0x0E,
		0x7F, 0xFE, 0x7F, 0xFE, 0x7F, 0xFE, 0x00, 0x00,
	},
}

var boxcursor2 = draw.Cursor2{
	Point: draw.Point{-15, -15},
	White: [...]uint8{
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xC0, 0x03, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
		0xFF, 0xFF, 0xFF, 0xFF,
	},
	Black: [...]uint8{
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0x00, 0x00, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x3F, 0xFF, 0xFF, 0xFC,
		0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00,
	},
}

func iconinit() {
	if tagcols[frame.BACK] == nil {
		// Blue
		tagcols[frame.BACK] = display.AllocImageMix(draw.PaleBlueGreen, draw.White)
		tagcols[frame.HIGH], _ = display.AllocImage(draw.Rect(0, 0, 1, 1), display.ScreenImage.Pix, true, draw.PaleGreyGreen)
		tagcols[frame.BORD], _ = display.AllocImage(draw.Rect(0, 0, 1, 1), display.ScreenImage.Pix, true, draw.PurpleBlue)
		tagcols[frame.TEXT] = display.Black
		tagcols[frame.HTEXT] = display.Black

		// Yellow
		textcols[frame.BACK] = display.AllocImageMix(draw.PaleYellow, draw.White)
		textcols[frame.HIGH], _ = display.AllocImage(draw.Rect(0, 0, 1, 1), display.ScreenImage.Pix, true, draw.DarkYellow)
		textcols[frame.BORD], _ = display.AllocImage(draw.Rect(0, 0, 1, 1), display.ScreenImage.Pix, true, draw.YellowGreen)
		textcols[frame.TEXT] = display.Black
		textcols[frame.HTEXT] = display.Black
	}

	r := draw.Rect(0, 0, Scrollwid()+ButtonBorder(), font.Height+1)
	if button != nil && r == button.R {
		return
	}

	if button != nil {
		button.Free()
		modbutton.Free()
		colbutton.Free()
	}

	button, _ = display.AllocImage(r, display.ScreenImage.Pix, false, draw.NoFill)
	button.Draw(r, tagcols[frame.BACK], nil, r.Min)
	r.Max.X -= ButtonBorder()
	button.Border(r, ButtonBorder(), tagcols[frame.BORD], draw.ZP)

	r = button.R
	modbutton, _ = display.AllocImage(r, display.ScreenImage.Pix, false, draw.NoFill)
	modbutton.Draw(r, tagcols[frame.BACK], nil, r.Min)
	r.Max.X -= ButtonBorder()
	modbutton.Border(r, ButtonBorder(), tagcols[frame.BORD], draw.ZP)
	r = r.Inset(ButtonBorder())
	tmp, _ := display.AllocImage(draw.Rect(0, 0, 1, 1), display.ScreenImage.Pix, true, draw.MedBlue)
	modbutton.Draw(r, tmp, nil, draw.ZP)
	tmp.Free()

	r = button.R
	colbutton, _ = display.AllocImage(r, display.ScreenImage.Pix, false, draw.PurpleBlue)

	but2col, _ = display.AllocImage(r, display.ScreenImage.Pix, true, 0xAA0000FF)
	but3col, _ = display.AllocImage(r, display.ScreenImage.Pix, true, 0x006600FF)
}

/*
 * /dev/snarf updates when the file is closed, so we must open our own
 * fd here rather than use snarffd
 */

/* rio truncates larges snarf buffers, so this avoids using the
 * service if the string is huge */

const MAXSNARF = 100 * 1024

func appendRune(buf []byte, r rune) []byte {
	n := len(buf)
	for cap(buf)-n < utf8.UTFMax {
		buf = append(buf[:cap(buf)], 0)[:n]
	}
	w := utf8.EncodeRune(buf[n:n+utf8.UTFMax], r)
	return buf[:n+w]
}

func acmeputsnarf() {
	if snarfbuf.Len() == 0 {
		return
	}
	if snarfbuf.Len() > MAXSNARF {
		return
	}

	var buf []byte
	var n int
	for i := 0; i < snarfbuf.Len(); i += n {
		n = snarfbuf.Len() - i
		if n >= NSnarf {
			n = NSnarf
		}
		snarfbuf.Read(i, snarfrune[:n])
		var rbuf [utf8.UTFMax]byte
		for _, r := range snarfrune[:n] {
			w := utf8.EncodeRune(rbuf[:], r)
			buf = append(buf, rbuf[:w]...)
		}
	}
	if len(buf) > 0 {
		display.WriteSnarf(buf)
	}
}

func acmegetsnarf() {
	_, m, err := display.ReadSnarf(nil)
	if err != nil {
		return
	}
	buf := make([]byte, m+100)
	n, _, err := display.ReadSnarf(buf)
	if n == 0 || err != nil {
		return
	}
	buf = buf[:n]

	r := make([]rune, utf8.RuneCount(buf))
	_, nr, _ := runes.Convert(buf, r, true)
	snarfbuf.Reset()
	snarfbuf.Insert(0, r[:nr])
}

func ismtpt(file string) bool {
	if mtpt == "" {
		return false
	}

	// This is not foolproof, but it will stop a lot of them.
	return strings.HasPrefix(file, mtpt) && (len(file) == len(mtpt) || file[len(mtpt)] == '/')
}

const timefmt = "2006/01/02 15:04:05"

var big sync.Mutex
var stk = make([]byte, 1<<20)

func bigLock() {
	big.Lock()
	//n := runtime.Stack(stk, true)
	//print("\n\nbig.Lock:\n", string(stk[:n]))
}

func bigUnlock() {
	//n := runtime.Stack(stk, true)
	//print("\n\nbig.Unlock:\n", string(stk[:n]))
	big.Unlock()
}
