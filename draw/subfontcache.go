package draw

/*
 * Easy versions of the cache routines; may be substituted by fancier ones for other purposes
 */

var (
	lastname    string
	lastsubfont *Subfont
)

func lookupsubfont(d *Display, name string) *Subfont {
	if d != nil && name == "*default*" {
		return d.DefaultSubfont
	}
	if lastname == name && d == lastsubfont.Bits.Display {
		lastsubfont.Ref++
		return lastsubfont
	}
	return nil
}

func installsubfont(name string, subfont *Subfont) {
	lastname = name
	lastsubfont = subfont /* notice we don't free the old one; that's your business */
}

func uninstallsubfont(subfont *Subfont) {
	if subfont == lastsubfont {
		lastname = ""
		lastsubfont = nil
	}
}
