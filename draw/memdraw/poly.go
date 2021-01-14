// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>

package memdraw

import "9fans.net/go/draw"

func Poly(dst *Image, vert []draw.Point, end0, end1 draw.End, radius int, src *Image, sp draw.Point, op draw.Op) {
	nvert := len(vert)
	if nvert < 2 {
		return
	}
	d := sp.Sub(vert[0])
	for i := 1; i < nvert; i++ {
		e1 := draw.EndDisc
		e0 := e1
		if i == 1 {
			e0 = end0
		}
		if i == nvert-1 {
			e1 = end1
		}
		Line(dst, vert[i-1], vert[i], e0, e1, radius, src, d.Add(vert[i-1]), op)
	}
}
