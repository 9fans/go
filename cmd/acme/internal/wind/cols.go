package wind

import (
	"sort"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

type Column struct {
	R    draw.Rectangle
	Tag  Text
	Row  *Row
	W    []*Window
	Safe bool
}

var Activecol *Column

func colinit(c *Column, r draw.Rectangle) {
	adraw.Display.ScreenImage.Draw(r, adraw.Display.White, nil, draw.ZP)
	c.R = r
	c.W = nil
	t := &c.Tag
	t.W = nil
	t.Col = c
	r1 := r
	r1.Max.Y = r1.Min.Y + adraw.Font.Height
	textinit(t, fileaddtext(nil, t), r1, &adraw.RefFont1, adraw.TagCols[:])
	t.What = Columntag
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += adraw.Border()
	adraw.Display.ScreenImage.Draw(r1, adraw.Display.Black, nil, draw.ZP)
	Textinsert(t, 0, []rune("New Cut Paste Snarf Sort Zerox Delcol "), true)
	Textsetselect(t, t.Len(), t.Len())
	adraw.Display.ScreenImage.Draw(t.ScrollR, adraw.ColButton, nil, adraw.ColButton.R.Min)
	c.Safe = true
}

func Coladd(c *Column, w *Window, clone *Window, y int) *Window {
	var v *Window
	r := c.R
	r.Min.Y = c.Tag.Fr.R.Max.Y + adraw.Border()
	if y < r.Min.Y && len(c.W) > 0 { // steal half of last window by default
		v = c.W[len(c.W)-1]
		y = v.Body.Fr.R.Min.Y + v.Body.Fr.R.Dy()/2
	}
	var i int
	// look for window we'll land on
	for i = 0; i < len(c.W); i++ {
		v = c.W[i]
		if y < v.R.Max.Y {
			break
		}
	}
	buggered := 0
	if len(c.W) > 0 {
		if i < len(c.W) {
			i++ // new window will go after v
		}
		/*
		 * if landing window (v) is too small, grow it first.
		 */
		minht := v.Tag.Fr.Font.Height + adraw.Border() + 1
		j := 0
		for !c.Safe || v.Body.Fr.MaxLines <= 3 || v.Body.All.Dy() <= minht {
			j++
			if j > 10 {
				buggered = 1 // too many windows in column
				break
			}
			Colgrow(c, v, 1)
		}
		var ymax int

		/*
		 * figure out where to split v to make room for w
		 */

		// new window stops where next window begins
		if i < len(c.W) {
			ymax = c.W[i].R.Min.Y - adraw.Border()
		} else {
			ymax = c.R.Max.Y
		}

		// new window must start after v's tag ends
		y = util.Max(y, v.tagtop.Max.Y+adraw.Border())

		// new window must start early enough to end before ymax
		y = util.Min(y, ymax-minht)

		// if y is too small, too many windows in column
		if y < v.tagtop.Max.Y+adraw.Border() {
			buggered = 1
		}

		/*
		 * resize & redraw v
		 */
		r = v.R
		r.Max.Y = ymax
		adraw.Display.ScreenImage.Draw(r, adraw.TextCols[frame.BACK], nil, draw.ZP)
		r1 := r
		y = util.Min(y, ymax-(v.Tag.Fr.Font.Height*v.Taglines+v.Body.Fr.Font.Height+adraw.Border()+1))
		r1.Max.Y = util.Min(y, v.Body.Fr.R.Min.Y+v.Body.Fr.NumLines*v.Body.Fr.Font.Height)
		r1.Min.Y = Winresize(v, r1, false, false)
		r1.Max.Y = r1.Min.Y + adraw.Border()
		adraw.Display.ScreenImage.Draw(r1, adraw.Display.Black, nil, draw.ZP)

		/*
		 * leave r with w's coordinates
		 */
		r.Min.Y = r1.Max.Y
	}
	if w == nil {
		w = new(Window)
		w.Col = c
		adraw.Display.ScreenImage.Draw(r, adraw.TextCols[frame.BACK], nil, draw.ZP)
		Init(w, clone, r)
	} else {
		w.Col = c
		Winresize(w, r, false, true)
	}
	w.Tag.Col = c
	w.Tag.Row = c.Row
	w.Body.Col = c
	w.Body.Row = c.Row
	c.W = append(c.W, nil)
	copy(c.W[i+1:], c.W[i:])
	c.W[i] = w
	c.Safe = true

	// if there were too many windows, redraw the whole column
	if buggered != 0 {
		Colresize(c, c.R)
	}

	return w
}

func Colclose(c *Column, w *Window, dofree bool) *Window {
	// w is locked
	if !c.Safe {
		Colgrow(c, w, 1)
	}
	var i int
	for i = 0; i < len(c.W); i++ {
		if c.W[i] == w {
			goto Found
		}
	}
	util.Fatal("can't find window")
Found:
	r := w.R
	w.Tag.Col = nil
	w.Body.Col = nil
	w.Col = nil
	if dofree {
		windelete(w)
		Winclose(w)
	}
	copy(c.W[i:], c.W[i+1:])
	c.W = c.W[:len(c.W)-1]
	if len(c.W) == 0 {
		adraw.Display.ScreenImage.Draw(r, adraw.Display.White, nil, draw.ZP)
		return nil
	}
	if i == len(c.W) { // extend last window down
		w = c.W[i-1]
		r.Min.Y = w.R.Min.Y
		r.Max.Y = c.R.Max.Y
	} else { // extend next window up
		w = c.W[i]
		r.Max.Y = w.R.Max.Y
	}
	adraw.Display.ScreenImage.Draw(r, adraw.TextCols[frame.BACK], nil, draw.ZP)
	if c.Safe {
		Winresize(w, r, false, true)
	}
	return w
}

func colcloseall(c *Column) {
	if c == Activecol {
		Activecol = nil
	}
	textclose(&c.Tag)
	for i := 0; i < len(c.W); i++ {
		w := c.W[i]
		Winclose(w)
	}
}

func Colresize(c *Column, r draw.Rectangle) {
	r1 := r
	r1.Max.Y = r1.Min.Y + c.Tag.Fr.Font.Height
	Textresize(&c.Tag, r1, true)
	adraw.Display.ScreenImage.Draw(c.Tag.ScrollR, adraw.ColButton, nil, adraw.ColButton.R.Min)
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += adraw.Border()
	adraw.Display.ScreenImage.Draw(r1, adraw.Display.Black, nil, draw.ZP)
	r1.Max.Y = r.Max.Y
	new_ := r.Dy() - len(c.W)*(adraw.Border()+adraw.Font.Height)
	old := c.R.Dy() - len(c.W)*(adraw.Border()+adraw.Font.Height)
	for i := 0; i < len(c.W); i++ {
		w := c.W[i]
		w.Maxlines = 0
		if i == len(c.W)-1 {
			r1.Max.Y = r.Max.Y
		} else {
			r1.Max.Y = r1.Min.Y
			if new_ > 0 && old > 0 && w.R.Dy() > adraw.Border()+adraw.Font.Height {
				r1.Max.Y += (w.R.Dy()-adraw.Border()-adraw.Font.Height)*new_/old + adraw.Border() + adraw.Font.Height
			}
		}
		r1.Max.Y = util.Max(r1.Max.Y, r1.Min.Y+adraw.Border()+adraw.Font.Height)
		r2 := r1
		r2.Max.Y = r2.Min.Y + adraw.Border()
		adraw.Display.ScreenImage.Draw(r2, adraw.Display.Black, nil, draw.ZP)
		r1.Min.Y = r2.Max.Y
		r1.Min.Y = Winresize(w, r1, false, i == len(c.W)-1)
	}
	c.R = r
}

func Colclean(c *Column) bool {
	clean := true
	for i := 0; i < len(c.W); i++ {
		clean = Winclean(c.W[i], true) && clean
	}
	return clean
}

func Colsort(c *Column) {
	if len(c.W) == 0 {
		return
	}
	rp := make([]draw.Rectangle, len(c.W))
	wp := make([]*Window, len(c.W))
	copy(wp, c.W)
	sort.Slice(wp, func(i, j int) bool {
		return runes.Compare(wp[i].Body.File.Name(), wp[j].Body.File.Name()) < 0
	})

	for i := 0; i < len(c.W); i++ {
		rp[i] = wp[i].R
	}
	r := c.R
	r.Min.Y = c.Tag.Fr.R.Max.Y
	adraw.Display.ScreenImage.Draw(r, adraw.TextCols[frame.BACK], nil, draw.ZP)
	y := r.Min.Y
	for i := 0; i < len(c.W); i++ {
		w := wp[i]
		r.Min.Y = y
		if i == len(c.W)-1 {
			r.Max.Y = c.R.Max.Y
		} else {
			r.Max.Y = r.Min.Y + w.R.Dy() + adraw.Border()
		}
		r1 := r
		r1.Max.Y = r1.Min.Y + adraw.Border()
		adraw.Display.ScreenImage.Draw(r1, adraw.Display.Black, nil, draw.ZP)
		r.Min.Y = r1.Max.Y
		y = Winresize(w, r, false, i == len(c.W)-1)
	}
	c.W = wp
}

func Colgrow(c *Column, w *Window, but int) {
	var i int
	for i = 0; i < len(c.W); i++ {
		if c.W[i] == w {
			goto Found
		}
	}
	util.Fatal("can't find window")

Found:
	cr := c.R
	var r draw.Rectangle
	if but < 0 { // make sure window fills its own space properly
		r = w.R
		if i == len(c.W)-1 || !c.Safe {
			r.Max.Y = cr.Max.Y
		} else {
			r.Max.Y = c.W[i+1].R.Min.Y - adraw.Border()
		}
		Winresize(w, r, false, true)
		return
	}
	cr.Min.Y = c.W[0].R.Min.Y
	var v *Window
	if but == 3 { // full size
		if i != 0 {
			v = c.W[0]
			c.W[0] = w
			c.W[i] = v
		}
		adraw.Display.ScreenImage.Draw(cr, adraw.TextCols[frame.BACK], nil, draw.ZP)
		Winresize(w, cr, false, true)
		for i = 1; i < len(c.W); i++ {
			c.W[i].Body.Fr.MaxLines = 0
		}
		c.Safe = false
		return
	}
	// store old #lines for each window
	onl := w.Body.Fr.MaxLines
	nl := make([]int, len(c.W))
	ny := make([]int, len(c.W))
	tot := 0
	var j int
	var l int
	for j = 0; j < len(c.W); j++ {
		l = c.W[j].Taglines - 1 + c.W[j].Body.Fr.MaxLines
		nl[j] = l
		tot += l
	}
	// approximate new #lines for this window
	if but == 2 { // as big as can be
		for i := range nl {
			nl[i] = 0
		}
	} else {
		nnl := util.Min(onl+util.Max(util.Min(5, w.Taglines-1+w.Maxlines), onl/2), tot)
		if nnl < w.Taglines-1+w.Maxlines {
			nnl = (w.Taglines - 1 + w.Maxlines + nnl) / 2
		}
		if nnl == 0 {
			nnl = 2
		}
		dnl := nnl - onl
		// compute new #lines for each window
		for k := 1; k < len(c.W); k++ {
			// prune from later window
			j = i + k
			if j < len(c.W) && nl[j] != 0 {
				l = util.Min(dnl, util.Max(1, nl[j]/2))
				nl[j] -= l
				nl[i] += l
				dnl -= l
			}
			// prune from earlier window
			j = i - k
			if j >= 0 && nl[j] != 0 {
				l = util.Min(dnl, util.Max(1, nl[j]/2))
				nl[j] -= l
				nl[i] += l
				dnl -= l
			}
		}
	}
	// pack everyone above
	y1 := cr.Min.Y
	for j = 0; j < i; j++ {
		v = c.W[j]
		r = v.R
		r.Min.Y = y1
		r.Max.Y = y1 + v.tagtop.Dy()
		if nl[j] != 0 {
			r.Max.Y += 1 + nl[j]*v.Body.Fr.Font.Height
		}
		r.Min.Y = Winresize(v, r, c.Safe, false)
		r.Max.Y += adraw.Border()
		adraw.Display.ScreenImage.Draw(r, adraw.Display.Black, nil, draw.ZP)
		y1 = r.Max.Y
	}
	// scan to see new size of everyone below
	y2 := c.R.Max.Y
	for j = len(c.W) - 1; j > i; j-- {
		v = c.W[j]
		r = v.R
		r.Min.Y = y2 - v.tagtop.Dy()
		if nl[j] != 0 {
			r.Min.Y -= 1 + nl[j]*v.Body.Fr.Font.Height
		}
		r.Min.Y -= adraw.Border()
		ny[j] = r.Min.Y
		y2 = r.Min.Y
	}
	// compute new size of window
	r = w.R
	r.Min.Y = y1
	r.Max.Y = y2
	h := w.Body.Fr.Font.Height
	if r.Dy() < w.tagtop.Dy()+1+h+adraw.Border() {
		r.Max.Y = r.Min.Y + w.tagtop.Dy() + 1 + h + adraw.Border()
	}
	// draw window
	r.Max.Y = Winresize(w, r, c.Safe, true)
	if i < len(c.W)-1 {
		r.Min.Y = r.Max.Y
		r.Max.Y += adraw.Border()
		adraw.Display.ScreenImage.Draw(r, adraw.Display.Black, nil, draw.ZP)
		for j = i + 1; j < len(c.W); j++ {
			ny[j] -= (y2 - r.Max.Y)
		}
	}
	// pack everyone below
	y1 = r.Max.Y
	for j = i + 1; j < len(c.W); j++ {
		v = c.W[j]
		r = v.R
		r.Min.Y = y1
		r.Max.Y = y1 + v.tagtop.Dy()
		if nl[j] != 0 {
			r.Max.Y += 1 + nl[j]*v.Body.Fr.Font.Height
		}
		y1 = Winresize(v, r, c.Safe, j == len(c.W)-1)
		if j < len(c.W)-1 { // no border on last window
			r.Min.Y = y1
			r.Max.Y += adraw.Border()
			adraw.Display.ScreenImage.Draw(r, adraw.Display.Black, nil, draw.ZP)
			y1 = r.Max.Y
		}
	}
	c.Safe = true
}

func Coldragwin1(c *Column, w *Window, but int, op, p draw.Point) {
	var i int

	for i = 0; i < len(c.W); i++ {
		if c.W[i] == w {
			goto Found
		}
	}
	util.Fatal("can't find window")

Found:
	if w.Tagexpand { // force recomputation of window tag size
		w.Taglines = 1
	}
	if util.Abs(p.X-op.X) < 5 && util.Abs(p.Y-op.Y) < 5 {
		Colgrow(c, w, but)
		return
	}
	// is it a flick to the right?
	if util.Abs(p.Y-op.Y) < 10 && p.X > op.X+30 && rowwhichcol(c.Row, p) == c {
		p.X = op.X + w.R.Dx() // yes: toss to next column
	}
	nc := rowwhichcol(c.Row, p)
	if nc != nil && nc != c {
		Colclose(c, w, false)
		Coladd(nc, w, nil, p.Y)
		return
	}
	if i == 0 && len(c.W) == 1 {
		return // can't do it
	}
	if (i > 0 && p.Y < c.W[i-1].R.Min.Y) || (i < len(c.W)-1 && p.Y > w.R.Max.Y) || (i == 0 && p.Y > w.R.Max.Y) {
		// shuffle
		Colclose(c, w, false)
		Coladd(c, w, nil, p.Y)
		return
	}
	if i == 0 {
		return
	}
	v := c.W[i-1]
	if p.Y < v.tagtop.Max.Y {
		p.Y = v.tagtop.Max.Y
	}
	if p.Y > w.R.Max.Y-w.tagtop.Dy()-adraw.Border() {
		p.Y = w.R.Max.Y - w.tagtop.Dy() - adraw.Border()
	}
	r := v.R
	r.Max.Y = p.Y
	if r.Max.Y > v.Body.Fr.R.Min.Y {
		r.Max.Y -= (r.Max.Y - v.Body.Fr.R.Min.Y) % v.Body.Fr.Font.Height
		if v.Body.Fr.R.Min.Y == v.Body.Fr.R.Max.Y {
			r.Max.Y++
		}
	}
	r.Min.Y = Winresize(v, r, c.Safe, false)
	r.Max.Y = r.Min.Y + adraw.Border()
	adraw.Display.ScreenImage.Draw(r, adraw.Display.Black, nil, draw.ZP)
	r.Min.Y = r.Max.Y
	if i == len(c.W)-1 {
		r.Max.Y = c.R.Max.Y
	} else {
		r.Max.Y = c.W[i+1].R.Min.Y - adraw.Border()
	}
	Winresize(w, r, c.Safe, true)
	c.Safe = true
}

func colwhich(c *Column, p draw.Point) *Text {
	if !p.In(c.R) {
		return nil
	}
	if p.In(c.Tag.All) {
		return &c.Tag
	}
	for i := 0; i < len(c.W); i++ {
		w := c.W[i]
		if p.In(w.R) {
			if p.In(w.tagtop) || p.In(w.Tag.All) {
				return &w.Tag
			}
			// exclude partial line at bottom
			if p.X >= w.Body.ScrollR.Max.X && p.Y >= w.Body.Fr.R.Max.Y {
				return nil
			}
			return &w.Body
		}
	}
	return nil
}
