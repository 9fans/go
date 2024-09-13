// Copyright 2024 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build ignore

// Super2 is like super but reads from an SSTable instead of raw trace files.
package main

import (
	"fmt"
	"log"
	"os"

	"9fans.net/go/p9trace"
	"github.com/golang/leveldb/table"
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
	f, err := os.Open(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	r := table.NewReader(f, nil)
	it := r.Find(nil, nil)
	for it.Next() {
		var b p9trace.Record
		if err := b.UnmarshalBinary(it.Value()); err != nil {
			log.Fatal(err)
		}

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
	if err := it.Close(); err != nil {
		log.Fatalf("iter: %v", err)
	}

	for i := range p9trace.TagMax {
		fmt.Fprintf(os.Stderr, "%v: %v\n", i, count[i])
	}
	fmt.Fprintf(os.Stderr, "num = %v ndir = %v nind = %v\n", num, ndir, nind)
}
