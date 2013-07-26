package draw

func (f *Subfont) Free() {
	if f == nil {
		return
	}
	f.Bits.Display.mu.Lock()
	defer f.Bits.Display.mu.Unlock()
	f.free()
}

func (f *Subfont) free() {
	if f == nil {
		return
	}
	f.Ref--
	if f.Ref > 0 {
		return
	}
	uninstallsubfont(f)
	f.Bits.free()
}
