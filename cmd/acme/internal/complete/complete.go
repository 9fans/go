// libcomplete/complete.c

package complete

import (
	"fmt"
	"io/ioutil"
	"strings"
	"unicode/utf8"
)

type Completion struct {
	Progress bool     /* whether forward progress has been made */
	Done     bool     /* whether the completion now represents a file or directory */
	Text     string   /* the string to advance, suffixed " " or "/" for file or directory */
	NumMatch int      /* number of files that matched */
	Files    []string /* files returned */
}

func Complete(dir, s string) (*Completion, error) {
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
			c.Files = append(c.Files, name+suffix)
		}
	}

	if len(c.Files) > 0 {
		/* report interesting results */
		/* trim length back to longest common initial string */
		minlen := len(c.Files[0])
		for i := 1; i < len(c.Files); i++ {
			minlen = longestprefixlength(c.Files[0], c.Files[i], minlen)
		}

		/* build the answer */
		c.Done = len(c.Files) == 1
		c.Progress = c.Done || minlen > len(s)
		c.Text = c.Files[0][len(s):minlen]
		if c.Done && !strings.HasSuffix(c.Text, "/") {
			c.Text += " "
		}
		c.NumMatch = len(c.Files)
	} else {
		/* no match, so return all possible strings */
		for _, info := range dirs {
			suffix := ""
			if info.IsDir() {
				suffix = "/"
			}
			c.Files = append(c.Files, info.Name()+suffix)
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
