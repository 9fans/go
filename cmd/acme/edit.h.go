/*#pragma	varargck	argpos	editerror	1*/

package main

type String struct {
	r []rune
}

type Addr struct {
	typ rune
	u   struct {
		re   *String
		left *Addr
	}
	num  int
	next *Addr
}

type Address struct {
	r Range
	f *File
}

type Cmd struct {
	addr *Addr
	re   *String
	u    struct {
		cmd    *Cmd
		text   *String
		mtaddr *Addr
	}
	next *Cmd
	num  int
	flag bool
	cmdc rune
}

/* extern var cmdtab [unknown]cmdtab */

// #define	INCR	25	/* delta when growing list */

type List struct {
	nalloc int
	nused  int
	u      struct {
		listptr   *[0]byte
		ptr       **[0]byte
		ucharptr  **uint8
		stringptr **String
	}
}

type Defaddr int

const (
	aNo Defaddr = iota
	aDot
	aAll
)
