package draw

// Cursor describes a single cursor.
type Cursor struct {
	Point
	Clr [2 * 16]uint8
	Set [2 * 16]uint8
}

// Cursor2 describes a high-DPI cursor.
type Cursor2 struct {
	Point
	Clr [4 * 32]uint8
	Set [4 * 32]uint8
}

var expand = [16]uint8{
	0x00, 0x03, 0x0c, 0x0f,
	0x30, 0x33, 0x3c, 0x3f,
	0xc0, 0xc3, 0xcc, 0xcf,
	0xf0, 0xf3, 0xfc, 0xff,
}

// Scale returns a high-DPI version of c.
func (c *Cursor) ScaleTo(c2 *Cursor2) {
	*c2 = Cursor2{}
	c2.X = 2 * c.X
	c2.Y = 2 * c.Y
	for y := 0; y < 16; y++ {
		c2.Clr[8*y+4] = expand[c.Clr[2*y]>>4]
		c2.Clr[8*y] = c2.Clr[8*y+4]
		c2.Set[8*y+4] = expand[c.Set[2*y]>>4]
		c2.Set[8*y] = c2.Set[8*y+4]
		c2.Clr[8*y+5] = expand[c.Clr[2*y]&15]
		c2.Clr[8*y+1] = c2.Clr[8*y+5]
		c2.Set[8*y+5] = expand[c.Set[2*y]&15]
		c2.Set[8*y+1] = c2.Set[8*y+5]
		c2.Clr[8*y+6] = expand[c.Clr[2*y+1]>>4]
		c2.Clr[8*y+2] = c2.Clr[8*y+6]
		c2.Set[8*y+6] = expand[c.Set[2*y+1]>>4]
		c2.Set[8*y+2] = c2.Set[8*y+6]
		c2.Clr[8*y+7] = expand[c.Clr[2*y+1]&15]
		c2.Clr[8*y+3] = c2.Clr[8*y+7]
		c2.Set[8*y+7] = expand[c.Set[2*y+1]&15]
		c2.Set[8*y+3] = c2.Set[8*y+7]
	}
}
