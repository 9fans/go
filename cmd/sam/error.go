// #include "sam.h"

package main

import (
	"fmt"
	"os"
)

var emsg = [47]string{
	/* error_s */
	"can't open",
	"can't create",
	"not in menu:",
	"changes to",
	"I/O error:",
	"can't write while changing:",
	/* error_c */
	"unknown command",
	"no operand for",
	"bad delimiter",
	/* error */
	"can't fork",
	"interrupt",
	"address",
	"search",
	"pattern",
	"newline expected",
	"blank expected",
	"pattern expected",
	"can't nest X or Y",
	"unmatched `}'",
	"command takes no address",
	"addresses overlap",
	"substitution",
	"& match too long",
	"bad \\ in rhs",
	"address range",
	"changes not in sequence",
	"addresses out of order",
	"no file name",
	"unmatched `('",
	"unmatched `)'",
	"malformed `[]'",
	"malformed regexp",
	"reg. exp. list overflow",
	"plan 9 command",
	"can't pipe",
	"no current file",
	"string too long",
	"changed files",
	"empty string",
	"file search",
	"non-unique match for \"\"",
	"tag match too long",
	"too many subexpressions",
	"temporary file too large",
	"file is append-only",
	"no destination for plumb message",
	"internal read error in buffer load",
}
var wmsg = [8]string{
	/* warn_s */
	"duplicate file name",
	"no such file",
	"write might change good version of",
	/* warn_S */
	"files might be aliased",
	/* warn */
	"null characters elided",
	"can't run pwd",
	"last char not newline",
	"exit status not 0",
}

func error_(s Err) {
	hiccough(fmt.Sprintf("?%s", emsg[s]))
}

func error_s(s Err, a string) {
	hiccough(fmt.Sprintf("?%s \"%s\"", emsg[s], a))
}

func error_r(s Err, a string, err error) {
	if pe, ok := err.(*os.PathError); ok && pe.Path == a {
		err = pe.Err
	}
	hiccough(fmt.Sprintf("?%s \"%s\": %v", emsg[s], a, err))
}

func error_c(s Err, c rune) {
	hiccough(fmt.Sprintf("?%s `%c'", emsg[s], c))
}

func warn(s Warn) {
	dprint("?warning: %s\n", wmsg[s])
}

func warn_S(s Warn, a *String) {
	print_s(wmsg[s], a)
}

func warn_SS(s Warn, a *String, b *String) {
	print_ss(wmsg[s], a, b)
}

func warn_s(s Warn, a string) {
	dprint("?warning: %s `%s'\n", wmsg[s], a)
}

func termwrite(s string) {
	if downloaded {
		p := tmpcstr(s)
		if cmd != nil {
			loginsert(cmd, cmdpt, p.s)
		} else {
			Strinsert(&cmdstr, p, len(cmdstr.s))
		}
		cmdptadv += len(p.s)
		freetmpstr(p)
	} else {
		os.Stderr.WriteString(s)
	}
}
