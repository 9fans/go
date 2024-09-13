//go:build ignore

// Reorder reorders the traces so that block pointers only ever point to earlier blocks
// and writes the result to a single trace file.
package main

import (
	"bufio"
	"bytes"
	"compress/flate"
	"encoding/binary"
	"log"
	"os"

	"9fans.net/go/p9trace"
)

var all []*p9trace.Record
var remap []uint32
var nbad int
var nextOut uint32
var bw *bufio.Writer

func main() {
	f, err := os.Create(os.Args[1])
	if err != nil {
		log.Fatal(err)
	}
	bw = bufio.NewWriterSize(f, 1<<20)

	for r := range p9trace.FileRecords(os.Args[2:]...) {
		all = append(all, r)
		if all[r.Addr] != r {
			panic("mismatch")
		}
	}
	println("loaded", all[173927], all[173927].Addr, all[173927].Tag)

	remap = make([]uint32, len(all))
	for addr, r := range all {
		if r.Tag == p9trace.TagSuper {
			writeRecord(uint32(addr))
		}
	}
	println(len(all), nbad, nextOut)

	bw.Flush()
	if err := bw.Flush(); err != nil {
		log.Fatal(err)
	}
	if err := f.Close(); err != nil {
		log.Fatal(err)
	}
}

var stack []*p9trace.Record

func writeRecord(addr uint32) {
	if remap[addr] != 0 && remap[addr] != ^uint32(0) {
		return
	}
	r := all[addr]
	if all[r.Addr] != r {
		println("ALL", r.Addr, all[r.Addr], r)
		panic("all mismatch")
	}

	stack = append(stack, r)
	for p := range r.Pointers() {
		if r.Super != nil && p == &r.Super.Next {
			continue
		}
		if int(*p) > len(all) {
			println("BAD", r.Addr, *p, len(all))
			nbad++
			r.Dir = nil
			r.Ind = nil
			if r.Super != nil {
				*r.Super = p9trace.Super{}
			}
			break
		}
	}

	remap[r.Addr] = ^uint32(0)
	if r.Super != nil {
		r.Super.Next = 0
	}
	for p := range r.Pointers() {
		if *p == 0 {
			continue
		}
		if remap[*p] == 0 {
			writeRecord(*p)
		}
		if remap[*p] == ^uint32(0) {
			if *p == r.Addr {
				*p = 0
				continue
			}
			println("BLOCK LOOP", r.Addr, r.Tag, *p, all[*p].Addr, all[*p].Tag)
			for i, r1 := range stack {
				println("STACK #", i, r1.Addr, r1.Tag, r1, all[r1.Addr].Addr, all[r1.Addr].Tag, all[r1.Addr])
			}
			panic("block loop")
		}
		*p = remap[*p]
	}
	remap[r.Addr] = nextOut
	r.Addr = nextOut
	nextOut++

	if true {
		data, err := r.MarshalBinary()
		if err != nil {
			log.Fatal(err)
		}
		var hdr [2]byte
		if len(data) > p9trace.HeaderSize {
			var fb bytes.Buffer
			fw, err := flate.NewWriter(&fb, flate.BestSpeed)
			if err != nil {
				log.Fatal(err)
			}
			fw.Write(data)
			fw.Close()
			if fb.Len() < len(data) {
				binary.BigEndian.PutUint16(hdr[:], uint16(fb.Len())|0x8000)
				bw.Write(hdr[:])
				bw.Write(fb.Bytes())
				goto Done
			}
		}
		binary.BigEndian.PutUint16(hdr[:], uint16(len(data)))
		bw.Write(hdr[:])
		bw.Write(data)
	Done:
	}

	stack = stack[:len(stack)-1]
}
