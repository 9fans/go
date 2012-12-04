package draw

func (d *Display) SetDebug(debug bool) {
	a := d.bufimage(2)
	a[0] = 'D'
	a[1] = 0
	if debug {
		a[1] = 1
	}
}
