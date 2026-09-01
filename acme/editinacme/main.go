// Copyright 2015 The Go Authors.  All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

// Editinacme can be used as $EDITOR in a Unix environment.
//
// Usage:
//
//	editinacme [-nw] <file>
//
// Editinacme uses the plumber to ask acme to open the file, waits until
// the file's acme window is deleted, and exits. Use the -nw flag to exit
// immediately after opening the file.
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"9fans.net/go/acme"
	"9fans.net/go/plan9"
	"9fans.net/go/plumb"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("editinacme: ")

	nowait := flag.Bool("nw", false, "Don't wait for Acme to close the file")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: editinacme file\n")
		os.Exit(2)
	}
	flag.Parse()
	if flag.NArg() != 1 {
		flag.Usage()
	}

	file := flag.Arg(0)

	fullpath, err := filepath.Abs(file)
	if err != nil {
		log.Fatal(err)
	}
	file = fullpath

	r, err := acme.Log()
	if err != nil {
		log.Fatal(err)
	}
	defer r.Close()

	filenamechan := make(chan string)
	if !*nowait {
		fid, err := plumb.Open("edit", plan9.OREAD)
		if err != nil {
			log.Fatalf("can't open plumber: %v", err)
		}
		defer fid.Close()
		brd := bufio.NewReader(fid)
		m := new(plumb.Message)

		go func() {
			for {
				err := m.Recv(brd)
				if err != nil {
					log.Fatalf("recv: %s", err)
				}
				if filename, likelymy := likelymyplumbrequest(m, file); likelymy {
					filenamechan <- filename
					return
				}
			}
		}()
	}

	log.Printf("editing %s", file)
	out, err := exec.Command("plumb", "-d", "edit", file).CombinedOutput()
	if err != nil {
		log.Fatalf("executing plumb: %v\n%s", err, out)
	}

	if !*nowait {
		filename := <-filenamechan
		for {
			ev, err := r.Read()
			if err != nil {
				log.Fatalf("reading acme log: %v", err)
			}
			if ev.Op == "del" && ev.Name == filename {
				break
			}
		}
	}
}

// likelymyplumbrequest applies some heuristics to determine if this
// message likely corresponds to the just made plumb edit request and
// returns the filename that the plumber asked Acme to open.
func likelymyplumbrequest(msg *plumb.Message, arg string) (string, bool) {
	cwd, err := os.Getwd()
	if err != nil {
		log.Fatalf("aww shucks! %v", err)
	}
	plumbedfname := string(msg.Data)
	myreq := (msg.Dir == cwd && strings.HasPrefix(arg, plumbedfname))
	return plumbedfname, myreq
}
