package draw

import "image"

func (d *Display) AllocImageMix(color1, color3 Color) *Image {
	if d.ScreenImage.Depth <= 8 { // create a 2x2 texture
		t, _ := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage.Pix, false, color1)
		b, _ := d.AllocImage(image.Rect(0, 0, 2, 2), d.ScreenImage.Pix, true, color3)
		b.Draw(image.Rect(0, 0, 1, 1), t, nil, image.ZP)
		t.Free()
		return b
	}

	// use a solid color, blended using alpha
	if d.qmask == nil {
		d.qmask, _ = d.AllocImage(image.Rect(0, 0, 1, 1), GREY8, true, 0x3F3F3FFF)
	}
	t, _ := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage.Pix, true, color1)
	b, _ := d.AllocImage(image.Rect(0, 0, 1, 1), d.ScreenImage.Pix, true, color3)
	b.Draw(b.R, t, d.qmask, image.ZP)
	return b
}
