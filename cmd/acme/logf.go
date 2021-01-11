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

// State for global log file.

package main

import (
	"fmt"
	"sync"

	"9fans.net/go/plan9"
)

type Log struct {
	lk    sync.Mutex
	r     sync.Cond
	start int64
	ev    []string
	f     []*Fid
	read  []*Xfid
}

var eventlog Log

func init() {
	eventlog.r.L = &eventlog.lk
}

func xfidlogopen(x *Xfid) {
	eventlog.lk.Lock()
	eventlog.f = append(eventlog.f, x.f)
	x.f.logoff = eventlog.start + int64(len(eventlog.ev))
	eventlog.lk.Unlock()
}

func xfidlogclose(x *Xfid) {
	eventlog.lk.Lock()
	for i := 0; i < len(eventlog.f); i++ {
		if eventlog.f[i] == x.f {
			eventlog.f[i] = eventlog.f[len(eventlog.f)-1]
			eventlog.f = eventlog.f[:len(eventlog.f)-1]
			break
		}
	}
	eventlog.lk.Unlock()
}

func xfidlogread(x *Xfid) {
	eventlog.lk.Lock()
	eventlog.read = append(eventlog.read, x)

	x.flushed = false
	for x.f.logoff >= eventlog.start+int64(len(eventlog.ev)) && !x.flushed {
		big.Unlock()
		eventlog.r.Wait()
		big.Lock()
	}
	var i int

	for i = 0; i < len(eventlog.read); i++ {
		if eventlog.read[i] == x {
			eventlog.read[i] = eventlog.read[len(eventlog.read)-1]
			eventlog.read = eventlog.read[:len(eventlog.read)-1]
			break
		}
	}

	if x.flushed {
		eventlog.lk.Unlock()
		return
	}

	i = int(x.f.logoff - eventlog.start)
	p := eventlog.ev[i]
	x.f.logoff++
	eventlog.lk.Unlock()

	var fc plan9.Fcall
	fc.Data = []byte(p)
	fc.Count = uint32(len(fc.Data))
	respond(x, &fc, "")
}

func xfidlogflush(x *Xfid) {
	eventlog.lk.Lock()
	for i := 0; i < len(eventlog.read); i++ {
		rx := eventlog.read[i]
		if rx.fcall.Tag == x.fcall.Oldtag {
			rx.flushed = true
			eventlog.r.Broadcast()
		}
	}
	eventlog.lk.Unlock()
}

/*
 * add a log entry for op on w.
 * expected calls:
 *
 * op == "new" for each new window
 *	- caller of coladd or makenewwindow responsible for calling
 *		xfidlog after setting window name
 *	- exception: zerox
 *
 * op == "zerox" for new window created via zerox
 *	- called from zeroxx
 *
 * op == "get" for Get executed on window
 *	- called from get
 *
 * op == "put" for Put executed on window
 *	- called from put
 *
 * op == "del" for deleted window
 *	- called from winclose
 */
func xfidlog(w *Window, op string) {
	eventlog.lk.Lock()
	if len(eventlog.ev) >= cap(eventlog.ev) {
		// Remove and free any entries that all readers have read.
		min := eventlog.start + int64(len(eventlog.ev))
		for i := 0; i < len(eventlog.f); i++ {
			if min > eventlog.f[i].logoff {
				min = eventlog.f[i].logoff
			}
		}
		if min > eventlog.start {
			n := int(min - eventlog.start)
			copy(eventlog.ev, eventlog.ev[n:])
			eventlog.ev = eventlog.ev[:len(eventlog.ev)-n]
			eventlog.start += int64(n)
		}

		// Otherwise grow (in append below).
	}

	f := w.body.file
	name := runetobyte(f.name)
	eventlog.ev = append(eventlog.ev, fmt.Sprintf("%d %s %s\n", w.id, op, name))
	eventlog.r.Broadcast()
	eventlog.lk.Unlock()
}
