package main

import (
	"os"
	"sync"

	"9fans.net/go/draw"
	"9fans.net/go/draw/memdraw"
)

const (
	NHASH    = 1 << 5
	HASHMASK = NHASH - 1
)

type Kbdbuf struct {
	r      [256]rune
	ri     int
	wi     int
	stall  int
	alting bool
	k      [10]rune
	nk     int
}

type Mousebuf struct {
	m       [256]draw.Mouse
	last    draw.Mouse
	ri      int
	wi      int
	stall   int
	resized bool
}

type Tagbuf struct {
	t  [256]int
	ri int
	wi int
}

type ClientImpl interface {
	rpc_resizeimg(*Client)
	rpc_resizewindow(*Client, draw.Rectangle)
	rpc_setcursor(*Client, *draw.Cursor, *draw.Cursor2)
	rpc_setlabel(*Client, string)
	rpc_setmouse(*Client, draw.Point)
	rpc_topwin(*Client)
	rpc_bouncemouse(*Client, draw.Mouse)
	rpc_flush(*Client, draw.Rectangle)
}

/* extern var drawlk QLock */

type Client struct {
	rfd     *os.File
	wfdlk   sync.Mutex
	wfd     *os.File
	mbuf    *uint8
	nmbuf   int
	wsysid  string
	dimage  [NHASH]*DImage
	cscreen *CScreen
	refresh *Refresh
	// refrend     Rendez
	readdata    []uint8
	busy        int
	clientid    int
	slot        int
	refreshme   int
	infoid      int
	op          draw.Op
	displaydpi  int
	forcedpi    int
	waste       int
	flushrect   draw.Rectangle
	screenimage *memdraw.Image
	dscreen     *DScreen
	name        []DName
	namevers    int
	impl        ClientImpl
	view        *[0]byte
	eventlk     sync.Mutex
	kbd         Kbdbuf
	mouse       Mousebuf
	kbdtags     Tagbuf
	mousetags   Tagbuf
	mouserect   draw.Rectangle
}

type Refresh struct {
	dimage *DImage
	r      draw.Rectangle
	next   *Refresh
}

type Refx struct {
	client *Client
	dimage *DImage
}

type DName struct {
	name   string
	client *Client
	dimage *DImage
	vers   int
}

type FChar struct {
	minx  int
	maxx  int
	miny  uint8
	maxy  uint8
	left  int8
	width uint8
}

/*
 * Reference counts in DImages:
 *	one per open by original client
 *	one per screen image or fill
 * 	one per image derived from this one by name
 */
type DImage struct {
	id       int
	ref      int
	name     string
	vers     int
	image    *memdraw.Image
	ascent   int
	fchar    []FChar
	dscreen  *DScreen
	fromname *DImage
	next     *DImage
}

type CScreen struct {
	dscreen *DScreen
	next    *CScreen
}

type DScreen struct {
	id     int
	public int
	ref    int
	dimage *DImage
	dfill  *DImage
	screen *memdraw.Screen
	owner  *Client
	next   *DScreen
}

// For the most part, the graphics driver-specific code in files
// like mac-screen.m runs in the graphics library's main thread,
// while the RPC service code in srv.c runs on the RPC service thread.
// The exceptions in each file, which are called by the other,
// are marked with special prefixes: gfx_* indicates code that
// is in srv.c but nonetheless runs on the main graphics thread,
// while rpc_* indicates code that is in, say, mac-screen.m but
// nonetheless runs on the RPC service thread.
//
// The gfx_* and rpc_* calls typically synchronize with the other
// code in the file by acquiring a lock (or running a callback on the
// target thread, which amounts to the same thing).
// To avoid deadlock, callers of those routines must not hold any locks.

// gfx_* routines are called on the graphics thread,
// invoked from graphics driver callbacks to do RPC work.
// No locks are held on entry.

// rpc_* routines are called on the RPC thread,
// invoked by the RPC server code to do graphics work.
// No locks are held on entry.

// rpc_gfxdrawlock and rpc_gfxdrawunlock
// are called around drawing operations to lock and unlock
// access to the graphics display, for systems where the
// individual memdraw operations use the graphics display (X11, not macOS).

// draw* routines are called on the RPC thread,
// invoked by the RPC server to do pixel pushing.
// No locks are held on entry.

// utility routines

/* extern var client0 *Client */ // set in single-client mode
