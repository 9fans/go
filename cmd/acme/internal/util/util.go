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
	"sync"
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

var locks struct {
	mu   sync.Mutex
	cond sync.Cond
}

func init() {
	locks.cond.L = &locks.mu
}

type QLock struct {
	held uint32
}

func (l *QLock) TryLock() bool {
	return atomic.CompareAndSwapUint32(&l.held, 0, 1)
}

func (l *QLock) Unlock() {
	v := atomic.SwapUint32(&l.held, 0)
	if v == 0 {
		panic("Unlock of unlocked lock")
	}
	if v > 1 {
		locks.cond.Broadcast()
	}
}

func (l *QLock) Lock() {
	if atomic.AddUint32(&l.held, 1) == 1 {
		return
	}
	locks.mu.Lock()
	defer locks.mu.Unlock()

	for atomic.AddUint32(&l.held, 1) != 1 {
		locks.cond.Wait()
	}
}

func Incref(p *uint32) { atomic.AddUint32(p, 1) }

func Decref(p *uint32) uint32 {
	return atomic.AddUint32(p, ^uint32(0))
}
