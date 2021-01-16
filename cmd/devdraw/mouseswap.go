// #include <u.h>
// #include <libc.h>
// #include <draw.h>
// #include <memdraw.h>
// #include <memlayer.h>
// #include <mouse.h>
// #include <cursor.h>
// #include <keyboard.h>
// #include <drawfcall.h>
// #include "devdraw.h"

package main

import (
	"fmt"
	"os"
	"strconv"
)

const (
	Nbutton = 10
)

var debug int

var mousemap struct {
	b    [Nbutton]int
	init bool
}

func initmap() {
	p := os.Getenv("mousedebug")
	if p != "" {
		debug, _ = strconv.Atoi(p)
	}
	var i int
	for i = 0; i < Nbutton; i++ {
		mousemap.b[i] = i
	}
	mousemap.init = true
	p = os.Getenv("mousebuttonmap")
	if p != "" {
		for i := 0; i < Nbutton && i < len(p); i++ {
			if '0' <= p[i] && p[i] <= '9' {
				mousemap.b[i] = int(p[i]) - '1'
			}
		}
	}
	if debug != 0 {
		fmt.Fprintf(os.Stderr, "mousemap: ")
		for i := 0; i < Nbutton; i++ {
			fmt.Fprintf(os.Stderr, " %d", 1+mousemap.b[i])
		}
		fmt.Fprintf(os.Stderr, "\n")
	}
}

func mouseswap(but int) int {
	if !mousemap.init {
		initmap()
	}

	nbut := 0
	for i := 0; i < Nbutton; i++ {
		if but&(1<<i) != 0 && mousemap.b[i] >= 0 {
			nbut |= 1 << mousemap.b[i]
		}
	}
	if debug != 0 {
		fmt.Fprintf(os.Stderr, "swap %#b -> %#b\n", but, nbut)
	}
	return nbut
}
