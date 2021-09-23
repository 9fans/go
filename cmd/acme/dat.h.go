package main

import (
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
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

type Elog struct {
	typ int
	q0  int
	nd  int
	r   []rune
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
	w      *wind.Window
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

const XXX = false

// editing

const (
	Inactive = 0 + iota
	Inserting
	Collecting
)

var screen *draw.Image
var keyboardctl *draw.Keyboardctl
var timerpid int
var fsyspid int
var cputype string
var home string
var acmeshell string

var dodollarsigns bool

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
	cnewwindow = make(chan *wind.Window) // TODO bidi
	mouseexit0 chan int
	mouseexit1 chan int
	cexit      = make(chan int)
	cerr       = make(chan []byte)
	cedit      = make(chan int)
	cwarn      = make(chan int, 1)
)

var editoutlk util.QLock // atomic flag

// #define	STACK	65536
