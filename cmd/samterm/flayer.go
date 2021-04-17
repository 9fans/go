package main

import (
	"image"

	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var llist []*Flayer /* front to back */
var nllist int
var nlalloc int
var lDrect image.Rectangle

var maincols [frame.NCOL]*draw.Image
var cmdcols [frame.NCOL]*draw.Image

func flstart(r image.Rectangle) {
	lDrect = r

	/* Main text is yellowish */
	maincols[frame.BACK] = display.AllocImageMix(draw.PaleYellow, draw.White)
	maincols[frame.HIGH], _ = display.AllocImage(image.Rect(0, 0, 1, 1), screen.Pix, true, draw.DarkYellow)
	maincols[frame.BORD], _ = display.AllocImage(image.Rect(0, 0, 2, 2), screen.Pix, true, draw.YellowGreen)
	maincols[frame.TEXT] = display.Black
	maincols[frame.HTEXT] = display.Black

	/* Command text is blueish */
	cmdcols[frame.BACK] = display.AllocImageMix(draw.PaleBlueGreen, draw.White)
	cmdcols[frame.HIGH], _ = display.AllocImage(image.Rect(0, 0, 1, 1), screen.Pix, true, draw.PaleGreyGreen)
	cmdcols[frame.BORD], _ = display.AllocImage(image.Rect(0, 0, 2, 2), screen.Pix, true, draw.PurpleBlue)
	cmdcols[frame.TEXT] = display.Black
	cmdcols[frame.HTEXT] = display.Black
}

func flnew(l *Flayer, fn func(*Flayer, int) []rune, text *Text) {
	l.textfn = fn
	l.text = text
	l.lastsr = draw.ZR
	llinsert(l)
}

func flrect(l *Flayer, r image.Rectangle) image.Rectangle {
	draw.RectClip(&r, lDrect)
	l.entire = r
	l.scroll = r.Inset(FLMARGIN(l))
	l.scroll.Max.X = r.Min.X + FLMARGIN(l) + FLSCROLLWID(l) + (FLGAP(l) - FLMARGIN(l))
	r.Min.X = l.scroll.Max.X
	return r
}

func flinit(l *Flayer, r image.Rectangle, ft *draw.Font, cols []*draw.Image) {
	lldelete(l)
	llinsert(l)
	l.visible = All
	l.p1 = 0
	l.p0 = l.p1
	l.origin = l.p0
	l.f.Display = display // for FLMARGIN
	l.f.Init(flrect(l, r).Inset(FLMARGIN(l)), ft, screen, cols)
	l.f.MaxTab = maxtab * ft.StringWidth("0")
	newvisibilities(true)
	screen.Draw(l.entire, l.f.Cols[frame.BACK], nil, draw.ZP)
	scrdraw(l, 0)
	flborder(l, false)
}

func flclose(l *Flayer) {
	if l.visible == All {
		screen.Draw(l.entire, display.White, nil, draw.ZP)
	} else if l.visible == Some {
		if l.f.B == nil {
			l.f.B, _ = display.AllocImage(l.entire, screen.Pix, false, draw.NoFill)
		}
		if l.f.B != nil {
			l.f.B.Draw(l.entire, display.White, nil, draw.ZP)
			flrefresh(l, l.entire, 0)
		}
	}
	l.f.Clear(true)
	lldelete(l)
	if l.f.B != nil && l.visible != All {
		l.f.B.Free()
	}
	l.textfn = nil
	newvisibilities(true)
}

func flborder(l *Flayer, wide bool) {
	if flprepare(l) {
		l.f.B.Border(l.entire, FLMARGIN(l), l.f.Cols[frame.BACK], draw.ZP)
		w := 1
		if wide {
			w = FLMARGIN(l)
		}
		l.f.B.Border(l.entire, w, l.f.Cols[frame.BORD], draw.ZP)
		if l.visible == Some {
			flrefresh(l, l.entire, 0)
		}
	}
}

func flwhich(p image.Point) *Flayer {
	if p.X == 0 && p.Y == 0 {
		if len(llist) > 0 {
			return llist[0]
		}
		return nil
	}
	for _, l := range llist {
		if p.In(l.entire) {
			return l
		}
	}
	return nil
}

func flupfront(l *Flayer) {
	v := l.visible
	lldelete(l)
	llinsert(l)
	if v != All {
		newvisibilities(false)
	}
}

func newvisibilities(redraw bool) {
	/* if redraw false, we know it's a flupfront, and needn't
	 * redraw anyone becoming partially covered */
	for _, l := range llist {
		l.lastsr = draw.ZR /* make sure scroll bar gets redrawn */
		ov := l.visible
		l.visible = visibility(l)
		V := func(a, b Vis) int { return int(a)<<2 | int(b) }
		switch V(ov, l.visible) {
		case V(Some, None):
			if l.f.B != nil {
				l.f.B.Free()
			}
			fallthrough
		case V(All, None),
			V(All, Some):
			l.f.B = nil
			l.f.Clear(false)

		case V(Some, Some),
			V(None, Some):
			if ov == None || (l.f.B == nil && redraw) {
				flprepare(l)
			}
			if l.f.B != nil && redraw {
				flrefresh(l, l.entire, 0)
				l.f.B.Free()
				l.f.B = nil
				l.f.Clear(false)
			}
			fallthrough
		case V(None, None),
			V(All, All):
			break

		case V(Some, All):
			if l.f.B != nil {
				screen.Draw(l.entire, l.f.B, nil, l.entire.Min)
				l.f.B.Free()
				l.f.B = screen
				break
			}
			fallthrough
		case V(None, All):
			flprepare(l)
		}
		if ov == None && l.visible != None {
			flnewlyvisible(l)
		}
	}
}

func llinsert(l *Flayer) {
	llist = append(llist, nil)
	copy(llist[1:], llist)
	llist[0] = l
}

func lldelete(l *Flayer) {
	for i := range llist {
		if llist[i] == l {
			copy(llist[i:], llist[i+1:])
			llist = llist[:len(llist)-1]
			return
		}
	}
	panic("lldelete")
}

func flinsert(l *Flayer, rp []rune, p0 int) {
	if flprepare(l) {
		l.f.Insert(rp, p0-l.origin)
		scrdraw(l, scrtotal(l))
		if l.visible == Some {
			flrefresh(l, l.entire, 0)
		}
	}
}

func fldelete(l *Flayer, p0 int, p1 int) {
	if flprepare(l) {
		p0 -= l.origin
		if p0 < 0 {
			p0 = 0
		}
		p1 -= l.origin
		if p1 < 0 {
			p1 = 0
		}
		l.f.Delete(p0, p1)
		scrdraw(l, scrtotal(l))
		if l.visible == Some {
			flrefresh(l, l.entire, 0)
		}
	}
}

func flselect(l *Flayer) bool {
	if l.visible != All {
		flupfront(l)
	}
	l.f.Select(mousectl)
	ret := false
	if l.f.P0 == l.f.P1 {
		if mousep.Msec-l.click < Clicktime && l.f.P0+l.origin == l.p0 {
			ret = true
			l.click = 0
		} else {
			l.click = mousep.Msec
		}
	} else {
		l.click = 0
	}
	l.p0 = l.f.P0 + l.origin
	l.p1 = l.f.P1 + l.origin
	return ret
}

func flsetselect(l *Flayer, p0 int, p1 int) {
	l.click = 0
	if l.visible == None || !flprepare(l) {
		l.p0 = p0
		l.p1 = p1
		return
	}
	l.p0 = p0
	l.p1 = p1
	var fp0 int
	var fp1 int
	var ticked bool
	flfp0p1(l, &fp0, &fp1, &ticked)
	if fp0 == l.f.P0 && fp1 == l.f.P1 {
		if l.f.Ticked != ticked {
			l.f.Tick(l.f.PointOf(fp0), ticked)
		}
		return
	}

	if fp1 <= l.f.P0 || fp0 >= l.f.P1 || l.f.P0 == l.f.P1 || fp0 == fp1 {
		/* no overlap or trivial repainting */
		l.f.Drawsel(l.f.PointOf(l.f.P0), l.f.P0, l.f.P1, false)
		if fp0 != fp1 || ticked {
			l.f.Drawsel(l.f.PointOf(fp0), fp0, fp1, true)
		}
		goto Refresh
	}
	/* the current selection and the desired selection overlap and are both non-empty */
	if fp0 < l.f.P0 {
		/* extend selection backwards */
		l.f.Drawsel(l.f.PointOf(fp0), fp0, l.f.P0, true)
	} else if fp0 > l.f.P0 {
		/* trim first part of selection */
		l.f.Drawsel(l.f.PointOf(l.f.P0), l.f.P0, fp0, false)
	}
	if fp1 > l.f.P1 {
		/* extend selection forwards */
		l.f.Drawsel(l.f.PointOf(l.f.P1), l.f.P1, fp1, true)
	} else if fp1 < l.f.P1 {
		/* trim last part of selection */
		l.f.Drawsel(l.f.PointOf(fp1), fp1, l.f.P1, false)
	}

Refresh:
	l.f.P0 = fp0
	l.f.P1 = fp1
	if l.visible == Some {
		flrefresh(l, l.entire, 0)
	}
}

func flfp0p1(l *Flayer, pp0 *int, pp1 *int, ticked *bool) {
	p0 := l.p0 - l.origin
	p1 := l.p1 - l.origin

	*ticked = p0 == p1
	if p0 < 0 {
		*ticked = false
		p0 = 0
	}
	if p1 < 0 {
		p1 = 0
	}
	if p0 > l.f.NumChars {
		p0 = l.f.NumChars
	}
	if p1 > l.f.NumChars {
		*ticked = false
		p1 = l.f.NumChars
	}
	*pp0 = p0
	*pp1 = p1
}

func rscale(r image.Rectangle, old image.Point, new image.Point) image.Rectangle {
	r.Min.X = r.Min.X * new.X / old.X
	r.Min.Y = r.Min.Y * new.Y / old.Y
	r.Max.X = r.Max.X * new.X / old.X
	r.Max.Y = r.Max.Y * new.Y / old.Y
	return r
}

func flresize(dr image.Rectangle) {
	olDrect := lDrect
	lDrect = dr
	move := false
	/* no moving on rio; must repaint */
	if false && dr.Dx() == olDrect.Dx() && dr.Dy() == olDrect.Dy() {
		move = true
	} else {
		screen.Draw(lDrect, display.White, nil, draw.ZP)
	}
	for i := 0; i < nllist; i++ {
		l := llist[i]
		l.lastsr = draw.ZR
		f := &l.f
		var r image.Rectangle
		if move {
			r = l.entire.Sub(olDrect.Min).Add(dr.Min)
		} else {
			r = rscale(l.entire.Sub(olDrect.Min), olDrect.Max.Sub(olDrect.Min), dr.Max.Sub(dr.Min)).Add(dr.Min)
			if l.visible == Some && f.B != nil {
				f.B.Free()
				f.Clear(false)
			}
			f.B = nil
			if l.visible != None {
				f.Clear(false)
			}
		}
		if !draw.RectClip(&r, dr) {
			panic("flresize")
		}
		if r.Max.X-r.Min.X < 100 {
			r.Min.X = dr.Min.X
		}
		if r.Max.X-r.Min.X < 100 {
			r.Max.X = dr.Max.X
		}
		if r.Max.Y-r.Min.Y < 2*FLMARGIN(l)+f.Font.Height {
			r.Min.Y = dr.Min.Y
		}
		if r.Max.Y-r.Min.Y < 2*FLMARGIN(l)+f.Font.Height {
			r.Max.Y = dr.Max.Y
		}
		if !move {
			l.visible = None
		}
		f.SetRects(flrect(l, r).Inset(FLMARGIN(l)), f.B)
		if !move && f.B != nil {
			scrdraw(l, scrtotal(l))
		}
	}
	newvisibilities(true)
}

func flprepare(l *Flayer) bool {
	if l.visible == None {
		return false
	}
	f := &l.f
	if f.B == nil {
		if l.visible == All {
			f.B = screen
		} else {
			f.B, _ = display.AllocImage(l.entire, screen.Pix, false, 0)
			if f.B == nil {
				return false
			}
		}
		f.B.Draw(l.entire, f.Cols[frame.BACK], nil, draw.ZP)
		w := 1
		if l == llist[0] {
			w = FLMARGIN(l)
		}
		f.B.Border(l.entire, w, f.Cols[frame.BORD], draw.ZP)
		n := f.NumChars
		f.Init(f.Entire, f.Font, f.B, nil)
		f.MaxTab = maxtab * f.Font.StringWidth("0")
		rp := l.textfn(l, n)
		f.Insert(rp, 0)
		f.Drawsel(f.PointOf(f.P0), f.P0, f.P1, false)
		var ticked bool
		flfp0p1(l, &f.P0, &f.P1, &ticked)
		if f.P0 != f.P1 || ticked {
			f.Drawsel(f.PointOf(f.P0), f.P0, f.P1, true)
		}
		l.lastsr = draw.ZR
		scrdraw(l, scrtotal(l))
	}
	return true
}

var somevis, someinvis, justvis bool

func visibility(l *Flayer) Vis {
	someinvis = false
	somevis = someinvis
	justvis = true
	flrefresh(l, l.entire, 0)
	justvis = false
	if !somevis {
		return None
	}
	if !someinvis {
		return All
	}
	return Some
}

func flrefresh(l *Flayer, r image.Rectangle, i int) {
Top:
	t := llist[i]
	i++
	if t == l {
		if !justvis {
			screen.Draw(r, l.f.B, nil, r.Min)
		}
		somevis = true
	} else {
		if !draw.RectXRect(t.entire, r) {
			goto Top /* avoid stacking unnecessarily */
		}
		var s image.Rectangle
		if t.entire.Min.X > r.Min.X {
			s = r
			s.Max.X = t.entire.Min.X
			flrefresh(l, s, i)
			r.Min.X = t.entire.Min.X
		}
		if t.entire.Min.Y > r.Min.Y {
			s = r
			s.Max.Y = t.entire.Min.Y
			flrefresh(l, s, i)
			r.Min.Y = t.entire.Min.Y
		}
		if t.entire.Max.X < r.Max.X {
			s = r
			s.Min.X = t.entire.Max.X
			flrefresh(l, s, i)
			r.Max.X = t.entire.Max.X
		}
		if t.entire.Max.Y < r.Max.Y {
			s = r
			s.Min.Y = t.entire.Max.Y
			flrefresh(l, s, i)
			r.Max.Y = t.entire.Max.Y
		}
		/* remaining piece of r is blocked by t; forget about it */
		someinvis = true
	}
}

func flscale(l *Flayer, n int) int {
	if l == nil {
		return n
	}
	return l.f.Display.ScaleSize(n)
}
