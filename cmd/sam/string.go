package main

func Strinit(p *String) {
	p.s = nil
}

func Strinit0(p *String) {
	p.s = nil
}

func Strclose(p *String) {
	// free(p.s)
}

const MAXSIZE = 256

func Strzero(p *String) {
	if cap(p.s) > MAXSIZE {
		p.s = nil /* throw away the garbage */
	}
	p.s = p.s[:0]
}

func Strlen(r []rune) int {
	return len(r)
}

func Strdupl(p *String, s []rune) {
	Strinsure(p, len(s))
	copy(p.s, s)
}

func Strduplstr(p *String, q *String) {
	Strinsure(p, len(q.s))
	copy(p.s, q.s)
}

func Straddc(p *String, c rune) {
	p.s = append(p.s, c)
}

func Strinsure(p *String, n int) {
	if n > STRSIZE {
		error_(Etoolong)
	}
	for cap(p.s) < n {
		p.s = append(p.s[:cap(p.s)], 0)
	}
	p.s = p.s[:n]
}

func Strinsert(p *String, q *String, p0 Posn) {
	Strinsure(p, len(p.s)+len(q.s))
	copy(p.s[p0+len(q.s):], p.s[p0:])
	copy(p.s[p0:], q.s)
}

func Strdelete(p *String, p1 Posn, p2 Posn) {
	if p1 <= len(p.s) && p2 == len(p.s)+1 {
		// "deleting" the NUL at the end is OK
		p2--
	}
	copy(p.s[p1:], p.s[p2:])
	p.s = p.s[:len(p.s)-(p2-p1)]
}

func Strcmp(a *String, b *String) int {
	var i int
	for i = 0; i < len(a.s) && i < len(b.s); i++ {
		c := int(a.s[i] - b.s[i])
		if c != 0 { /* assign = */
			return c
		}
	}
	return len(a.s) - len(b.s)
}

func Strispre(prefix, s *String) bool {
	for i := 0; i < len(prefix.s); i++ {
		if i >= len(s.s) || s.s[i] != prefix.s[i] {
			return false
		}
	}
	return true
}

func Strtoc(s *String) string {
	return string(s.s)
}

/*
 * Build very temporary String from Rune*
 */
var tmprstr_p String

func tmprstr(r []rune) *String {
	return &String{r}
}

/*
 * Convert null-terminated char* into String
 */
func tmpcstr(s string) *String {
	if len(s) > 0 && s[len(s)-1] == '\x00' {
		s = s[:len(s)-1]
	}
	r := []rune(s)
	return &String{r}
}

func freetmpstr(s *String) {
	// free(s.s)
	// free(s)
}
