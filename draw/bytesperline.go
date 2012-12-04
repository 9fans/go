package draw

import "image"

func WordsPerLine(r image.Rectangle, depth int) int {
	return unitsPerLine(r, depth, 32)
}

func BytesPerLine(r image.Rectangle, depth int) int {
	return unitsPerLine(r, depth, 8)
}

func unitsPerLine(r image.Rectangle, depth, bitsperunit int) int {
	if depth <= 0 || depth > 32 {
		panic("invalid depth")
	}

	var l int
	if r.Min.X >= 0 {
		l = (r.Max.X*depth + bitsperunit - 1) / bitsperunit
		l -= (r.Min.X * depth) / bitsperunit
	} else {
		// make positive before divide
		t := (-r.Min.X*depth + bitsperunit - 1) / bitsperunit
		l = t + (r.Max.X*depth+bitsperunit-1)/bitsperunit
	}
	return l
}
