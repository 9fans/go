package main

import (
	"os"
	"sync"
	"unsafe"

	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
	"9fans.net/go/plan9"
	"9fans.net/go/plumb"
)

const (
	Qdir = iota
	Qacme
	Qcons
	Qconsctl
	Qdraw
	Qeditout
	Qindex
	Qlabel
	Qlog
	Qnew
	QWaddr
	QWbody
	QWctl
	QWdata
	QWeditout
	QWerrors
	QWevent
	QWrdsel
	QWwrsel
	QWtag
	QWxdata
	QMAX
)

const Blockincr = 256
const Maxblock = 8 * 1024
const NRange = 10

type Block struct {
	addr int64
	u    struct {
		n    int
		next *Block
	}
}

type Disk struct {
	fd   *os.File
	addr int64
	free [Maxblock/Blockincr + 1]*Block
}

type Buffer struct {
	nc     int
	c      []rune // cnc was len(c), cmax was cap(c)
	cq     int
	cdirty bool
	cbi    int
	bl     []*Block // nbl was len(bl) == cap(bl)
}

type Elog struct {
	typ int
	q0  int
	nd  int
	r   []rune
}

type File struct {
	b         Buffer
	delta     Buffer
	epsilon   Buffer
	elogbuf   *Buffer
	elog      Elog
	name      []rune
	info      os.FileInfo
	sha1      [20]uint8
	unread    bool
	editclean bool
	seq       int
	mod       bool
	curtext   *Text
	text      []*Text
	dumpid    int
}

/* Text.what */

const (
	Columntag = iota
	Rowtag
	Tag
	Body
)

type Text struct {
	file     *File
	fr       frame.Frame
	reffont  *Reffont
	org      int
	q0       int
	q1       int
	what     int
	tabstop  int
	w        *Window
	scrollr  draw.Rectangle
	lastsr   draw.Rectangle
	all      draw.Rectangle
	row      *Row
	col      *Column
	iq1      int
	eq0      int
	cq0      int
	cache    []rune
	nofill   bool
	needundo bool
}

type Window struct {
	lk          sync.Mutex
	ref         uint32
	tag         Text
	body        Text
	r           draw.Rectangle
	isdir       bool
	isscratch   bool
	filemenu    bool
	dirty       bool
	autoindent  bool
	showdel     bool
	id          int
	addr        runes.Range
	limit       runes.Range
	nopen       [QMAX]uint8
	nomark      bool
	wrselrange  runes.Range
	rdselfd     *os.File
	col         *Column
	eventx      *Xfid
	events      []byte
	owner       rune
	maxlines    int
	dlp         []*Dirlist
	putseq      int
	incl        [][]rune
	reffont     *Reffont
	ctllock     sync.Mutex
	ctlfid      int
	dumpstr     string
	dumpdir     string
	dumpid      int
	utflastqid  int
	utflastboff int64
	utflastq    int
	tagsafe     bool
	tagexpand   bool
	taglines    int
	tagtop      draw.Rectangle
	editoutlk   util.QLock
}

type Column struct {
	r    draw.Rectangle
	tag  Text
	row  *Row
	w    []*Window
	safe bool
}

type Row struct {
	lk  sync.Mutex
	r   draw.Rectangle
	tag Text
	col []*Column
}

type Timer struct {
	dt     int
	cancel int
	c      chan int
	next   *Timer
}

type Command struct {
	pid       int
	name      []rune
	text      string
	av        []string
	iseditcmd bool
	md        *Mntdir
	next      *Command
}

type Dirtab struct {
	name string
	typ  uint8
	qid  int
	perm int
}

type Mntdir struct {
	id   int
	ref  int
	dir  []rune
	next *Mntdir
	incl [][]rune
}

type Fid struct {
	fid    int
	busy   bool
	open   bool
	qid    plan9.Qid
	w      *Window
	dir    []Dirtab
	next   *Fid
	mntdir *Mntdir
	rpart  []byte
	logoff int64
}

type Xfid struct {
	arg   interface{}
	fcall *plan9.Fcall
	next  *Xfid
	c     chan func(*Xfid)
	f     *Fid
	// buf     *uint8
	flushed bool
}

type Reffont struct {
	lk  sync.Mutex
	ref uint32
	f   *draw.Font
}

type Rangeset struct {
	r [NRange]runes.Range
}

type Dirlist struct {
	r   []rune
	wid int
}

type Expand struct {
	q0    int
	q1    int
	name  []rune
	bname string
	jump  bool
	arg   interface{}
	agetc func(interface{}, int) rune
	a0    int
	a1    int
}

/* fbufalloc() guarantees room off end of BUFSIZE */
const (
	BUFSIZE   = Maxblock
	RUNESIZE  = int(unsafe.Sizeof(rune(0)))
	RBUFSIZE  = BUFSIZE / runes.RuneSize
	EVENTSIZE = 256
)

func Scrollwid() int    { return display.Scale(12) }
func Scrollgap() int    { return display.Scale(4) }
func Margin() int       { return display.Scale(4) }
func Border() int       { return display.Scale(2) }
func ButtonBorder() int { return display.Scale(2) }

const XXX = false

const (
	Empty    = 0
	Null     = '-'
	Delete   = 'd'
	Insert   = 'i'
	Replace  = 'r'
	Filename = 'f'
)

/* editing */

const (
	Inactive = 0 + iota
	Inserting
	Collecting
)

var globalincref int
var seq int
var maxtab int /* size of a tab, in units of the '0' character */

var display *draw.Display
var screen *draw.Image
var font *draw.Font
var mouse *draw.Mouse
var mousectl *draw.Mousectl
var keyboardctl *draw.Keyboardctl
var reffont Reffont
var modbutton *draw.Image
var colbutton *draw.Image
var button *draw.Image
var but2col *draw.Image
var but3col *draw.Image
var row Row
var timerpid int
var disk *Disk
var seltext *Text
var argtext *Text
var mousetext *Text /* global because Text.close needs to clear it */
var typetext *Text  /* global because Text.close needs to clear it */
var barttext *Text  /* shared between mousetask and keyboardthread */
var bartflag bool
var activewin *Window
var activecol *Column
var nullrect draw.Rectangle
var fsyspid int
var cputype string
var objtype string
var home string
var acmeshell string

/* extern var wdir [unknown]C.char */ /* must use extern because no dimension given */
var globalautoindent bool
var dodollarsigns bool

const (
	Kscrolloneup   = draw.KeyFn | 0x20
	Kscrollonedown = draw.KeyFn | 0x21
)

type Waitmsg struct {
	pid int
	msg string
}

var (
	cplumb     = make(chan *plumb.Message)
	cwait      = make(chan *Waitmsg)
	ccommand   = make(chan *Command)
	ckill      = make(chan []rune)
	cxfidalloc = make(chan *Xfid) // TODO bidi
	cxfidfree  = make(chan *Xfid)
	cnewwindow = make(chan *Window) // TODO bidi
	mouseexit0 chan int
	mouseexit1 chan int
	cexit      = make(chan int)
	cerr       = make(chan []byte)
	cedit      = make(chan int)
	cwarn      = make(chan int, 1)
)

var editoutlk util.QLock // atomic flag

// #define	STACK	65536
