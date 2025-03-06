//go:build plan9

// Roundtrip demonstrates a write-read round trip with a synthetic file like /dev/rot13.
package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"syscall"
)

func usage() {
	fmt.Fprintf(os.Stderr, "usage: roundtrip file\n")
	os.Exit(1)
}

func main() {
	log.SetFlags(0)
	log.SetPrefix("roundtrip: ")
	flag.Usage = usage
	flag.Parse()
	if flag.NArg() != 1 {
		usage()
	}

	f, err := os.OpenFile(flag.Arg(0), os.O_RDWR, 0)
	if err != nil {
		log.Fatal(err)
	}

	check(io.Copy(f, os.Stdin))
	check(syscall.Write(int(f.Fd()), nil)) // syscall.Write because f.Write(nil) doesn't make a system call
	check(f.Seek(0, 0))
	check(io.Copy(os.Stdout, f))
}

func check[T any](_ T, err error) {
	if err != nil {
		log.Fatal(err)
	}
}
