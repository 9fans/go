package main

import (
	"fmt"
	"strconv"

	"9fans.net/go/draw"
)

func parsewinsize(s string, r *draw.Rectangle, havemin *bool) error {
	os := s
	isdigit := func(c byte) bool { return '0' <= c && c <= '9' }
	strtol := func(s string, sp *string, base int) int {
		i := 0
		for i < len(s) && isdigit(s[i]) {
			i++
		}
		*sp = s[i:]
		n, _ := strconv.ParseInt(s[:i], base, 0)
		return int(n)
	}

	*havemin = false
	*r = draw.Rect(0, 0, 0, 0)
	var i, j, k, l int
	var c byte
	if s == "" || !isdigit(s[0]) {
		goto oops
	}
	i = strtol(s, &s, 0)
	if s[0] == 'x' {
		s = s[1:]
		if s == "" || !isdigit(s[0]) {
			goto oops
		}
		j = strtol(s, &s, 0)
		r.Max.X = i
		r.Max.Y = j
		if len(s) == 0 {
			return nil
		}
		if s[0] != '@' {
			goto oops
		}

		s = s[1:]
		if s == "" || !isdigit(s[0]) {
			goto oops
		}
		i = strtol(s, &s, 0)
		if s[0] != ',' && s[0] != ' ' {
			goto oops
		}
		s = s[1:]
		if s == "" || !isdigit(s[0]) {
			goto oops
		}
		j = strtol(s, &s, 0)
		if s[0] != 0 {
			goto oops
		}
		*r = r.Add(draw.Pt(i, j))
		*havemin = true
		return nil
	}

	c = s[0]
	if c != ' ' && c != ',' {
		goto oops
	}
	s = s[1:]
	if len(s) == 0 || !isdigit(s[0]) {
		goto oops
	}
	j = strtol(s, &s, 0)
	if s[0] != c {
		goto oops
	}
	s = s[1:]
	if len(s) == 0 || !isdigit(s[0]) {
		goto oops
	}
	k = strtol(s, &s, 0)
	if s[0] != c {
		goto oops
	}
	s = s[1:]
	if len(s) == 0 || !isdigit(s[0]) {
		goto oops
	}
	l = strtol(s, &s, 0)
	if s[0] != 0 {
		goto oops
	}
	*r = draw.Rect(i, j, k, l)
	*havemin = true
	return nil

oops:
	return fmt.Errorf("bad syntax in window size '%s'", os)
}
