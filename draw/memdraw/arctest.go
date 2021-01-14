// +build ignore

// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import (
	"strconv"
	"syscall"

	"9fans.net/go/draw"
)

func main(argc int, argv []*int8) {
	c := draw.Point{208, 871}
	a := 441
	b := 441
	thick := 0
	sp := draw.Point{0, 0}
	alpha := 51
	phi := 3
	memimageinit()

	x := allocmemimage(draw.Rect(0, 0, 1000, 1000), draw.CMAP8)
	n := strconv.Atoi(argv[1])

	t0 := nsec()
	t0 = nsec()
	t0 = nsec()
	t1 := nsec()
	del := t1 - t0
	t0 = nsec()
	for i := 0; i < n; i++ {
		memarc(x, c, a, b, thick, memblack, sp, alpha, phi, SoverD)
	}
	t1 = nsec()
	print("%lld %lld\n", t1-t0-del, del)
}

/* extern var drawdebug int = 0 */

func rdb() {
}

func iprint(fmt_ *int8, args ...interface{}) int {
	var va va_list
	va_start(va, fmt_)
	var buf [1024]int8
	n := doprint(buf, buf+sizeof(buf), fmt_, va) - buf
	va_end(va)

	syscall.Write(1, buf, n)
	return 1
}
