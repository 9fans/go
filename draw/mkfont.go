package draw

/*
 * Cobble fake font using existing subfont
 */

func (subfont *Subfont) MakeFont(min rune) *Font {
	font := &Font{
		Display: subfont.Bits.Display,
		Name:    "<synthetic>",
		Height:  subfont.Height,
		Ascent:  subfont.Ascent,
		cache:   make([]cacheinfo, NFCACHE+NFLOOK),
		subf:    make([]cachesubf, NFSUBF),
		age:     1,
		sub: []*cachefont{{
			min: min,
			max: min + rune(subfont.N) - 1,
		}},
	}
	font.subf[0].cf = font.sub[0]
	font.subf[0].f = subfont
	return font
}
