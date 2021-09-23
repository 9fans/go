// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <regexp.h>
// #include <9pclient.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package ui

import (
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/wind"
)

func New(et, t, argt *wind.Text, flag1, flag2 bool, arg []rune) {
	var a []rune
	Getarg(argt, false, true, &a)
	if a != nil {
		New(et, t, nil, flag1, flag2, a)
		if len(arg) == 0 {
			return
		}
	}
	// loop condition: *arg is not a blank
	for ndone := 0; ; ndone++ {
		a = runes.SkipNonBlank(arg)
		if len(a) == len(arg) {
			if ndone == 0 && et.Col != nil {
				w := ColaddAndMouse(et.Col, nil, nil, -1)
				wind.Winsettag(w)
				OnNewWindow(w)
			}
			break
		}
		nf := len(arg) - len(a)
		f := runes.Clone(arg[:nf])
		rs := wind.Dirname(et, f)
		var e Expand
		e.Name = rs
		e.Bname = string(rs)
		e.Jump = true
		Openfile(et, &e)
		arg = runes.SkipBlank(a)
	}
}
