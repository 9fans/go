package draw

func (f *Subfont) Free() {
	if f == nil {
		return
	}
	f.Ref--
	if f.Ref > 0 {
		return
	}
	uninstallsubfont(f)
	f.Bits.Free()
}
