package main

import (
	"os"
	"unsafe"
)

/*
 * BLOCKSIZE is relatively small to keep memory consumption down.
 */

const (
	BLOCKSIZE = 2048
	NDISC     = 5
	NBUFFILES = 3 + 2*NDISC /* plan 9+undo+snarf+NDISC*(transcript+buf) */
	NSUBEXP   = 10

	RUNESIZE = int(unsafe.Sizeof(rune(0)))
	INFINITY = 0x7FFFFFFF
	INCR     = 25
	STRSIZE  = 2 * BLOCKSIZE /* TODO(rsc): Is that 2 a stale RUNESIZE? */
)

type Posn = int /* file position or address */
type Mod int    /* modification number */

type State int

const (
	Clean  State = ' '
	Dirty  State = '\''
	Unread State = '-'
)

type Range struct {
	p1 Posn
	p2 Posn
}

type Rangeset struct {
	p [NSUBEXP]Range
}

type Address struct {
	r Range
	f *File
}

type String struct {
	s []rune // n was len(s), size was cap(s)
}

func (s String) String() string { return string(s.s) }

type List struct {
	type_  int
	nalloc int
	nused  int
	g      struct {
		listp   *[0]byte
		voidp   **[0]byte
		posnp   *Posn
		stringp **String
		filep   **File
	}
}

// #define	listptr		g.listp
// #define	voidpptr	g.voidp
// #define	posnptr		g.posnp
// #define	stringpptr	g.stringp
// #define	filepptr	g.filep

const (
	Blockincr = 256
	Maxblock  = 8 * 1024
	BUFSIZE   = Maxblock
	RBUFSIZE  = BUFSIZE / RUNESIZE
)

/* size from fbufalloc() */

const (
	Null     = '-'
	Delete   = 'd'
	Insert   = 'i'
	Filename = 'f'
	Dot      = 'D'
	Mark     = 'm'
)

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

type File struct {
	b        Buffer
	delta    Buffer
	epsilon  Buffer
	name     String
	info     os.FileInfo
	unread   bool
	seq      int
	cleanseq int
	mod      bool
	rescuing int8
	hiposn   Posn
	dot      Address
	ndot     Address
	tdot     Range
	mark     Range
	rasp     *PosnList
	tag      int
	closeok  bool
	deleted  bool
	prevdot  Range
	prevmark Range
	prevseq  int
	prevmod  bool
}

/*File*		fileaddtext(File*, Text*); */

/*void		filedeltext(File*, Text*); */

/*
 * acme fns
 */

// #define	runemalloc(a)		(Rune*)emalloc((a)*sizeof(Rune))
// #define	runerealloc(a, b)	(Rune*)realloc((a), (b)*sizeof(Rune))
// #define	runemove(a, b, c)	memmove((a), (b), (c)*sizeof(Rune))

/* extern var samname [unknown]Rune */ /* compiler dependent */
/* extern var left [unknown]*Rune */
/* extern var right [unknown]*Rune */

/* extern var RSAM [unknown]C.char */ /* system dependent */
/* extern var SAMTERM [unknown]C.char */
/* extern var HOME [unknown]C.char */
/* extern var TMPDIR [unknown]C.char */
/* extern var SH [unknown]C.char */
/* extern var SHPATH [unknown]C.char */
/* extern var RX [unknown]C.char */
/* extern var RXPATH [unknown]C.char */

/*
 * acme globals
 */
/* extern var seq int */
/* extern var disk *Disk */

/* extern var rsamname *C.char */ /* globals */
/* extern var samterm *C.char */
/* extern var genbuf [unknown]Rune */
/* extern var genc *C.char */
/* extern var io int */
/* extern var patset int */
/* extern var quitok int */
/* extern var addr Address */
/* extern var snarfbuf Buffer */
/* extern var plan9buf Buffer */
/* extern var file List */
/* extern var tempfile List */
/* extern var cmd *File */
/* extern var curfile *File */
/* extern var lastfile *File */
/* extern var modnum Mod */
/* extern var cmdpt Posn */
/* extern var cmdptadv Posn */
/* extern var sel Rangeset */
/* extern var curwd String */
/* extern var cmdstr String */
/* extern var genstr String */
/* extern var lastpat String */
/* extern var lastregexp String */
/* extern var plan9cmd String */
/* extern var downloaded int */
/* extern var eof int */
/* extern var bpipeok int */
/* extern var panicking int */
/* extern var empty [unknown]Rune */
/* extern var termlocked int */
/* extern var outbuffered int */

// #include "mesg.h"
