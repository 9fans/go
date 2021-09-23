package wind

import (
	"sync"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
)

type Row struct {
	Lk  sync.Mutex
	R   draw.Rectangle
	Tag Text
	Col []*Column
}

var TheRow Row

func RowInit(row *Row, r draw.Rectangle) {
	adraw.Display.ScreenImage.Draw(r, adraw.Display.White, nil, draw.ZP)
	row.R = r
	row.Col = nil
	r1 := r
	r1.Max.Y = r1.Min.Y + adraw.Font.Height
	t := &row.Tag
	textinit(t, fileaddtext(nil, t), r1, adraw.FindFont(false, false, false, ""), adraw.TagCols[:])
	t.What = Rowtag
	t.Row = row
	t.W = nil
	t.Col = nil
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += adraw.Border()
	adraw.Display.ScreenImage.Draw(r1, adraw.Display.Black, nil, draw.ZP)
	Textinsert(t, 0, []rune("Newcol Kill Putall Dump Exit "), true)
	Textsetselect(t, t.Len(), t.Len())
}

func RowAdd(row *Row, c *Column, x int) *Column {
	var d *Column
	r := row.R
	r.Min.Y = row.Tag.Fr.R.Max.Y + adraw.Border()
	if x < r.Min.X && len(row.Col) > 0 { //steal 40% of last column by default
		d = row.Col[len(row.Col)-1]
		x = d.R.Min.X + 3*d.R.Dx()/5
	}
	var i int
	// look for column we'll land on
	for i = 0; i < len(row.Col); i++ {
		d = row.Col[i]
		if x < d.R.Max.X {
			break
		}
	}
	if len(row.Col) > 0 {
		if i < len(row.Col) {
			i++ // new column will go after d
		}
		r = d.R
		if r.Dx() < 100 {
			return nil
		}
		adraw.Display.ScreenImage.Draw(r, adraw.Display.White, nil, draw.ZP)
		r1 := r
		r1.Max.X = util.Min(x-adraw.Border(), r.Max.X-50)
		if r1.Dx() < 50 {
			r1.Max.X = r1.Min.X + 50
		}
		Colresize(d, r1)
		r1.Min.X = r1.Max.X
		r1.Max.X = r1.Min.X + adraw.Border()
		adraw.Display.ScreenImage.Draw(r1, adraw.Display.Black, nil, draw.ZP)
		r.Min.X = r1.Max.X
	}
	if c == nil {
		c = new(Column)
		colinit(c, r)
		util.Incref(&adraw.RefFont1.Ref)
	} else {
		Colresize(c, r)
	}
	c.Row = row
	c.Tag.Row = row
	row.Col = append(row.Col, nil)
	copy(row.Col[i+1:], row.Col[i:])
	row.Col[i] = c
	return c
}

func Rowclose(row *Row, c *Column, dofree bool) {
	var i int
	for i = 0; i < len(row.Col); i++ {
		if row.Col[i] == c {
			goto Found
		}
	}
	util.Fatal("can't find column")
Found:
	r := c.R
	if dofree {
		colcloseall(c)
	}
	copy(row.Col[i:], row.Col[i+1:])
	row.Col = row.Col[:len(row.Col)-1]
	if len(row.Col) == 0 {
		adraw.Display.ScreenImage.Draw(r, adraw.Display.White, nil, draw.ZP)
		return
	}
	if i == len(row.Col) { // extend last column right
		c = row.Col[i-1]
		r.Min.X = c.R.Min.X
		r.Max.X = row.R.Max.X
	} else { // extend next window left
		c = row.Col[i]
		r.Max.X = c.R.Max.X
	}
	adraw.Display.ScreenImage.Draw(r, adraw.Display.White, nil, draw.ZP)
	Colresize(c, r)
}

func Rowresize(row *Row, r draw.Rectangle) {
	or := row.R
	deltax := r.Min.X - or.Min.X
	row.R = r
	r1 := r
	r1.Max.Y = r1.Min.Y + adraw.Font.Height
	Textresize(&row.Tag, r1, true)
	r1.Min.Y = r1.Max.Y
	r1.Max.Y += adraw.Border()
	adraw.Display.ScreenImage.Draw(r1, adraw.Display.Black, nil, draw.ZP)
	r.Min.Y = r1.Max.Y
	r1 = r
	r1.Max.X = r1.Min.X
	for i := 0; i < len(row.Col); i++ {
		c := row.Col[i]
		r1.Min.X = r1.Max.X
		// the test should not be necessary, but guarantee we don't lose a pixel
		if i == len(row.Col)-1 {
			r1.Max.X = r.Max.X
		} else {
			r1.Max.X = (c.R.Max.X-or.Min.X)*r.Dx()/or.Dx() + deltax
		}
		if i > 0 {
			r2 := r1
			r2.Max.X = r2.Min.X + adraw.Border()
			adraw.Display.ScreenImage.Draw(r2, adraw.Display.Black, nil, draw.ZP)
			r1.Min.X = r2.Max.X
		}
		Colresize(c, r1)
	}
}

func Rowclean(row *Row) bool {
	clean := true
	for i := 0; i < len(row.Col); i++ {
		clean = Colclean(row.Col[i]) && clean
	}
	return clean
}

func Rowdragcol1(row *Row, c *Column, op, p draw.Point) {
	var i int
	for i = 0; i < len(row.Col); i++ {
		if row.Col[i] == c {
			goto Found
		}
	}
	util.Fatal("can't find column")

Found:
	if util.Abs(p.X-op.X) < 5 && util.Abs(p.Y-op.Y) < 5 {
		return
	}
	if (i > 0 && p.X < row.Col[i-1].R.Min.X) || (i < len(row.Col)-1 && p.X > c.R.Max.X) {
		// shuffle
		x := c.R.Min.X
		Rowclose(row, c, false)
		if RowAdd(row, c, p.X) == nil { // whoops!
			if RowAdd(row, c, x) == nil { // WHOOPS!
				if RowAdd(row, c, -1) == nil { // shit!
					Rowclose(row, c, true)
					return
				}
			}
		}
		return
	}
	if i == 0 {
		return
	}
	d := row.Col[i-1]
	if p.X < d.R.Min.X+80+adraw.Scrollwid() {
		p.X = d.R.Min.X + 80 + adraw.Scrollwid()
	}
	if p.X > c.R.Max.X-80-adraw.Scrollwid() {
		p.X = c.R.Max.X - 80 - adraw.Scrollwid()
	}
	r := d.R
	r.Max.X = c.R.Max.X
	adraw.Display.ScreenImage.Draw(r, adraw.Display.White, nil, draw.ZP)
	r.Max.X = p.X
	Colresize(d, r)
	r = c.R
	r.Min.X = p.X
	r.Max.X = r.Min.X
	r.Max.X += adraw.Border()
	adraw.Display.ScreenImage.Draw(r, adraw.Display.Black, nil, draw.ZP)
	r.Min.X = r.Max.X
	r.Max.X = c.R.Max.X
	Colresize(c, r)
}

func rowwhichcol(row *Row, p draw.Point) *Column {
	for i := 0; i < len(row.Col); i++ {
		c := row.Col[i]
		if p.In(c.R) {
			return c
		}
	}
	return nil
}

func Rowwhich(row *Row, p draw.Point) *Text {
	if p.In(row.Tag.All) {
		return &row.Tag
	}
	c := rowwhichcol(row, p)
	if c != nil {
		return colwhich(c, p)
	}
	return nil
}

func All(f func(*Window, interface{}), arg interface{}) {
	for _, c := range TheRow.Col {
		for _, w := range c.W {
			f(w, arg)
		}
	}
}
