// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"unicode/utf8"

	"9fans.net/go/draw"
)

func memimagestring(b *Image, p draw.Point, color *Image, cp draw.Point, f *subfont, s []byte) draw.Point {
	var width int
	for ; len(s) > 0; func() { p.X += width; cp.X += width }() {
		c := rune(s[0])
		width = 0
		if c < utf8.RuneSelf {
			s = s[1:]
		} else {
			var w int
			c, w = utf8.DecodeRune(s)
			s = s[w:]
		}
		if int(c) >= f.n {
			continue
		}
		i := &f.info[c]
		width = int(i.Width)
		Draw(b, draw.Rect(p.X+int(i.Left), p.Y+int(i.Top), p.X+int(i.Left)+(f.info[c+1].X-i.X), p.Y+int(i.Bottom)), color, cp, f.bits, draw.Pt(i.X, int(i.Top)), draw.SoverD)
	}
	return p
}

func memsubfontwidth(f *subfont, s []byte) draw.Point {
	p := draw.Pt(0, int(f.height))
	var width int
	for ; len(s) > 0; p.X += width {
		c := rune(s[0])
		width = 0
		if c < utf8.RuneSelf {
			s = s[1:]
		} else {
			var w int
			c, w = utf8.DecodeRune(s)
			s = s[w:]
		}
		if int(c) >= f.n {
			continue
		}
		i := &f.info[c]
		width = int(i.Width)
	}
	return p
}
