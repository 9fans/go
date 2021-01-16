package main

import "unicode/utf8"

/*
 * The code makes two assumptions: strlen(ld) is 1 or 2; latintab[i].ld can be a
 * prefix of latintab[j].ld only when j<i.
 */
type cvlist struct {
	ld string /* must be seen before using this conversion */
	si string /* options for last input characters */
	so []rune /* the corresponding Rune for each si entry */
}

/*
 * Given 5 characters k[0]..k[n], find the rune or return -1 for failure.
 */
func toUnicode(k []rune) rune {
	c := rune(0)
	for ; len(k) > 0; k = k[1:] {
		r := k[0]
		c <<= 4
		if '0' <= r && r <= '9' {
			c += rune(r) - '0'
		} else if 'a' <= r && r <= 'f' {
			c += 10 + r - 'a'
		} else if 'A' <= r && r <= 'F' {
			c += 10 + r - 'A'
		} else {
			return -1
		}
		if c > utf8.MaxRune {
			return -1
		}
	}
	return c
}

/*
 * Given n characters k[0]..k[n-1], find the corresponding rune or return -1 for
 * failure, or something < -1 if n is too small.  In the latter case, the result
 * is minus the required n.
 */
func toLatin1(k []rune) rune {
	n := len(k)
	if k[0] == 'X' {
		if n < 2 {
			return -2
		}
		if k[1] == 'X' {
			if n < 3 {
				return -3
			}
			if k[2] == 'X' {
				if n < 9 {
					if toUnicode(k[3:]) < 0 {
						return -1
					}
					return rune(-(n + 1))
				}
				return toUnicode(k[3:9])
			}
			if n < 7 {
				if toUnicode(k[2:]) < 0 {
					return -1
				}
				return rune(-(n + 1))
			}
			return toUnicode(k[2:7])
		}
		if n < 5 {
			if toUnicode(k[1:]) < 0 {
				return -1
			}
			return rune(-(n + 1))
		}
		return toUnicode(k[1:4])
	}

	for i := 0; i < len(latintab); i++ {
		l := &latintab[i]
		if k[0] == rune(l.ld[0]) {
			if n == 1 {
				return -2
			}
			var c rune
			if len(l.ld) == 1 {
				c = k[1]
			} else if rune(l.ld[1]) != k[1] {
				continue
			} else if n == 2 {
				return -3
			} else {
				c = k[2]
			}
			for i := range l.si {
				if rune(l.si[i]) == c {
					return l.so[i]
				}
			}
			return -1
		}
	}
	return -1
}
