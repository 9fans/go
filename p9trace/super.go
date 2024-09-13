//go:build ignore

// Super is a port of the C "super.c" trace reading demo.
// See https://github.com/9fans/p9trace/src/super.c.
package main

import (
	"fmt"
	"log"
	"os"

	"9fans.net/go/p9trace"
)

var (
	offset uint32 = ^uint32(0)
	last   uint32
	next   uint32

	count [p9trace.TagMax]int
	ndir  int
	nind  int
	num   int
)

func main() {

	for b := range p9trace.FileRecords(os.Args[1:]...) {
		if ^offset == 0 {
			offset = b.Addr
		}
		if b.Addr != offset {
			fmt.Fprintf(os.Stderr, "bad address %v %v\n", offset, b.Addr)
		}
		offset++
		if b.Tag >= p9trace.TagMax {
			log.Fatal("bad tag")
		}
		count[b.Tag]++

		switch b.Tag {
		case p9trace.TagDir:
			ndir += len(b.Dir)
		case p9trace.TagInd1, p9trace.TagInd2:
			nind += len(b.Ind)
		case p9trace.TagSuper:
			if next != 0 && next != b.Addr {
				fmt.Fprintf(os.Stderr, "missing super block %v\n", next)
			}
			if last != 0 && last != b.Super.Last {
				fmt.Fprintf(os.Stderr, "bad last block %v\n", b.Addr)
			}
			fmt.Printf("super %v: cwraddr %v roraddr %v last %v next %v\n",
				b.Addr, b.Super.CWRAddr, b.Super.RORAddr,
				b.Super.Last, b.Super.Next)
			next = b.Super.Next
			last = b.Addr
		}
		num++
	}

	for i := range p9trace.TagMax {
		fmt.Fprintf(os.Stderr, "%v: %v\n", i, count[i])
	}
	fmt.Fprintf(os.Stderr, "num = %v ndir = %v nind = %v\n", num, ndir, nind)
}
