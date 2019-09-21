// Copyright 2012 The go-plan9-auth Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package auth

import (
	"reflect"
	"testing"
)

type TokenizeTest struct {
	s      string
	n      int
	tokens []string
}

var tokenizeTests = []TokenizeTest{
	{"", 0, []string{}},
	{"abc", 1, []string{"abc"}},
	{"abc def", 2, []string{"abc", "def"}},
	{"abc def", 5, []string{"abc", "def"}},
	{"abc def ghi", 2, []string{"abc", "def"}},
	{"abc 'def ghi'", 2, []string{"abc", "def ghi"}},
	{"'gopher''s' 'go routines' are running", 2, []string{"gopher's", "go routines"}},
}

func TestTokenize(t *testing.T) {
	for _, tt := range tokenizeTests {
		tokens := tokenize(tt.s, tt.n)
		if !reflect.DeepEqual(tokens, tt.tokens) {
			t.Errorf("tokenize(%q, %d) returned %v; want %v\n",
				tt.s, tt.n, tokens, tt.tokens)
		}
	}
}
