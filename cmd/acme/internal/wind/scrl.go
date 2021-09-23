// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package wind

import (
	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/util"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

var scrtmp *draw.Image

func scrpos(r draw.Rectangle, p0 int, p1 int, tot int) draw.Rectangle {
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

func Scrlresize() {
	scrtmp.Free()
	var err error
	scrtmp, err = adraw.Display.AllocImage(draw.Rect(0, 0, 32, adraw.Display.ScreenImage.R.Max.Y), adraw.Display.ScreenImage.Pix, false, draw.NoFill)
	if err != nil {
		util.Fatal("scroll alloc")
	}
}

func Textscrdraw(t *Text) {
	if t.W == nil || t != &t.W.Body {
		return
	}
	if scrtmp == nil {
		Scrlresize()
	}
	r := t.ScrollR
	b := scrtmp
	r1 := r
	r1.Min.X = 0
	r1.Max.X = r.Dx()
	r2 := scrpos(r1, t.Org, t.Org+t.Fr.NumChars, t.Len())
	if !(r2 == t.lastsr) {
		t.lastsr = r2
		b.Draw(r1, t.Fr.Cols[frame.BORD], nil, draw.ZP)
		b.Draw(r2, t.Fr.Cols[frame.BACK], nil, draw.ZP)
		r2.Min.X = r2.Max.X - 1
		b.Draw(r2, t.Fr.Cols[frame.BORD], nil, draw.ZP)
		t.Fr.B.Draw(r, b, nil, draw.Pt(0, r1.Min.Y))
		//flushimage(display, 1); // BUG?
	}
}
