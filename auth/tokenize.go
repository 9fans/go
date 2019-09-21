// Copyright 2012 The go-plan9-auth Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import "bytes"

func isSpace(r rune) bool {
	return r == ' ' || r == '\t' || r == '\n' || r == '\r'
}

func isNotSpace(r rune) bool {
	return !isSpace(r)
}

func getToken(b []byte) (string, []byte) {
	quote := false
	w := 0
	r := 0
	for ; r < len(b); r++ {
		c := rune(b[r])
		if !quote && isSpace(c) {
			break
		}
		if c != '\'' {
			b[w] = b[r]
			w++
			continue
		}
		if !quote {
			quote = true
			continue
		}
		if r+1 == len(b) || b[r+1] != '\'' {
			quote = false
			continue
		}
		// found a doubled quote
		r++
		b[w] = b[r]
		w++
	}
	return string(b[:w]), b[r:]
}

// Tokenize returns the tokens in s separated by one or more
// whitespaces. Maximum of n tokens are returns. Tokens can
// be optionally quoted using rc-style quotes.
func tokenize(s string, n int) []string {
	tok := make([]string, n)
	b := []byte(s)
	i := 0
	for ; i < n; i++ {
		r := bytes.IndexFunc(b, isNotSpace)
		if r < 0 {
			break
		}
		tok[i], b = getToken(b[r:])
	}
	return tok[:i]
}
