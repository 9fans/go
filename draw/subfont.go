package draw

func (d *Display) AllocSubfont(name string, height, ascent int, info []Fontchar, i *Image) *Subfont {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.allocSubfont(name, height, ascent, info, i)
}

func (d *Display) allocSubfont(name string, height, ascent int, info []Fontchar, i *Image) *Subfont {
	f := &Subfont{
		Name:   name,
		N:      len(info) - 1,
		Height: height,
		Ascent: ascent,
		Bits:   i,
		Ref:    1,
		Info:   info,
	}
	if name != "" {
		/*
		 * if already caching this subfont, leave older
		 * (and hopefully more widely used) copy in cache.
		 * this case should not happen -- we got called
		 * because cachechars needed this subfont and it
		 * wasn't in the cache.
		 */
		cf := lookupsubfont(i.Display, name)
		if cf == nil {
			installsubfont(name, f)
		} else {
			cf.Free() /* drop ref we just picked up */
		}
	}
	return f
}
