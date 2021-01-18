package draw

// BorderOp draws a retangular border of size r and width n, with n positive
// meaning the border is inside r, drawn with the specified draw op.
func (dst *Image) BorderOp(r Rectangle, n int, color *Image, sp Point, op Op) {
	if n < 0 {
		r = r.Inset(n)
		sp = sp.Add(Pt(n, n))
		n = -n
	}
	dst.Display.mu.Lock()
	defer dst.Display.mu.Unlock()
	draw(dst, Rect(r.Min.X, r.Min.Y, r.Max.X, r.Min.Y+n),
		color, sp, nil, sp, op)
	pt := Pt(sp.X, sp.Y+r.Dy()-n)
	draw(dst, Rect(r.Min.X, r.Max.Y-n, r.Max.X, r.Max.Y),
		color, pt, nil, pt, op)
	pt = Pt(sp.X, sp.Y+n)
	draw(dst, Rect(r.Min.X, r.Min.Y+n, r.Min.X+n, r.Max.Y-n),
		color, pt, nil, pt, op)
	pt = Pt(sp.X+r.Dx()-n, sp.Y+n)
	draw(dst, Rect(r.Max.X-n, r.Min.Y+n, r.Max.X, r.Max.Y-n),
		color, pt, nil, pt, op)
}

// Border draws a retangular border of size r and width n, with n positive
// meaning the border is inside r. It uses SoverD.
func (dst *Image) Border(r Rectangle, n int, color *Image, sp Point) {
	dst.BorderOp(r, n, color, sp, SoverD)
}
