// +build ignore

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

package main

var ctimer *Channel /* chan(Timer*)[100] */
var timer *Timer

func msec() int {
	return nsec() / 1000000
}

func timerstop(t *Timer) {
	t.next = timer
	timer = t
}

func timercancel(t *Timer) {
	t.cancel = true
}

func timerproc(v *[0]byte) {
	threadsetname("timerproc")
	rfork(RFFDG)
	t := nil
	na := 0
	nt := 0
	old := msec()
	for {
		sleep(10) /* longer sleeps here delay recv on ctimer, but 10ms should not be noticeable */
		new_ := msec()
		dt := new_ - old
		old = new_
		if dt < 0 { /* timer wrapped; go around, losing a tick */
			continue
		}
		var x *Timer
		for i := 0; i < nt; i++ {
			x = t[i]
			x.dt -= dt
			del := false
			if x.cancel != 0 {
				timerstop(x)
				del = true
			} else if x.dt <= 0 {
				/*
				 * avoid possible deadlock if client is
				 * now sending on ctimer
				 */
				if nbsendul(x.c, 0) > 0 {
					del = true
				}
			}
			if del != 0 {
				memmove(&t[i], &t[i+1], (nt-i-1)*sizeof(t[0]))
				nt--
				i--
			}
		}
		if nt == 0 {
			x = recvp(ctimer)
		gotit:
			if nt == na {
				na += 10
				t = realloc(t, na*sizeof(*Timer))
				if t == nil {
					error_("timer realloc failed")
				}
			}
			t[nt] = x
			nt++
			old = msec()
		}
		if nbrecv(ctimer, &x) > 0 {
			goto gotit
		}
	}
}

func timerinit() {
	ctimer = chancreate(sizeof(*Timer), 100)
	chansetname(ctimer, "ctimer")
	proccreate(timerproc, nil, STACK)
}

func timerstart(dt int) *Timer {
	t := timer
	if t != nil {
		timer = timer.next
	} else {
		t = emalloc(sizeof(Timer))
		t.c = chancreate(sizeof(int), 0)
		chansetname(t.c, "tc%p", t.c)
	}
	t.next = nil
	t.dt = dt
	t.cancel = false
	sendp(ctimer, t)
	return t
}
