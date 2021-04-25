// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <thread.h>
// #include <cursor.h>
// #include <mouse.h>
// #include <keyboard.h>
// #include <frame.h>
// #include <fcall.h>
// #include <plumb.h>
// #include <libsec.h>
// #include "dat.h"
// #include "fns.h"

package main

import (
	"io"
	"os"
	"unicode/utf8"

	"9fans.net/go/cmd/acme/internal/disk"
	"9fans.net/go/cmd/acme/internal/runes"
	"9fans.net/go/cmd/acme/internal/util"
)

func bufloader(v interface{}, q0 int, r []rune) int {
	v.(*disk.Buffer).Insert(q0, r)
	return len(r)
}

func loadfile(fd *os.File, q0 int, nulls *bool, f func(interface{}, int, []rune) int, arg interface{}, h io.Writer) int {
	p := make([]byte, BUFSIZE+utf8.UTFMax+1)
	r := make([]rune, BUFSIZE)
	m := 0
	n := 1
	q1 := q0
	/*
	 * At top of loop, may have m bytes left over from
	 * last pass, possibly representing a partial rune.
	 */
	for n > 0 {
		var err error
		n, err = fd.Read(p[m : m+BUFSIZE])
		if err != nil && err != io.EOF {
			warning(nil, "read error in Buffer.load: %v", err)
			break
		}
		if h != nil {
			h.Write(p[m : m+n])
		}
		m += n
		nb, nr, nulls1 := runes.Convert(p[:m], r, err == io.EOF)
		if nulls1 {
			*nulls = true
		}
		copy(p, p[nb:m])
		m -= nb
		q1 += f(arg, q1, r[:nr])
	}
	return q1 - q0
}

func bufload(b *disk.Buffer, q0 int, fd *os.File, nulls *bool, h io.Writer) int {
	if q0 > b.Len() {
		util.Fatal("internal error: bufload")
	}
	return loadfile(fd, q0, nulls, bufloader, b, h)
}
