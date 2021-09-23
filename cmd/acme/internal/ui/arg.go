// #include <u.h>
// #include <libc.h>
// #include <bio.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <plumb.h>
// #include <libsec.h>
// #include <9pclient.h>
// #include "dat.h"
// #include "fns.h"

package ui

import (
	"fmt"

	"9fans.net/go/cmd/acme/internal/adraw"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/wind"
	"9fans.net/go/draw"
	"9fans.net/go/draw/frame"
)

func Fontx(et, t, argt *wind.Text, _, _ bool, arg []rune) {
	if et == nil || et.W == nil {
		return
	}
	t = &et.W.Body
	var flag []rune
	var file []rune
	// loop condition: *arg is not a blank
	var r []rune
	for {
		a := runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			break
		}
		r = runes.Clone(arg[:len(arg)-len(a)])
		if runes.Equal(r, []rune("fix")) || runes.Equal(r, []rune("var")) {
			flag = r
		} else {
			file = r
		}
		arg = runes.SkipBlank(a)
	}
	Getarg(argt, false, true, &r)
	if r != nil {
		if runes.Equal(r, []rune("fix")) || runes.Equal(r, []rune("var")) {
			flag = r
		} else {
			file = r
		}
	}
	fix := true
	var newfont *adraw.RefFont
	if flag != nil {
		fix = runes.Equal(flag, []rune("fix"))
	} else if file == nil {
		newfont = adraw.FindFont(false, false, false, "")
		if newfont != nil {
			fix = newfont.F.Name == t.Fr.Font.Name
		}
	}
	var aa string
	if file != nil {
		newfont = adraw.FindFont(fix, flag != nil, false, string(file))
	} else {
		newfont = adraw.FindFont(fix, false, false, "")
	}
	if newfont != nil {
		adraw.Display.ScreenImage.Draw(t.W.R, adraw.TextCols[frame.BACK], nil, draw.ZP)
		adraw.CloseFont(t.Reffont)
		t.Reffont = newfont
		t.Fr.Font = newfont.F
		t.Fr.InitTick()
		if t.W.IsDir {
			t.All.Min.X++ // force recolumnation; disgusting!
			for i := 0; i < len(t.W.Dlp); i++ {
				dp := t.W.Dlp[i]
				aa = string(dp.R)
				dp.Wid = newfont.F.StringWidth(aa)
			}
		}
		// avoid shrinking of window due to quantization
		wind.Colgrow(t.W.Col, t.W, -1)
	}
}

func Getarg(argt *wind.Text, doaddr, dofile bool, rp *[]rune) *string {
	*rp = nil
	if argt == nil {
		return nil
	}
	wind.Textcommit(argt, true)
	var e Expand
	var a *string
	if Expand_(argt, argt.Q0, argt.Q1, &e) {
		if len(e.Name) > 0 && dofile {
			if doaddr {
				a = Printarg(argt, e.Q0, e.Q1)
			}
			*rp = e.Name
			return a
		}
	} else {
		e.Q0 = argt.Q0
		e.Q1 = argt.Q1
	}
	n := e.Q1 - e.Q0
	*rp = make([]rune, n)
	argt.File.Read(e.Q0, *rp)
	if doaddr {
		a = Printarg(argt, e.Q0, e.Q1)
	}
	return a
}

func Printarg(argt *wind.Text, q0 int, q1 int) *string {
	if argt.What != wind.Body || argt.File.Name() == nil {
		return nil
	}
	var buf string
	if q0 == q1 {
		buf = fmt.Sprintf("%s:#%d", string(argt.File.Name()), q0)
	} else {
		buf = fmt.Sprintf("%s:#%d,#%d", string(argt.File.Name()), q0, q1)
	}
	return &buf
}
