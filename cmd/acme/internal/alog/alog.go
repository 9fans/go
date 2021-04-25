package alog

import (
	"fmt"
	"os"
)

var warn = func(msg string) {
	fmt.Fprintf(os.Stderr, "acme: %s", msg) // msg has final newline
}

func Init(w func(string)) {
	warn = w
}

func Printf(format string, args ...interface{}) {
	s := fmt.Sprintf(format, args...)
	if s != "" && s[len(s)-1] != '\n' {
		s += "\n"
	}
	warn(s)
}
