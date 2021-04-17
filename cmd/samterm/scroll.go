package main

import (
	"image"

	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var scrtmp *draw.Image
var scrback *draw.Image

func scrtemps() {
	if scrtmp != nil {
		return
	}
	var h int
	if !screensize(nil, &h) {
		h = 2048
	}
	scrtmp, _ = display.AllocImage(image.Rect(0, 0, 32, h), screen.Pix, false, 0)
	scrback, _ = display.AllocImage(image.Rect(0, 0, 32, h), screen.Pix, false, 0)
	if scrtmp == nil || scrback == nil {
		panic("scrtemps")
	}
}

func scrpos(r image.Rectangle, p0 int, p1 int, tot int) image.Rectangle {
	q := r
	h := q.Max.Y - q.Min.Y
	if tot == 0 {
		return q
	}
	if tot > 1024*1024 {
		tot >>= 10
		p0 >>= 10
		p1 >>= 10
	}
	if p0 > 0 {
		q.Min.Y += h * p0 / tot
	}
	if p1 < tot {
		q.Max.Y -= h * (tot - p1) / tot
	}
	if q.Max.Y < q.Min.Y+2 {
		if q.Min.Y+2 <= r.Max.Y {
			q.Max.Y = q.Min.Y + 2
		} else {
			q.Min.Y = q.Max.Y - 2
		}
	}
	return q
}

func scrmark(l *Flayer, r image.Rectangle) {
	r.Max.X--
	if draw.RectClip(&r, l.scroll) {
		l.f.B.Draw(r, l.f.Cols[frame.HIGH], nil, draw.ZP)
	}
}

func scrunmark(l *Flayer, r image.Rectangle) {
	if draw.RectClip(&r, l.scroll) {
		l.f.B.Draw(r, scrback, nil, image.Pt(0, r.Min.Y-l.scroll.Min.Y))
	}
}

func scrdraw(l *Flayer, tot int) {
	scrtemps()
	if l.f.B == nil {
		panic("scrdraw")
	}
	r := l.scroll
	r1 := r
	var b *draw.Image
	if l.visible == All {
		b = scrtmp
		r1.Min.X = 0
		r1.Max.X = r.Dx()
	} else {
		b = l.f.B
	}
	r2 := scrpos(r1, l.origin, l.origin+l.f.NumChars, tot)
	if !r2.Eq(l.lastsr) {
		l.lastsr = r2
		b.Draw(r1, l.f.Cols[frame.BORD], nil, draw.ZP)
		b.Draw(r2, l.f.Cols[frame.BACK], nil, r2.Min)
		r2 = r1
		r2.Min.X = r2.Max.X - 1
		b.Draw(r2, l.f.Cols[frame.BORD], nil, draw.ZP)
		if b != l.f.B {
			l.f.B.Draw(r, b, nil, r1.Min)
		}
	}
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func scroll(l *Flayer, but int) {
	in := false
	tot := scrtotal(l)
	s := l.scroll
	x := s.Min.X + FLSCROLLWID(l)/2
	scr := scrpos(l.scroll, l.origin, l.origin+l.f.NumChars, tot)
	r := scr
	y := scr.Min.Y
	my := mousep.Point.Y
	scrback.Draw(image.Rect(0, 0, l.scroll.Dx(), l.scroll.Dy()), l.f.B, nil, l.scroll.Min)
	var p0 int
	for {
		oin := in
		in = abs(x-mousep.Point.X) <= FLSCROLLWID(l)/2
		if oin && !in {
			scrunmark(l, r)
		}
		if in {
			scrmark(l, r)
			oy := y
			my = mousep.Point.Y
			if my < s.Min.Y {
				my = s.Min.Y
			}
			if my >= s.Max.Y {
				my = s.Max.Y
			}
			if !mousep.Point.Eq(image.Pt(x, my)) {
				display.MoveCursor(image.Pt(x, my))
			}
			if but == 1 {
				p0 = l.origin - l.f.CharOf(image.Pt(s.Max.X, my))
				rt := scrpos(l.scroll, p0, p0+l.f.NumChars, tot)
				y = rt.Min.Y
			} else if but == 2 {
				y = my
				if y > s.Max.Y-2 {
					y = s.Max.Y - 2
				}
			} else if but == 3 {
				p0 = l.origin + l.f.CharOf(image.Pt(s.Max.X, my))
				rt := scrpos(l.scroll, p0, p0+l.f.NumChars, tot)
				y = rt.Min.Y
			}
			if y != oy {
				scrunmark(l, r)
				r = scr.Add(image.Pt(0, y-scr.Min.Y))
				scrmark(l, r)
			}
		}
		if button(but) == 0 {
			break
		}
	}
	if in {
		h := s.Max.Y - s.Min.Y
		scrunmark(l, r)
		p0 = 0
		if but == 1 {
			p0 = int(my-s.Min.Y)/l.f.Font.Height + 1
		} else if but == 2 {
			if tot > 1024*1024 {
				p0 = ((tot >> 10) * (y - s.Min.Y) / h) << 10
			} else {
				p0 = tot * (y - s.Min.Y) / h
			}
		} else if but == 3 {
			p0 = l.origin + l.f.CharOf(image.Pt(s.Max.X, my))
			if p0 > tot {
				p0 = tot
			}
		}
		scrorigin(l, but, p0)
	}
}
