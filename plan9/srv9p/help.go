package srv9p

import "9fans.net/go/plan9"

// ReadDir satisfies the read request r by iterating through
// a directory defined by gen. Gen(n, d) must fill in d with
// the n'th directory entry and return true, or else return false.
func (f *Fid) ReadDir(data []byte, offset int64, gen func(int, *plan9.Dir) bool) (int, error) {
	start := 0
	if offset > 0 {
		start = int(f.dirIndex.Load())
	}

	n := 0
	for n < len(data) {
		var d plan9.Dir
		if !gen(start, &d) {
			break
		}
		stat, err := d.Bytes()
		if err != nil {
			if n == 0 {
				return 0, err
			}
			break
		}
		if len(data[n:]) < len(stat) {
			break
		}
		copy(data[n:], stat)
		n += len(stat)
		start++
	}
	f.dirIndex.Store(int64(start))
	return n, nil
}

// ReadBytes satsifies the read request r as if the
// entire file being read were the buffer b,
// handling offset and count appropriately.
func (f *Fid) ReadBytes(dst []byte, offset int64, src []byte) (int, error) {
	if offset < 0 || offset >= int64(len(src)) {
		return 0, nil
	}
	n := copy(dst, src[offset:])
	return n, nil
}

// ReadString satsifies the read request r as if the
// entire file being read were the string s.
// handling offset and count appropriately.
func (f *Fid) ReadString(dst []byte, offset int64, src string) (int, error) {
	if offset < 0 || offset >= int64(len(src)) {
		return 0, nil
	}
	n := copy(dst, src[offset:])
	return n, nil
}
