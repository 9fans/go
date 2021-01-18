package main

const (
	MAXFILES    = 256
	READBUFSIZE = 8192
	NL          = 5
)

const (
	Up = iota
	Down
)

type Section struct {
	nrunes int
	text   []rune
	next   *Section
}

type Rasp struct {
	nrunes int
	sect   *Section
}

const Untagged = 0xFFFF

type Text struct {
	rasp  Rasp
	nwin  int
	front int
	tag   int
	lock  int8
	l     [NL]Flayer
	id    int64
}

type Readbuf struct {
	n    int
	data [READBUFSIZE]uint8
}

type Resource int

const (
	RHost Resource = iota
	RKeyboard
	RMouse
	RPlumb
	RResize
	NRes
)
