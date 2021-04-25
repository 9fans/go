// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <regexp.h>
// #include <9pclient.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package runes

import (
	"path"
	"strings"
	"unicode/utf8"
)

func Index(r, s []rune) int {
	if len(s) == 0 {
		return 0
	}
	c := s[0]
	for i, ri := range r {
		if len(r)-i < len(s) {
			break
		}
		if ri == c && Equal(r[i:i+len(s)], s) {
			return i
		}
	}
	return -1
}

func IndexRune(rs []rune, c rune) int {
	for i, r := range rs {
		if r == c {
			return i
		}
	}
	return -1
}

func Compare(a, b []rune) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		if a[i] != b[i] {
			if a[i] < b[i] {
				return -1
			}
			return +1
		}
	}
	if len(a) < len(b) {
		return -1
	}
	if len(a) > len(b) {
		return +1
	}
	return 0
}

func Equal(s1, s2 []rune) bool {
	if len(s1) != len(s2) {
		return false
	}
	for i := 0; i < len(s1); i++ {
		if s1[i] != s2[i] {
			return false
		}
	}
	return true
}

func Clone(r []rune) []rune {
	s := make([]rune, len(r))
	copy(s, r)
	return s
}

func IsAlphaNum(c rune) bool {
	/*
	 * Hard to get absolutely right.  Use what we know about ASCII
	 * and assume anything above the Latin control characters is
	 * potentially an alphanumeric.
	 */
	if c <= ' ' {
		return false
	}
	if 0x7F <= c && c <= 0xA0 {
		return false
	}
	if strings.ContainsRune("!\"#$%&'()*+,-./:;<=>?@[\\]^`{|}~", c) {
		return false
	}
	return true
}

var isfilec_Lx = []rune(".-+/:@")

func IsFilename(r rune) bool {
	if IsAlphaNum(r) {
		return true
	}
	if IndexRune(isfilec_Lx, r) >= 0 {
		return true
	}
	return false
}

func SkipBlank(r []rune) []rune {
	for len(r) > 0 && (r[0] == ' ' || r[0] == '\t' || r[0] == '\n') {
		r = r[1:]
	}
	return r
}

func SkipNonBlank(r []rune) []rune {
	for len(r) > 0 && r[0] != ' ' && r[0] != '\t' && r[0] != '\n' {
		r = r[1:]
	}
	return r
}

// Convert converts bytes in b to runes in r,
// returning the number of bytes processed from b,
// the number of runes written to r,
// and whether any null bytes were elided.
// If eof is true, then any partial runes at the end of b
// should be processed, and nb == len(b) at return.
// Otherwise, partial runes are left behind and
// nb may be up to utf8.UTFMax-1 bytes short of len(b).
func Convert(b []byte, r []rune, eof bool) (nb, nr int, nulls bool) {
	b0 := b
	for len(b) > 0 && (eof || len(b) >= utf8.UTFMax || utf8.FullRune(b)) {
		rr, w := utf8.DecodeRune(b)
		if rr == 0 {
			nulls = true
		} else {
			r[nr] = rr
			nr++
		}
		b = b[w:]
	}
	nb = len(b0) - len(b)
	return nb, nr, nulls
}

func CleanPath(r []rune) []rune {
	return []rune(path.Clean(string(r)))
}

type Range struct {
	Pos int
	End int
}

func Rng(q0 int, q1 int) Range {
	var r Range
	r.Pos = q0
	r.End = q1
	return r
}

const Infinity = 0x7FFFFFFF

const RuneSize = 4
