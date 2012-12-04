package draw

import (
	"image"
)

func ReplXY(min, max, x int) int {
	sx := (x - min) % (max - min)
	if sx < 0 {
		sx += max - min
	}
	return sx + min
}

func Repl(r image.Rectangle, p image.Point) image.Point {
	return image.Point{
		ReplXY(r.Min.X, r.Max.X, p.X),
		ReplXY(r.Min.Y, r.Max.Y, p.Y),
	}
}
