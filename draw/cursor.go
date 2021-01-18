package draw

// Cursor describes a single cursor.
type Cursor struct {
	Point
	Clr [2 * 16]uint8
	Set [2 * 16]uint8
}
