package main

import "unicode/utf8"

/* VERSION 1 introduces plumbing
2 increases SNARFSIZE from 4096 to 32000
*/
const VERSION = 2

const (
	TBLOCKSIZE = 512                           /* largest piece of text sent to terminal */
	DATASIZE   = (utf8.UTFMax*TBLOCKSIZE + 30) /* ... including protocol header stuff */
	SNARFSIZE  = 32000                         /* maximum length of exchanged snarf buffer, must fit in 15 bits */
)

/*
 * Messages originating at the terminal
 */
type Tmesg int

const (
	Tversion Tmesg = iota
	Tstartcmdfile
	Tcheck
	Trequest
	Torigin
	Tstartfile
	Tworkfile
	Ttype
	Tcut
	Tpaste
	Tsnarf
	Tstartnewfile
	Twrite
	Tclose
	Tlook
	Tsearch
	Tsend
	Tdclick
	Tstartsnarf
	Tsetsnarf
	Tack
	Texit
	Tplumb
	TMAX
)

/*
 * Messages originating at the host
 */
type Hmesg int

const (
	Hversion Hmesg = iota
	Hbindname
	Hcurrent
	Hnewname
	Hmovname
	Hgrow
	Hcheck0
	Hcheck
	Hunlock
	Hdata
	Horigin
	Hunlockfile
	Hsetdot
	Hgrowdata
	Hmoveto
	Hclean
	Hdirty
	Hcut
	Hsetpat
	Hdelname
	Hclose
	Hsetsnarf
	Hsnarflen
	Hack
	Hexit
	Hplumb
	HMAX
)

type Header struct {
	typ    Tmesg
	count0 uint8
	count1 uint8
	data   [1]uint8
}
