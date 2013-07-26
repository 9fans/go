package draw

import "image"

func (dst *Image) BorderOp(r image.Rectangle, n int, color *Image, sp image.Point, op Op) {
	if n < 0 {
		r = r.Inset(n)
		sp = sp.Add(image.Pt(n, n))
		n = -n
	}
	dst.Display.mu.Lock()
	defer dst.Display.mu.Unlock()
	draw(dst, image.Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+n),
		color, sp, nil, sp, op)
	pt := image.Pt(sp.X, sp.Y+r.Dy()-n)
	draw(dst, image.Rect(r.Min.X, r.Max.Y-n, r.Max.X, r.Max.Y),
		color, pt, nil, pt, op)
	pt = image.Pt(sp.X, sp.Y+n)
	draw(dst, image.Rect(r.Min.X, r.Min.Y+n, r.Min.X+n, r.Max.Y-n),
		color, pt, nil, pt, op)
	pt = image.Pt(sp.X+r.Dx()-n, sp.Y+n)
	draw(dst, image.Rect(r.Max.X-n, r.Min.Y+n, r.Max.X, r.Max.Y-n),
		color, pt, nil, pt, op)
}

func (dst *Image) Border(r image.Rectangle, n int, color *Image, sp image.Point) {
	dst.BorderOp(r, n, color, sp, SoverD)
}
