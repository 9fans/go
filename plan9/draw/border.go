package draw

import "image"

func (dst *Image) BorderOp(r image.Rectangle, n int, color *Image, sp image.Point, op Op) {
	if n < 0 {
		r = r.Inset(n)
		sp = sp.Add(image.Pt(n, n))
		n = -n
	}
	dst.DrawOp(image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+n),
		color, nil, sp, op)
	dst.DrawOp(image.Rect(r.Min.X, r.Max.Y-n, r.Max.X, r.Max.Y),
		color, nil, image.Pt(sp.X, sp.Y+r.Dy()-n), op)
	dst.DrawOp(image.Rect(r.Min.X, r.Min.Y+n, r.Min.X+n, r.Max.Y-n),
		color, nil, image.Pt(sp.X, sp.Y+n), op)
	dst.DrawOp(image.Rect(r.Max.X-n, r.Min.Y+n, r.Max.X, r.Max.Y-n),
		color, nil, image.Pt(sp.X+r.Dx()-n, sp.Y+n), op)
}

func (dst *Image) Border(r image.Rectangle, n int, color *Image, sp image.Point) {
	dst.BorderOp(r, n, color, sp, SoverD)
}
