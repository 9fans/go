package draw

import "bytes"

func getdefont(d *Display) (*Subfont, error) {
	return d.ReadSubfont("*default*", bytes.NewReader(defontdata), false)
}
