/*
 * Window system protocol server.
 */

package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"strconv"
	"time"

	"9fans.net/go/draw"
	"9fans.net/go/draw/drawfcall"
	"9fans.net/go/draw/memdraw"
)

var client0 *Client

var trace int = 1
var srvname string
var afd int
var adir string

func usage() {
	fmt.Fprintf(os.Stderr, "usage: devdraw (don't run directly)\n")
	os.Exit(2)
}

func main() {
	log.SetPrefix("devdraw: ")
	log.SetFlags(0)

	flag.BoolVar(new(bool), "D", false, "ignored")
	flag.BoolVar(new(bool), "f", false, "ignored")
	flag.BoolVar(new(bool), "g", false, "ignored")
	flag.BoolVar(new(bool), "b", false, "ignored")
	flag.StringVar(&srvname, "s", srvname, "service name")
	flag.Usage = usage
	flag.Parse()

	memdraw.Init()
	p := os.Getenv("DEVDRAWTRACE")
	if p != "" {
		trace, _ = strconv.Atoi(p)
	}

	if srvname == "" {
		client0 = new(Client)
		client0.displaydpi = 100

		/*
		 * Move the protocol off stdin/stdout so that
		 * any inadvertent prints don't screw things up.
		 */
		client0.rfd = os.Stdin
		client0.wfd = os.Stdout
		os.Stdin, _ = os.Open("/dev/null")
		os.Stdout, _ = os.Create("/dev/null")
	}

	gfx_main()
}

func gfx_started() {
	if srvname == "" {
		// Legacy mode: serving single client on pipes.
		go serveproc(client0)
		return
	}

	panic("server mode")
	/*
		// Server mode.
		ns := getns()
		if ns == nil {
			sysfatal("out of memory")
		}

		addr := fmt.Sprintf("unix!%s/%s", ns, srvname)
		free(ns)
		if addr == nil {
			sysfatal("out of memory")
		}

		afd = announce(addr, adir)
		if afd < 0 {
			sysfatal("announce %s: %r", addr)
		}

		go listenproc()
	*/
}

/*
func listenproc() {
	for {
		var dir string
		fd := listen(adir, dir)
		if fd < 0 {
			sysfatal("listen: %r")
		}
		c := new(Client)
		if c == nil {
			fmt.Fprintf(os.Stderr, "initdraw: allocating client0: out of memory")
			abort()
		}
		c.displaydpi = 100
		c.rfd = fd
		c.wfd = fd
		go serveproc(c)
	}
}
*/

func serveproc(c *Client) {
	for {
		b, err := drawfcall.ReadMsg(c.rfd)
		if err != nil {
			if err != io.EOF {
				fmt.Fprintf(os.Stderr, "serveproc: cannot read message: %v\n", err)
			}
			break
		}

		var m drawfcall.Msg
		if err := m.Unmarshal(b); err != nil {
			fmt.Fprintf(os.Stderr, "serveproc: cannot convert message: %v\n", err)
			break
		}
		if trace != 0 {
			log.Printf("%v <- %v\n", time.Now().UnixNano()/1000000, &m)
		}
		runmsg(c, &m)
	}

	if c == client0 {
		rpc_shutdown()
		os.Exit(0)
	}
}

func replyerror(c *Client, m *drawfcall.Msg, err error) {
	m.Type = drawfcall.Rerror
	m.Error = err.Error()
	replymsg(c, m)
}

/*
 * Handle a single wsysmsg.
 * Might queue for later (kbd, mouse read)
 */
var runmsg_buf [65536]byte

func runmsg(c *Client, m *drawfcall.Msg) {
	switch m.Type {
	case drawfcall.Tctxt:
		c.wsysid = m.ID
		replymsg(c, m)

	case drawfcall.Tinit:
		i, err := rpc_attach(c, m.Label, m.Winsize)
		if err != nil {
			replyerror(c, m, err)
			break
		}
		println("I", i)
		draw_initdisplaymemimage(c, i)
		replymsg(c, m)

	case drawfcall.Trdmouse:
		c.eventlk.Lock()
		if (c.mousetags.wi+1)%len(c.mousetags.t) == c.mousetags.ri {
			c.eventlk.Unlock()
			replyerror(c, m, fmt.Errorf("too many queued mouse reads"))
			break
		}
		c.mousetags.t[c.mousetags.wi] = int(m.Tag)
		c.mousetags.wi++
		if c.mousetags.wi == len(c.mousetags.t) {
			c.mousetags.wi = 0
		}
		c.mouse.stall = 0
		matchmouse(c)
		c.eventlk.Unlock()

	case drawfcall.Trdkbd, drawfcall.Trdkbd4:
		c.eventlk.Lock()
		if (c.kbdtags.wi+1)%len(c.kbdtags.t) == c.kbdtags.ri {
			c.eventlk.Unlock()
			replyerror(c, m, fmt.Errorf("too many queued keyboard reads"))
			break
		}
		ext := 0
		if m.Type == drawfcall.Trdkbd4 {
			ext = 1
		}
		c.kbdtags.t[c.kbdtags.wi] = int(m.Tag)<<1 | ext
		c.kbdtags.wi++
		if c.kbdtags.wi == len(c.kbdtags.t) {
			c.kbdtags.wi = 0
		}
		c.kbd.stall = 0
		matchkbd(c)
		c.eventlk.Unlock()

	case drawfcall.Tmoveto:
		c.impl.rpc_setmouse(c, m.Mouse.Point)
		replymsg(c, m)

	case drawfcall.Tcursor:
		if m.Arrow {
			c.impl.rpc_setcursor(c, nil, nil)
		} else {
			cur := (*draw.Cursor)(&m.Cursor)
			cur2 := (*draw.Cursor2)(&m.Cursor2)
			*cur2 = draw.ScaleCursor(*cur)
			c.impl.rpc_setcursor(c, cur, cur2)
		}
		replymsg(c, m)

	case drawfcall.Tcursor2:
		if m.Arrow {
			c.impl.rpc_setcursor(c, nil, nil)
		} else {
			c.impl.rpc_setcursor(c, (*draw.Cursor)(&m.Cursor), (*draw.Cursor2)(&m.Cursor2))
		}
		replymsg(c, m)

	case drawfcall.Tbouncemouse:
		c.impl.rpc_bouncemouse(c, draw.Mouse(m.Mouse))
		replymsg(c, m)

	case drawfcall.Tlabel:
		c.impl.rpc_setlabel(c, m.Label)
		replymsg(c, m)

	case drawfcall.Trdsnarf:
		m.Snarf = rpc_getsnarf()
		replymsg(c, m)
		m.Snarf = nil

	case drawfcall.Twrsnarf:
		rpc_putsnarf(m.Snarf)
		replymsg(c, m)

	case drawfcall.Trddraw:
		n := m.Count
		if n > len(runmsg_buf) {
			n = len(runmsg_buf)
		}
		n, err := draw_dataread(c, runmsg_buf[:n])
		if err != nil {
			replyerror(c, m, err)
		} else {
			m.Count = n
			m.Data = runmsg_buf[:n]
			replymsg(c, m)
		}

	case drawfcall.Twrdraw:
		if _, err := draw_datawrite(c, m.Data); err != nil {
			replyerror(c, m, err)
		} else {
			m.Count = len(m.Data)
			replymsg(c, m)
		}

	case drawfcall.Ttop:
		c.impl.rpc_topwin(c)
		replymsg(c, m)

	case drawfcall.Tresize:
		c.impl.rpc_resizewindow(c, m.Rect)
		replymsg(c, m)
	}
}

/*
 * drawfcall.Reply to m.
 */
func replymsg(c *Client, m *drawfcall.Msg) {
	/* T -> R msg */
	if m.Type%2 == 0 {
		m.Type++
	}

	if trace != 0 {
		fmt.Fprintf(os.Stderr, "%d -> %v\n", time.Now().UnixNano()/1000000, m)
	}

	c.wfdlk.Lock()
	if _, err := c.wfd.Write(m.Marshal()); err != nil {
		fmt.Fprintf(os.Stderr, "client write: %v\n", err)
	}
	c.wfdlk.Unlock()
}

/*
 * Match queued kbd reads with queued kbd characters.
 */
func matchkbd(c *Client) {
	if c.kbd.stall != 0 {
		return
	}
	for c.kbd.ri != c.kbd.wi && c.kbdtags.ri != c.kbdtags.wi {
		tag := c.kbdtags.t[c.kbdtags.ri]
		c.kbdtags.ri++
		var m drawfcall.Msg
		m.Type = drawfcall.Rrdkbd
		if tag&1 != 0 {
			m.Type = drawfcall.Rrdkbd4
		}
		m.Tag = uint8(tag >> 1)
		if c.kbdtags.ri == len(c.kbdtags.t) {
			c.kbdtags.ri = 0
		}
		m.Rune = c.kbd.r[c.kbd.ri]
		c.kbd.ri++
		if c.kbd.ri == len(c.kbd.r) {
			c.kbd.ri = 0
		}
		replymsg(c, &m)
	}
}

// matchmouse matches queued mouse reads with queued mouse events.
// It must be called with c->eventlk held.
func matchmouse(c *Client) {
	for c.mouse.ri != c.mouse.wi && c.mousetags.ri != c.mousetags.wi {
		var m drawfcall.Msg
		m.Type = drawfcall.Rrdmouse
		m.Tag = uint8(c.mousetags.t[c.mousetags.ri])
		c.mousetags.ri++
		if c.mousetags.ri == len(c.mousetags.t) {
			c.mousetags.ri = 0
		}
		m.Mouse = drawfcall.Mouse(c.mouse.m[c.mouse.ri])
		m.Resized = c.mouse.resized
		c.mouse.resized = false
		/*
			if(m.resized)
				fmt.Fprintf(os.Stderr, "sending resize\n");
		*/
		c.mouse.ri++
		if c.mouse.ri == len(c.mouse.m) {
			c.mouse.ri = 0
		}
		replymsg(c, &m)
	}
}

func gfx_mouseresized(c *Client) {
	gfx_mousetrack(c, -1, -1, -1, ^uint32(0))
}

func gfx_mousetrack(c *Client, x int, y int, b int, ms uint32) {
	c.eventlk.Lock()
	if x == -1 && y == -1 && b == -1 && ms == ^uint32(0) {
		var copy *draw.Mouse
		// repeat last mouse event for resize
		if c.mouse.ri == 0 {
			copy = &c.mouse.m[len(c.mouse.m)-1]
		} else {
			copy = &c.mouse.m[c.mouse.ri-1]
		}
		x = copy.Point.X
		y = copy.Point.Y
		b = copy.Buttons
		ms = copy.Msec
		c.mouse.resized = true
	}
	if x < c.mouserect.Min.X {
		x = c.mouserect.Min.X
	}
	if x > c.mouserect.Max.X {
		x = c.mouserect.Max.X
	}
	if y < c.mouserect.Min.Y {
		y = c.mouserect.Min.Y
	}
	if y > c.mouserect.Max.Y {
		y = c.mouserect.Max.Y
	}

	// If reader has stopped reading, don't bother.
	// If reader is completely caught up, definitely queue.
	// Otherwise, queue only button change events.
	if c.mouse.stall == 0 {
		if c.mouse.wi == c.mouse.ri || c.mouse.last.Buttons != b {
			m := &c.mouse.last
			m.Point.X = x
			m.Point.Y = y
			m.Buttons = b
			m.Msec = ms

			c.mouse.m[c.mouse.wi] = *m
			c.mouse.wi++
			if c.mouse.wi == len(c.mouse.m) {
				c.mouse.wi = 0
			}
			if c.mouse.wi == c.mouse.ri {
				c.mouse.stall = 1
				c.mouse.ri = 0
				c.mouse.wi = 1
				c.mouse.m[0] = *m
			}
			matchmouse(c)
		}
	}
	c.eventlk.Unlock()
}

// kputc adds ch to the keyboard buffer.
// It must be called with c->eventlk held.
func kputc(c *Client, ch rune) {
	c.kbd.r[c.kbd.wi] = ch
	c.kbd.wi++
	if c.kbd.wi == len(c.kbd.r) {
		c.kbd.wi = 0
	}
	if c.kbd.ri == c.kbd.wi {
		c.kbd.stall = 1
	}
	matchkbd(c)
}

// gfx_abortcompose stops any pending compose sequence,
// because a mouse button has been clicked.
// It is called from the graphics thread with no locks held.
func gfx_abortcompose(c *Client) {
	c.eventlk.Lock()
	if c.kbd.alting {
		c.kbd.alting = false
		c.kbd.nk = 0
	}
	c.eventlk.Unlock()
}

// gfx_keystroke records a single-rune keystroke.
// It is called from the graphics thread with no locks held.
func gfx_keystroke(c *Client, ch rune) {
	c.eventlk.Lock()
	if ch == draw.KeyAlt {
		c.kbd.alting = !c.kbd.alting
		c.kbd.nk = 0
		c.eventlk.Unlock()
		return
	}
	if ch == draw.KeyCmd+'r' {
		if c.forcedpi != 0 {
			c.forcedpi = 0
		} else if c.displaydpi >= 200 {
			c.forcedpi = 100
		} else {
			c.forcedpi = 225
		}
		c.eventlk.Unlock()
		c.impl.rpc_resizeimg(c)
		return
	}
	if !c.kbd.alting {
		kputc(c, ch)
		c.eventlk.Unlock()
		return
	}
	if c.kbd.nk >= len(c.kbd.k) { // should not happen
		c.kbd.nk = 0
	}
	c.kbd.k[c.kbd.nk] = ch
	c.kbd.nk++
	ch = toLatin1(c.kbd.k[:c.kbd.nk])
	if ch > 0 {
		c.kbd.alting = false
		kputc(c, ch)
		c.kbd.nk = 0
		c.eventlk.Unlock()
		return
	}
	if ch == -1 {
		c.kbd.alting = false
		for i := 0; i < c.kbd.nk; i++ {
			kputc(c, c.kbd.k[i])
		}
		c.kbd.nk = 0
		c.eventlk.Unlock()
		return
	}
	// need more input
	c.eventlk.Unlock()
	return
}
