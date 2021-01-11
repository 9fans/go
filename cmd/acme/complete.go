// libcomplete/complete.c

package main

import (
	"fmt"
	"io/ioutil"
	"strings"
	"unicode/utf8"
)

type Completion struct {
	advance  bool     /* whether forward progress has been made */
	complete bool     /* whether the completion now represents a file or directory */
	string   string   /* the string to advance, suffixed " " or "/" for file or directory */
	nmatch   int      /* number of files that matched */
	filename []string /* files returned */
}

func complete(dir, s string) (*Completion, error) {
	if strings.Contains(s, "/") {
		return nil, fmt.Errorf("slash character in name argument to complete")
	}

	// Note: ioutil.ReadDir sorts, so no sort below.
	dirs, err := ioutil.ReadDir(dir)
	if err != nil {
		return nil, err
	}

	// find the matches
	c := new(Completion)
	for _, info := range dirs {
		if name := info.Name(); strings.HasPrefix(name, s) {
			suffix := ""
			if info.IsDir() {
				suffix = "/"
			}
			c.filename = append(c.filename, name+suffix)
		}
	}

	if len(c.filename) > 0 {
		/* report interesting results */
		/* trim length back to longest common initial string */
		minlen := len(c.filename[0])
		for i := 1; i < len(c.filename); i++ {
			minlen = longestprefixlength(c.filename[0], c.filename[i], minlen)
		}

		/* build the answer */
		c.complete = len(c.filename) == 1
		c.advance = c.complete || minlen > len(s)
		c.string = c.filename[0][len(s):minlen]
		if c.complete && !strings.HasSuffix(c.string, "/") {
			c.string += " "
		}
		c.nmatch = len(c.filename)
	} else {
		/* no match, so return all possible strings */
		for _, info := range dirs {
			suffix := ""
			if info.IsDir() {
				suffix = "/"
			}
			c.filename = append(c.filename, info.Name()+suffix)
		}
	}
	return c, nil
}

func longestprefixlength(a, b string, n int) int {
	var i int
	for i = 0; i < n && i < len(a) && i < len(b); {
		ra, wa := utf8.DecodeRuneInString(a)
		rb, wb := utf8.DecodeRuneInString(b)
		if ra != rb || wa != wb {
			break
		}
		i += wa
	}
	return i
}

func freecompletion(c *Completion) {
}
