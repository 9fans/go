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

package util

import (
	"log"
	"sync/atomic"
)

func Min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}

func Max(a int, b int) int {
	if a > b {
		return a
	}
	return b
}

func Fatal(s string) {
	log.Fatalf("acme: %s\n", s)
}

type QLock struct {
	held chan bool
}

func (l *QLock) TryLock() bool {
	if l.held == nil {
		panic("missing held")
	}
	select {
	case l.held <- true:
		return true
	default:
		return false
	}
}

func (l *QLock) Unlock() {
	<-l.held
}

func (l *QLock) Lock() {
	l.held <- true
}

func Incref(p *uint32) { atomic.AddUint32(p, 1) }

func Decref(p *uint32) uint32 {
	return atomic.AddUint32(p, ^uint32(0))
}
