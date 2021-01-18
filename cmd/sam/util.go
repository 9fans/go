// #include "sam.h"

package main

import "unicode/utf8"

// cvttorunes converts bytes in b to runes in r,
// returning the number of bytes processed from b,
// the number of runes written to r,
// and whether any null bytes were elided.
// If eof is true, then any partial runes at the end of b
// should be processed, and nb == len(b) at return.
// Otherwise, partial runes are left behind and
// nb may be up to utf8.UTFMax-1 bytes short of len(b).
func cvttorunes(b []byte, r []rune, eof bool) (nb, nr int, nulls bool) {
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

func fbufalloc() []rune {
	return make([]rune, RBUFSIZE)
}

func fbuffree(f []rune) {
	// free(f)
}

func min(a int, b int) int {
	if a < b {
		return a
	}
	return b
}
