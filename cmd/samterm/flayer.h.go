package main

import (
	"image"

	"9fans.net/go/draw/frame"
)

type Vis int

const (
	None Vis = 0 + iota
	Some
	All
)

const (
	Clicktime = 1000
) /* one second */

type Flayer struct {
	f       frame.Frame
	origin  int
	p0      int
	p1      int
	click   uint32
	textfn  func(*Flayer, int) []rune
	text    *Text
	entire  image.Rectangle
	scroll  image.Rectangle
	lastsr  image.Rectangle
	visible Vis
}

func FLMARGIN(l *Flayer) int    { return flscale(l, 4) }
func FLSCROLLWID(l *Flayer) int { return flscale(l, 12) }
func FLGAP(l *Flayer) int       { return flscale(l, 4) }
