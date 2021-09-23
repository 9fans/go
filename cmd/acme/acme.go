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

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/alog"
	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/regx"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
)

var wdir = "."
var snarffd = -1
var mainpid int
var swapscrollbuttons bool = false
var mtpt string

var mainthread sync.Mutex

const (
	NSnarf = 1000
)

var snarfrune [NSnarf + 1]rune

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
	flag.BoolVar(&wind.GlobalAutoindent, "a", wind.GlobalAutoindent, "autoindent")
	flag.BoolVar(&bartflag, "b", bartflag, "bartflag")
	flag.IntVar(&ncol, "c", ncol, "set number of `columns`")
	flag.StringVar(&adraw.FontNames[0], "f", adraw.FontNames[0], "font")
	flag.StringVar(&adraw.FontNames[1], "F", adraw.FontNames[1], "font")
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
	Ismtpt = ismtpt
	Textload = textload

	cputype = os.Getenv("cputype")
	objtype = os.Getenv("objtype")
	home = os.Getenv("HOME")
	acmeshell = os.Getenv("acmeshell")
	p := os.Getenv("tabstop")
	if p != "" {
		wind.MaxTab, _ = strconv.Atoi(p)
	}
	if wind.MaxTab == 0 {
		wind.MaxTab = 4
	}
	if loadfile != "" {
		rowloadfonts(loadfile)
	}
	os.Setenv("font", adraw.FontNames[0])
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
	d, err := draw.Init(ch, adraw.FontNames[0], "acme", winsize)
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

	adraw.Display = d
	adraw.Font = d.Font
	//assert(font);

	adraw.RefFont1.F = adraw.Font
	adraw.RefFonts[0] = &adraw.RefFont1
	util.Incref(&adraw.RefFont1.Ref) // one to hold up 'font' variable
	util.Incref(&adraw.RefFont1.Ref) // one to hold up reffonts[0]
	adraw.FontCache = make([]*adraw.RefFont, 1)
	adraw.FontCache[0] = &adraw.RefFont1

	adraw.Init()
	// TODO timerinit()
	regx.Init()

	wind.OnWinclose = func(w *wind.Window) {
		xfidlog(w, "del")
	}
	OnNewWindow = func(w *wind.Window) {
		xfidlog(w, "new")
	}

	Textcomplete = textcomplete

	mousectl = adraw.Display.InitMouse()
	if mousectl == nil {
		log.Fatal("can't initialize mouse")
	}
	mouse = &mousectl.Mouse
	keyboardctl = adraw.Display.InitKeyboard()
	if keyboardctl == nil {
		log.Fatal("can't initialize keyboard")
	}
	mainpid = os.Getpid()
	startplumbing()

	fsysinit()

	const WPERCOL = 8
	disk.Init()
	if loadfile == "" || !rowload(&wind.TheRow, &loadfile, true) {
		wind.RowInit(&wind.TheRow, adraw.Display.ScreenImage.Clipr)
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
		var c *wind.Column
		var i int
		for i = 0; i < ncol; i++ {
			c = wind.RowAdd(&wind.TheRow, nil, -1)
			if c == nil && i == 0 {
				util.Fatal("initializing columns")
			}
		}
		c = wind.TheRow.Col[len(wind.TheRow.Col)-1]
		if argc == 0 {
			readfile(c, wdir)
		} else {
			for i = 0; i < argc; i++ {
				j := strings.LastIndex(argv[i], "/")
				if j >= 0 && argv[i][j:] == "/guide" || i/WPERCOL >= len(wind.TheRow.Col) {
					readfile(c, argv[i])
				} else {
					readfile(wind.TheRow.Col[i/WPERCOL], argv[i])
				}
			}
		}
	}
	adraw.Display.Flush()

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

func readfile(c *wind.Column, s string) {
	w := coladdAndMouse(c, nil, nil, -1)
	var rb []rune
	if !strings.HasPrefix(s, "/") {
		rb = []rune(wdir + "/" + s)
	} else {
		rb = []rune(s)
	}
	rs := runes.CleanPath(rb)
	wind.Winsetname(w, rs)
	textload(&w.Body, 0, s, true)
	w.Body.File.SetMod(false)
	w.Dirty = false
	wind.Winsettag(w)
	winresizeAndMouse(w, w.R, false, true)
	wind.Textscrdraw(&w.Body)
	wind.Textsetselect(&w.Tag, w.Tag.Len(), w.Tag.Len())
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
		rowdump(&wind.TheRow, nil)
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
	wind.Typetext = nil
	for {
		var t *wind.Text
		bigUnlock()
		select {
		case <-timerc:
			bigLock()
			timer = nil
			timerc = nil
			t = wind.Typetext
			if t != nil && t.What == wind.Tag {
				wind.Winlock(t.W, 'K')
				wind.Wincommit(t.W, t)
				wind.Winunlock(t.W)
				adraw.Display.Flush()
			}

		case r = <-keyboardctl.C:
			bigLock()
		Loop:
			wind.Typetext = rowtype(&wind.TheRow, r, mouse.Point)
			t = wind.Typetext
			if t != nil && t.Col != nil && (!(r == draw.KeyDown || r == draw.KeyLeft) && !(r == draw.KeyRight)) { // scrolling doesn't change activecol
				wind.Activecol = t.Col
			}
			if t != nil && t.W != nil {
				t.W.Body.File.Curtext = &t.W.Body
			}
			if timer != nil {
				timer.Stop()
				timer = nil
			}
			if t != nil && t.What == wind.Tag {
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
			adraw.Display.Flush()
		}
	}
}

func mousethread() {
	bigLock()
	defer bigUnlock()

	for {
		wind.TheRow.Lk.Lock()
		flushwarnings()
		wind.TheRow.Lk.Unlock()

		adraw.Display.Flush()

		bigUnlock()
		select {
		case <-mousectl.Resize:
			bigLock()
			if err := adraw.Display.Attach(draw.RefNone); err != nil {
				util.Fatal("attach to window: " + err.Error())
			}
			adraw.Display.ScreenImage.Draw(adraw.Display.ScreenImage.R, adraw.Display.White, nil, draw.ZP)
			adraw.Init()
			wind.Scrlresize()
			wind.Rowresize(&wind.TheRow, adraw.Display.ScreenImage.Clipr)
			clearmouse()

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
			wind.TheRow.Lk.Lock()
			t := wind.Rowwhich(&wind.TheRow, m.Point)

			if (t != wind.Mousetext && t != nil && t.W != nil) && (wind.Mousetext == nil || wind.Mousetext.W == nil || t.W.ID != wind.Mousetext.W.ID) {
				xfidlog(t.W, "focus")
			}

			if t != wind.Mousetext && wind.Mousetext != nil && wind.Mousetext.W != nil {
				wind.Winlock(wind.Mousetext.W, 'M')
				wind.Mousetext.Eq0 = ^0
				wind.Wincommit(wind.Mousetext.W, wind.Mousetext)
				wind.Winunlock(wind.Mousetext.W)
			}
			wind.Mousetext = t
			var but int
			var w *wind.Window
			if t == nil {
				goto Continue
			}
			w = t.W
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
			wind.Barttext = t
			if t.What == wind.Body && m.Point.In(t.ScrollR) {
				if but != 0 {
					if swapscrollbuttons {
						if but == 1 {
							but = 3
						} else if but == 3 {
							but = 1
						}
					}
					wind.Winlock(w, 'M')
					t.Eq0 = ^0
					textscroll(t, but)
					wind.Winunlock(w)
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
				wind.Winlock(w, 'M')
				t.Eq0 = ^0
				texttype(t, ch)
				wind.Winunlock(w)
				goto Continue
			}
			if m.Point.In(t.ScrollR) {
				if but != 0 {
					if t.What == wind.Columntag {
						rowdragcol(&wind.TheRow, t.Col, but)
					} else if t.What == wind.Tag {
						coldragwin(t.Col, t.W, but)
						if t.W != nil {
							wind.Barttext = &t.W.Body
						}
					}
					if t.Col != nil {
						wind.Activecol = t.Col
					}
				}
				goto Continue
			}
			if m.Buttons != 0 {
				if w != nil {
					wind.Winlock(w, 'M')
				}
				t.Eq0 = ^0
				if w != nil {
					wind.Wincommit(w, t)
				} else {
					wind.Textcommit(t, true)
				}
				if m.Buttons&1 != 0 {
					textselect(t)
					if w != nil {
						wind.Winsettag(w)
					}
					wind.Argtext = t
					wind.Seltext = t
					if t.Col != nil {
						wind.Activecol = t.Col // button 1 only
					}
					if t.W != nil && t == &t.W.Body {
						wind.Activewin = t.W
					}
				} else if m.Buttons&2 != 0 {
					var argt *wind.Text
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
					wind.Winunlock(w)
				}
				goto Continue
			}
		Continue:
			wind.TheRow.Lk.Unlock()
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
			wind.TheRow.Lk.Lock()
			alog.Printf("%s", errb)
			adraw.Display.Flush()
			wind.TheRow.Lk.Unlock()

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
			wind.TheRow.Lk.Lock()
			t := &wind.TheRow.Tag
			wind.Textcommit(t, true)
			if c == nil {
				p := new(Pid)
				p.pid = pid
				p.msg = w.msg
				p.next = pids
				pids = p
			} else {
				if search(t, c.name) {
					wind.Textdelete(t, t.Q0, t.Q1, true)
					wind.Textsetselect(t, 0, 0)
				}
				if w.msg[0] != 0 {
					warning(c.md, "%s: exit %s\n", string(c.name[:len(c.name)-1]), w.msg)
				}
				adraw.Display.Flush()
			}
			wind.TheRow.Lk.Unlock()
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
			wind.TheRow.Lk.Lock()
			t := &wind.TheRow.Tag
			wind.Textcommit(t, true)
			wind.Textinsert(t, 0, c.name, true)
			wind.Textsetselect(t, 0, 0)
			adraw.Display.Flush()
			wind.TheRow.Lk.Unlock()
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
		wind.Winsettag(w)
		winmousebut(w)
		xfidlog(w, "new")
		bigUnlock()
		cnewwindow <- w
	}
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
		adraw.Display.WriteSnarf(buf)
	}
}

func acmegetsnarf() {
	_, m, err := adraw.Display.ReadSnarf(nil)
	if err != nil {
		return
	}
	buf := make([]byte, m+100)
	n, _, err := adraw.Display.ReadSnarf(buf)
	if n == 0 || err != nil {
		return
	}
	buf = buf[:n]

	r := make([]rune, utf8.RuneCount(buf))
	_, nr, _ := runes.Convert(buf, r, true)
	snarfbuf.Reset()
	snarfbuf.Insert(0, r[:nr])
}

var Ismtpt func(string)bool

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
