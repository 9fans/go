package draw

import (
	"fmt"
	"strconv"
	"unicode"
)

func skip(b []byte) []byte {
	for len(b) > 0 && (b[0] == ' ' || b[0] == '\t' || b[0] == '\n') {
		b = b[1:]
	}
	return b
}

func strtol(b []byte) (int, []byte) {
	b = skip(b)
	i := 0
	if len(b) == 0 || b[0] < '0' || '9' < b[0] {
		return 0, b
	}
	for i < len(b) && '0' <= b[i] && b[i] <= '9' || 'A' <= b[i] && b[i] <= 'Z' || 'a' <= b[i] && b[i] <= 'z' {
		i++
	}
	n, _ := strconv.ParseInt(string(b[:i]), 0, 0)
	return int(n), skip(b[i:])
}

func (d *Display) BuildFont(buf []byte, name string) (*Font, error) {
	fnt := &Font{
		Display: d,
		Name:    name,
		cache:   make([]cacheinfo, NFCACHE+NFLOOK),
		subf:    make([]cachesubf, NFSUBF),
		age:     1,
	}
	s := buf
	fnt.Height, s = strtol(s)
	fnt.Ascent, s = strtol(s)
	if fnt.Height <= 0 || fnt.Ascent <= 0 {
		return nil, fmt.Errorf("bad height or ascent in font file")
	}
	for {
		if len(s) == 0 || s[0] < '0' || '9' < s[0] {
			goto Errbad
		}
		var min, max int
		min, s = strtol(s)
		if len(s) == 0 || s[0] < '0' || '9' < s[0] {
			goto Errbad
		}
		max, s = strtol(s)
		if len(s) == 0 || min > unicode.MaxRune || max > unicode.MaxRune || min > max {
			return nil, fmt.Errorf("illegal subfont range")
		}
		offset, t := strtol(s)
		if len(t) < len(s) {
			s = t
		}
		c := &cachefont{
			min:    rune(min),
			max:    rune(max),
			offset: offset,
		}
		t = s
		for len(s) > 0 && s[0] != ' ' && s[0] != '\n' && s[0] != '\t' {
			s = s[1:]
		}
		c.name = string(t[:len(t)-len(s)])
		fnt.sub = append(fnt.sub, c)
		s = skip(s)
		if len(s) == 0 {
			break
		}
	}
	return fnt, nil

Errbad:
	return nil, fmt.Errorf("bad font format: number expected (char position %d)", len(buf)-len(s))
}

func (f *Font) Free() {
	if f == nil {
		return
	}

	for _, subf := range f.subf {
		s := subf.f
		if s != nil && (f.Display == nil || s != f.Display.DefaultSubfont) {
			s.Free()
		}
	}
	f.cacheimage.Free()
}
